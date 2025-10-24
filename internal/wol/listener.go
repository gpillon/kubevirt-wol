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
	"syscall"

	"github.com/go-logr/logr"
	"golang.org/x/sys/unix"
)

const (
	// DefaultWOLPort is the standard Wake-on-LAN UDP port
	DefaultWOLPort = 9
	// MagicPacketSize is the minimum size of a WOL magic packet (6 + 6*16 = 102 bytes)
	MagicPacketSize = 102
)

// Listener handles incoming Wake-on-LAN packets
type Listener struct {
	port      int
	mapper    *MACMapper
	vmStarter *VMStarter
	log       logr.Logger
	conn      *net.UDPConn
}

// NewListener creates a new WOL listener
func NewListener(port int, mapper *MACMapper, vmStarter *VMStarter, log logr.Logger) *Listener {
	if port <= 0 {
		port = DefaultWOLPort
	}
	return &Listener{
		port:      port,
		mapper:    mapper,
		vmStarter: vmStarter,
		log:       log,
	}
}

// Start begins listening for WOL packets
func (l *Listener) Start(ctx context.Context) error {
	addr := &net.UDPAddr{
		Port: l.port,
		IP:   net.IPv4zero, // 0.0.0.0 - listen on all interfaces
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", l.port, err)
	}
	l.conn = conn

	// Get underlying file descriptor to set socket options
	file, err := conn.File()
	if err != nil {
		l.log.Error(err, "Failed to get socket file descriptor")
	} else {
		fd := int(file.Fd())

		// Enable SO_REUSEADDR to allow multiple binds
		if err := syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			l.log.Error(err, "Failed to enable SO_REUSEADDR")
		} else {
			l.log.Info("SO_REUSEADDR enabled")
		}

		// Enable SO_REUSEPORT to allow multiple processes to bind
		if err := syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
			l.log.Error(err, "Failed to enable SO_REUSEPORT")
		} else {
			l.log.Info("SO_REUSEPORT enabled")
		}

		// Enable SO_BROADCAST to handle broadcast packets
		if err := syscallSetBroadcast(fd); err != nil {
			l.log.Error(err, "Failed to enable SO_BROADCAST")
		} else {
			l.log.Info("SO_BROADCAST enabled")
		}

		// Close the file descriptor copy (conn still owns the original)
		file.Close()
	}

	// Set read buffer size (larger for handling multiple packets)
	if err := conn.SetReadBuffer(1024 * 64); err != nil {
		l.log.Error(err, "Failed to set read buffer size")
	}

	// Log actual local address we're bound to
	localAddr := conn.LocalAddr().String()
	l.log.Info("WOL listener started", "port", l.port, "bindAddress", "0.0.0.0", "actualAddress", localAddr)

	// Start listening in a goroutine
	go l.listen(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	l.Stop()
	return nil
}

// listen is the main listening loop
func (l *Listener) listen(ctx context.Context) {
	buffer := make([]byte, 1024)

	l.log.Info("UDP listener loop started, waiting for packets...")

	for {
		select {
		case <-ctx.Done():
			l.log.Info("Listener context done, exiting")
			return
		default:
			n, addr, err := l.conn.ReadFromUDP(buffer)
			if err != nil {
				if ctx.Err() != nil {
					// Context was cancelled, exit gracefully
					return
				}
				l.log.Error(err, "Error reading UDP packet")
				ErrorsTotal.Inc()
				continue
			}

			// Log EVERY packet received for debugging
			l.log.Info("UDP packet received", "from", addr.String(), "size", n, "port", addr.Port)

			// Process packet in background to avoid blocking
			go l.processPacket(ctx, buffer[:n], addr)
		}
	}
}

// processPacket processes a received WOL packet
func (l *Listener) processPacket(ctx context.Context, packet []byte, addr *net.UDPAddr) {
	WOLPacketsTotal.Inc()

	// Parse and validate magic packet
	mac, valid := parseMagicPacket(packet)
	if !valid {
		l.log.V(1).Info("Invalid WOL packet received", "from", addr.String(), "size", len(packet))
		return
	}

	l.log.Info("Valid WOL packet received", "mac", mac, "from", addr.String())

	// Lookup VM for this MAC address
	vmInfo, found := l.mapper.Lookup(mac)
	if !found {
		l.log.Info("No VM found for MAC address", "mac", mac)
		return
	}

	l.log.Info("Starting VM for WOL request",
		"mac", mac,
		"vm", vmInfo.Name,
		"namespace", vmInfo.Namespace,
		"from", addr.String())

	// Start the VM
	if err := l.vmStarter.StartVM(ctx, vmInfo.Namespace, vmInfo.Name); err != nil {
		l.log.Error(err, "Failed to start VM",
			"vm", vmInfo.Name,
			"namespace", vmInfo.Namespace,
			"mac", mac)
	}
}

// Stop stops the listener
func (l *Listener) Stop() {
	if l.conn != nil {
		l.conn.Close()
		l.log.Info("WOL listener stopped")
	}
}

// UpdatePort updates the listening port (requires restart)
func (l *Listener) UpdatePort(port int) {
	l.port = port
}

// parseMagicPacket validates and extracts the MAC address from a WOL magic packet
// A valid magic packet contains:
// - 6 bytes of 0xFF
// - 16 repetitions of the target MAC address (6 bytes each)
func parseMagicPacket(packet []byte) (string, bool) {
	// Check minimum size
	if len(packet) < MagicPacketSize {
		return "", false
	}

	// Check for 6 bytes of 0xFF at the start
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			return "", false
		}
	}

	// Extract the MAC address from the first repetition (bytes 6-11)
	macBytes := packet[6:12]

	// Verify that the MAC is repeated 16 times
	for i := 1; i < 16; i++ {
		offset := 6 + (i * 6)
		for j := 0; j < 6; j++ {
			if packet[offset+j] != macBytes[j] {
				return "", false
			}
		}
	}

	// Format MAC address as string (lowercase with colons)
	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		macBytes[0], macBytes[1], macBytes[2],
		macBytes[3], macBytes[4], macBytes[5])

	return mac, true
}

// syscallSetBroadcast enables SO_BROADCAST on the socket to receive broadcast packets
func syscallSetBroadcast(fd int) error {
	// SO_BROADCAST = 6 on most systems, but use unix.SO_BROADCAST for portability
	return syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
}
