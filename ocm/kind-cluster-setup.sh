#!/usr/bin/env bash

set -euo pipefail
log() { echo ">>> $@"; }
sep() { echo "---------------------------------------------------------"; }

# wait_for is filled with commands by functions that deploy resources
# that need to be waited for before finishing the script.
declare -a wait_for=()

# Check if required tools are installed
check_dependencies() {
  sep
  local missing_deps=()

  if ! command -v kind &>/dev/null; then
    missing_deps+=("kind")
  fi

  if ! command -v kubectl &>/dev/null; then
    missing_deps+=("kubectl")
  fi

  if ! command -v helm &>/dev/null; then
    missing_deps+=("helm")
  fi

  if ! command -v jq &>/dev/null; then
    missing_deps+=("jq")
  fi

  if [ ${#missing_deps[@]} -ne 0 ]; then
    log "Error: The following required tools are missing:"
    printf '%s\n' "${missing_deps[@]}"
    log "Please install them and try again."
    exit 1
  fi

  log "All dependencies are available."
  return 0
}

# Check if cluster already exists
check_existing_cluster() {
  sep
  if kind get clusters | grep -q "^poc-example-httpbin-operator$"; then
    log "Warning: Kind cluster 'poc-example-httpbin-operator' already exists."
    while true; do
      read -p "Do you want to delete it and continue? (y/n): " choice
      case "$choice" in
      y | Y)
        log "Deleting existing cluster..."
        kind delete cluster --name poc-example-httpbin-operator
        return 0
        ;;
      n | N)
        log "Aborting setup..."
        exit 0
        ;;
      *)
        log "Please answer y or n."
        ;;
      esac
    done
  fi
  return 0
}

# Create Kind cluster
create_cluster() {
  sep
  log "Creating Kind cluster 'poc-example-httpbin-operator'..."
  kind create cluster -n poc-example-httpbin-operator --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
  - containerPort: 443
    hostPort: 443
EOF

  kubectl cluster-info --context kind-poc-example-httpbin-operator
  log "Kind cluster created successfully."
}

preload_images() {
  sep
  log "Preloading images into the cluster if present..."
  ./ocm/imagemgr.sh push poc-example-httpbin-operator || true
}

configure_coredns() {
  if ! docker ps | grep -q openmfp-control-plane; then
      return
  fi
  sep
  log "Configuring CoreDNS..."

  openmfp_ip="$(docker inspect "openmfp-control-plane" | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress')"
  log "Pointing OpenMFP addresses to ${openmfp_ip}..."
  sed -e "s#TARGET#${openmfp_ip}#g" ./ocm/coredns.yaml.template \
      | kubectl apply -f-
}

wait_ingress(){
    log "Waiting for nginx-ingress to be ready..."
    kubectl -n ingress-nginx rollout status deployment ingress-nginx-controller
}

install_ingress() {
    sep
    log "Installing nginx ingress..."
    kubectl apply -f https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml

    # Disabling HSTS and ssl-redirects, otherwise browser refuse to
    # access the http pages
    kubectl apply -f- <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
data:
  hsts: "false"
  ssl-redirect: "false"
EOF

  wait_for+=("wait_ingress")
}

wait_flux() {
  log "Waiting for flux to be ready..."
  kubectl --namespace flux-system rollout status deployment --timeout=5m
}

# Install Flux
install_flux() {
  sep
  log "Installing Flux..."
  kubectl apply -f https://github.com/fluxcd/flux2/releases/download/v2.6.2/install.yaml

  wait_for+=("wait_flux")
}

wait_kro() {
  log "Waiting for kro.run to be ready..."
  kubectl --namespace kro rollout status deployment --timeout=5m
}

# Add kro.run Helm repository and install chart
install_kro_run() {
  sep
  log "Installing kro.run..."

  # Get latest version with error handling
  log "Found kro.run version"

  # Install kro.run with error handling
  log "Installing kro.run via Helm..."
  helm install kro oci://ghcr.io/kro-run/kro/kro \
    --namespace kro \
    --create-namespace

  log "✓ kro.run installed successfully"
  wait_for+=("wait_kro")
}

wait_kro_instance() {
  log "Waiting for example-httpbin-operator to be ready..."
  kubectl wait --for jsonpath='{.status.state}'=ACTIVE httpbinoperators.kro.run example-httpbin-operator-simple
  kubectl rollout status deployment example-httpbin-operator
}

# Apply kro instance if CRD exists
apply_kro_instance() {
  sep
  local retries=0
  local max_retries=10

  #while [ $retries -lt $max_retries ]; do
  while true; do
    log "Checking for Kro.run HttpbinOperator CRD (attempt $((retries + 1))/$max_retries)..."

    if kubectl get crd httpbinoperators.kro.run &>/dev/null; then
      log "HttpbinOperator CRD found, applying kro-instance.yaml..."
      if [ -f "ocm/k8s/kro-instance.yaml" ]; then
        kubectl apply -f ocm/k8s/kro-instance.yaml
        log "Successfully applied kro-instance.yaml"
        wait_for+=("wait_kro_instance")
        return 0
      else
        log "Warning: ocm/k8s/kro-instance.yaml not found"
        return 1
      fi
    fi

    retries=$((retries + 1))
    if [ $retries -eq $max_retries ]; then
      log "CRD not found after $max_retries attempts."
      while true; do
        read -p "Do you want to continue checking for the CRD? (y/n): " choice
        case "$choice" in
        y | Y)
          log "Continuing to check..."
          retries=0 # Reset retry counter
          break
          ;;
        n | N)
          log "Exiting..."
          return 1
          ;;
        *)
          log "Please answer y or n."
          ;;
        esac
      done
    else
      log "CRD not found. Waiting 5 seconds before retry..."
      sleep 5
    fi
  done
}

