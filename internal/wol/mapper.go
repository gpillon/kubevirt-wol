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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wolv1beta1 "github.com/gpillon/kubevirt-wol/api/v1beta1"
)

// VMInfo stores information about a discovered VM
type VMInfo struct {
	Name      string
	Namespace string
}

// MACMapper manages the mapping between MAC addresses and VMs
type MACMapper struct {
	client   client.Client
	log      logr.Logger
	mu       sync.RWMutex
	mapping  map[string]VMInfo // MAC address (lowercase) -> VM info
	lastSync time.Time
	cacheTTL time.Duration
	config   *wolv1beta1.WolConfig
}

// NewMACMapper creates a new MAC to VM mapper
func NewMACMapper(client client.Client, log logr.Logger) *MACMapper {
	return &MACMapper{
		client:   client,
		log:      log,
		mapping:  make(map[string]VMInfo),
		cacheTTL: 300 * time.Second, // default 5 minutes
	}
}

// UpdateConfig updates the mapper configuration
func (m *MACMapper) UpdateConfig(config *wolv1beta1.WolConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
	if config.Spec.CacheTTL > 0 {
		m.cacheTTL = time.Duration(config.Spec.CacheTTL) * time.Second
	}
}

// RefreshMapping refreshes the MAC to VM mapping based on current config
func (m *MACMapper) RefreshMapping(ctx context.Context) error {
	m.mu.Lock()
	config := m.config
	m.mu.Unlock()

	if config == nil {
		return fmt.Errorf("no config set")
	}

	newMapping := make(map[string]VMInfo)

	switch config.Spec.DiscoveryMode {
	case wolv1beta1.DiscoveryModeExplicit:
		// Use explicit mappings from config
		for _, mapping := range config.Spec.ExplicitMappings {
			mac := normalizeMACAddress(mapping.MACAddress)
			newMapping[mac] = VMInfo{
				Name:      mapping.VMName,
				Namespace: mapping.Namespace,
			}
		}
		m.log.Info("Using explicit MAC mappings", "count", len(newMapping))

	case wolv1beta1.DiscoveryModeLabelSelector:
		// Discover VMs using label selector
		if err := m.discoverVMsWithSelector(ctx, config, newMapping); err != nil {
			return fmt.Errorf("failed to discover VMs with selector: %w", err)
		}

	default: // DiscoveryModeAll
		// Discover all VMs in selected namespaces
		if err := m.discoverAllVMs(ctx, config, newMapping); err != nil {
			return fmt.Errorf("failed to discover all VMs: %w", err)
		}
	}

	// Update mapping
	m.mu.Lock()
	m.mapping = newMapping
	m.lastSync = time.Now()
	m.mu.Unlock()

	// Update metrics
	ManagedVMs.Set(float64(len(newMapping)))

	m.log.Info("MAC mapping refreshed", "vmCount", len(newMapping))
	return nil
}

// discoverAllVMs discovers all VMs in selected namespaces
func (m *MACMapper) discoverAllVMs(ctx context.Context, config *wolv1beta1.WolConfig, mapping map[string]VMInfo) error {
	namespaces := config.Spec.NamespaceSelectors
	if len(namespaces) == 0 {
		// If no namespaces specified, list all VMs across all namespaces
		vmList := &kubevirtv1.VirtualMachineList{}
		if err := m.client.List(ctx, vmList); err != nil {
			return fmt.Errorf("failed to list VMs: %w", err)
		}
		m.extractMACsFromVMs(vmList.Items, mapping)
	} else {
		// List VMs in each specified namespace
		for _, ns := range namespaces {
			vmList := &kubevirtv1.VirtualMachineList{}
			if err := m.client.List(ctx, vmList, client.InNamespace(ns)); err != nil {
				m.log.Error(err, "Failed to list VMs in namespace", "namespace", ns)
				continue
			}
			m.extractMACsFromVMs(vmList.Items, mapping)
		}
	}
	return nil
}

// discoverVMsWithSelector discovers VMs matching the label selector
func (m *MACMapper) discoverVMsWithSelector(ctx context.Context, config *wolv1beta1.WolConfig, mapping map[string]VMInfo) error {
	if config.Spec.VMSelector == nil {
		return fmt.Errorf("VMSelector is nil in LabelSelector mode")
	}

	selector, err := metav1.LabelSelectorAsSelector(config.Spec.VMSelector)
	if err != nil {
		return fmt.Errorf("invalid label selector: %w", err)
	}

	namespaces := config.Spec.NamespaceSelectors
	if len(namespaces) == 0 {
		// List across all namespaces with label selector
		vmList := &kubevirtv1.VirtualMachineList{}
		if err := m.client.List(ctx, vmList, &client.ListOptions{
			LabelSelector: selector,
		}); err != nil {
			return fmt.Errorf("failed to list VMs with selector: %w", err)
		}
		m.extractMACsFromVMs(vmList.Items, mapping)
	} else {
		// List in each namespace with label selector
		for _, ns := range namespaces {
			vmList := &kubevirtv1.VirtualMachineList{}
			if err := m.client.List(ctx, vmList, &client.ListOptions{
				Namespace:     ns,
				LabelSelector: selector,
			}); err != nil {
				m.log.Error(err, "Failed to list VMs in namespace with selector", "namespace", ns)
				continue
			}
			m.extractMACsFromVMs(vmList.Items, mapping)
		}
	}
	return nil
}

// extractMACsFromVMs extracts MAC addresses from VM specs
func (m *MACMapper) extractMACsFromVMs(vms []kubevirtv1.VirtualMachine, mapping map[string]VMInfo) {
	for _, vm := range vms {
		if vm.Spec.Template == nil {
			continue
		}

		// Extract MAC addresses from network interfaces
		networks := vm.Spec.Template.Spec.Domain.Devices.Interfaces
		for _, iface := range networks {
			if iface.MacAddress != "" {
				mac := normalizeMACAddress(iface.MacAddress)
				mapping[mac] = VMInfo{
					Name:      vm.Name,
					Namespace: vm.Namespace,
				}
				m.log.V(1).Info("Discovered VM MAC",
					"mac", mac,
					"vm", vm.Name,
					"namespace", vm.Namespace)
			}
		}
	}
}

// Lookup returns the VM info for a given MAC address
func (m *MACMapper) Lookup(macAddress string) (VMInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mac := normalizeMACAddress(macAddress)
	vmInfo, found := m.mapping[mac]
	return vmInfo, found
}

// GetMappingCount returns the number of MAC addresses in the mapping
func (m *MACMapper) GetMappingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.mapping)
}

// NeedRefresh returns true if the mapping needs to be refreshed
func (m *MACMapper) NeedRefresh() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.lastSync) > m.cacheTTL
}

// GetLastSync returns the last sync time
func (m *MACMapper) GetLastSync() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSync
}

// normalizeMACAddress converts MAC address to lowercase and standardized format
func normalizeMACAddress(mac string) string {
	return strings.ToLower(strings.TrimSpace(mac))
}

// MatchesSelector checks if VM labels match the selector
func MatchesSelector(vmLabels map[string]string, selector *metav1.LabelSelector) (bool, error) {
	if selector == nil {
		return true, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return false, err
	}

	return labelSelector.Matches(labels.Set(vmLabels)), nil
}
