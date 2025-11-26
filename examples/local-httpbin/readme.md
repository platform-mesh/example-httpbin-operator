# local-httpbin

This example shows how to run example-httpbin-operator to deploy HttpBins on in
a single cluster.

## Prerequisites

- kind
- kubectl
- docker
- helm

## Steps

Setup a kind cluster:

```bash noci
kind create cluster --name local-httpbin
```

Build and load the docker image into the kind cluster:

```bash
make docker-install IMG=local/local-httpbin:dev KIND_CLUSTER=local-httpbin
```

Install the CRDs and operator:

Using Helm (from OCI registry):
```bash
# Install using helm from GHCR
helm --kube-context kind-local-httpbin upgrade --install \
    example-httpbin-operator oci://ghcr.io/platform-mesh/helm-charts/example-httpbin-operator \
    --set image.registry=local \
    --set image.repository=local-httpbin \
    --set image.tag=dev \
    --set operator.args={--local-ingress} \
    --wait
```