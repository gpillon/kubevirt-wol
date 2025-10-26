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
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	wolv1 "github.com/gpillon/kubevirt-wol/api/wol/v1"
)

// Agent ascolta pacchetti WOL e li invia all'operatore centrale via gRPC
type Agent struct {
	port           int
	nodeName       string
	operatorAddr   string
	rawListeners   []*RawListener
	log            logr.Logger
	conn           *net.UDPConn
	grpcConn       *grpc.ClientConn
	grpcClient     wolv1.WOLServiceClient
	dedupeCache    map[string]time.Time
	dedupeLock     sync.RWMutex
	dedupeDuration time.Duration
	enableRawWoL   bool // Enable raw Ethernet WoL listener (Layer 2)
}

// NewAgent crea un nuovo agente WOL
func NewAgent(port int, nodeName, operatorAddr string, log logr.Logger) *Agent {
	if port <= 0 {
		port = DefaultWOLPort
	}

	return &Agent{
		port:           port,
		nodeName:       nodeName,
		operatorAddr:   operatorAddr,
		log:            log,
		dedupeCache:    make(map[string]time.Time),
		dedupeDuration: 2 * time.Second, // Deduplica locale veloce (2s)
		enableRawWoL:   true,            // Enable raw Ethernet WoL by default
	}
}

// SetEnableRawWoL enables or disables the raw Ethernet WoL listener
func (a *Agent) SetEnableRawWoL(enable bool) {
	a.enableRawWoL = enable
}

// Start avvia l'agente
func (a *Agent) Start(ctx context.Context) error {
	// Connetti a gRPC server con retry
	a.log.Info("Connecting to operator gRPC server", "address", a.operatorAddr)

	var err error
	a.grpcConn, err = grpc.NewClient(
		a.operatorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(1024*1024),
			grpc.MaxCallSendMsgSize(1024*1024),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to operator: %w", err)
	}

	a.grpcClient = wolv1.NewWOLServiceClient(a.grpcConn)
	a.log.Info("Connected to operator gRPC server")

	// Test connection with health check
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	healthResp, err := a.grpcClient.HealthCheck(healthCtx, &wolv1.HealthCheckRequest{Service: "wol"})
	if err != nil {
		a.log.Error(err, "Failed to check operator health, but continuing anyway")
	} else {
		a.log.Info("Operator health check", "status", healthResp.Status.String())
	}

	// Setup UDP listener
	addr := &net.UDPAddr{
		Port: a.port,
		IP:   net.IPv4zero, // 0.0.0.0 - listen on all interfaces
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", a.port, err)
	}
	a.conn = conn

	// Configura socket options
	if err := a.configureSocket(); err != nil {
		a.log.Error(err, "Failed to configure socket (continuing anyway)")
	}

	a.log.Info("WOL Agent started successfully",
		"node", a.nodeName,
		"port", a.port,
		"operatorAddr", a.operatorAddr)

	// Start raw Ethernet WoL listener (Layer 2) if enabled
	if a.enableRawWoL {
		a.log.Info("Raw Ethernet WoL listener enabled, attempting to start...")
		if err := a.startRawListener(ctx); err != nil {
			a.log.Error(err, "Failed to start raw Ethernet WoL listener (continuing with UDP only)")
			a.log.Info("Raw WoL requires NET_RAW capability - check SecurityContext")
		} else {
			a.log.Info("Raw Ethernet WoL listener started - can now receive classic WoL packets")
		}
	}

	// Start health check server
	go a.startHealthServer(ctx)

	// Start listeners
	go a.listen(ctx)
	go a.cleanupCache(ctx)

	<-ctx.Done()
	a.Stop()
	return nil
}

// configureSocket configura opzioni socket UDP per ricevere broadcast
func (a *Agent) configureSocket() error {
	file, err := a.conn.File()
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			a.log.Error(err, "Failed to close file descriptor")
		}
	}()

	fd := int(file.Fd())

	// Enable SO_REUSEADDR
	if err := syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		a.log.Error(err, "Failed to enable SO_REUSEADDR")
	} else {
		a.log.V(1).Info("SO_REUSEADDR enabled")
	}

	// Enable SO_REUSEPORT (allows multiple processes to bind to same port)
	if err := syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		a.log.Error(err, "Failed to enable SO_REUSEPORT")
	} else {
		a.log.V(1).Info("SO_REUSEPORT enabled")
	}

	// Enable SO_BROADCAST (essential for WOL)
	if err := syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_BROADCAST, 1); err != nil {
		return fmt.Errorf("SO_BROADCAST: %w", err)
	}
	a.log.Info("SO_BROADCAST enabled")

	// Enable IP_PKTINFO to receive broadcast packets sent to 255.255.255.255
	// This is crucial for receiving global broadcast packets
	if err := syscall.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_PKTINFO, 1); err != nil {
		a.log.Error(err, "Failed to enable IP_PKTINFO (continuing anyway)")
	} else {
		a.log.Info("IP_PKTINFO enabled - can now receive global broadcast (255.255.255.255)")
	}

	// Set larger read buffer
	if err := a.conn.SetReadBuffer(1024 * 64); err != nil {
		a.log.Error(err, "Failed to set read buffer size")
	}

	return nil
}

