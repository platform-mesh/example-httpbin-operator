GO ?= go
KUBECTL ?= kubectl
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
	@# update CRDs in helm - this is using the intended CRD install of helm: https://helm.sh/docs/chart_best_practices/custom_resource_definitions/
	@# while helm itself has some problems with updating CRDs commonly used replacements like fluxcd and kustomize have no problem with that
	rm -f ./charts/example-httpbin-operator/crds/*
	cp ./config/crd/bases/*.yaml ./charts/example-httpbin-operator/crds/
	@# update the RBAC -- TODO: replace the entire helm chart with the generated version. makes it much easier.
	# TODO fails in CI
	# $(KUBEBUILDER) edit --plugins helm/v1-alpha
	# rm -rf ./charts/example-httpbin-operator/templates/rbac
	# cp -r ./dist/chart/templates/rbac ./charts/example-httpbin-operator/templates/rbac

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
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./test/... -v -timeout 30m $(if $(TEST_NAME),-run "^$(TEST_NAME)$$")

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
	kind load docker-image $(IMG) --name $(KIND_CLUSTER)

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

##@ Helm Chart

CHART_VERSION_RAW=$(shell git describe --tags --always --dirty --match 'charts-*')
CHART_VERSION=$(patsubst charts-%,%,$(CHART_VERSION_RAW))

print-helm-version:
	@echo $(CHART_VERSION)

.PHONY: chart-push
chart-push:  ## Push Helm chart to GHCR (after pushing multi-arch container image)
	@echo "Pushing Helm chart to $(REGISTRY)..."
	helm push dist/example-httpbin-operator-$(CHART_VERSION).tgz oci://$(REGISTRY)/charts
	helm push dist/example-httpbin-operator-crds-$(CHART_VERSION).tgz oci://$(REGISTRY)/charts

.PHONY: charts
charts: manifests generate ## Generate and package Helm chart with multi-arch image support. Always runs manifests first to ensure CRDs are up to date
	helm dependency update charts/example-httpbin-operator
	mkdir -p dist
	helm package charts/example-httpbin-operator -d dist

## OCM

OCM_VERSION_RAW=$(shell git describe --tags --always --dirty --match 'ocm-*')
OCM_VERSION=$(patsubst ocm-%,%,$(OCM_VERSION_RAW))

print-ocm-version: ## Print the OCM version
	@echo $(OCM_VERSION)

##@ Testing

.PHONY: kind-test
kind-test: kind-test-cleanup docker-build charts ## Create kind cluster, load image, and deploy helm chart
	@echo "Creating kind cluster..."
	kind create cluster --name example-httpbin-operator
	kind get kubeconfig --name example-httpbin-operator > msp.kubeconfig.yaml
	@echo "Loading operator image into kind..."
	kind load docker-image ${IMG} --name example-httpbin-operator
	@echo "Installing helm chart..."
	helm install example-httpbin-operator dist/example-httpbin-operator-0.0.0.tgz \
		--create-namespace \
		--force
	@echo "Waiting for operator deployment..."
	kubectl wait --for=condition=available deployment/example-httpbin-operator --timeout=60s
	@echo "Deployment status:"
	kubectl get deployment example-httpbin-operator
	@echo "CRD status:"
	kubectl get crds | grep httpbin

.PHONY: kind-test-cleanup
kind-test-cleanup: ## Delete the kind test cluster
	@echo "Deleting kind cluster..."
	@kind delete cluster --name example-httpbin-operator 2>/dev/null || true
	@kind delete cluster --name example-httpbin-operator-crds 2>/dev/null || true

.PHONY: kind-test-crds
kind-test-crds: charts ## Create kind cluster and deploy helm chart CRDs only
	@echo "Creating kind cluster..."
	kind create cluster --name example-httpbin-operator-crds
	kind get kubeconfig --name example-httpbin-operator-crds > msp-cp.kubeconfig.yaml
	@echo "Installing helm chart CRDs only..."
	helm install example-httpbin-operator dist/example-httpbin-operator-crds-0.0.0.tgz
	@echo "CRD status:"
	kubectl get crds | grep httpbin

.PHONY: kind-test-sample
kind-test-sample: ## Deploy a sample httpbin deployment to test the operator
	@echo "Creating sample httpbin deployment..."
	kubectl apply -f config/samples/orchestrate_v1alpha1_httpbindeployment.yaml
	@echo "Waiting for deployment to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment -l app=httpbin
