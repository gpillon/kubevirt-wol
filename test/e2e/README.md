# End-to-End Tests

This directory contains end-to-end tests for the KubeVirt Wake-on-LAN Operator.

## Running Tests

```bash
# Run all E2E tests (creates Kind cluster automatically)
make test-e2e

# Run specific test
go test -v ./test/e2e -ginkgo.focus="ServiceMonitor"

# Clean up test cluster
make cleanup-test-e2e
```

## Test Coverage

The E2E test suite verifies the following scenarios:

### 1. Controller Deployment ✅
- Controller pod starts and becomes Ready
- Health probes are working
- Leader election is successful

### 2. ServiceMonitor (Prometheus Integration) ✅
- ServiceMonitor resource is created
- Configured for HTTPS metrics endpoint (port 8443)
- Metrics endpoint is accessible

### 3. WolConfig Reconciliation ✅
- WolConfig CRD creation
- Controller reconciliation loop
- Status updates with DaemonSet name

### 4. Agent DaemonSet Management ✅
- DaemonSet created dynamically per WolConfig
- Agent pods scheduled and start successfully
- **Agents have `hostNetwork: true`** (required for UDP broadcast)
- **Manager does NOT have `hostNetwork`** (security best practice)

### 5. gRPC Communication ✅
- gRPC service created (port 9090)
- Agents connect to controller
- Agent logs show UDP listener startup
- Controller logs show reconciliation

### 6. Cleanup and Garbage Collection ✅
- Deleting WolConfig removes DaemonSet
- OwnerReference ensures automatic cleanup
- No orphaned resources

## Test Architecture

```
test/e2e/
├── e2e_suite_test.go    # Test suite setup
├── e2e_test.go          # Main test scenarios
└── README.md            # This file

test/utils/
└── utils.go             # Helper functions
```

## Key Test Functions

### Controller Verification
```go
verifyControllerUp() error
// Checks:
// - Pod exists and is named correctly
// - Pod status is Running
// - No deletion timestamp
```

### ServiceMonitor Validation
```go
// Validates ServiceMonitor resource
kubectl get servicemonitor <name> -o jsonpath={.spec.endpoints[0].port}
// Expected: "https"
```

### Network Configuration Check
```go
// Agent DaemonSet should have hostNetwork
kubectl get daemonset <name> -o jsonpath={.spec.template.spec.hostNetwork}
// Expected: "true"

// Manager Deployment should NOT have hostNetwork
kubectl get deployment <name> -o jsonpath={.spec.template.spec.hostNetwork}
// Expected: "" (empty)
```

### gRPC Connectivity
```go
// Verify gRPC service
kubectl get service kubevirt-wol-kubevirt-wol-grpc
// Expected port: 9090

// Check agent logs for connection
kubectl logs <agent-pod> | grep "Starting UDP listener"
```

## Adding New Tests

1. Add test scenario in `e2e_test.go`:
```go
By("testing new feature")
Eventually(func() error {
    // Test logic
    return nil
}, timeout, interval).Should(Succeed())
```

2. Add helper functions in `test/utils/utils.go` if needed

3. Update this README with the new test coverage

## Debugging Failed Tests

```bash
# Get controller logs
kubectl logs -n kubevirt-wol-system -l control-plane=controller-manager

# Get agent logs
kubectl logs -n kubevirt-wol-system -l app=wol-agent

# Get all resources
kubectl get all -n kubevirt-wol-system

# Describe resources for events
kubectl describe wolconfig default
kubectl describe daemonset -n kubevirt-wol-system
```

## CI/CD Integration

These tests run in GitHub Actions on:
- Pull requests (optional)
- Main branch commits
- Release tags

See `.github/workflows/e2e.yml` for CI configuration.

## Known Limitations

- Tests require Kind cluster (or equivalent Kubernetes environment)
- ServiceMonitor tests require Prometheus Operator installed
- Some tests may be flaky on slow networks (increased timeouts where needed)

## References

- [Ginkgo Documentation](https://onsi.github.io/ginkgo/)
- [Operator SDK E2E Testing](https://sdk.operatorframework.io/docs/building-operators/golang/testing/)
- [Main Testing Guide](../../docs/TESTING.md)