// listen loop principale per ricevere pacchetti UDP
func (a *Agent) listen(ctx context.Context) {
	buffer := make([]byte, 1024)

	a.log.Info("UDP listener loop started, waiting for WOL packets...")

	for {
		select {
		case <-ctx.Done():
			a.log.Info("Context cancelled, stopping listener")
			return
		default:
			// Set read deadline per permettere check periodici del context
			if err := a.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
				a.log.Error(err, "Failed to set read deadline")
			}

			n, addr, err := a.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout normale, continua
				}
				if ctx.Err() != nil {
					return // Context cancelled
				}
				a.log.Error(err, "Error reading UDP packet")
				ErrorsTotal.Inc()
				continue
			}

			a.log.V(1).Info("UDP packet received", "from", addr.String(), "size", n)

			// Process packet in background to avoid blocking
			go a.processPacket(ctx, buffer[:n], addr)
		}
	}
}

// processPacket processa un pacchetto WOL ricevuto
func (a *Agent) processPacket(ctx context.Context, packet []byte, addr *net.UDPAddr) {
	startTime := time.Now()

	// Parse magic packet
	mac, valid := parseMagicPacket(packet)
	if !valid {
		a.log.V(1).Info("Invalid WOL packet (not a magic packet)", "from", addr.String(), "size", len(packet))
		return
	}

	a.log.Info("Valid WOL magic packet received", "mac", mac, "from", addr.String())

	// Deduplica locale (evita di inviare stesso MAC più volte in pochi secondi)
	if !a.shouldProcess(mac) {
		a.log.V(1).Info("Skipping duplicate packet (local dedupe cache)", "mac", mac)
		return
	}

	// Crea evento gRPC
	event := &wolv1.WOLEvent{
		MacAddress: mac,
		Timestamp:  timestamppb.Now(),
		NodeName:   a.nodeName,
		SourceIp:   addr.IP.String(),
		SourcePort: uint32(addr.Port),
		PacketSize: uint32(len(packet)),
	}

	// Invia evento all'operatore via gRPC con timeout
	grpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := a.grpcClient.ReportWOLEvent(grpcCtx, event)
	if err != nil {
		a.log.Error(err, "Failed to report WOL event to operator", "mac", mac)
		ErrorsTotal.Inc()
		return
	}

	processingTime := time.Since(startTime)

	a.log.Info("Event reported to operator successfully",
		"mac", mac,
		"status", resp.Status.String(),
		"message", resp.Message,
		"wasDuplicate", resp.WasDuplicate,
		"processingTimeMs", resp.ProcessingTimeMs,
		"totalTimeMs", processingTime.Milliseconds())

	if resp.VmInfo != nil {
		a.log.Info("VM action initiated by operator",
			"mac", mac,
			"vm", resp.VmInfo.Name,
			"namespace", resp.VmInfo.Namespace,
			"state", resp.VmInfo.CurrentState)
	}

	WOLPacketsTotal.Inc()
}

// shouldProcess verifica se processare un MAC (deduplica locale)
func (a *Agent) shouldProcess(mac string) bool {
	a.dedupeLock.Lock()
	defer a.dedupeLock.Unlock()

	if lastSeen, exists := a.dedupeCache[mac]; exists {
		elapsed := time.Since(lastSeen)
		if elapsed < a.dedupeDuration {
			a.log.V(1).Info("Skipping duplicate MAC (dedupe)",
				"mac", mac,
				"lastSeenAgo", elapsed.String(),
				"dedupeWindow", a.dedupeDuration.String())
			return false
		}
	}

	a.dedupeCache[mac] = time.Now()
	return true
}

// cleanupCache pulisce periodicamente la cache di deduplica
func (a *Agent) cleanupCache(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dedupeLock.Lock()
			now := time.Now()
			for mac, lastSeen := range a.dedupeCache {
				if now.Sub(lastSeen) > a.dedupeDuration*3 {
					delete(a.dedupeCache, mac)
				}
			}
			a.dedupeLock.Unlock()
			a.log.V(1).Info("Cleaned up dedupe cache", "remaining", len(a.dedupeCache))
		}
	}
}

