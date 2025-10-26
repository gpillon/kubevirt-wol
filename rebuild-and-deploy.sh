#!/bin/bash
set -e

echo "=========================================="
echo "Rebuild and Deploy KubeVirt WOL Operator"
echo "    (Distributed Architecture)"
echo "=========================================="
echo ""

#se quest oscript è chiamatocon il paramentro testing, devo mettere i tag di test, tipo "development"
if [ "x$1" != "x" ]; then
    TIMESTAMP=$1
else
    TIMESTAMP=$(date +%Y%m%d-%H%M%S)
fi

MANAGER_IMG=${IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:$TIMESTAMP}
# Agent image is automatically derived from manager image (same tag)
AGENT_IMG=$(echo $MANAGER_IMG | sed 's/manager/agent/g')

echo "Manager Image: $MANAGER_IMG"
echo "Agent Image:   $AGENT_IMG (auto-managed)"
echo ""

# Build and push both manager and agent
echo "[1/5] Building manager and agent images..."
export CONTAINER_TOOL=podman
make docker-build-all IMG=$MANAGER_IMG
echo "✓ Images built"
echo ""

echo "[2/5] Pushing manager image..."
podman push $MANAGER_IMG
echo "✓ Manager pushed"
echo ""

echo "[3/5] Pushing agent image..."
podman push $AGENT_IMG
echo "✓ Agent pushed"
echo ""

# Deploy
echo "[4/5] Deploying to OpenShift..."
make install
# The Makefile will automatically inject AGENT_IMAGE env var with the correct tag
oc delete deployment kubevirt-wol-controller-manager -n kubevirt-wol-system --ignore-not-found=true
make deploy-openshift IMG=$MANAGER_IMG

# Note: Agent DaemonSets are now created dynamically by the controller
#       based on WolConfig CRDs. The AGENT_IMAGE env var is automatically
#       set in the manager deployment by the Makefile.

echo "✓ Deployed"
echo ""

echo "[5/5] Apply sample CRD (will trigger dynamic agent deployment)"
oc apply -f config/samples/wol_v1beta1_wolconfig-default.yaml

# Wait for rollout
echo "Waiting for pods..."
sleep 5
echo "- Checking manager..."
oc rollout status deployment/kubevirt-wol-controller-manager -n kubevirt-wol-system --timeout=120s || true
echo "- Checking agents (dynamically created by controller)..."
oc delete daemonset wol-agent-default -n kubevirt-wol-system --ignore-not-found=true
sleep 3  # Give controller time to create DaemonSet
# oc rollout status daemonset/wol-agent-default -n kubevirt-wol-system --timeout=120s || true
# oc get daemonset -n kubevirt-wol-system wol-agent-default -o wide 2>/dev/null || echo "  Agent DaemonSet deploying..."


echo ""
echo "=========================================="
echo "✅ Deploy Complete!"
echo "=========================================="
echo ""
echo "Manager: $MANAGER_IMG"
echo "Agent:   $AGENT_IMG (injected via AGENT_IMAGE env var)"
echo ""
echo "Note: Agent DaemonSets are created dynamically by the controller"
echo "      when WolConfig resources are applied. They will use the"
echo "      same version tag as the manager automatically."
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

