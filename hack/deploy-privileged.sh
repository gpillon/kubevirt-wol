#!/bin/bash
set -e

echo "=========================================="
echo "Deploy KubeVirt WOL - PRIVILEGED MODE"
echo "=========================================="
echo ""
echo "⚠️  WARNING: This will run the operator as a privileged container"
echo "    to allow binding to privileged port 9 (UDP)."
echo ""

# Step 1: Apply privileged SCC
echo "[1/3] Applying privileged SCC..."
oc apply -f config/openshift/scc.yaml
echo "✓ SCC updated to allow privileged containers"
echo ""

# Step 2: Delete existing deployment
echo "[2/3] Removing existing deployment..."
oc delete deployment kubevirt-wol-controller-manager -n kubevirt-wol-system --ignore-not-found=true
echo "✓ Old deployment removed"
echo ""

# Step 3: Deploy with privileged configuration
echo "[3/3] Deploying privileged operator..."
IMG=${IMG:-<your-registry>/kubevirt-wol:latest}
make deploy-openshift IMG=$IMG
echo "✓ Privileged operator deployed"
echo ""

# Wait for pod
echo "Waiting for pod to be ready..."
if oc wait --for=condition=Ready pod -l control-plane=controller-manager -n kubevirt-wol-system --timeout=90s 2>/dev/null; then
    echo "✓ Pod is ready"
    echo ""
    
    # Check logs
    echo "Checking WOL listener status..."
    sleep 3
    
    if oc logs -n kubevirt-wol-system -l control-plane=controller-manager 2>&1 | grep -q "WOL listener started"; then
        echo ""
        echo "✅✅✅ SUCCESS! WOL Listener is running on port 9!"
        echo ""
        oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep "WOL listener"
    else
        echo "⚠️  Pod ready but WOL listener status unclear. Showing logs:"
        oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=30
    fi
else
    echo ""
    echo "❌ Pod not ready after 90s"
    echo ""
    echo "Pod status:"
    oc get pods -n kubevirt-wol-system
    echo ""
    echo "Recent logs:"
    oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=50 2>&1 || echo "No logs available yet"
    exit 1
fi

echo ""
echo "=========================================="
echo "Verification"
echo "=========================================="
echo "SCC: $(oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].metadata.annotations.openshift\.io/scc}' 2>/dev/null || echo 'N/A')"
echo "Privileged: $(oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].spec.containers[0].securityContext.privileged}' 2>/dev/null || echo 'N/A')"
echo ""
echo "✓ Deployment complete!"
echo ""
echo "⚠️  SECURITY NOTE: This operator is running in privileged mode."
echo "    This is necessary to bind to UDP port 9 (< 1024)."
echo "    Ensure your cluster security policies allow this."

