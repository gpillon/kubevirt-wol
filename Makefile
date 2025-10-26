# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.1

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# pillon.org/kubevirt-wol-bundle:$VERSION and pillon.org/kubevirt-wol-catalog:$VERSION.
IMAGE_TAG_BASE ?= kubevirtwol/kubevirt-wol

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# IS_OPENSHIFT defines if the bundle should be generated for OpenShift
# When true, uses config/openshift for kustomize manifests
IS_OPENSHIFT ?= true

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.41.1
# Image URL to use all building/pushing image targets
IMG ?= quay.io/kubevirtwol/kubevirt-wol-manager:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
# Automatically derived from controller-runtime version in go.mod
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: proto
proto: ## Generate protobuf code from .proto files.
	@which protoc > /dev/null || (echo "ERROR: protoc not found. Install it first." && exit 1)
	@which protoc-gen-go > /dev/null || (echo "Installing protoc-gen-go..." && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest)
	@which protoc-gen-go-grpc > /dev/null || (echo "Installing protoc-gen-go-grpc..." && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest)
	PATH=$(PATH):$(shell go env GOPATH)/bin protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/wol/v1/wol.proto
	@echo "Protobuf code generated successfully"

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: setup-envtest
setup-envtest: envtest ## Setup ENVTEST binaries
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
	  echo "Error setting up envtest"; exit 1; }

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
KIND_CLUSTER ?= kubevirt-wol-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v kind >/dev/null 2>&1 || { \
	  echo "Kind is not installed. Please install Kind manually."; \
	  exit 1; \
	}
	@case "$$(kind get clusters)" in \
	  *"$(KIND_CLUSTER)"*) \
	    echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
	  *) \
	    echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
	    kind create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Clean up the Kind cluster for e2e tests
	kind delete cluster --name $(KIND_CLUSTER)

.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e: setup-test-e2e
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: build-manager build-agent ## Build all binaries.

.PHONY: build-manager
build-manager: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/manager/main.go

.PHONY: build-agent
build-agent: manifests generate fmt vet ## Build agent binary.
	go build -o bin/agent cmd/agent/main.go

.PHONY: run
run: manifests generate fmt vet ## Run the manager from your host.
	go run ./cmd/manager/main.go

.PHONY: run-agent
run-agent: ## Run the agent from your host (requires NODE_NAME env var).
	@if [ -z "$$NODE_NAME" ]; then \
		echo "ERROR: NODE_NAME environment variable must be set"; \
		echo "Example: NODE_NAME=localhost make run-agent"; \
		exit 1; \
	fi
	go run ./cmd/agent/main.go --node-name=$$NODE_NAME --operator-address=localhost:9090

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: docker-build-manager ## Build docker image with the manager.

.PHONY: docker-build-manager
docker-build-manager: ## Build docker image for manager.
	$(CONTAINER_TOOL) build --build-arg BINARY=manager -t ${IMG} .

.PHONY: docker-build-agent
docker-build-agent: ## Build docker image for agent.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	$(CONTAINER_TOOL) build --build-arg BINARY=agent -t ${AGENT_IMG} .

.PHONY: docker-build-all
docker-build-all: docker-build-manager docker-build-agent ## Build both manager and agent images.

.PHONY: docker-push
docker-push: docker-push-manager ## Push docker image with the manager.

.PHONY: docker-push-manager
docker-push-manager: ## Push manager docker image.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-push-agent
docker-push-agent: ## Push agent docker image.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	$(CONTAINER_TOOL) push ${AGENT_IMG}

.PHONY: docker-push-all
docker-push-all: docker-push-manager docker-push-agent ## Push both manager and agent images.

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name kubevirt-wol-builder
	$(CONTAINER_TOOL) buildx use kubevirt-wol-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm kubevirt-wol-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy-dry-run
