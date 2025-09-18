#!/usr/bin/env bash

set -euo pipefail
log() { echo ">>> $@"; }

cd "$(dirname "$0")"

if ! kind get clusters | grep -q httpbin-operator; then
    log Creating kind cluster
    kind create cluster --name httpbin-operator --config kind-httpbin.yaml
fi

_kubectl() {
    kubectl --context kind-httpbin-operator "$@"
}

log Deploying ingress
_kubectl apply -f https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml
_kubectl apply -f ./ingress-nginx-cm.yaml
_kubectl -n ingress-nginx rollout status deployment ingress-nginx-controller

log Deploying httpbin-operator
make -C ../../ docker-install manifests IMG=controller:dev KIND_CLUSTER=httpbin-operator
_kubectl apply -k ./operator

log Creating test HttpBin
_kubectl apply -f ../../examples/httpbin.yaml
_kubectl wait --for=jsonpath=status.url httpbin example-httpbin
while ! curl -L 'http://example-httpbin.localhost/get?a=b'; do
    log "Waiting for HttpBin to be ready..."
    sleep 1
done

log Deplyoing api-syncagent
_kubectl create ns api-syncagent || true
if ! _kubectl -n api-syncagent get secret provider-kubeconfig; then
    if [[ -z "$SYNCAGENT_KUBECONFIG" ]]; then
        log "Please set the PROVIDER_KUBECONFIG environment variable to the kubeconfig for api-syncagent."
        exit 1
    fi
    _kubectl -n api-syncagent create secret generic provider-kubeconfig --from-file=kubeconfig="$SYNCAGENT_KUBECONFIG"
fi

helm repo add kcp https://kcp-dev.github.io/helm-charts
helm repo update
helm --kube-context kind-httpbin-operator upgrade --install api-syncagent kcp/api-syncagent \
    --create-namespace \
    --namespace api-syncagent \
    --version 0.3.0 \
    --set image.tag=v0.3.0 \
    --set apiExportName=demo.platform-mesh.io \
    --set agentName=kcp-api-syncagent \
    --set kcpKubeconfig=provider-kubeconfig

_kubectl apply -f ./api-syncagent-rbac.yaml
_kubectl -n api-syncagent apply -f ./published-resource.yaml

log Deploy httpbin-ui
docker pull ghcr.io/platform-mesh/httpbin-ui2:v0.0.3
kind load docker-image ghcr.io/platform-mesh/httpbin-ui2:v0.0.3 --name httpbin-operator
_kubectl apply -k ./httpbin-ui
{
    _kubectl port-forward svc/httpbin-ui 8000:80 >/dev/null &!
} || true

log DONE

echo "httpbin-ui2 is available at http://localhost:8000"
