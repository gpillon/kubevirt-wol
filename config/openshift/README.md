# OpenShift Configuration

This directory contains OpenShift-specific configuration for deploying the KubeVirt WOL Operator.

## What's Different

This overlay adds:

1. **Custom Security Context Constraint (SCC)** - `scc.yaml`
   - Allows `hostNetwork: true` (required for broadcast UDP)
   - Allows `NET_BIND_SERVICE` capability (required for port 9)
   - Minimum permissions (no privilege escalation, must run as non-root)
   - Bound to the operator's ServiceAccount

2. **Port Configuration Patch** - `manager_patch.yaml`
   - Changes health probe port from 8081 → 8088
   - Adds `NET_BIND_SERVICE` capability to container
   - Avoids conflicts when using hostNetwork with OpenShift nodes
   - Metrics remain on 8443 (HTTPS)
   - WOL listener remains on 9 (UDP)

## Usage

### Deploy

```bash
# Quick deploy (recommended)
IMG=quay.io/gpillon/kubevirt-wol:latest ./deploy-openshift.sh

# Or manually
make deploy-openshift IMG=quay.io/gpillon/kubevirt-wol:latest
```

### Verify

```bash
# Check SCC assignment
oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].metadata.annotations.openshift\.io/scc}'

# Should output: kubevirt-wol-scc
```

### Build Manifests

```bash
# Preview the generated manifests
kustomize build config/openshift

# Apply
kustomize build config/openshift | oc apply -f -
```

## Files

- `kustomization.yaml` - Main kustomize overlay for OpenShift
- `scc.yaml` - Custom SecurityContextConstraint + RBAC
- `manager_patch.yaml` - Deployment patch for non-conflicting ports
- `README.md` - This file

## Troubleshooting

See [OPENSHIFT.md](../../OPENSHIFT.md) in the root directory for detailed troubleshooting.

## Why Not Use Standard Deployment?

OpenShift has two key differences from standard Kubernetes:

1. **Security Context Constraints (SCC)** - More restrictive than PodSecurityPolicies
   - Default SCCs don't allow hostNetwork
   - Need custom SCC with explicit permissions

2. **Host Network Port Conflicts** - OpenShift nodes run more system services
   - Port 8081 often in use by node services
   - Need to use alternative ports (8088)

Using `make deploy` (standard K8s) on OpenShift will result in:
- Pod rejected by SCC (hostNetwork forbidden)
- OR if SCC is manually granted, pod crashes (port 8081 in use)

Always use `make deploy-openshift` for OpenShift clusters.

