#!/usr/bin/env bash

# Continually checks for NodePort services and starts forwarding the
# port from the host. E.g. if a service whoami is started with type
# NodePort 32291 assigned by Kube, then the script will forward the port
# 32291 to the service.
# On errors it should retry.

list_nodeports() {
    kubectl get svc --field-selector spec.type==NodePort --ignore-not-found --all-namespaces --output name
}

get_nodeport() {
    kubectl get "$1" --output jsonpath='{.spec.ports[0].nodePort}'
}

get_port() {
    kubectl get "$1" --output jsonpath='{.spec.ports[0].port}'
}

forward() {
    service="$1"
    nodeport="$2"
    port="$3"
    kubectl port-forward "$service" "$nodeport:$port" &
}

declare -A bound_ports
bound_ports=()

while sleep 1; do
    for service in $(list_nodeports); do
        nodeport="$(get_nodeport $service)"
        if [[ "${bound_ports[$nodeport]}" != "" ]]; then
            continue
        fi
        echo "binding port $nodeport"
        port="$(get_port $service)"

        if forward "$service" "$nodeport" "$port" ; then
            bound_ports[$nodeport]="$port"
        fi
    done
done
