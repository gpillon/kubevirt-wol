#!/bin/bash
set -e

echo "=========================================="
echo "Fixing KubeVirt WOL Operator SCC Issues"
echo "=========================================="
echo ""

# Step 1: Apply updated SCC with correct ServiceAccount binding
echo "[1/3] Applying updated SCC configuration..."
oc apply -f config/openshift/scc.yaml
echo "✓ SCC updated"
echo ""

# Step 2: Delete existing deployment to force recreation
echo "[2/3] Deleting existing deployment..."
oc delete deployment kubevirt-wol-controller-manager -n kubevirt-wol-system --ignore-not-found=true
echo "✓ Deployment deleted"
echo ""

# Step 3: Redeploy with fixed configuration
echo "[3/3] Redeploying with corrected configuration..."
IMG=${IMG:-quay.io/rh-ee-gpillon/kubevirt-wol:latest}
make deploy-openshift IMG=$IMG
echo "✓ Redeployed"
echo ""

# Wait for pod to be ready
echo "Waiting for pod to be ready..."
oc wait --for=condition=Ready pod -l control-plane=controller-manager -n kubevirt-wol-system --timeout=120s

echo ""
echo "=========================================="
echo "Verification"
echo "=========================================="

# Verify SCC
echo "SCC assigned:"
oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].metadata.annotations.openshift\.io/scc}'
echo ""

# Verify capabilities
echo ""
echo "Capabilities:"
oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].spec.containers[0].securityContext.capabilities}'
echo ""

# Check logs
echo ""
echo "Recent logs:"
oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=20

echo ""
echo "=========================================="
echo "✓ Fix complete!"
echo "=========================================="

