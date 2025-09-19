#!/usr/bin/env bash

# Before a demo setup the environment, then run
#   ./ocm/imagemgr.sh list
# Then sort the images into the pull_images and images arrays below.
#
# Then you can pull the relevant images ahead of a demo with:
#  ./ocm/imagemgr.sh pull
#
# The ocm setup script calls this script to push the images into the
# kind cluster automatically.

# pull_images are pulled ahead of time put not loaded into kind clusters
# TODO(ntnn): Check if this actually speeds up - I'd hope that kind uses
# images out of the local docker cache if they are available.
pull_images=(
    docker.io/kindest/kindnetd:v20250512-df8de77b
    docker.io/kindest/local-path-provisioner:v20250214-acbabc1a
    registry.k8s.io/coredns/coredns:v1.12.0
    registry.k8s.io/etcd:3.5.21-0
    registry.k8s.io/kube-apiserver:v1.33.1
    registry.k8s.io/kube-controller-manager:v1.33.1
    registry.k8s.io/kube-proxy:v1.33.1
    registry.k8s.io/kube-scheduler:v1.33.1
    registry.k8s.io/etcd:3.5.16-0
)

images=(
    ghcr.io/fluxcd/helm-controller:v1.3.0
    ghcr.io/fluxcd/image-automation-controller:v0.41.1
    ghcr.io/fluxcd/image-reflector-controller:v0.35.2
    ghcr.io/fluxcd/kustomize-controller:v1.6.0
    ghcr.io/fluxcd/notification-controller:v1.6.0
    ghcr.io/fluxcd/source-controller:v1.6.1

    ghcr.io/kcp-dev/api-syncagent:v0.3.0

    ghcr.io/kro-run/kro/controller:0.4.0

    ghcr.io/open-component-model/ocm-k8s-toolkit:latest
    ghcr.io/open-component-model/ocm-k8s-toolkit:latest

    ghcr.io/platform-mesh/httpbin-operator:0.5.10
    ghcr.io/platform-mesh/httpbin-ui2:v0.0.5

    nwallus308/httpbin:latest

    registry.k8s.io/ingress-nginx/controller:v1.12.1
    registry.k8s.io/ingress-nginx/kube-webhook-certgen:v1.4.4
)

case "$1" in
    (list)
        kubectl get pods --all-namespaces \
            -o jsonpath="{.items[*].spec['initContainers', 'containers'][*].image}" \
            | tr -s '[[:space:]]' '\n' \
            | sort \
            | uniq
        ;;
    (pull)
        # If docker desktop is used with containerd to pull and store
        # images this fails, see:
        # https://github.com/kubernetes-sigs/kind/issues/3795
        echo "!! Check that you are not using containerd to pull and store images when using Docker Desktop. !!"
        echo "The option is in Options > General > Use containerd for pulling and storing images"
        echo "If you had it enabled you need to disable it, restart Docker Desktop and then pull again."
        for image in ${images[@]} ${pull_images}; do
            echo "Pulling image: $image"
            docker pull "$image" &
        done
        wait $(jobs -p)
        ;;
    (push)
        kind_cluster="$2"
        if [[ -z "$kind_cluster" ]]; then
            echo "Usage: $0 push <kind-cluster-name>"
            exit 1
        fi
        should_wait=0
        for image in ${images[@]}; do
            if ! docker image inspect "$image" &>/dev/null; then
                # Skip trying to load images that are not present
                # locally
                continue
            fi
            kind load docker-image --name "$kind_cluster" "$image" &
            should_wait=1
        done
        [ "$should_wait" -eq 1 ] && wait $(jobs -p)
        ;;
esac
