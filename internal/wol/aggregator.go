/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package wol

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	wolv1 "github.com/gpillon/kubevirt-wol/api/wol/v1"
)

// Aggregator implementa il gRPC server per ricevere eventi WOL dagli agent
type Aggregator struct {
	wolv1.UnimplementedWOLServiceServer

	mapper         *MACMapper
	vmStarter      *VMStarter
	log            logr.Logger
	dedupeMap      map[string]*dedupeEntry
	dedupeLock     sync.RWMutex
	dedupeDuration time.Duration
}

type dedupeEntry struct {
	lastSeen     time.Time
	count        int
	nodes        []string
	lastResponse *wolv1.WOLEventResponse
}

// NewAggregator crea un nuovo aggregatore
func NewAggregator(mapper *MACMapper, vmStarter *VMStarter, log logr.Logger) *Aggregator {
	return &Aggregator{
		mapper:         mapper,
		vmStarter:      vmStarter,
		log:            log,
		dedupeMap:      make(map[string]*dedupeEntry),
		dedupeDuration: 10 * time.Second, // Deduplica globale per 10 secondi
	}
}

// ReportWOLEvent implementa il metodo gRPC unary
func (a *Aggregator) ReportWOLEvent(ctx context.Context, event *wolv1.WOLEvent) (*wolv1.WOLEventResponse, error) {
	startTime := time.Now()

	a.log.Info("Received WOL event via gRPC",
		"mac", event.MacAddress,
		"node", event.NodeName,
		"source", event.SourceIp,
		"port", event.SourcePort,
		"packetSize", event.PacketSize)

	WOLPacketsTotal.Inc()

	// Deduplica globale
	isDuplicate, cachedResp := a.checkDuplicate(event)
	if isDuplicate && cachedResp != nil {
		a.log.V(1).Info("Duplicate WOL event (global dedupe)",
			"mac", event.MacAddress,
			"node", event.NodeName,
			"originalNode", cachedResp.Message)

		// Aggiorna il cached response con processing time
		cachedResp.ProcessingTimeMs = time.Since(startTime).Milliseconds()
		return cachedResp, nil
	}

	// Lookup VM per questo MAC
	vmInfo, found := a.mapper.Lookup(event.MacAddress)
	if !found {
		a.log.Info("No VM found for MAC address", "mac", event.MacAddress)

		resp := &wolv1.WOLEventResponse{
			Status:           wolv1.ResponseStatus_VM_NOT_FOUND,
			Message:          fmt.Sprintf("No VM configured for MAC %s", event.MacAddress),
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}

		a.recordEvent(event, resp)
		return resp, nil
	}

	a.log.Info("Starting VM for WOL request",
		"mac", event.MacAddress,
		"vm", vmInfo.Name,
		"namespace", vmInfo.Namespace,
		"node", event.NodeName,
		"source", event.SourceIp)

	// Avvia VM
	err := a.vmStarter.StartVM(ctx, vmInfo.Namespace, vmInfo.Name)
	if err != nil {
		a.log.Error(err, "Failed to start VM",
			"vm", vmInfo.Name,
			"namespace", vmInfo.Namespace,
			"mac", event.MacAddress)
		ErrorsTotal.Inc()

		resp := &wolv1.WOLEventResponse{
			Status:  wolv1.ResponseStatus_ERROR,
			Message: fmt.Sprintf("Failed to start VM: %v", err),
			VmInfo: &wolv1.VMInfo{
				Name:      vmInfo.Name,
				Namespace: vmInfo.Namespace,
			},
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}

		a.recordEvent(event, resp)
		return resp, nil
	}

	VMStartedTotal.Inc()

	resp := &wolv1.WOLEventResponse{
		Status:  wolv1.ResponseStatus_VM_START_INITIATED,
		Message: fmt.Sprintf("VM start initiated successfully from node %s", event.NodeName),
		VmInfo: &wolv1.VMInfo{
			Name:         vmInfo.Name,
			Namespace:    vmInfo.Namespace,
			CurrentState: "Starting",
		},
		ProcessingTimeMs: time.Since(startTime).Milliseconds(),
	}

	a.recordEvent(event, resp)
	return resp, nil
}

