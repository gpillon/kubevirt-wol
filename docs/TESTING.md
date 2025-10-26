# Testing Guide

This document describes the testing strategy and procedures for the KubeVirt Wake-on-LAN Operator.

## Test Types

### 1. Unit Tests

Unit tests verify individual components and functions.

```bash
# Run all unit tests
make test

# Run tests with coverage
make test TESTFLAGS="-coverprofile=coverage.out"

# View coverage report
go tool cover -html=coverage.out
```

### 2. End-to-End (E2E) Tests

E2E tests verify the complete operator functionality in a real Kubernetes cluster.

#### Prerequisites

- Kind cluster (or any Kubernetes 1.19+ cluster)
- kubectl configured
- Docker or Podman

#### Running E2E Tests

```bash
# Run all E2E tests
make test-e2e

# Run specific test
go test -v ./test/e2e -ginkgo.focus="should run successfully"
```

#### What E2E Tests Verify

The E2E test suite (`test/e2e/e2e_test.go`) verifies:

1. **Controller Deployment**
   - Controller pod starts successfully
   - Pod reaches Running state
   - Health checks are passing

2. **ServiceMonitor (Prometheus)**
   - ServiceMonitor resource is created
   - Correctly configured for HTTPS metrics endpoint
   - Metrics are exposed on port 8443

3. **WolConfig Reconciliation**
   - WolConfig CRD can be created
   - Controller reconciles the resource
   - Status is updated with DaemonSet name

4. **Agent DaemonSet Creation**
   - DaemonSet is created dynamically
   - Agent pods are scheduled and start
   - Pods become Ready

5. **Network Configuration**
   - ✅ **Agents have `hostNetwork: true`** (required for UDP broadcast)
   - ✅ **Manager does NOT have `hostNetwork`** (security best practice)
   - Agents can bind to WOL ports (default: UDP 9)

6. **gRPC Communication**
   - gRPC service is created (port 9090)
   - Agents can connect to controller via gRPC
   - Agent logs show successful UDP listener startup
   - Controller logs show reconciliation activity

7. **Metrics Endpoint**
   - Metrics service is accessible
   - HTTPS endpoint responds correctly
   - Prometheus can scrape metrics

8. **Cleanup and Garbage Collection**
   - Deleting WolConfig removes DaemonSet
   - OwnerReference ensures automatic cleanup
   - No orphaned resources remain

### 3. Integration Tests

Integration tests verify interactions between components without requiring a full cluster.

```bash
# Run integration tests (if implemented)
go test -v ./internal/controller/... -tags=integration
```

## Test Coverage Goals

- Unit tests: > 70%
- Controller logic: > 80%
- Critical paths (WOL handling, gRPC): 100%

## Testing Checklist

Before submitting a PR:

- [ ] All unit tests pass
- [ ] E2E tests pass
- [ ] No linter errors (`make lint`)
- [ ] Code coverage maintained or improved
- [ ] New features have corresponding tests

## Manual Testing

### Testing WOL Functionality

1. **Deploy the operator**:
   ```bash
   make deploy-openshift IMG=<your-image>
   ```

2. **Create a test VM**:
   ```bash
   kubectl apply -f - <<EOF
   apiVersion: kubevirt.io/v1
   kind: VirtualMachine
   metadata:
     name: test-vm
     namespace: default
   spec:
     running: false
     template:
       spec:
         domain:
           devices:
             interfaces:
             - name: default
               masquerade: {}
           resources:
             requests:
               memory: 1Gi
         networks:
         - name: default
           pod: {}
   EOF
   ```

3. **Create WolConfig**:
   ```bash
   kubectl apply -f config/samples/wol_v1beta1_wolconfig-default.yaml
   ```

4. **Get VM MAC address**:
   ```bash
   kubectl get vmi test-vm -o jsonpath='{.status.interfaces[0].mac}'
   ```

5. **Send WOL packet**:
   ```bash
   # From a node with access to the pod network
   ./hack/test-wol.sh <MAC_ADDRESS> <NODE_IP>
   ```

6. **Verify VM starts**:
   ```bash
   kubectl get vm test-vm -w
   # Should transition to Running state
   ```

