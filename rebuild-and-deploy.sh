#!/bin/bash
set -e

echo "=========================================="
echo "Rebuild and Deploy KubeVirt WOL Operator"
echo "    (Distributed Architecture)"
echo "=========================================="
echo ""

# Parse arguments
DEPLOY=true
TIMESTAMP=""
NEXT_IS_TAG=false

for arg in "$@"; do
    if [ "$NEXT_IS_TAG" = true ]; then
        TIMESTAMP=$arg
        NEXT_IS_TAG=false
    elif [ "$arg" = "--no-deploy" ]; then
        DEPLOY=false
    elif [ "$arg" = "--tag" ]; then
        NEXT_IS_TAG=true
    else
        echo "Unknown argument: $arg"
        echo "Usage: $0 [--tag <timestamp>] [--no-deploy]"
        exit 1
    fi
done

# If no timestamp provided, generate one
if [ -z "$TIMESTAMP" ]; then
    TIMESTAMP=$(date +%Y%m%d-%H%M%S)
fi

MANAGER_IMG=${IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:$TIMESTAMP}
# Agent image is automatically derived from manager image (same tag)
AGENT_IMG=$(echo $MANAGER_IMG | sed 's/manager/agent/g')

echo "Manager Image: $MANAGER_IMG"
echo "Agent Image:   $AGENT_IMG (auto-managed)"
if [ "$DEPLOY" = false ]; then
    echo "Mode:          BUILD ONLY (--no-deploy)"
else
    echo "Mode:          BUILD + DEPLOY"
fi
echo ""

# Build and push both manager and agent
if [ "$DEPLOY" = false ]; then
    TOTAL_STEPS=3
else
    TOTAL_STEPS=5
fi

echo "[1/$TOTAL_STEPS] Building manager and agent images..."
export CONTAINER_TOOL=podman
make docker-build-all IMG=$MANAGER_IMG
echo "✓ Images built"
echo ""

echo "[2/$TOTAL_STEPS] Pushing manager image..."
podman push $MANAGER_IMG
echo "✓ Manager pushed"
echo ""

echo "[3/$TOTAL_STEPS] Pushing agent image..."
podman push $AGENT_IMG
echo "✓ Agent pushed"
echo ""

# Deploy
if [ "$DEPLOY" = true ]; then
    echo "[4/5] Deploying to OpenShift..."
    make install
    # The Makefile will automatically inject AGENT_IMAGE env var with the correct tag
    oc delete deployment kubevirt-wol-controller-manager -n kubevirt-wol-system --ignore-not-found=true
    make deploy IMG=$MANAGER_IMG

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
fi


echo ""
echo "=========================================="
if [ "$DEPLOY" = true ]; then
    echo "✅ Deploy Complete!"
else
    echo "✅ Build Complete!"
fi
echo "=========================================="
echo ""
echo "Manager: $MANAGER_IMG"
echo "Agent:   $AGENT_IMG (injected via AGENT_IMAGE env var)"
echo ""

if [ "$DEPLOY" = true ]; then
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
else
    echo "Images built and pushed successfully."
    echo ""
    echo "To deploy, run without --no-deploy flag:"
    echo "  ./rebuild-and-deploy.sh --tag $TIMESTAMP"
    echo ""
fi