// ReportWOLEventStream implementa streaming bidirezionale (opzionale per future)
func (a *Aggregator) ReportWOLEventStream(stream wolv1.WOLService_ReportWOLEventStreamServer) error {
	a.log.Info("Client opened WOL event stream")

	for {
		event, err := stream.Recv()
		if err != nil {
			a.log.V(1).Info("Stream closed", "error", err)
			return err
		}

		resp, err := a.ReportWOLEvent(stream.Context(), event)
		if err != nil {
			return err
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// HealthCheck implementa health check gRPC
func (a *Aggregator) HealthCheck(ctx context.Context, req *wolv1.HealthCheckRequest) (*wolv1.HealthCheckResponse, error) {
	a.log.V(1).Info("Health check requested", "service", req.Service)

	// Check se mapper ha configurazione
	mappingCount := a.mapper.GetMappingCount()

	status := wolv1.HealthCheckResponse_SERVING
	if mappingCount == 0 {
		a.log.V(1).Info("Health check: no VM mappings configured")
		// Comunque SERVING, solo non ci sono VM configurate ancora
	}

	return &wolv1.HealthCheckResponse{
		Status: status,
	}, nil
}

// checkDuplicate verifica se un evento è un duplicato (deduplica globale)
func (a *Aggregator) checkDuplicate(event *wolv1.WOLEvent) (bool, *wolv1.WOLEventResponse) {
	a.dedupeLock.Lock()
	defer a.dedupeLock.Unlock()

	now := time.Now()
	mac := event.MacAddress

	if entry, exists := a.dedupeMap[mac]; exists {
		if now.Sub(entry.lastSeen) < a.dedupeDuration {
			// Duplicato! Aggiorna stats
			entry.count++
			entry.nodes = append(entry.nodes, event.NodeName)
			entry.lastSeen = now

			// Crea response duplicate
			resp := &wolv1.WOLEventResponse{
				Status:       wolv1.ResponseStatus_DUPLICATE,
				Message:      fmt.Sprintf("Event already processed recently (seen on %d nodes)", entry.count),
				WasDuplicate: true,
			}

			// Se abbiamo VM info dalla prima risposta, includiamola
			if entry.lastResponse != nil && entry.lastResponse.VmInfo != nil {
				resp.VmInfo = entry.lastResponse.VmInfo
			}

			return true, resp
		}
	}

	return false, nil
}

// recordEvent registra un evento per la deduplica
func (a *Aggregator) recordEvent(event *wolv1.WOLEvent, resp *wolv1.WOLEventResponse) {
	a.dedupeLock.Lock()
	defer a.dedupeLock.Unlock()

	a.dedupeMap[event.MacAddress] = &dedupeEntry{
		lastSeen:     time.Now(),
		count:        1,
		nodes:        []string{event.NodeName},
		lastResponse: resp,
	}
}

// StartCleanup avvia la routine di pulizia della cache di deduplica
func (a *Aggregator) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	a.log.Info("Started dedupe cache cleanup routine")

	for {
		select {
		case <-ctx.Done():
			a.log.Info("Stopping dedupe cache cleanup routine")
			return
		case <-ticker.C:
			a.cleanup()
		}
	}
}

func (a *Aggregator) cleanup() {
	a.dedupeLock.Lock()
	defer a.dedupeLock.Unlock()

	now := time.Now()
	cleaned := 0

	for mac, entry := range a.dedupeMap {
		if now.Sub(entry.lastSeen) > a.dedupeDuration*2 {
			delete(a.dedupeMap, mac)
			cleaned++
		}
	}

	if cleaned > 0 {
		a.log.V(1).Info("Cleaned up dedupe cache",
			"cleaned", cleaned,
			"remaining", len(a.dedupeMap))
	}
}

// GetStats returns aggregator statistics
func (a *Aggregator) GetStats() map[string]interface{} {
	a.dedupeLock.RLock()
	defer a.dedupeLock.RUnlock()

	return map[string]interface{}{
		"dedupe_cache_size": len(a.dedupeMap),
		"vm_mappings":       a.mapper.GetMappingCount(),
	}
}
