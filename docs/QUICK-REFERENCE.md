# Quick Reference - KubeVirt WOL Operator

**Version:** v2-final  
**Architecture:** Dynamic DaemonSet Management

---

## 🚀 Quick Start

```bash
# 1. Deploy Operator
make install
make deploy-openshift IMG=quay.io/kubevirtwol/kubevirt-wol-manager:v2-final

# 2. Create WolConfig (Agent auto-deploys)
oc apply -f - <<EOF
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: my-wol
spec:
  discoveryMode: All
  namespaceSelectors: [default]
  wolPorts: [9]
EOF

# 3. Test
./hack/test-wol.sh MAC_ADDRESS NODE_IP
```

---

## 📋 WolConfig Examples

### Basic Configuration
```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: basic-wol
spec:
  wolPorts: [9]  # Default WOL port
  discoveryMode: All
  namespaceSelectors: [default]
```

### Advanced Configuration
```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: advanced-wol
spec:
  wolPorts: [9, 7]  # Multiple ports
  discoveryMode: LabelSelector
  vmSelector:
    matchLabels:
      wol.enabled: "true"
  namespaceSelectors: [production]
  
  # Agent DaemonSet config
  agent:
    nodeSelector:
      node-role.kubernetes.io/worker: ""
    tolerations:
    - key: node-role.kubernetes.io/master
      operator: Exists
      effect: NoSchedule
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "200m"
        memory: "256Mi"
    imagePullPolicy: IfNotPresent
```

### Explicit Mappings
```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: explicit-wol
spec:
  discoveryMode: Explicit
  wolPorts: [9]
  explicitMappings:
  - macAddress: "52:54:00:12:34:56"
    vmName: my-vm-1
    namespace: default
  - macAddress: "52:54:00:ab:cd:ef"
    vmName: my-vm-2
    namespace: production
```

---

## 🔍 Common Commands

### Check Status
```bash
# WolConfigs
oc get wolconfig
oc get wolcfg  # short name

# Agent DaemonSets (created by controller)
oc get daemonset -n kubevirt-wol-system

# Pods
oc get pods -n kubevirt-wol-system -o wide

# Detailed status
oc describe wolconfig my-wol
```

### Logs
```bash
# Manager logs
oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f

# Agent logs
oc logs -n kubevirt-wol-system -l app=wol-agent -f

# Specific WolConfig's agents
oc logs -n kubevirt-wol-system -l wol.pillon.org/wolconfig=my-wol -f
```

### Test WOL
```bash
# Send WOL packet
./hack/test-wol.sh 02:f1:ef:00:00:0b 192.168.5.37

# Or with wakeonlan
wakeonlan -i 192.168.5.37 -p 9 02:f1:ef:00:00:0b
```

---

## 🎯 Key Concepts

### Dynamic DaemonSet
- **NO static DaemonSet** - Everything managed by controller
- **One DaemonSet per WolConfig** - Independent configurations
- **OwnerReference** - Automatic cleanup when WolConfig deleted

### Configuration
- **wolPorts: []int** - Array of UDP ports (default: [9])
- **agent: AgentSpec** - Full DaemonSet configuration
- **Per-WolConfig settings** - Different configs for different use cases

### Lifecycle
```
Create WolConfig → DaemonSet created → Agents deployed
Update WolConfig → DaemonSet updated → Agents rolled out
Delete WolConfig → DaemonSet deleted → Agents terminated
```

---

## 🐛 Troubleshooting

### No DaemonSet Created

**Check:**
```bash
# Controller logs
oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep DaemonSet

# WolConfig status
oc get wolconfig my-wol -o yaml
```

**Common causes:**
- Controller not running
- RBAC permissions missing
- WolConfig validation errors

### Agent Pod CrashLoopBackOff

**Check:**
```bash
# Agent logs
oc logs -n kubevirt-wol-system -l app=wol-agent

# Events
oc describe pod -n kubevirt-wol-system -l app=wol-agent
```

**Common causes:**
- Port already in use (check with `lsof -i UDP:9`)
- SCC permissions missing
- gRPC service not found

### WOL Packets Not Received

**Check:**
```bash
# Agent is listening
oc logs -n kubevirt-wol-system -l app=wol-agent | grep "UDP listener"

# Send to correct node IP
oc get pods -n kubevirt-wol-system -l app=wol-agent -o wide

# Test locally
oc exec -n kubevirt-wol-system POD_NAME -- netstat -ulpn | grep :9
```

---

## 📚 Documentation

- `FINAL-STATUS.md` - Complete status and summary
- `REFACTORING-SUCCESS.md` - Technical refactoring details
- `COMPLETE-TEST-REPORT.md` - Full test results
- `ARCHITECTURE.md` - Architecture deep dive
- `QUICK-REFERENCE.md` - This file

---

## 🎉 Success Indicators

When everything works correctly, you should see:

1. ✅ `oc get wolconfig` shows your configs
2. ✅ `oc get daemonset -n kubevirt-wol-system` shows one per WolConfig
3. ✅ `oc get pods -n kubevirt-wol-system` shows manager + agents (all 1/1 READY)
4. ✅ Test WOL → Manager logs "Received WOL event via gRPC"
5. ✅ Delete WolConfig → DaemonSet disappears automatically

**If you see all 5: CONGRATULATIONS! 🎊**

---

Quick commands:
```bash
# Status check
oc get wolconfig,daemonset,pods -n kubevirt-wol-system

# Create sample
oc apply -f config/samples/wol_v1beta1_wolconfig.yaml

# Test
./hack/test-wol.sh 02:f1:ef:00:00:0b 192.168.5.37

# Cleanup
oc delete wolconfig --all
```

