# Fulfilling HttpBin order with example-httpbin-operator and kcp-api-syncagent

## Prerequisites

A kind cluster with example-httpbin-operator configured as noted in
`ocm/readme.md`.

A kubeconfig with a service account with RBAC to manage resources in the
consumer cluster as documented here:

https://docs.kcp.io/api-syncagent/main/getting-started/#kcp-rbac

## Setup api-syncagent

Create a namespace for the api-syncagent:

    kubectl create namespace api-syncagent

Create a secret with the kubeconfig mentioned above for the consumer cluster:

    kubectl create secret generic syncagent-provider-kubeconfig --from-file=kubeconfig=<path-to-kubeconfig> -n api-syncagent

Install the api-syncagent with flux:

    kubectl apply -f docs/kcp-api-syncagent/api-syncagent.yaml

Wait for the api-syncagent to be ready:

    kubectl wait --for condition=Ready pod -l app.kubernetes.io/name=kcp-api-syncagent -n api-syncagent

## Setup RBAC

The api-syncagent service accounts needs a cluster role to manage the
resources that can be ordered in the consumer cluster:

    kubectl apply -f docs/kcp-api-syncagent/rbac.yaml

The HttpBin resources is managed by the api-syncagent and triggers the
example-httpbin-operator to create the actual HttpBinDeployment.

After the example-httpbin-operator updated the status of the HttpBin resource
api-syncagent synchronizes the status back to the consumer cluster.

## Create the PublishedResources

The published resources tells the api-syncagent which resources to
synchronize between the provider and consumer clusters.

    kubectl apply -f ./docs/kcp-api-syncagent/publishedresources.yaml

The `HttpBin` resource will show up within a couple seconds in the
provider cluster:

    kubectl get httpbin --all-namespaces
