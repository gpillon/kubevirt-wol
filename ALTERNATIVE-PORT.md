# Alternative: Using Non-Privileged Port for WOL

If you're still getting "permission denied" on port 9 despite having `NET_BIND_SERVICE` capability, OpenShift's security policies may be preventing it. Here's an alternative solution using a non-privileged port.

## Why This Happens

Port 9 is a privileged port (< 1024). Even with:
- ✅ `NET_BIND_SERVICE` capability configured
- ✅ Correct SCC applied
- ✅ All permissions seemingly correct

OpenShift may still block it due to:
- SELinux enforcement
- Additional security contexts
- Container runtime restrictions

## Solution: Use Port 9009

Instead of fighting OpenShift's security, use a higher port and configure network forwarding.

### Step 1: Configure WOL on Port 9009

Create a WOLConfig with a non-privileged port:

```yaml
apiVersion: wol.pillon.org/v1beta1
kind: Config
metadata:
  name: wol-config
spec:
  discoveryMode: All
  namespaceSelectors:
    - default
  wolPort: 9009  # Non-privileged port
  cacheTTL: 300
```

### Step 2: Apply Configuration

```bash
kubectl apply -f - <<EOF
apiVersion: wol.pillon.org/v1beta1
kind: Config
metadata:
  name: wol-config
spec:
  discoveryMode: All
  wolPort: 9009
  cacheTTL: 300
EOF
```

### Step 3: Network Forwarding

Forward UDP port 9 to 9009 at the network level. Choose one method:

#### Option A: iptables on OpenShift Nodes

```bash
# On each OpenShift node where the operator may run
sudo iptables -t nat -A PREROUTING -p udp --dport 9 -j REDIRECT --to-port 9009
sudo iptables-save > /etc/sysconfig/iptables
```

#### Option B: External Load Balancer / Firewall

Configure your external network infrastructure to forward:
- UDP 9 → UDP 9009 to the OpenShift node

#### Option C: NetworkPolicy / Service

Create a NodePort service (if your network allows):

```yaml
apiVersion: v1
kind: Service
metadata:
  name: wol-nodeport
  namespace: kubevirt-wol-system
spec:
  type: NodePort
  selector:
    control-plane: controller-manager
  ports:
    - protocol: UDP
      port: 9009
      targetPort: 9009
      nodePort: 30009  # Or let K8s assign
```

Then forward external UDP 9 → NodePort 30009

### Step 4: Verify

```bash
# Check operator logs
oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep "WOL listener"

# Expected output:
# INFO    listener        WOL listener started    {"port": 9009}

# Test from another machine
wakeonlan -i <node-ip> -p 9 52:54:00:12:34:56
# or
wakeonlan -i <node-ip> -p 9009 52:54:00:12:34:56  # Direct to 9009
```

## Pros and Cons

### Using Port 9009 (Non-Privileged)

**Pros:**
- ✅ No capability issues
- ✅ Works with restrictive OpenShift policies
- ✅ More portable across environments
- ✅ No SCC modifications needed

**Cons:**
- ❌ Requires network configuration
- ❌ Not the "standard" WOL port
- ❌ Clients need to know about port mapping

### Using Port 9 (Standard)

**Pros:**
- ✅ Standard WOL port
- ✅ No network configuration needed
- ✅ Works with any WOL client

**Cons:**
- ❌ Requires privileged port binding
- ❌ Complex OpenShift SCC setup
- ❌ May not work on all platforms

## Recommendation

For production OpenShift deployments, **use port 9009** with network forwarding. It's more reliable and avoids fighting with OpenShift's security model.

For development or less restrictive environments, port 9 with proper SCC is fine.

## Troubleshooting Port 9009

If WOL doesn't work on 9009:

1. **Verify operator is listening:**
   ```bash
   oc exec -n kubevirt-wol-system $(oc get pod -n kubevirt-wol-system -l control-plane=controller-manager -o name) -- netstat -ulpn | grep 9009
   ```

2. **Test directly to 9009:**
   ```bash
   # From a pod in the cluster
   echo -ne '\xFF\xFF\xFF\xFF\xFF\xFF\x52\x54\x00\x12\x34\x56' | nc -u <pod-ip> 9009
   ```

3. **Check network forwarding:**
   ```bash
   # Verify iptables rule
   sudo iptables -t nat -L PREROUTING -n -v | grep 9009
   ```

4. **Check firewall:**
   ```bash
   # On OpenShift node
   sudo firewall-cmd --list-all | grep 9009
   sudo firewall-cmd --add-port=9009/udp --permanent
   sudo firewall-cmd --reload
   ```

## Automated Setup

We can add this to the operator in the future:
- Auto-detect if port 9 fails
- Fall back to 9009 automatically
- Log clear instructions for network forwarding