wait_ocm_k8s_toolkit() {
  log "Waiting for OCM K8s Toolkit to be ready..."
  kubectl --namespace ocm-k8s-toolkit-system rollout status deployment --timeout=5m
}

install_ocm_k8s_toolkit() {
  sep
  log "Installing OCM K8s Toolkit..."

  # This is a workaround to generate the manifests and pin the image to
  # a known working version. When the reference for the ocm toolkit is
  # updated the image version also needs to be updated:
  # https://github.com/open-component-model/open-component-model/pkgs/container/kubernetes%2Fcontroller
  kubectl apply -k https://github.com/open-component-model/open-component-model/kubernetes/controller/config/default?ref=9a481652c88b68f5be2c34e7b821b9c2b7b70aee --dry-run=client -o yaml \
      | sed -e 's#ghcr.io/open-component-model/kubernetes/controller:latest#ghcr.io/open-component-model/kubernetes/controller@sha256:f3d60b409af3d1c332151435ef3a38b6fc7babcd413224865896b524e25681ad#g' \
      | kubectl apply -f -

  wait_for+=("wait_ocm_k8s_toolkit")
}

init_credentials() {
  sep
  log "Proceeding with OCM configuration..."
  kubectl create ns api-syncagent || true
  # Check if credentials directory exists and contains files
  # if [ -n "$SYNCAGENT_KUBECONFIG" ]; then
  #   kubectl create secret -n api-syncagent generic kubeconfig --from-file=kubeconfig="$SYNCAGENT_KUBECONFIG" --dry-run=client -o yaml | kubectl apply -f -
  # fi

  if [ -d "ocm/k8s/credentials" ]; then
    log "Applying Kubernetes manifests from ocm/k8s/credentials..."
    kubectl apply -f ocm/k8s/credentials/
    log "OCM credentials applied successfully!"
  else
    log "Warning: No credentials found"
  fi
}

# Watch httpbin resources with retry logic
watch_httpbin_orders() {
  sep
  log "Watching mesh service provider orders of httpbin instances from ApeiRO showroom..."
  kubectl get httpbin --all-namespaces --watch
}

wait_kcp_syncagent() {
  log "Waiting for KCP sync agent to be ready..."
  kubectl wait --for condition=Ready pod -l app.kubernetes.io/name=kcp-api-syncagent -n api-syncagent
}

# Setup KCP sync agent
setup_kcp_syncagent() {
  sep
  log "Setting up KCP sync agent..."

  # Install the HttpBin CRDs - the OCM CV also deploys the CRDs but
  # since the api-syncagent is deployed before the CV api-syncagent will
  # go into a backoff loop. It would eventually sync _but_ for a demo it
  # needs to be ready immediately.
  kubectl apply -k ./config/crd

  # Add Helm repository
  helm repo add kcp https://kcp-dev.github.io/helm-charts
  helm repo update kcp

  # Apply RBAC and restart deployment
  log "Applying RBAC configuration..."
  kubectl apply -f docs/kcp-api-syncagent/rbac.yaml

  # Install/upgrade KCP sync agent
  helm upgrade --install api-syncagent kcp/api-syncagent --create-namespace \
    --namespace api-syncagent \
    --version 0.3.0 \
    --set image.tag=v0.3.0 \
    --set apiExportName=httpbin.cloud.sap \
    --set agentName=kcp-api-syncagent \
    --set kcpKubeconfig=kubeconfig

  # Apply published resources
  log "Applying published resources..."
  kubectl apply -f ./docs/localhost-ingress-httpbinui/published-resource.yaml

  # log "Restarting KCP sync agent deployment..."
  # kubectl rollout restart deployment api-syncagent -n api-syncagent

  log "✓ KCP sync agent setup completed"
  wait_for+=("wait_kcp_syncagent")
}

wait_httpbin_ui2() {
  log "Waiting for httpbin-ui2 to be ready..."
  kubectl rollout status deployment httpbin-ui --timeout=5m

  log "Setting up port-forward for httpbin-ui2..."
  {
    kubectl port-forward svc/httpbin-ui 8000:80 >/dev/null &!
  } || true
}

setup_httpbin_ui2() {
  sep
  log "Deploying httpbin-ui2..."

  # Apply Kubernetes manifests
  kubectl apply -k ./docs/localhost-ingress-httpbinui/httpbin-ui/

  log "httpbin-ui2 is available at http://localhost:8000"
  wait_for+=("wait_httpbin_ui2")
}

main() {
  log "Starting kind cluster setup..."

  # Check dependencies
  check_dependencies

  # Check existing cluster
  check_existing_cluster

  # Create cluster
  create_cluster

  # Preload images into the kind cluster
  preload_images

  # Install ingress
  install_ingress

  # Install Flux
  install_flux

  # Install kro.run
  install_kro_run

  install_ocm_k8s_toolkit
  log "Initial Setup completed successfully!"

  # Initialize k8s credentials
  init_credentials

  # Commented out for demo - see the readme for executing these steps
  # manually
  # kubectl apply -f ocm/k8s/bootstrap.yaml
  # kubectl get repositories.delivery.ocm.software,components,resource,deployers.delivery.ocm.software,resourcegraphdefinitions.kro.run -owide
  # apply_kro_instance

  # Setup KCP sync agent
  setup_kcp_syncagent

  # Deploy httpbin-ui2
  # setup_httpbin_ui2

  # Run the wait functions to check that all components are ready
  for w in ${wait_for[@]}; do
    "$w"
  done

  log "Setup completed successfully!"

  # Watch httpbin resources
  # watch_httpbin_orders
}

main
