#!/bin/bash
set -e

echo "=========================================="
echo "KubeVirt WOL - Deploy Completo da Zero"
echo "    Distributed Architecture"
echo "=========================================="
echo ""

# Configurazione
MANAGER_IMG=${MANAGER_IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:20251024-132534}
AGENT_IMG=${AGENT_IMG:-quay.io/kubevirtwol/kubevirt-wol-agent:20251024-132534}

echo "Manager Image: $MANAGER_IMG"
echo "Agent Image:   $AGENT_IMG"
echo ""

# Step 1: Installa CRDs
echo "[1/5] Installazione CRDs..."
make install
echo "✅ CRDs installate"
echo ""

# Step 2: Deploy manager (operator)
echo "[2/5] Deploy manager (operator)..."
cd config/manager && ../../bin/kustomize edit set image controller=$MANAGER_IMG && cd ../..
make deploy-openshift IMG=$MANAGER_IMG
echo "✅ Manager deployato"
echo ""

# Step 3: Deploy agent (DaemonSet)
echo "[3/5] Deploy agent (DaemonSet)..."
cd config/agent && ../../bin/kustomize edit set image agent=$AGENT_IMG && cd ../..
oc apply -k config/agent
echo "✅ Agent deployato"
echo ""

# Step 4: Verifica SCC
echo "[4/5] Verifica SCC per agent..."
echo "SCC kubevirt-wol-scc configurato nei manifest ✅"
echo ""

# Step 5: Verifica deployment
echo "[5/5] Verifica deployment..."
sleep 10

echo "Pods:"
oc get pods -n kubevirt-wol-system -o wide

echo ""
echo "Services:"
oc get svc -n kubevirt-wol-system

echo ""
echo "ServiceMonitor:"
oc get servicemonitor -n kubevirt-wol-system 2>/dev/null || echo "  ServiceMonitor non trovato (potrebbe richiedere Prometheus Operator)"

echo ""
echo "CRDs:"
oc get crd | grep wolconfig

echo ""
echo "=========================================="
echo "✅ Deploy Completato!"
echo "=========================================="
echo ""

echo "Attendi che i pods siano pronti..."
echo "  oc get pods -n kubevirt-wol-system -w"
echo ""
echo "Verifica logs:"
echo "  Manager: oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f"
echo "  Agent:   oc logs -n kubevirt-wol-system -l app=wol-agent -f"
echo ""
echo "Test WOL:"
echo "  ./hack/test-wol.sh MAC_ADDRESS NODE_IP"
echo ""

