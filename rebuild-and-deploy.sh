#!/bin/bash
set -e

echo "=========================================="
echo "Rebuild and Deploy KubeVirt WOL Operator"
echo "=========================================="
echo ""

# Set image
IMG=${IMG:-quay.io/rh-ee-gpillon/kubevirt-wol:$(date +%Y%m%d-%H%M%S)}
echo "Building image: $IMG"
echo ""

# Build and push
echo "[1/3] Building and pushing image..."
make docker-build docker-push IMG=$IMG
echo "✓ Image built and pushed"
echo ""

# Deploy
echo "[2/3] Deploying to OpenShift..."
make deploy-openshift IMG=$IMG
echo "✓ Deployed"
echo ""

# Wait for rollout
echo "[3/3] Waiting for new pods..."
sleep 5
oc rollout status deployment/kubevirt-wol-controller-manager -n kubevirt-wol-system --timeout=120s

echo ""
echo "=========================================="
echo "✅ Deploy Complete!"
echo "=========================================="
echo ""
echo "Image: $IMG"
echo ""
echo "Check logs:"
echo "  oc logs -n kubevirt-wol-system -l control-plane=controller-manager -f"
echo ""
echo "Test WOL:"
echo "  ./test-wol.sh 02:f1:ef:00:00:0b"

