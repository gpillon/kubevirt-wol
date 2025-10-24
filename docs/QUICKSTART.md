# Quick Start - KubeVirt WOL Distributed Architecture

## TL;DR

```bash
# 1. Crea repositories su quay.io (manualmente via web)
#    - quay.io/kubevirtwol/manager
#    - quay.io/kubevirtwol/agent

# 2. Build e push
cd /root/workdir/kubevirt-wol
make manifests generate
make docker-build-all IMG=quay.io/kubevirtwol/manager:latest
podman push quay.io/kubevirtwol/manager:latest
podman push quay.io/kubevirtwol/agent:latest

# 3. Deploy
make install
make deploy-all IMG=quay.io/kubevirtwol/manager:latest

# 4. Crea config
kubectl apply -f config/samples/wol_v1beta1_wolconfig.yaml

# 5. Test WOL packet
# From any machine on the network:
wakeonlan 00:11:22:33:44:55  # MAC della tua VM
```

## Verifica Deployment

```bash
# Check pods
kubectl get pods -n kubevirt-wol-system

# Should see:
# - 1 manager pod (operator)
# - N agent pods (one per node, DaemonSet)

# Check CRDs
kubectl get wolconfig

# Check service
kubectl get svc -n kubevirt-wol-system kubevirt-wol-grpc
```

## Architecture

**OLD**: Single pod with hostNetwork listening UDP
```
[Single Operator Pod] --UDP:9--> [Receives WOL]
```

**NEW**: Distributed with DaemonSet + gRPC
```
[Agent on Node1] --UDP:9--> [WOL packet]
[Agent on Node2] --UDP:9--> [WOL packet]
[Agent on Node3] --UDP:9--> [WOL packet]
         |
         +--> gRPC:9090 --> [Operator] --> [Start VM]
```

## What Changed

- ✅ CRD renamed: `Config` → `WolConfig`  
- ✅ Two binaries: `manager` + `agent`
- ✅ gRPC communication (Protobuf)
- ✅ DaemonSet for agents (one per node)
- ✅ Two-level deduplication (local + global)
- ✅ Following Kubernetes operator best practices

## Images

Both images built from same Dockerfile with different `BINARY` arg:
- `quay.io/kubevirtwol/manager:latest` - Operator + gRPC server
- `quay.io/kubevirtwol/agent:latest` - UDP listener + gRPC client