deploy-dry-run: manifests kustomize yq ## Generate final manifests without applying (dry-run). Use 'make -s' for clean YAML output.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	@cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	@echo "# Manager Image: ${IMG}" >&2
	@echo "# Agent Image:   $(AGENT_IMG)" >&2
	@echo "---" >&2
	@$(KUSTOMIZE) build config/default | \
		$(YQ) eval '(select(.kind == "Deployment" and .metadata.name == "kubevirt-wol-controller-manager") | .spec.template.spec.containers[] | select(.name == "manager") | .env[] | select(.name == "AGENT_IMAGE") | .value) = "$(AGENT_IMG)"' -

.PHONY: deploy
deploy: manifests kustomize yq ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | \
		$(YQ) eval '(select(.kind == "Deployment" and .metadata.name == "kubevirt-wol-controller-manager") | .spec.template.spec.containers[] | select(.name == "manager") | .env[] | select(.name == "AGENT_IMAGE") | .value) = "$(AGENT_IMG)"' - | \
		$(KUBECTL) apply -f -

.PHONY: deploy-agent
deploy-agent: kustomize ## Deploy agent DaemonSet to the K8s cluster.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	cd config/agent && $(KUSTOMIZE) edit set image agent=${AGENT_IMG}
	$(KUSTOMIZE) build config/agent | $(KUBECTL) apply -f -

.PHONY: deploy-all
deploy-all: deploy deploy-agent ## Deploy both manager and agent to the cluster.

.PHONY: deploy-openshift
deploy-openshift: manifests kustomize yq ## Deploy controller to OpenShift cluster with custom SCC.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/openshift | \
		$(YQ) eval '(select(.kind == "Deployment" and .metadata.name == "kubevirt-wol-controller-manager") | .spec.template.spec.containers[] | select(.name == "manager") | .env[] | select(.name == "AGENT_IMAGE") | .value) = "$(AGENT_IMG)"' - | \
		$(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: undeploy-openshift
undeploy-openshift: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/openshift | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
YQ ?= $(LOCALBIN)/yq

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.18.0
ENVTEST_TOOL_VERSION ?= $(ENVTEST_VERSION)
GOLANGCI_LINT_VERSION ?= v2.1.0
YQ_VERSION ?= v4.44.3

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_TOOL_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	@[ -f "$(YQ)-$(YQ_VERSION)" ] || { \
	set -e ;\
	echo "Downloading yq $(YQ_VERSION)" ;\
	mkdir -p $(LOCALBIN) ;\
	OS=$$(go env GOOS) && ARCH=$$(go env GOARCH) ;\
	curl -sSLo $(YQ) https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$${OS}_$${ARCH} ;\
	chmod +x $(YQ) ;\
	mv $(YQ) $(YQ)-$(YQ_VERSION) ;\
	}
	@ln -sf $(YQ)-$(YQ_VERSION) $(YQ)

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif

.PHONY: bundle
bundle: manifests kustomize operator-sdk yq ## Generate bundle manifests and metadata, then validate generated files.
	$(eval AGENT_IMG ?= $(shell echo ${IMG} | sed 's/manager/agent/g'))
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | \
		$(YQ) eval 'select(.kind != "SecurityContextConstraints")' - | \
		$(YQ) eval '(select(.kind == "Deployment" and .metadata.name == "kubevirt-wol-controller-manager") | .spec.template.spec.containers[] | select(.name == "manager") | .env[] | select(.name == "AGENT_IMAGE") | .value) = "$(AGENT_IMG)"' - | \
		$(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS) --extra-service-accounts kubevirt-wol-wol-agent
	@echo "Temporarily removing SCC for validation (operator-sdk doesn't support OpenShift-specific resources)..."
	@rm -f bundle/manifests/*securitycontextconstraints*.yaml 2>/dev/null || true
	@echo "Validating bundle..."
	@$(OPERATOR_SDK) bundle validate ./bundle
	@echo "Adding SCC to bundle manifests..."
	@$(YQ) eval '.metadata.creationTimestamp = null' config/manifests/scc.yaml > bundle/manifests/kubevirt-wol-wol-scc_security.openshift.io_v1_securitycontextconstraints.yaml
	@echo "Bundle generation completed successfully"

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = $(LOCALBIN)/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.55.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)
