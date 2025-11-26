GO ?= go
KUBECTL ?= kubectl
KIND ?= kind
HELM ?= helm
TASK ?= task
KUSTOMIZE ?= $(GO) tool kustomize
CONTROLLER_GEN ?= $(GO) tool controller-gen
API_GEN ?= $(GO) tool apigen
ENVTEST ?= $(GO) tool setup-envtest
GOLANGCI_LINT ?= $(GO) tool golangci-lint
KUBEBUILDER ?= $(GO) tool sigs.k8s.io/kubebuilder/v3/cmd

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.30.0

REGISTRY ?= ghcr.io/platform-mesh

.PHONY: print-registry
print-registry: ## Print the registry URL
	@echo $(REGISTRY)

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# BAKE_TOOL defines the tool to be used for building images with
# buildkit bake files.
BAKE_TOOL ?= $(CONTAINER_TOOL) buildx bake

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
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $$(go list ./... | grep -v /test) -coverprofile cover.out

# Optional test name pattern for filtering e2e tests
TEST_NAME ?=

.PHONY: test-e2e
test-e2e: manifests generate fmt vet ## Run e2e tests. Optionally specify TEST_NAME=<test_name> to run a specific test.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" SETUP_CLUSTER=true IMG=$(IMG) go test ./test/... -v -timeout 30m $(if $(TEST_NAME),-run "^$(TEST_NAME)$$")

.PHONY: lint
lint: ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	$(GO) build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	$(GO) run ./cmd/main.go

DOCKER_VERSION_RAW=$(shell git describe --tags --always --dirty --match 'docker-*')
DOCKER_VERSION=$(patsubst docker-%,%,$(DOCKER_VERSION_RAW))

.PHONY: print-docker-version
print-docker-version: ## Print the Docker version
	@echo $(DOCKER_VERSION)

IMG ?= $(REGISTRY)/example-httpbin-operator:$(DOCKER_VERSION)

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: manifests generate fmt vet ## Build docker image with the manager.
	$(CONTAINER_TOOL) buildx build -t $(IMG) --load .

KIND_CLUSTER ?= kind

docker-install: docker-build
	$(KIND) load docker-image $(IMG) --name $(KIND_CLUSTER)

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ignore-not-found ?= false

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests docker-build ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

## OCM

OCM_VERSION_RAW=$(shell git describe --tags --always --dirty --match 'ocm-*')
OCM_VERSION=$(patsubst ocm-%,%,$(OCM_VERSION_RAW))

print-ocm-version: ## Print the OCM version
	@echo $(OCM_VERSION)

##@ Testing

.PHONY: kind-test
kind-test: kind-test-cleanup docker-build ## Create kind cluster, load image, and deploy with Helm chart from GHCR
	@echo "Creating kind cluster..."
	$(KIND) create cluster --name example-httpbin-operator
	$(KIND) get kubeconfig --name example-httpbin-operator > msp.kubeconfig.yaml
	@echo "Loading operator image into kind..."
	$(KIND) load docker-image ${IMG} --name example-httpbin-operator
	@echo "Installing helm chart from GHCR..."
	$(HELM) upgrade --install example-httpbin-operator \
		oci://ghcr.io/platform-mesh/helm-charts/example-httpbin-operator \
		--namespace example-httpbin-operator-system \
		--create-namespace \
		--set image.tag=$(word 2,$(subst :, ,${IMG})) \
		--wait
	@echo "Waiting for operator deployment..."
	$(KUBECTL) wait --for=condition=available deployment/example-httpbin-operator --namespace example-httpbin-operator-system --timeout=60s
	@echo "Deployment status:"
	$(KUBECTL) get deployment example-httpbin-operator --namespace example-httpbin-operator-system
	@echo "CRD status:"
	$(KUBECTL) get crds | grep httpbin

.PHONY: kind-test-cleanup
kind-test-cleanup: ## Delete the kind test cluster
	@echo "Deleting kind cluster..."
	@$(KIND) delete cluster --name example-httpbin-operator 2>/dev/null || true
	@$(KIND) delete cluster --name example-httpbin-operator-crds 2>/dev/null || true

.PHONY: kind-test-crds
kind-test-crds: manifests ## Create kind cluster and deploy CRDs only
	@echo "Creating kind cluster..."
	$(KIND) create cluster --name example-httpbin-operator-crds
	$(KIND) get kubeconfig --name example-httpbin-operator-crds > msp-cp.kubeconfig.yaml
	@echo "Installing CRDs only..."
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -
	@echo "CRD status:"
	$(KUBECTL) get crds | grep httpbin

.PHONY: kind-test-sample
kind-test-sample: ## Deploy a sample httpbin to test the operator
	@echo "Creating sample httpbin..."
	$(KUBECTL) apply -f examples/httpbin.yaml

.PHONY: kind-test-e2e
kind-test-e2e: kind-test
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" IMG=$(IMG) go test ./test/... -v -timeout 30m $(if $(TEST_NAME),-run "^$(TEST_NAME)$$")

.PHONY: local-platform-mesh
local-platform-mesh: setup-hosts ## Install local platform mesh components
	rm -rf .helm-charts || true
	git clone https://github.com/platform-mesh/helm-charts.git -o platform-mesh .helm-charts
	cp hack/ocm/Taskfile.yaml .helm-charts/Taskfile.yaml
	cp hack/ocm/component-constructor-prerelease.yaml .helm-charts/.ocm/component-constructor-prerelease.yaml
	cd .helm-charts && $(TASK) local-setup-cached
	@echo "preparing ocm deployment..."
	cd .helm-charts && $(TASK) ocm:deploy
	cd .helm-charts && $(TASK) ocm:build ocm:apply
	cp hack/ocm/platform-mesh.yaml .helm-charts/local-setup/kustomize/components/platform-mesh-operator-resource/platform-mesh.yaml
	cd .helm-charts && $(TASK) local-setup-cached:iterate


.PHONY: setup-hosts
setup-hosts: ## Add local development hosts to /etc/hosts
	@echo "Adding development hosts to /etc/hosts..."
	@if ! grep -q "portal.dev.local" /etc/hosts; then \
		echo "127.0.0.1 demo.portal.dev.local default.portal.dev.local portal.dev.local kcp.api.portal.dev.local" | sudo tee -a /etc/hosts; \
	else \
		echo "Hosts already configured"; \
	fi
