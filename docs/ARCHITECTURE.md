# KubeVirt WOL - Distributed Architecture

## Overview

The project has been completely restructured with a distributed architecture that follows enterprise-grade Kubernetes operator best practices (like Prometheus Operator, Cilium, Istio).

## Architecture

```
┌─────────────────────────────────────────────────────┐
│              Kubernetes Cluster                      │
│                                                      │
│  ┌─────────┐    ┌─────────┐    ┌─────────┐         │
│  │ Node 1  │    │ Node 2  │    │ Node 3  │         │
│  │         │    │         │    │         │         │
│  │ ┌─────┐ │    │ ┌─────┐ │    │ ┌─────┐ │         │
│  │ │ WOL │ │    │ │ WOL │ │    │ │ WOL │ │         │
│  │ │Agent│ │    │ │Agent│ │    │ │Agent│ │         │
│  │ └──┬──┘ │    │ └──┬──┘ │    │ └──┬──┘ │         │
│  └────┼────┘    └────┼────┘    └────┼────┘         │
│       │UDP:9         │UDP:9         │UDP:9          │
│       │              │              │               │
│       └──────────────┼──────────────┘               │
│                gRPC  │                               │
│              ┌───────▼────────┐                      │
│              │  WOL Manager   │                      │
│              │   (Operator)   │                      │
│              │                │                      │
│              │ - Aggregator   │                      │
│              │ - Dedup        │                      │
│              │ - MAC→VM map   │                      │
│              │ - Start VM     │                      │
│              └────────────────┘                      │
└─────────────────────────────────────────────────────┘
```

## Components

### 1. WOL Manager (Operator)
- **Deployment** (can scale horizontally)
- **Port**: 9090 (gRPC server)
- **Responsibilities**:
  - Receives WOL events from agents via gRPC
  - Global deduplication (same VM requested from multiple nodes)
  - Maintains MAC→VM mapping
  - Starts KubeVirt VirtualMachines
  - Reconciliation loop for `WolConfig` CRD

### 2. WOL Agent (DaemonSet)
- **DaemonSet** (one pod per node)
- **Port**: 9 UDP (standard Wake-on-LAN)
- **Configuration**: `hostNetwork: true` (required for broadcast UDP)
- **Responsibilities**:
  - Listens for broadcast WOL packets on each node
  - Local deduplication (2 seconds)
  - Sends events to Manager via gRPC
  - Lightweight and stateless

### 3. gRPC Communication
- **Protocol**: gRPC with Protobuf (type-safe, performant)
- **Health checks**: Natively supported
- **Two-level deduplication**:
  - **Local** (agent): 2 seconds
  - **Global** (manager): 10 seconds

## Custom Resource Definition

### WolConfig (formerly Config)

```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: default
spec:
  discoveryMode: All  # All | LabelSelector | Explicit
  wolPort: 9
  cacheTTL: 300
  # Optional: specific to discovery mode
  namespaceSelectors: []
  vmSelector: {}
  explicitMappings: []
```

## Build & Deploy

### 1. Create Repository on Quay.io

Before pushing, create the repositories:
- https://quay.io/new/?namespace=<your-org> (create `manager` and `agent`)

### 2. Build Images

```bash
# Build both images
make docker-build-all IMG=<your-registry>/manager:latest

# Verify
podman images | grep <your-org>
```

### 3. Push Images

```bash
# Login (if needed)
podman login <your-registry>

# Push both
make docker-push-all IMG=<your-registry>/manager:latest

# Or individually
podman push <your-registry>/manager:latest
podman push <your-registry>/agent:latest
```

### 4. Deploy to Kubernetes

```bash
# Install CRDs
make install

# Deploy everything (manager + agent)
make deploy-all IMG=<your-registry>/manager:latest

# Or separately
make deploy IMG=<your-registry>/manager:latest
make deploy-agent IMG=<your-registry>/manager:latest
```

### 5. Deploy to OpenShift

```bash
# OpenShift needs custom SCC
make deploy-openshift IMG=<your-registry>/manager:latest

# Then deploy agent
make deploy-agent IMG=<your-registry>/manager:latest
```

### 6. Create a WolConfig

```bash
kubectl apply -f config/samples/wol_v1beta1_wolconfig.yaml
```

## Development

### Generate Protobuf

```bash
make proto
```

### Local Builds

```bash
# Build both binaries
make build

# Individually
make build-manager
make build-agent
```

### Local Testing

```bash
# Terminal 1: Manager
make run

# Terminal 2: Agent (requires NODE_NAME)
NODE_NAME=test-node make run-agent
```

## Makefile Targets

### Build
- `make build` - Build all binaries
- `make build-manager` - Build only manager
- `make build-agent` - Build only agent

### Docker
- `make docker-build-all` - Build both images
- `make docker-build-manager` - Build manager image
- `make docker-build-agent` - Build agent image
- `make docker-push-all` - Push both images

### Deploy
- `make deploy-all` - Deploy manager + agent
- `make deploy` - Deploy only manager
- `make deploy-agent` - Deploy only agent
- `make deploy-openshift` - Deploy to OpenShift

### Development
- `make proto` - Generate code from .proto files
- `make manifests` - Generate CRD and RBAC
- `make generate` - Generate DeepCopy methods

## Advantages of New Architecture

1. ✅ **High Availability**: Agent on every node ensures WOL reception anywhere
2. ✅ **Scalability**: Manager can have multiple replicas
3. ✅ **Smart Deduplication**: Local + Global = 0 duplicates
4. ✅ **Observability**: See which node each WOL comes from
5. ✅ **Security**: Only agent uses `hostNetwork`, manager is isolated
6. ✅ **Cloud-native**: Follows standard Kubernetes patterns
7. ✅ **Best Practices**: Architecture identical to Prometheus Operator, Cilium, etc.

## Troubleshooting

### Agent not receiving WOL packets

```bash
# Verify agent is using hostNetwork
kubectl get pod -n kubevirt-wol-system -l app=wol-agent -o yaml | grep hostNetwork

# Verify logs
kubectl logs -n kubevirt-wol-system -l app=wol-agent --tail=50
```

### Manager not receiving events from agent

```bash
# Verify gRPC Service
kubectl get svc -n kubevirt-wol-system kubevirt-wol-grpc

# Test connectivity from agent to manager
kubectl exec -n kubevirt-wol-system deploy/kubevirt-wol-controller-manager -- \
  nc -zv kubevirt-wol-grpc 9090
```

### VM not starting

```bash
# Verify MAC→VM mapping
kubectl get wolconfig default -o yaml

# Verify manager logs
kubectl logs -n kubevirt-wol-system deploy/kubevirt-wol-controller-manager
```

## Next Steps (Optional)

- [ ] Add gRPC authentication (mTLS)
- [ ] Implement gRPC streaming for performance
- [ ] Add Grafana dashboard for metrics
- [ ] Support IPv6
- [ ] Helm chart for simplified deployment
