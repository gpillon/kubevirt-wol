#!/bin/bash
set -e

echo "=========================================="
echo "Rebuild and Deploy KubeVirt WOL Operator"
echo "    (Distributed Architecture)"
echo "=========================================="
echo ""

#se quest oscript è chiamatocon il paramentro testing, devo mettere i tag di test, tipo "development"
if [ "$1" == "development" ]; then
    TIMESTAMP="development"
else
    TIMESTAMP=$(date +%Y%m%d-%H%M%S)
fi

# Set images
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
MANAGER_IMG=${IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:$TIMESTAMP}
AGENT_IMG=${AGENT_IMG:-quay.io/kubevirtwol/kubevirt-wol-agent:$TIMESTAMP}

echo "Manager Image: $MANAGER_IMG"
echo "Agent Image: $AGENT_IMG"
echo ""

# Build and push both manager and agent
echo "[1/6] Building manager and agent images..."
export CONTAINER_TOOL=podman
make docker-build-all IMG=$MANAGER_IMG
echo "✓ Images built"
echo ""

# Tag agent correctly (fix makefile naming issue)
echo "[2/6] Tagging images correctly..."
podman tag quay.io/kubevirtwol/kubevirt-wol-agent-agent:$TIMESTAMP $AGENT_IMG 2>/dev/null || \
podman tag localhost/quay.io/kubevirtwol/kubevirt-wol-agent-agent:$TIMESTAMP $AGENT_IMG 2>/dev/null || true
echo "✓ Tagged"
echo ""

echo "[3/6] Pushing manager image..."
podman push $MANAGER_IMG
echo "✓ Manager pushed"
echo ""

echo "[4/6] Pushing agent image..."
podman push $AGENT_IMG
echo "✓ Agent pushed"
echo ""

# Deploy
echo "[5/6] Deploying to OpenShift..."
make install
make deploy-openshift IMG=$MANAGER_IMG

# Deploy agent with correct image
cd config/agent && ../../bin/kustomize edit set image agent=$AGENT_IMG && cd ../..
oc apply -k config/agent

echo "✓ Deployed"
echo ""

echo "[6/6] Apply sample CRD"
oc apply -f config/samples/wol_v1beta1_wolconfig-default.yaml

# Wait for rollout
echo "Waiting for pods..."
sleep 5
echo "- Checking manager..."
oc rollout status deployment/kubevirt-wol-controller-manager -n kubevirt-wol-system --timeout=120s || true
echo "- Checking agents..."
oc rollout status daemonset/wol-agent-default -n kubevirt-wol-system --timeout=120s || true
oc get daemonset -n kubevirt-wol-system wol-agent-default -o wide 2>/dev/null || echo "  Agent DaemonSet deploying..."

echo ""
echo "=========================================="
echo "✅ Deploy Complete!"
echo "=========================================="
echo ""
echo "Manager: $MANAGER_IMG"
echo "Agent:   $AGENT_IMG"
echo ""
echo "Check pods:"
echo "  oc get pods -n kubevirt-wol-system -o wide"
echo ""
echo "Manager logs:"
echo "  oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f"
echo ""
echo "Agent logs:"
echo "  oc logs -n kubevirt-wol-system -l app=wol-agent --all-containers -f"
echo ""
echo "Test WOL:"
echo "  ./hack/test-wol.sh 02:f1:ef:00:00:0b 192.168.5.255"
echo ""

