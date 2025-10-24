# Deploying on OpenShift

This guide provides instructions for deploying the KubeVirt WOL Operator on OpenShift, which has additional security requirements via Security Context Constraints (SCC).

## Prerequisites

- OpenShift cluster with KubeVirt installed
- Cluster admin privileges (required to create SCC)
- `oc` CLI tool configured

## Why Special Configuration is Needed

The WOL operator needs to:
1. Use `hostNetwork: true` to receive broadcast UDP packets
2. Listen on UDP port 9 (or configured port)

OpenShift's default SCCs don't allow host network access for security reasons. We provide a custom SCC that grants only the minimum necessary permissions.

Additionally, when using `hostNetwork: true`, the operator shares the network namespace with the host node. To avoid port conflicts with services running on the node, the OpenShift deployment uses non-standard ports:

- **Health probes**: Port 8088 (instead of 8081)
- **Metrics**: Port 8443 (HTTPS)
- **WOL listener**: Port 9 (UDP, standard WOL port)

## Deployment Steps

### Option 1: Using Kustomize with OpenShift Overlay (Recommended)

```bash
# 1. Build and push the image
make docker-build docker-push IMG=quay.io/gpillon/kubevirt-wol:latest

# 2. Deploy using OpenShift kustomization (includes SCC)
oc apply -k config/openshift

# 3. Set the image
cd config/openshift && oc kustomize edit set image controller=quay.io/gpillon/kubevirt-wol:latest
oc apply -k config/openshift
```

### Option 2: Manual Deployment

```bash
# 1. Install CRDs
make install

# 2. Create the custom SCC
oc apply -f config/openshift/scc.yaml

# 3. Deploy the operator
make deploy IMG=quay.io/gpillon/kubevirt-wol:latest

# 4. Verify SCC is bound
oc get clusterrolebinding kubevirt-wol-scc-binding
```

### Option 3: Using Existing Privileged SCC (Not Recommended)

If you cannot create a custom SCC, you can use OpenShift's `hostnetwork` SCC:

```bash
# Grant hostnetwork SCC to the service account
oc adm policy add-scc-to-user hostnetwork \
  system:serviceaccount:kubevirt-wol-system:controller-manager

# Deploy the operator
make deploy IMG=quay.io/gpillon/kubevirt-wol:latest
```

⚠️ **Warning**: This grants more permissions than necessary. Use the custom SCC when possible.

## Verification

### Check Pod Status

```bash
oc get pods -n kubevirt-wol-system
```

Expected output:
```
NAME                                                READY   STATUS    RESTARTS   AGE
kubevirt-wol-controller-manager-xxxxxxxxxx-xxxxx    1/1     Running   0          30s
```

### Check SCC Assignment

```bash
oc get pod -n kubevirt-wol-system -o yaml | grep openshift.io/scc
```

Expected output:
```
openshift.io/scc: kubevirt-wol-scc
```

### Check Logs

```bash
oc logs -n kubevirt-wol-system deployment/kubevirt-wol-controller-manager
```

Look for:
```
INFO    setup   initializing WOL components
INFO    setup   starting WOL listener
INFO    listener        WOL listener started    {"port": 9}
```

## Troubleshooting

### WOL Listener Fails with "permission denied"

**Symptom:**
```
ERROR setup WOL listener error {"error": "failed to listen on UDP port 9: listen udp4 0.0.0.0:9: bind: permission denied"}
```

**Cause:**
Port 9 is a privileged port (< 1024) and requires the `NET_BIND_SERVICE` capability to bind as non-root.

**Solution:**
This should be automatically handled by the OpenShift configuration. If you see this error:

```bash
# 1. Verify the SCC allows NET_BIND_SERVICE
oc get scc kubevirt-wol-scc -o yaml | grep -A 5 allowedCapabilities

# Should show:
# allowedCapabilities:
# - NET_BIND_SERVICE

# 2. Check the pod's security context
oc get pod -n kubevirt-wol-system -o yaml | grep -A 5 "capabilities:"

# Should show:
# capabilities:
#   add:
#   - NET_BIND_SERVICE
#   drop:
#   - ALL

# 3. If missing, reapply the SCC and redeploy
oc apply -f config/openshift/scc.yaml
oc delete pod -n kubevirt-wol-system -l control-plane=controller-manager
```

### Pod Crashes with "address already in use"

**Symptom:**
```
ERROR setup unable to start manager {"error": "error listening on :8081: listen tcp :8081: bind: address already in use"}
```

**Cause:**
With `hostNetwork: true`, the pod shares the host's network namespace. Port 8081 is likely already in use by another service on the node.

**Solution:**
This should not happen with the OpenShift deployment as it uses port 8088 for health probes. If you deployed using the standard `make deploy` instead of `make deploy-openshift`, redeploy with:

```bash
# Undeploy standard version
make undeploy

# Deploy OpenShift version with correct ports
make deploy-openshift IMG=quay.io/gpillon/kubevirt-wol:latest
```