### Testing ServiceMonitor

1. **Deploy Prometheus Operator** (if not already deployed)

2. **Verify ServiceMonitor is created**:
   ```bash
   kubectl get servicemonitor -n kubevirt-wol-system
   ```

3. **Check metrics endpoint**:
   ```bash
   kubectl run curl-test --image=curlimages/curl:latest --rm -i -- \
     curl -k https://kubevirt-wol-controller-manager-metrics-service.kubevirt-wol-system.svc.cluster.local:8443/metrics
   ```

4. **Verify in Prometheus UI**:
   - Look for `controller_runtime_*` metrics
   - Custom metrics: `wolconfig_*`, `wol_packets_*`

### Testing gRPC Communication

1. **Check controller gRPC service**:
   ```bash
   kubectl get svc -n kubevirt-wol-system kubevirt-wol-kubevirt-wol-grpc
   ```

2. **Verify agent connectivity**:
   ```bash
   # Get agent pod name
   AGENT_POD=$(kubectl get pods -n kubevirt-wol-system -l app=wol-agent -o name | head -1)
   
   # Check logs for gRPC connection
   kubectl logs -n kubevirt-wol-system $AGENT_POD | grep -i "grpc\|connected"
   ```

3. **Test packet flow**:
   ```bash
   # Send test WOL packet
   ./hack/test-wol.sh <MAC> <NODE_IP>
   
   # Check agent logs
   kubectl logs -n kubevirt-wol-system $AGENT_POD --tail=50
   
   # Check controller logs
   kubectl logs -n kubevirt-wol-system -l control-plane=controller-manager --tail=50
   ```

## Debugging Failed Tests

### E2E Test Failures

1. **Get test artifacts**:
   ```bash
   # Controller logs
   kubectl logs -n kubevirt-wol-system -l control-plane=controller-manager > controller.log
   
   # Agent logs
   kubectl logs -n kubevirt-wol-system -l app=wol-agent > agent.log
   
   # Resource states
   kubectl get all -n kubevirt-wol-system -o yaml > resources.yaml
   ```

2. **Common issues**:
   - **Timeout waiting for pods**: Check image pull policies and network
   - **gRPC connection failed**: Verify service exists and DNS resolution
   - **hostNetwork permission denied**: Check SCC bindings (OpenShift)
   - **ServiceMonitor not found**: Ensure Prometheus Operator is installed

### Unit Test Failures

1. **Run with verbose output**:
   ```bash
   go test -v ./internal/... -run TestFailingTest
   ```

2. **Enable debug logging**:
   ```bash
   go test -v ./internal/... -args -ginkgo.v
   ```

## Continuous Integration

The project uses GitHub Actions for CI:

- `.github/workflows/test.yml`: Runs unit tests on every PR
- `.github/workflows/e2e.yml`: Runs E2E tests on main branch

## Performance Testing

### Load Testing

Test with multiple WolConfigs and many VMs:

```bash
# Create 10 WolConfigs
for i in {1..10}; do
  kubectl apply -f - <<EOF
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: test-config-$i
spec:
  namespaceSelectors: [default]
  wolPorts: [9]
EOF
done

# Monitor resource usage
kubectl top pods -n kubevirt-wol-system
```

### Stress Testing

Send many WOL packets rapidly:

```bash
# Send 100 packets
for i in {1..100}; do
  ./hack/test-wol.sh <MAC> <NODE_IP> &
done
wait

# Check for packet loss or errors
kubectl logs -n kubevirt-wol-system -l app=wol-agent | grep -i error
```

## Test Data

Sample test resources are in:
- `config/samples/` - Example WolConfigs
- `test/testdata/` - Additional test fixtures (if any)

## Contributing Tests

When adding new features:

1. Add unit tests in `*_test.go` files next to the code
2. Update E2E tests if the feature affects the operator behavior
3. Document any new test scenarios in this file
4. Ensure all tests pass before submitting PR

## References

- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Gomega Matcher Library](https://onsi.github.io/gomega/)
- [Kubernetes E2E Testing](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-testing/e2e-tests.md)
- [Operator SDK Testing](https://sdk.operatorframework.io/docs/building-operators/golang/testing/)
