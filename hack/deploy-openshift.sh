#!/bin/bash
set -e

# KubeVirt WOL Operator - OpenShift Quick Deploy Script

echo "=========================================="
echo "KubeVirt WOL Operator - OpenShift Deploy"
echo "=========================================="
echo ""

# Check if oc is installed
if ! command -v oc &> /dev/null; then
    echo "ERROR: 'oc' command not found. Please install OpenShift CLI."
    exit 1
fi

# Check if we're logged in to OpenShift
if ! oc whoami &> /dev/null; then
    echo "ERROR: Not logged in to OpenShift. Please run 'oc login' first."
    exit 1
fi

# Check if user is cluster-admin
if ! oc auth can-i create securitycontextconstraints &> /dev/null; then
    echo "WARNING: You may not have cluster-admin privileges."
    echo "Creating SCC requires cluster-admin. Continue anyway? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        echo "Deployment cancelled."
        exit 1
    fi
fi

# Set default image if not provided
IMG=${IMG:-<your-registry>/kubevirt-wol:latest}
echo "Using image: $IMG"
echo ""

# Step 1: Install CRDs
echo "[1/4] Installing CRDs..."
make install
echo "✓ CRDs installed"
echo ""

# Step 2: Create SCC and RBAC
echo "[2/4] Creating Security Context Constraint..."
oc apply -f config/openshift/scc.yaml
echo "✓ SCC created"
echo ""

# Step 3: Deploy operator
echo "[3/4] Deploying operator..."
make deploy-openshift IMG=$IMG
echo "✓ Operator deployed"
echo ""

# Step 4: Wait for pod to be ready
echo "[4/4] Waiting for operator pod to be ready..."
oc wait --for=condition=Ready pod -l control-plane=controller-manager -n kubevirt-wol-system --timeout=120s || {
    echo "❌ WARNING: Pod not ready after 120s. Investigating..."
    echo ""
    echo "Pod Status:"
    oc get pods -n kubevirt-wol-system
    echo ""
    
    # Check if pod is in CrashLoopBackOff
    POD_STATUS=$(oc get pods -n kubevirt-wol-system -l control-plane=controller-manager -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")
    if [ "$POD_STATUS" = "CrashLoopBackOff" ] || [ "$POD_STATUS" = "Error" ]; then
        echo "Pod is crashing. Checking logs..."
        echo ""
        oc logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=50
        echo ""
        
        # Check for common errors
        if oc logs -n kubevirt-wol-system -l control-plane=controller-manager 2>&1 | grep -q "address already in use"; then
            echo "❌ ERROR: Port conflict detected!"
            echo "This usually means you deployed using 'make deploy' instead of 'make deploy-openshift'"
            echo ""
            echo "FIX: Run these commands:"
            echo "  oc delete deployment -n kubevirt-wol-system kubevirt-wol-controller-manager"
            echo "  make deploy-openshift IMG=$IMG"
            exit 1
        fi
    fi
    
    echo "Recent events:"
    oc get events -n kubevirt-wol-system --sort-by='.lastTimestamp' | tail -10
    exit 1
}
echo "✓ Operator is ready"
echo ""

# Verify SCC assignment
echo "Verifying SCC assignment..."
SCC=$(oc get pod -n kubevirt-wol-system -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.annotations.openshift\.io/scc}' 2>/dev/null || echo "unknown")
echo "Pod is using SCC: $SCC"

if [ "$SCC" = "kubevirt-wol-scc" ]; then
    echo "✓ Correct SCC assigned"
else
    echo "WARNING: Expected 'kubevirt-wol-scc' but got '$SCC'"
    echo "The operator may not function correctly without host network access."
fi
echo ""

# Show deployment info
echo "=========================================="
echo "Deployment Summary"
echo "=========================================="
echo "Namespace: kubevirt-wol-system"
echo "Image: $IMG"
echo "SCC: $SCC"
echo ""
echo "Pod Status:"
oc get pods -n kubevirt-wol-system
echo ""
echo "To check logs:"
echo "  oc logs -n kubevirt-wol-system deployment/kubevirt-wol-controller-manager"
echo ""
echo "To create a WOLConfig:"
echo "  oc apply -f config/samples/wol_v1beta1_config.yaml"
echo ""
echo "To view WOLConfig status:"
echo "  oc get config -o wide"
echo ""
echo "Deployment complete! ✓"

