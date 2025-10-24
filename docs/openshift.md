# OpenShift Deployment Guide

This guide covers OpenShift-specific configuration for deploying the KubeVirt WOL Operator.

## What's Different on OpenShift

OpenShift adds additional security requirements compared to standard Kubernetes:

1. **Custom Security Context Constraint (SCC)**
   - Allows `hostNetwork: true` (required for broadcast UDP)
   - Allows `NET_BIND_SERVICE` capability (required for privileged ports)
   - Minimum permissions (no privilege escalation, must run as non-root)
   - Bound to the operator's ServiceAccount

2. **Port Configuration**
   - Changes health probe port from 8081 → 8088
   - Adds `NET_BIND_SERVICE` capability to container
   - Avoids conflicts when using hostNetwork with OpenShift nodes
   - Metrics remain on 8443 (HTTPS)
   - WOL listener remains on configured UDP ports

## Quick Deployment

### Prerequisites

- OpenShift cluster with KubeVirt installed
- Cluster admin privileges (required for SCC creation)
- Container registry access (e.g., Quay.io)

### Deploy with Make

```bash
# Build and push images
make docker-build docker-push IMG=<your-registry>/kubevirt-wol:tag

# Install CRDs
make install

# Deploy to OpenShift (includes SCC configuration)
make deploy-openshift IMG=<your-registry>/kubevirt-wol:tag
```

### Deploy with Script

```bash
# Using the deployment script
IMG=<your-registry>/kubevirt-wol:tag ./deploy-openshift.sh
```

### Manual Deployment

```bash
# Build manifests with OpenShift overlay
kustomize build config/openshift | oc apply -f -

# Create WolConfig instance
oc apply -f config/samples/wol_v1beta1_wolconfig.yaml
```

## Configuration Files

The OpenShift overlay is located in `config/openshift/` and includes:

- `kustomization.yaml` - Main kustomize overlay
- `scc.yaml` - Custom SecurityContextConstraint and RBAC
- `manager_patch.yaml` - Deployment patches for OpenShift compatibility

## Verify Deployment

### Check SCC Assignment

```bash
# Verify SCC is assigned to pods
oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].metadata.annotations.openshift\.io/scc}'
# Should output: kubevirt-wol-scc
```

### Check Pods Status

```bash
# Check manager and agent pods
oc get pods -n kubevirt-wol-system

# Check logs
oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f
oc logs -n kubevirt-wol-system -l app=wol-agent -f
```

### Check WolConfig

```bash
# List WolConfigs
oc get wolconfig

# Check details
oc describe wolconfig <config-name>
```

## Troubleshooting

### Pod Rejected by SCC

**Symptom:** Pod status shows `CreateContainerConfigError` or `FailedCreate`

**Solution:**
```bash
# Check SCC exists
oc get scc kubevirt-wol-scc

# Check ServiceAccount has SCC permissions
oc get rolebinding -n kubevirt-wol-system | grep kubevirt-wol

# Manually grant SCC if needed
oc adm policy add-scc-to-user kubevirt-wol-scc -z kubevirt-wol-controller-manager -n kubevirt-wol-system
```

### Health Check Port Conflicts

**Symptom:** Pod crashes with "address already in use" error on port 8081

**Solution:**
The OpenShift overlay already configures an alternative port (8088). Verify the deployment is using the OpenShift kustomization:
```bash
# Should show port 8088 in health probe
oc get deployment -n kubevirt-wol-system kubevirt-wol-controller-manager -o yaml | grep 8088
```

### Agent Not Receiving WOL Packets

**Symptom:** No logs when sending WOL packets

**Checks:**
```bash
# Verify agent is using hostNetwork
oc get pod -n kubevirt-wol-system -l app=wol-agent -o yaml | grep hostNetwork

# Check agent logs
oc logs -n kubevirt-wol-system -l app=wol-agent

# Verify firewall rules on nodes
# (SSH to node if accessible)
sudo firewall-cmd --list-ports | grep udp
```

## Security Considerations

### Custom SCC Requirements

The operator requires a custom SCC with these capabilities:

- `hostNetwork: true` - To receive broadcast UDP packets
- `NET_BIND_SERVICE` capability - To bind to privileged ports (< 1024)

These are the minimum permissions needed for Wake-on-LAN functionality.

### RBAC Permissions

The operator requires cluster-wide permissions for:
- Reading VirtualMachine resources
- Starting/stopping VirtualMachines
- Managing DaemonSets

Review the RBAC configuration in `config/rbac/` before deployment.

## Uninstall

```bash
# Delete WolConfig instances
oc delete wolconfig --all

# Uninstall operator
make undeploy

# Remove CRDs
make uninstall

# Remove SCC (if needed)
oc delete scc kubevirt-wol-scc
```

## Additional Resources

- OpenShift Security Context Constraints: https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html
- KubeVirt Documentation: https://kubevirt.io/user-guide/

