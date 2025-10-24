# Testing Wake-on-LAN Operator

The operator looks running successfully! Here's how to test it.

## Current Status

✅ **WOL Listener**: Running on UDP port 9
✅ **Managed VMs**: 5 VMs discovered
✅ **Pod Node**: <OCP_NODE>

### Discovered VMs

```
02:f1:ef:00:00:0b → <VM_NAME_1> (default)
02:f1:ef:00:00:03 → <VM_NAME_2> (default)
#...other VMs
```

## Testing Methods

### Method 1: Using Test Script

```bash
# Test with broadcast
./test-wol.sh 02:f1:ef:00:00:0b

# Test to specific node
./test-wol.sh 02:f1:ef:00:00:0b <IP_OF_OCP_NODE>
```

### Method 2: Using wakeonlan Command

```bash
# Install if needed
dnf install -y wol

# Send WOL packet
wakeonlan -i <NODE_IP> -p 9 02:f1:ef:00:00:0b

# Or broadcast
wakeonlan -i 255.255.255.255 -p 9 02:f1:ef:00:00:0b
```

### Method 3: Using Python

```python
import socket

mac = '02:f1:ef:00:00:0b'
target_ip = '<IP_OF_OCP_NODE>'  # or '255.255.255.255' for broadcast
port = 9

# Create magic packet
mac_bytes = bytes.fromhex(mac.replace(':', ''))
magic_packet = b'\xff' * 6 + mac_bytes * 16

# Send
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
sock.sendto(magic_packet, (target_ip, port))
sock.close()

print(f"WOL packet sent to {target_ip}:9 for MAC {mac}")
```

## Troubleshooting

### Issue: No logs when sending WOL packet

**Possible causes:**

1. **Packet not reaching the node**
   - The operator runs on node `<OCP_NODE>` with `hostNetwork: true`
   - WOL packets must reach this specific node
   - Check your network routing

2. **Firewall blocking UDP port 9**
   ```bash
   # On the OpenShift node
   sudo firewall-cmd --list-all | grep 9
   sudo firewall-cmd --add-port=9/udp --permanent
   sudo firewall-cmd --reload
   ```

3. **Wrong network interface**
   - WOL broadcasts need to be on the same network segment
   - Find the node's IP:
     ```bash
     oc get node <OCP_NODE> -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}'
     ```

### Verification Steps

#### 1. Check listener is actually running

```bash
oc exec -n kubevirt-wol-system $(oc get pod -n kubevirt-wol-system -l control-plane=controller-manager -o name) -- netstat -ulpn 2>/dev/null || echo "netstat not available"
```

Expected output should show something listening on `:9`

#### 2. Monitor logs in real-time

```bash
oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f
```

Then send a WOL packet from another terminal.

#### 3. Get node IP address

```bash
oc get node <OCP_NODE> -o wide
```

Use the INTERNAL-IP to send WOL packets directly to that node.

#### 4. Test from inside the cluster

Create a test pod:

```bash
oc run wol-test --image=alpine --rm -it -- sh
```

Inside the pod:

```sh
# Install tools
apk add --no-cache python3

# Create and send WOL packet
python3 << 'EOF'
import socket
mac = '02:f1:ef:00:00:0b'
mac_bytes = bytes.fromhex(mac.replace(':', ''))
magic = b'\xff' * 6 + mac_bytes * 16
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
# Send to the operator pod's node
sock.sendto(magic, ('<NODE_IP>', 9))
print("Sent WOL packet")
EOF
```

#### 5. Check operator can start VMs

The MAC mapping is correct, so if a WOL packet arrives, it should try to start the VM. Check:

```bash
# Watch for VM start attempts
oc get vm -n <MANESPACE> <VM_NAME> -o jsonpath='{.spec.running}'

# Should change from false to true when WOL is received
```

### Expected Log Output

When a WOL packet is successfully received, you should see:

```
INFO    listener   Valid WOL packet received    {"mac": "02:f1:ef:00:00:0b", "from": "<source_ip>:<port>"}
INFO    listener   Starting VM for WOL request  {"mac": "02:f1:ef:00:00:0b", "vm": "<VM_NAME>", "namespace": "default", "from": "<source_ip>:<port>"}
INFO    vmstarter  Successfully started VM      {"vm": "<VM_NAME>", "namespace": "default"}
```

## Network Requirements

For WOL to work correctly:

1. **Same Network Segment**: Your WOL sender should be on the same network as the OpenShift node
2. **Broadcast Support**: Network must support UDP broadcast
3. **Firewall**: Port 9/UDP must be open on the node
4. **Routing**: If using specific IPs, ensure routing is correct

## Alternative: Test with oc rsh

```bash
# Shell into the operator pod
oc rsh -n kubevirt-wol-system $(oc get pod -n kubevirt-wol-system -l control-plane=controller-manager -o name)

# Check if something is listening on port 9
# (if netstat/ss are available)
netstat -ulpn | grep :9
ss -ulpn | grep :9
```

## Quick Test: Send WOL from Same Node

If you have access to the OpenShift node directly:

```bash
# SSH to <NODE_NAME>
# Then send WOL locally
echo -ne '\xFF\xFF\xFF\xFF\xFF\xFF\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B\x02\xF1\xEF\x00\x00\x0B' | nc -u localhost 9
```

This sends the WOL packet to localhost, which should definitely work if the listener is running.

## Still Not Working?

Enable debug logging by editing the deployment:

```bash
oc edit deployment -n kubevirt-wol-system kubevirt-wol-controller-manager
```

Add to args:
```yaml
- --zap-log-level=debug
- --zap-devel=true
```

This will show much more detailed logging including packet reception attempts.