// startRawListener starts raw Ethernet WoL listeners on all suitable interfaces.
func (a *Agent) startRawListener(ctx context.Context) error {
	a.log.Info("Starting Raw Ethernet WoL listeners (multi-interface mode)")

	// 1️⃣ Trova tutte le interfacce candidate
	interfaces, err := GetCandidateInterfaces(a.log)
	if err != nil {
		return fmt.Errorf("failed to detect network interfaces: %w", err)
	}
	if len(interfaces) == 0 {
		return fmt.Errorf("no suitable network interfaces found for WoL listening")
	}

	// 2️⃣ Packet handler (riusa processPacket)
	packetHandler := func(mac string, srcMAC net.HardwareAddr) {
		addr := &net.UDPAddr{IP: net.IPv4bcast, Port: 0}

		packet := make([]byte, MagicPacketSize)
		for i := 0; i < 6; i++ {
			packet[i] = 0xFF
		}
		macBytes, _ := net.ParseMAC(mac)
		for i := 0; i < 16; i++ {
			copy(packet[6+i*6:6+(i+1)*6], macBytes)
		}

		a.log.V(7).Info("Raw Ethernet WoL packet forwarded to processing",
			"targetMAC", mac,
			"sourceMAC", srcMAC.String())

		// Usa la logica esistente per gestire l'evento
		go a.processPacket(ctx, packet, addr)
	}

	// 3️⃣ Avvia un listener per ciascuna interfaccia
	var started []string
	a.rawListeners = nil // slice dei listener per stop futuro

	for _, iface := range interfaces {
		name := iface.Name
		listener := NewRawListenerWithOptions(
			name,
			packetHandler,
			a.log.WithValues("iface", name),
			RawListenerOptions{
				Promiscuous:    true, // cattura tutto il broadcast
				AttachBPF:      true, // TEMP DISABLED FOR DEBUG
				RecvTimeoutSec: 1,
			},
		)

		if err := listener.Start(ctx); err != nil {
			a.log.Error(err, "Failed to start WoL listener", "iface", name)
			continue
		}

		a.rawListeners = append(a.rawListeners, listener)
		started = append(started, name)
	}

	// 4️⃣ Log riassuntivo
	if len(started) == 0 {
		return fmt.Errorf("no WoL listeners started successfully")
	}

	a.log.Info("Raw Ethernet WoL listeners started",
		"count", len(started),
		"interfaces", strings.Join(started, ", "))

	return nil
}

// Stop ferma l'agente
func (a *Agent) Stop() {
	a.log.Info("Stopping WOL Agent...")

	if a.conn != nil {
		if err := a.conn.Close(); err != nil {
			a.log.Error(err, "Failed to close UDP connection")
		}
		a.log.Info("UDP listener stopped")
	}

	a.stopRawListeners()

	if a.grpcConn != nil {
		if err := a.grpcConn.Close(); err != nil {
			a.log.Error(err, "Failed to close gRPC connection")
		}
		a.log.Info("gRPC connection closed")
	}

	a.log.Info("WOL Agent stopped successfully")
}

func (a *Agent) stopRawListeners() {
	for _, l := range a.rawListeners {
		l.Stop()
	}
	a.log.Info("All raw listeners stopped")
}

// startHealthServer starts HTTP server for health checks and metrics
func (a *Agent) startHealthServer(ctx context.Context) {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// Check if gRPC connection is healthy
		if a.grpcConn == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte("gRPC connection not established")); err != nil {
				a.log.Error(err, "Failed to write health check response")
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			a.log.Error(err, "Failed to write health check response")
		}
	})

	// Readiness check endpoint
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Check if UDP listener is active
		if a.conn == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte("UDP listener not active")); err != nil {
				a.log.Error(err, "Failed to write readiness check response")
			}
			return
		}
		// Check gRPC connection
		if a.grpcConn == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte("gRPC connection not established")); err != nil {
				a.log.Error(err, "Failed to write readiness check response")
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ready")); err != nil {
			a.log.Error(err, "Failed to write readiness check response")
		}
	})

	// Metrics endpoint (basic Prometheus format)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		a.dedupeLock.RLock()
		cacheSize := len(a.dedupeCache)
		a.dedupeLock.RUnlock()

		w.Header().Set("Content-Type", "text/plain")
		if _, err := fmt.Fprintf(w, "# HELP wol_agent_dedupe_cache_size Number of entries in deduplication cache\n"); err != nil {
			a.log.Error(err, "Failed to write metrics")
		}
		if _, err := fmt.Fprintf(w, "# TYPE wol_agent_dedupe_cache_size gauge\n"); err != nil {
			a.log.Error(err, "Failed to write metrics")
		}
		if _, err := fmt.Fprintf(w, "wol_agent_dedupe_cache_size{node=\"%s\"} %d\n", a.nodeName, cacheSize); err != nil {
			a.log.Error(err, "Failed to write metrics")
		}
		if _, err := fmt.Fprintf(w, "# HELP wol_agent_info Agent information\n"); err != nil {
			a.log.Error(err, "Failed to write metrics")
		}
		if _, err := fmt.Fprintf(w, "# TYPE wol_agent_info gauge\n"); err != nil {
			a.log.Error(err, "Failed to write metrics")
		}
		if _, err := fmt.Fprintf(w, "wol_agent_info{node=\"%s\",port=\"%d\",operator=\"%s\"} 1\n",
			a.nodeName, a.port, a.operatorAddr); err != nil {
			a.log.Error(err, "Failed to write metrics")
		}
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	a.log.Info("Starting health check server", "port", 8080)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			a.log.Error(err, "Failed to shutdown health check server")
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		a.log.Error(err, "Health check server failed")
	}
}