If using a custom deployment, ensure you're using the OpenShift configuration which patches the ports.

### Pod Fails with SCC Error

**Symptom:**
```
unable to validate against any security context constraint: [...] provider restricted-v2: .spec.securityContext.hostNetwork: Invalid value: true: Host network is not allowed to be used
```

**Solutions:**

1. **Verify SCC was created:**
   ```bash
   oc get scc kubevirt-wol-scc
   ```

2. **Verify ClusterRoleBinding exists:**
   ```bash
   oc get clusterrolebinding kubevirt-wol-scc-binding
   ```

3. **Check ServiceAccount has SCC access:**
   ```bash
   oc adm policy who-can use scc kubevirt-wol-scc
   ```
   Should show `system:serviceaccount:kubevirt-wol-system:controller-manager`

4. **Manually bind SCC if needed:**
   ```bash
   oc adm policy add-scc-to-user kubevirt-wol-scc \
     system:serviceaccount:kubevirt-wol-system:controller-manager
   ```

5. **Delete and recreate the pod:**
   ```bash
   oc delete pod -n kubevirt-wol-system -l control-plane=controller-manager
   ```

### SCC Cannot Be Created (No Cluster Admin)

If you don't have cluster admin privileges, ask your cluster administrator to:

1. Create the SCC:
   ```bash
   oc apply -f config/openshift/scc.yaml
   ```

2. Grant access to the operator's ServiceAccount:
   ```bash
   oc adm policy add-scc-to-user kubevirt-wol-scc \
     system:serviceaccount:kubevirt-wol-system:controller-manager
   ```

### Operator Not Receiving WOL Packets

1. **Verify hostNetwork is enabled:**
   ```bash
   oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].spec.hostNetwork}'
   ```
   Should output: `true`

2. **Check operator is listening on correct interface:**
   ```bash
   oc exec -n kubevirt-wol-system deployment/kubevirt-wol-controller-manager -- netstat -ulpn
   ```
   Should show: `0.0.0.0:9` (or configured port)

3. **Test with local WOL packet:**
   ```bash
   # From a pod on the same node
   oc debug node/<node-name>
   # Then send WOL packet
   echo -ne '\xFF\xFF\xFF\xFF\xFF\xFF' > /tmp/wol
   for i in {1..16}; do echo -ne '\x52\x54\x00\x12\x34\x56' >> /tmp/wol; done
   cat /tmp/wol | nc -u -w1 <node-ip> 9
   ```

## Security Considerations

The `kubevirt-wol-scc` SCC grants minimal permissions:

✅ **Allowed:**
- Host network access (required for broadcast UDP)
- Host ports (required to listen on UDP port 9)
- Run as non-root user
- `NET_BIND_SERVICE` capability (required to bind to port 9, which is < 1024)

❌ **Not Allowed:**
- Host PID namespace
- Host IPC namespace
- Privileged containers
- Host path volumes
- Privilege escalation
- Running as root
- Any other capabilities beyond NET_BIND_SERVICE

## Network Configuration

### Firewall Rules

Ensure UDP port 9 (or configured port) is allowed:

```bash
# On the OpenShift node (if using firewalld)
firewall-cmd --add-port=9/udp --permanent
firewall-cmd --reload
```

### Network Policies

If using NetworkPolicies, allow UDP ingress on port 9:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-wol-udp
  namespace: kubevirt-wol-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  ingress:
    - ports:
        - protocol: UDP
          port: 9
  policyTypes:
    - Ingress
```

## Upgrading

```bash
# Update the image
make deploy IMG=quay.io/gpillon/kubevirt-wol:v2.0.0

# Restart pods
oc rollout restart deployment/kubevirt-wol-controller-manager -n kubevirt-wol-system
```

## Uninstalling

```bash
# Delete the operator
oc delete -k config/openshift

# Or manually
make undeploy
oc delete -f config/openshift/scc.yaml

# Delete CRDs
make uninstall
```

## Production Recommendations

1. **Use a dedicated node pool** for the WOL operator with appropriate network access
2. **Set resource limits** in the deployment
3. **Configure RBAC** to limit VM management to specific namespaces
4. **Monitor metrics** via Prometheus ServiceMonitor
5. **Set up alerts** for WOL errors and failed VM starts
6. **Use WOLConfig with explicit mappings** for production environments

## Example Production WOLConfig

```yaml
apiVersion: wol.pillon.org/v1beta1
kind: Config
metadata:
  name: production-wol
spec:
  discoveryMode: LabelSelector
  vmSelector:
    matchLabels:
      environment: production
      wol.pillon.org/enabled: "true"
  namespaceSelectors:
    - production-vms
  wolPort: 9
  cacheTTL: 600
```

## Support

For OpenShift-specific issues, check:
1. Operator logs: `oc logs -n kubevirt-wol-system deployment/kubevirt-wol-controller-manager`
2. Events: `oc get events -n kubevirt-wol-system --sort-by='.lastTimestamp'`
3. SCC audit logs: `oc get pod -n kubevirt-wol-system -o yaml`

