#!/bin/bash
set -e

echo "=========================================="
echo "Bundle Testing for KubeVirt WOL Operator"
echo "=========================================="
echo ""

if [ "x$1" != "x" ]; then
    TAG=$1
    MANAGER_IMG=${IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:$TAG}
    BUNDLE_IMG=${BUNDLE_IMG:-quay.io/kubevirtwol/operator-bundle:$TAG}
else
    # Use the manager image (not the generic one)
    MANAGER_IMG=${IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:development}
    BUNDLE_IMG=${BUNDLE_IMG:-quay.io/kubevirtwol/operator-bundle:development}
fi

echo "Manager Image: $MANAGER_IMG"
echo "Bundle Image:  $BUNDLE_IMG"
echo ""

# Step 1: Generate bundle
echo "[1/5] Generating bundle..."
make bundle IMG=$MANAGER_IMG
echo "✓ Bundle generated"
echo ""

# Step 2: Build bundle image
echo "[2/5] Building bundle image..."
make bundle-build BUNDLE_IMG=$BUNDLE_IMG
echo "✓ Bundle image built"
echo ""

# Step 3: Push bundle image
echo "[3/5] Pushing bundle image..."
make bundle-push BUNDLE_IMG=$BUNDLE_IMG
echo "✓ Bundle image pushed"
echo ""

# Step 4: Cleanup old operator (if exists)
echo "[4/5] Cleaning up old operator..."
operator-sdk cleanup kubevirt-wol --namespace kubevirt-wol-system || echo "No previous installation found"
operator-sdk cleanup kubevirt-wol || echo "No previous installation found"
echo "✓ Cleanup complete"
echo ""

# Step 5: Deploy via OLM
echo "[5/5] Deploying operator via OLM..."
#oc create namespace kubevirt-wol-system || echo "Namespace already exists"
# operator-sdk run bundle $BUNDLE_IMG -n kubevirt-wol-system --install-mode=OwnNamespace
oc create namespace sdfsdfsfsvsdfsdffsf || echo "Namespace already exists"
operator-sdk run bundle $BUNDLE_IMG -n sdfsdfsfsvsdfsdffsf --install-mode=OwnNamespace
echo "✓ Operator deployed"
echo ""

echo "=========================================="
echo "✅ Bundle Test Complete!"
echo "=========================================="
echo ""
echo "Check status:"
echo "  kubectl get csv -A | grep kubevirt-wol"
echo "  kubectl get pods -n operators"
echo ""
echo "Cleanup when done:"
echo "  operator-sdk cleanup kubevirt-wol"

