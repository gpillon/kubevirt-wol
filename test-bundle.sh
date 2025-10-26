#!/bin/bash
set -e

echo "=========================================="
echo "Bundle Testing for KubeVirt WOL Operator"
echo "=========================================="
echo ""

# Parse arguments
PUSH=true
DEPLOY=true
TAG=""
NEXT_IS_TAG=false

for arg in "$@"; do
    if [ "$NEXT_IS_TAG" = true ]; then
        TAG=$arg
        NEXT_IS_TAG=false
    elif [ "$arg" = "--no-push" ]; then
        PUSH=false
        DEPLOY=false  # Can't deploy without push
    elif [ "$arg" = "--no-deploy" ]; then
        DEPLOY=false
    elif [ "$arg" = "--tag" ]; then
        NEXT_IS_TAG=true
    else
        echo "Unknown argument: $arg"
        echo "Usage: $0 [--tag <tag>] [--no-push] [--no-deploy]"
        exit 1
    fi
done

# If no tag provided, use development
if [ -z "$TAG" ]; then
    TAG="development"
fi

MANAGER_IMG=${IMG:-quay.io/kubevirtwol/kubevirt-wol-manager:$TAG}
BUNDLE_IMG=${BUNDLE_IMG:-quay.io/kubevirtwol/operator-bundle:$TAG}

echo "Manager Image: $MANAGER_IMG"
echo "Bundle Image:  $BUNDLE_IMG"
if [ "$PUSH" = false ]; then
    echo "Mode:          BUILD ONLY (--no-push)"
elif [ "$DEPLOY" = false ]; then
    echo "Mode:          BUILD + PUSH (--no-deploy)"
else
    echo "Mode:          BUILD + PUSH + DEPLOY"
fi
echo ""

# Calculate total steps
if [ "$PUSH" = false ]; then
    TOTAL_STEPS=2
elif [ "$DEPLOY" = false ]; then
    TOTAL_STEPS=3
else
    TOTAL_STEPS=5
fi

CURRENT_STEP=1

# Step 1: Generate bundle
echo "[$CURRENT_STEP/$TOTAL_STEPS] Generating bundle..."
make bundle IMG=$MANAGER_IMG
echo "✓ Bundle generated"
echo ""
CURRENT_STEP=$((CURRENT_STEP + 1))

# Step 2: Build bundle image
echo "[$CURRENT_STEP/$TOTAL_STEPS] Building bundle image..."
make bundle-build BUNDLE_IMG=$BUNDLE_IMG
echo "✓ Bundle image built"
echo ""
CURRENT_STEP=$((CURRENT_STEP + 1))

# Step 3: Push bundle image
if [ "$PUSH" = true ]; then
    echo "[$CURRENT_STEP/$TOTAL_STEPS] Pushing bundle image..."
    make bundle-push BUNDLE_IMG=$BUNDLE_IMG
    echo "✓ Bundle image pushed"
    echo ""
    CURRENT_STEP=$((CURRENT_STEP + 1))
fi

# Step 4: Cleanup old operator (if exists)
if [ "$DEPLOY" = true ]; then
    echo "[$CURRENT_STEP/$TOTAL_STEPS] Cleaning up old operator..."
    operator-sdk cleanup kubevirt-wol --namespace kubevirt-wol-system || echo "No previous installation found"
    operator-sdk cleanup kubevirt-wol --namespace sdfsdfsfsvsdfsdffsf || echo "No previous installation found"
    operator-sdk cleanup kubevirt-wol || echo "No previous installation found"
    echo "✓ Cleanup complete"
    echo ""
    CURRENT_STEP=$((CURRENT_STEP + 1))
fi

# Step 5: Deploy via OLM
if [ "$DEPLOY" = true ]; then
    echo "[$CURRENT_STEP/$TOTAL_STEPS] Deploying operator via OLM..."
    #oc create namespace kubevirt-wol-system || echo "Namespace already exists"
    # operator-sdk run bundle $BUNDLE_IMG -n kubevirt-wol-system --install-mode=OwnNamespace
    oc create namespace sdfsdfsfsvsdfsdffsf || echo "Namespace already exists"
    operator-sdk run bundle $BUNDLE_IMG -n sdfsdfsfsvsdfsdffsf --install-mode=OwnNamespace
    oc apply -f config/samples/wol_v1beta1_wolconfig-default.yaml -n sdfsdfsfsvsdfsdffsf
    echo "✓ Operator deployed"
    echo ""
fi

echo "=========================================="
if [ "$PUSH" = false ]; then
    echo "✅ Bundle Build Complete!"
elif [ "$DEPLOY" = false ]; then
    echo "✅ Bundle Build + Push Complete!"
else
    echo "✅ Bundle Test Complete!"
fi
echo "=========================================="
echo ""

if [ "$DEPLOY" = true ]; then
    echo "Check status:"
    echo "  kubectl get csv -A | grep kubevirt-wol"
    echo "  kubectl get pods -n operators"
    echo ""
    echo "Cleanup when done:"
    echo "  operator-sdk cleanup kubevirt-wol"
elif [ "$PUSH" = false ]; then
    echo "Bundle built locally."
    echo ""
    echo "To push and deploy, run:"
    echo "  ./test-bundle.sh --tag $TAG"
else
    echo "Bundle pushed to registry."
    echo ""
    echo "To deploy, run:"
    echo "  ./test-bundle.sh --tag $TAG"
fi

