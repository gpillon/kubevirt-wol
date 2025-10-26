package wol

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"golang.org/x/sys/unix"
)

// -------------------- Opzioni & costruttori --------------------

type RawListenerOptions struct {
	Promiscuous    bool // default true
	AttachBPF      bool // default true
	RecvTimeoutSec int  // default 1
}

type RawListener struct {
	interfaceName string
	fd            int
	log           logr.Logger
	packetHandler func(mac string, srcMAC net.HardwareAddr)

	promisc   bool
	attachBPF bool
	rcvTOsec  int

	stopOnce sync.Once
	closed   atomic.Bool
	wg       sync.WaitGroup // Per aspettare che la goroutine finisca
}

// Backward-compatible constructor (same signature as prima)
func NewRawListener(interfaceName string, packetHandler func(mac string, srcMAC net.HardwareAddr), log logr.Logger) *RawListener {
	return NewRawListenerWithOptions(interfaceName, packetHandler, log, RawListenerOptions{
		Promiscuous:    true,
		AttachBPF:      true,
		RecvTimeoutSec: 1,
	})
}

func NewRawListenerWithOptions(interfaceName string, packetHandler func(mac string, srcMAC net.HardwareAddr), log logr.Logger, opt RawListenerOptions) *RawListener {
	if opt.RecvTimeoutSec <= 0 {
		opt.RecvTimeoutSec = 1
	}
	return &RawListener{
		interfaceName: interfaceName,
		fd:            -1,
		log:           log,
		packetHandler: packetHandler,
		promisc:       opt.Promiscuous,
		attachBPF:     opt.AttachBPF,
		rcvTOsec:      opt.RecvTimeoutSec,
	}
}

// -------------------- Avvio / Arresto --------------------

func (r *RawListener) Start(ctx context.Context) error {
	// Get interface
	ifi, err := net.InterfaceByName(r.interfaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %w", r.interfaceName, err)
	}

	r.log.Info("Starting raw Ethernet WoL listener",
		"interface", ifi.Name,
		"mac", ifi.HardwareAddr.String(),
		"mtu", ifi.MTU)

	// Create raw socket
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		return fmt.Errorf("failed to create raw socket: %w (requires CAP_NET_RAW)", err)
	}
	r.fd = fd

	// Bind to interface
	addr := &unix.SockaddrLinklayer{
		Protocol: htons(unix.ETH_P_ALL),
		Ifindex:  ifi.Index,
	}
	if err := unix.Bind(fd, addr); err != nil {
		unix.Close(fd)
		r.fd = -1
		return fmt.Errorf("failed to bind to interface %s: %w", ifi.Name, err)
	}

	// Optional: promiscuous mode
	if r.promisc {
		mreq := &unix.PacketMreq{
			Ifindex: int32(ifi.Index),
			Type:    unix.PACKET_MR_PROMISC,
		}
		if err := unix.SetsockoptPacketMreq(fd, unix.SOL_PACKET, unix.PACKET_ADD_MEMBERSHIP, mreq); err != nil {
			r.log.V(1).Info("Failed to set promiscuous mode (continuing)", "error", err)
		}
	}

	// Optional: attach BPF to accept only EtherType 0x0842 (WoL L2)
	if r.attachBPF {
		// Classic BPF program:
		//  Load half at [12] (EtherType), accept if == 0x0842, else drop
		bpf := []unix.SockFilter{
			// ldh [12] - Load halfword (16-bit) at offset 12 (EtherType position)
			{Code: 0x28, Jt: 0, Jf: 0, K: 12},
			// jeq #0x0842 - Jump if equal to 0x0842
			// Jt: 0 = if match, go to next instruction (accept)
			// Jf: 1 = if no match, skip 1 instruction (drop)
			{Code: 0x15, Jt: 0, Jf: 1, K: 0x0842},
			// ret #0x40000 (accept entire packet - snaplen)
			{Code: 0x6, Jt: 0, Jf: 0, K: 0x00040000},
			// ret #0 (drop packet)
			{Code: 0x6, Jt: 0, Jf: 0, K: 0x00000000},
		}
		fprog := unix.SockFprog{
			Len:    uint16(len(bpf)),
			Filter: &bpf[0],
		}
		if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &fprog); err != nil {
			r.log.V(1).Info("Failed to attach BPF filter (continuing)", "error", err)
		}
	}

	// Set socket receive timeout ONCE (avoid doing it in loop)
	tv := &unix.Timeval{Sec: int64(r.rcvTOsec), Usec: 0}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, tv); err != nil {
		r.log.V(1).Info("Failed to set SO_RCVTIMEO (continuing)", "error", err)
	}

	r.log.Info("Raw Ethernet listener started", "interface", r.interfaceName, "fd", fd)

	// Start loop
	r.wg.Add(1)
	go r.listen(ctx)
	return nil
}

func (r *RawListener) Stop() {
	r.stopOnce.Do(func() {
		if r.closed.Load() {
			return
		}
		r.closed.Store(true)
		if r.fd >= 0 {
			// Unblock any Recvfrom
			_ = unix.Shutdown(r.fd, unix.SHUT_RD)
			if err := unix.Close(r.fd); err != nil {
				r.log.Error(err, "Failed to close raw socket")
			}
			r.fd = -1
		}
		// Aspetta che la goroutine finisca
		r.wg.Wait()
		r.log.Info("Raw Ethernet listener stopped")
	})
}

// -------------------- Loop di ascolto --------------------

