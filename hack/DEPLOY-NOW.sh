#!/bin/bash
set -e

echo "=========================================="
echo "DEPLOY KUBEVIRT WOL - PRIVILEGED"
echo "=========================================="
echo ""

# Apply SCC
echo "[1/3] Applying SCC..."
oc apply -f config/openshift/scc.yaml
echo "✓ SCC applied"

# Delete old deployment
echo ""
echo "[2/3] Removing old deployment..."
oc delete deployment kubevirt-wol-controller-manager -n kubevirt-wol-system --ignore-not-found=true
sleep 2
echo "✓ Old deployment removed"

# Deploy
echo ""
echo "[3/3] Deploying..."
IMG=${IMG:-<your-registry>/kubevirt-wol:latest}
make deploy-openshift IMG=$IMG

echo ""
echo "Waiting for pod..."
sleep 5

# Wait and check
if oc wait --for=condition=Ready pod -l control-plane=controller-manager -n kubevirt-wol-system --timeout=90s 2>/dev/null; then
    echo ""
    echo "✅ Pod ready! Checking logs..."
    sleep 3
    
    oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=20
    
    echo ""
    if oc logs -n kubevirt-wol-system -l control-plane=controller-manager 2>&1 | grep -q "WOL listener started"; then
        echo "=========================================="
        echo "✅✅✅ SUCCESS!"
        echo "=========================================="
        echo ""
        oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep "WOL listener"
    fi
else
    echo ""
    echo "Pod status:"
    oc get pods -n kubevirt-wol-system
    echo ""
    echo "Logs:"
    oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=50
fi

