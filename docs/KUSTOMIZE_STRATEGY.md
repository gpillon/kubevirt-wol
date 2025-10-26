# Kustomize Strategy for Dynamic Values

## Overview

This project uses a clean approach to inject dynamic values (like image tags) at deploy time without modifying tracked files or using `sed` hacks.

## The Problem

Previously, we used `sed` to modify `config/manager/manager.yaml` directly and then restored it with `git checkout`. This approach had several issues:

- **Dirty working tree**: Modified tracked files during build
- **Error prone**: String-based replacement could break with formatting changes
- **Not idiomatic**: Not using Kustomize's intended workflow

## The Solution: yq Post-Processing

We now use a two-stage approach:

1. **Kustomize builds** the complete manifest with all patches applied
2. **yq modifies** the YAML stream before applying to cluster

```bash
# Build with kustomize, modify with yq, apply with kubectl
$(KUSTOMIZE) build config/default | \
    $(YQ) eval '(select(.kind == "Deployment" ...) | .value) = "$(AGENT_IMG)"' - | \
    $(KUBECTL) apply -f -
```

## Why This Approach?

### ✅ Advantages

1. **No file modifications**: Working tree stays clean
2. **Type-safe**: yq understands YAML structure
3. **Idiomatic**: Kustomize does structure, yq does final values
4. **Composable**: Can chain multiple transformations
5. **Testable**: Can verify transformations work correctly

### 📝 Comparison with Alternatives

| Approach | Pros | Cons |
|----------|------|------|
| **sed on files** | Simple | Modifies tracked files, fragile |
| **kustomize patches** | Pure kustomize | Requires placeholder files, verbose |
| **yq post-processing** ✅ | Clean, powerful | Adds yq dependency |
| **envsubst** | Unix standard | Not YAML-aware, fragile |

## Implementation Details

### yq Expression Breakdown

```yaml
(select(.kind == "Deployment" and .metadata.name == "kubevirt-wol-controller-manager") |
  .spec.template.spec.containers[] |
  select(.name == "manager") |
  .env[] |
  select(.name == "AGENT_IMAGE") |
  .value) = "$(AGENT_IMG)"
```

This expression:
1. Selects the controller Deployment
2. Finds the manager container
3. Locates the AGENT_IMAGE env var
4. Sets its value to the variable

### Tool Management

yq is automatically downloaded by the Makefile:

```bash
make yq  # Downloads to ./bin/yq
```

Version is controlled in Makefile:
```makefile
YQ_VERSION ?= v4.44.3
```

## Usage Examples

### Deploy with Custom Image

```bash
# Deploy manager and inject agent image
make deploy IMG=quay.io/myrepo/manager:v1.0.0

# Agent image is automatically: quay.io/myrepo/agent:v1.0.0
```

### OpenShift Deployment

```bash
make deploy-openshift IMG=quay.io/myrepo/manager:v1.0.0
```

### Bundle Generation

```bash
make bundle IMG=quay.io/myrepo/manager:v1.0.0
```

## Testing the Strategy

### Dry-Run Deployment

Use the `deploy-dry-run` target to see the final manifests without applying:

```bash
# See all manifests with transformations applied
make -s deploy-dry-run IMG=quay.io/myrepo/manager:v1.0.0 > manifests.yaml

# Check specific values
make -s deploy-dry-run IMG=quay.io/test/manager:v1.0.0 | \
  yq eval 'select(.kind == "Deployment") | .spec.template.spec.containers[].env[]' -
```

### Manual Testing

You can also test the transformation manually:

```bash
# See what values kustomize generates
make kustomize yq
./bin/kustomize build config/default | \
  ./bin/yq eval 'select(.kind == "Deployment") | .spec.template.spec.containers[].env[]' -

# Test a transformation
./bin/kustomize build config/default | \
  ./bin/yq eval '(select(.kind == "Deployment") | ... | select(.name == "AGENT_IMAGE") | .value) = "test:v999"' - | \
  ./bin/yq eval 'select(.kind == "Deployment") | .spec.template.spec.containers[].env[] | select(.name == "AGENT_IMAGE")' -
```

## Alternative: Pure Kustomize (Not Used)

We considered a pure kustomize approach using patches:

```yaml
# config/manager/agent_image_patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: AGENT_IMAGE
          value: PLACEHOLDER
```

Then modify the placeholder with sed. However, this still requires file modification and offers no advantage over yq post-processing.

## Future Improvements

Potential enhancements:

1. **Validation**: Add schema validation after yq transformation
2. **Dry-run**: Add `make deploy-dry-run` to see final manifests
3. **Multi-value**: Extend to inject multiple runtime values
4. **Templates**: Consider using yq templates for complex transformations

## Related

- [yq documentation](https://mikefarah.gitbook.io/yq/)
- [Kustomize documentation](https://kustomize.io/)
- Makefile targets: `deploy`, `deploy-openshift`, `bundle`

