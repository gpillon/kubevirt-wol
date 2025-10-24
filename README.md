# KubeVirt Wake-on-LAN Operator

A Kubernetes Operator that enables Wake-on-LAN functionality for KubeVirt VirtualMachines. This operator listens for WOL magic packets on the network and automatically starts the corresponding VirtualMachines in your Kubernetes cluster.

## Description

The KubeVirt WOL Operator bridges traditional Wake-on-LAN functionality with modern cloud-native virtualization. It monitors UDP broadcast packets on port 9 (configurable) and when a valid WOL magic packet is received, it identifies the target VirtualMachine by MAC address and starts it automatically.

### Features

- **Multiple Discovery Modes**: 
  - `All`: Monitor all VirtualMachines in selected namespaces (default)
  - `LabelSelector`: Only manage VMs with specific labels
  - `Explicit`: Use explicit MAC-to-VM mappings
- **Cluster-wide Configuration**: Single CRD instance manages all WOL functionality
- **Namespace Filtering**: Optionally limit VM discovery to specific namespaces
- **Prometheus Metrics**: Built-in metrics for monitoring WOL activity
- **Automatic MAC Discovery**: Automatically discovers MAC addresses from VM specifications
- **Configurable**: Adjustable WOL port, cache TTL, and discovery modes

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster with KubeVirt installed
- Host network access for the operator pod (to receive broadcast UDP packets)
- **For OpenShift**: Cluster admin privileges to create custom SCC (see [OpenShift Guide](docs/openshift.md))

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/kubevirt-wol:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/kubevirt-wol:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### Usage Examples

After deploying the operator, create a WOLConfig resource to enable Wake-on-LAN:

**Example 1: Monitor all VMs in specific namespaces**
```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: wol-config
spec:
  discoveryMode: All
  namespaceSelectors:
    - default
    - production
  wolPorts: [9]
  cacheTTL: 300
```

**Example 2: Only manage VMs with specific labels**
```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: wol-config
spec:
  discoveryMode: LabelSelector
  vmSelector:
    matchLabels:
      wol.pillon.org/enabled: "true"
  wolPorts: [9]
```

**Example 3: Explicit MAC-to-VM mappings**
```yaml
apiVersion: wol.pillon.org/v1beta1
kind: WolConfig
metadata:
  name: wol-config
spec:
  discoveryMode: Explicit
  explicitMappings:
    - macAddress: "52:54:00:12:34:56"
      vmName: my-vm
      namespace: default
  wolPorts: [9]
```

**Testing Wake-on-LAN**

Once configured, you can send a WOL magic packet to wake up a VM:

```bash
# Using wakeonlan tool
wakeonlan 52:54:00:12:34:56

# Or using Python
python3 -c "import socket; s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM); \
s.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1); \
mac = '525400123456'; \
s.sendto(b'\\xff'*6 + bytes.fromhex(mac)*16, ('<broadcast>', 9))"
```

**Monitoring**

The operator exposes Prometheus metrics:
- `wol_packets_total`: Number of WOL packets received
- `wol_vm_started_total`: Number of VMs started via WOL
- `wol_errors_total`: Number of errors during WOL handling
- `wol_managed_vms`: Number of VMs currently being monitored

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/kubevirt-wol:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kubevirt-wol/<tag or branch>/dist/install.yaml
```

## Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory:

- [**Architecture**](docs/ARCHITECTURE.md) - System architecture and design
- [**Quick Start**](docs/QUICKSTART.md) - Fast deployment guide
- [**Testing**](docs/TESTING.md) - Testing procedures and examples
- [**OpenShift**](docs/openshift.md) - OpenShift-specific deployment guide
- [**Quick Reference**](docs/QUICK-REFERENCE.md) - Common commands and configurations

## Contributing

Contributions are welcome! Please ensure:
- All documentation goes in the `docs/` directory (except this README)
- Use generic examples (no specific user configurations)
- Run `make manifests generate` after API changes
- All tests pass with `make test`

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