func (r *RawListener) listen(ctx context.Context) {
	defer r.wg.Done()
	buffer := make([]byte, 2000) // un po' più di 1500 per eventuali tag
	r.log.Info("Raw Ethernet listener loop started, waiting for WoL packets...")

	for {
		if ctx.Err() != nil || r.closed.Load() {
			r.log.Info("Context cancelled or listener closed, stopping raw listener loop")
			return
		}

		n, _, err := unix.Recvfrom(r.fd, buffer, 0)
		if err != nil {
			// normal timeouts or interruptions
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK || err == unix.EINTR {
				continue
			}
			if ctx.Err() != nil || r.closed.Load() {
				return
			}
			r.log.Error(err, "Error reading raw packet")
			// If you have metrics:
			// ErrorsTotal.Inc()
			continue
		}
		if n <= 14 {
			continue
		}

		r.processEthernetFrame(buffer[:n])
	}
}

// -------------------- Parsing frame --------------------

func (r *RawListener) processEthernetFrame(frame []byte) {
	// Ethernet header: 14 bytes
	dstMAC := frame[0:6]
	srcMAC := frame[6:12]
	etherType := binary.BigEndian.Uint16(frame[12:14])
	payload := frame[14:]

	// VLAN 802.1Q tag (0x8100): shift di 4 byte e leggi EtherType interno
	if etherType == 0x8100 {
		if len(payload) < 4 {
			return
		}
		// payload[0:2] = TCI, payload[2:4] = inner EtherType
		etherType = binary.BigEndian.Uint16(payload[2:4])
		payload = payload[4:]
	}

	// // Log ALL broadcast packets for debugging (temporary - change to V(1) in production)
	// if isBroadcastMAC(dstMAC) {
	// 	r.log.V(1).Info("DEBUG: Broadcast packet received",
	// 		"etherType", fmt.Sprintf("0x%04x", etherType),
	// 		"srcMAC", net.HardwareAddr(srcMAC).String(),
	// 		"payloadSize", len(payload),
	// 		"interface", r.interfaceName)
	// }

	// WoL L2 classico: EtherType 0x0842
	if etherType != 0x0842 {
		// Non è WoL L2; se vuoi, potresti anche analizzare IPv4/UDP:9 qui
		return
	}

	// Deve essere broadcast
	if !isBroadcastMAC(dstMAC) {
		return
	}

	// Payload deve contenere magic packet
	mac, valid := parseMagicPacket(payload)
	if !valid {
		return
	}

	src := net.HardwareAddr(append([]byte{}, srcMAC...)) // copia
	r.log.Info("Valid WoL magic packet received (raw Ethernet)",
		"targetMAC", mac,
		"sourceMAC", src.String(),
		"etherType", fmt.Sprintf("0x%04x", etherType),
		"interface", r.interfaceName,
		"payloadSize", len(payload))

	if r.packetHandler != nil {
		r.packetHandler(mac, src)
	}

	// If you have metrics:
	// WoLDetectedTotal.Inc()
}

// -------------------- Helpers --------------------

func isBroadcastMAC(b []byte) bool {
	if len(b) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if b[i] != 0xFF {
			return false
		}
	}
	return true
}

// htons converts uint16 from host to network byte order (big-endian)
func htons(v uint16) uint16 { return (v << 8) | (v >> 8) }

func GetCandidateInterfaces(log logr.Logger) ([]net.Interface, error) {
	var result []net.Interface
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Fase 1: raccogli tutte le interfacce candidabili
	for _, iface := range interfaces {
		name := iface.Name

		// Skip loopback or down
		if (iface.Flags&net.FlagLoopback) != 0 || (iface.Flags&net.FlagUp) == 0 {
			continue
		}
		if (iface.Flags & net.FlagBroadcast) == 0 {
			continue
		}

		// Skip virtual / OVS internal interfaces
		if strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "br-int") ||
			strings.HasPrefix(name, "ovn-") ||
			strings.HasPrefix(name, "tap") ||
			strings.HasPrefix(name, "ovs-system") ||
			strings.Contains(name, "@if") {
			continue
		}

		// Include physical NICs, Wi-Fi, and Linux bridges
		if strings.HasPrefix(name, "en") ||
			strings.HasPrefix(name, "eth") ||
			strings.HasPrefix(name, "wlp") ||
			strings.HasPrefix(name, "br-") {
			result = append(result, iface)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no suitable interfaces found")
	}

	// Fase 2: deduplica per MAC (bridge > fisico)
	deduped := make(map[string]net.Interface)
	for _, iface := range result {
		mac := iface.HardwareAddr.String()
		if existing, ok := deduped[mac]; ok {
			// Regola: preferisci i bridge (br-*) ai fisici
			if strings.HasPrefix(existing.Name, "br-") {
				log.V(1).Info("Skipping duplicate MAC (bridge already present)",
					"iface", iface.Name, "mac", mac, "kept", existing.Name)
				continue
			}
			if strings.HasPrefix(iface.Name, "br-") {
				log.V(1).Info("Replacing physical interface with bridge (same MAC)",
					"iface", iface.Name, "mac", mac, "replaced", existing.Name)
				deduped[mac] = iface
				continue
			}
			// altrimenti tieni il primo e ignora il secondo
			log.V(1).Info("Skipping duplicate MAC", "iface", iface.Name, "mac", mac)
			continue
		}
		deduped[mac] = iface
	}

	final := make([]net.Interface, 0, len(deduped))
	for _, iface := range deduped {
		log.Info("Selected WoL interface candidate",
			"interface", iface.Name,
			"mac", iface.HardwareAddr.String())
		final = append(final, iface)
	}

	sort.Slice(final, func(i, j int) bool {
		return final[i].Name < final[j].Name
	})

	return final, nil
}
