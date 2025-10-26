# Raw Ethernet Wake-on-LAN Support

## Overview

This document describes the support for **raw Ethernet (Layer 2) Wake-on-LAN packets** added to the kubevirt-wol operator.

## Problem Statement

Traditional WoL implementations (like Mikrotik routers) send magic packets directly as **raw Ethernet frames** without UDP/IP encapsulation. The original kubevirt-wol implementation only supported **WoL over UDP** (Layer 4), which meant these classic WoL packets were not detected.

### Packet Format Comparison

**Classic WoL (Layer 2) - NOW SUPPORTED:**
```
Ethernet Frame
├─ Destination MAC: FF:FF:FF:FF:FF:FF (broadcast)
├─ Source MAC: <sender MAC>
├─ EtherType: 0x0842 (or various)
└─ Payload: Magic Packet (FF FF FF FF FF FF + target MAC × 16)
```

**WoL over UDP (Layer 4) - ALREADY SUPPORTED:**
```
Ethernet Frame
├─ IP Header
│  ├─ Source: <sender IP>
│  └─ Destination: <broadcast IP>
├─ UDP Header
│  └─ Port: 9
└─ Payload: Magic Packet (FF FF FF FF FF FF + target MAC × 16)
```

## Solution

The agent now supports **BOTH** protocols simultaneously:
- ✅ UDP WoL (existing functionality)
- ✅ **Raw Ethernet WoL** (new functionality)

### Architecture

1. **UDP Listener** (existing): Listens on UDP port 9 (or configured port)
2. **Raw Ethernet Listener** (new): Captures raw Ethernet frames using `AF_PACKET` sockets

Both listeners feed into the same processing pipeline, ensuring unified deduplication and VM startup logic.

## Implementation Details

### New Files

- **`internal/wol/raw_listener.go`**: Raw Ethernet packet listener using Linux packet sockets

### Modified Files

- **`internal/wol/agent.go`**: 
  - Added `RawListener` integration
  - Added `enableRawWoL` flag (default: true)
  - Added `startRawListener()` method
  - Updated `Stop()` to clean up raw listener

- **`internal/wol/listener.go`**:
  - Added `IP_PKTINFO` socket option for better broadcast support

- **`internal/controller/daemonset.go`**:
  - Added `NET_RAW` capability to agent pods

- **`config/openshift/scc.yaml`**:
  - Added `NET_RAW` to allowed capabilities for OpenShift

### Security Requirements

The raw Ethernet listener requires the **`NET_RAW`** Linux capability to create packet sockets. This is automatically granted to agent pods via:

**Kubernetes:**
```yaml
securityContext:
  capabilities:
    add:
    - NET_BIND_SERVICE
    - NET_RAW
    drop:
    - ALL
```

**OpenShift:**
The `kubevirt-wol-scc` SecurityContextConstraints includes `NET_RAW` in allowed capabilities.

## Usage

### Enabling/Disabling Raw WoL

Raw Ethernet WoL is **enabled by default**. It can be disabled programmatically:

```go
agent := wol.NewAgent(port, nodeName, operatorAddr, log)
agent.SetEnableRawWoL(false) // Disable raw WoL
agent.Start(ctx)
```

### Interface Selection

The raw listener automatically selects the **default network interface**:
- First non-loopback interface
- With an active IPv4 address
- Interface UP

This is typically the host's primary network interface (e.g., `enp2s0`, `eth0`).

### Graceful Degradation

If the raw listener fails to start (e.g., missing `NET_RAW` capability), the agent continues operating with **UDP-only mode**:

```
2025-10-25T01:46:25Z INFO  Raw Ethernet WoL listener enabled, attempting to start...
2025-10-25T01:46:25Z ERROR Failed to start raw Ethernet WoL listener (continuing with UDP only)
2025-10-25T01:46:25Z INFO  Raw WoL requires NET_RAW capability - check SecurityContext
```

## Testing

### With tcpdump

Verify raw WoL packets are being received:

```bash
# On any cluster node
tcpdump -i any -nn -vv 'ether dst ff:ff:ff:ff:ff:ff' | grep -A 10 "ethertype"
```

### With wakeonlan

Send a test WoL packet:

```bash
# Classic WoL (Layer 2) - will be received by raw listener
wakeonlan <MAC-ADDRESS>

# UDP WoL (Layer 4) - will be received by UDP listener  
wakeonlan -i <broadcast-ip> -p 9 <MAC-ADDRESS>
```

### Log Messages

**Successful raw WoL reception:**
```
INFO  Valid WoL magic packet received (raw Ethernet)
      targetMAC=b0:6e:bf:c3:27:d0
      sourceMAC=d4:01:c3:78:a0:8a
      etherType=0x0842
      interface=enp2s0
```

**UDP WoL reception:**
```
INFO  Valid WOL magic packet received
      mac=b0:6e:bf:c3:27:d0
      from=192.168.5.1:54321
```

## Performance Impact

- **Minimal CPU overhead**: Raw listener processes only broadcast Ethernet frames
- **No network performance impact**: Uses promiscuous mode only for capturing, not for forwarding
- **Memory**: ~1 MB additional per agent pod (for packet buffer)

## Compatibility

### Supported WoL Sources

✅ Mikrotik RouterOS  
✅ Linux `wakeonlan` / `wol` / `etherwake` tools  
✅ Windows WoL tools (e.g., Depicus)  
✅ Router WoL features  
✅ Network management tools  
✅ Custom WoL implementations  

### Kubernetes Distributions

✅ Standard Kubernetes (1.24+)  
✅ OpenShift (4.10+)  
✅ K3s  
✅ Kind  
✅ Minikube (with `hostNetwork`)  

**Note**: Requires `hostNetwork: true` on agent pods (already configured).

## Troubleshooting

### Raw listener not starting

**Symptom:**
```
ERROR Failed to start raw Ethernet WoL listener
```

**Solution:**
1. Verify `NET_RAW` capability is granted to agent pods:
   ```bash
   kubectl get pod <agent-pod> -n kubevirt-wol-system -o json | \
     jq '.spec.containers[0].securityContext.capabilities'
   ```

2. For OpenShift, verify SCC is applied:
   ```bash
   oc get pod <agent-pod> -n kubevirt-wol-system -o yaml | \
     grep 'openshift.io/scc'
   ```

### Packets not detected

**Symptom:** WoL packets sent but VM not starting

**Debug steps:**

1. Check if packets arrive at the interface:
   ```bash
   kubectl exec -n kubevirt-wol-system <agent-pod> -- \
     tcpdump -i any -nn -c 5 'ether dst ff:ff:ff:ff:ff:ff'
   ```

2. Check agent logs for raw WoL events:
   ```bash
   kubectl logs -n kubevirt-wol-system <agent-pod> | \
     grep "raw Ethernet"
   ```

3. Verify MAC address mapping in WolConfig:
   ```bash
   kubectl get wolconfig <config-name> -o yaml
   ```

### "interrupted system call" errors

These are **normal during shutdown** and indicate clean termination. They occur when the agent receives a SIGTERM while blocked on socket read.

## Technical Notes

### Why AF_PACKET?

Linux `AF_PACKET` sockets allow capturing raw Ethernet frames before IP processing. This is necessary because WoL magic packets may:
- Use non-standard EtherTypes
- Lack IP headers entirely
- Be filtered by the kernel's IP stack

### Deduplication

Both UDP and raw WoL packets go through the same deduplication logic:
- **Local (agent)**: 2-second cache per MAC address
- **Global (manager)**: 10-second cache per MAC address

This prevents duplicate VM starts when packets are received on multiple interfaces or nodes.

### Interface Selection Logic

```go
// Priority order:
1. Non-loopback interface
2. Interface in UP state
3. Has at least one IPv4 address
4. First matching interface is selected
```

For multi-interface nodes, the raw listener binds to the first eligible interface, typically the primary network interface.

## Future Enhancements

Potential improvements:

- [ ] Multi-interface listening (capture on all interfaces)
- [ ] Configurable interface selection via WolConfig
- [ ] BPF filtering for improved performance
- [ ] IPv6 support for raw packets
- [ ] Metrics for raw vs UDP packet counts

## References

- **Wake-on-LAN Specification**: AMD Magic Packet Technology White Paper
- **Linux Packet Sockets**: `man 7 packet`
- **AF_PACKET Programming**: https://www.kernel.org/doc/Documentation/networking/packet_mmap.txt

