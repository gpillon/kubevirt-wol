#!/bin/bash
set -e

echo "=========================================="
echo "Quick Fix: NET_BIND_SERVICE Permissions"
echo "=========================================="
echo ""

# Update SCC to allow RunAsAny (less restrictive on UID)
echo "[1/2] Updating SCC to allow more flexible UID/SELinux..."
oc apply -f config/openshift/scc.yaml
echo "✓ SCC updated"
echo ""

# Force pod recreation
echo "[2/2] Recreating pod..."
oc delete pod -n kubevirt-wol-system -l control-plane=controller-manager
echo "✓ Pod deleted, waiting for new pod..."
echo ""

# Wait for pod to be ready (or fail)
echo "Waiting up to 60s for pod to be ready..."
if oc wait --for=condition=Ready pod -l control-plane=controller-manager -n kubevirt-wol-system --timeout=60s 2>/dev/null; then
    echo "✓ Pod is ready!"
    echo ""
    echo "Checking logs for WOL listener..."
    sleep 2
    if oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep -q "WOL listener started"; then
        echo "✓✓✓ SUCCESS! WOL listener is running on port 9"
        oc logs -n kubevirt-wol-system -l control-plane=controller-manager | grep "WOL listener"
    else
        echo "⚠ Pod is ready but checking logs..."
        oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=30
    fi
else
    echo "❌ Pod not ready. Checking status..."
    oc get pods -n kubevirt-wol-system
    echo ""
    echo "Recent logs:"
    oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=50
    echo ""
    
    # Check if still permission denied
    if oc logs -n kubevirt-wol-system -l control-plane=controller-manager 2>&1 | grep -q "permission denied"; then
        echo ""
        echo "❌ Still getting permission denied on port 9."
        echo ""
        echo "ALTERNATIVE SOLUTION:"
        echo "Use a non-privileged port (>1024) to avoid this issue entirely."
        echo ""
        echo "To configure WOL on port 9009 instead:"
        echo "  1. Edit your WOLConfig: spec.wolPort: 9009"
        echo "  2. Configure your network/firewall to forward UDP 9 -> 9009"
        echo ""
        exit 1
    fi
fi

echo ""
echo "=========================================="
echo "Verification"
echo "=========================================="
echo "SCC: $(oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].metadata.annotations.openshift\.io/scc}')"
echo "UID: $(oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].spec.securityContext.runAsUser}')"
echo "Capabilities: $(oc get pod -n kubevirt-wol-system -o jsonpath='{.items[0].spec.containers[0].securityContext.capabilities}')"

