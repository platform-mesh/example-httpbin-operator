# Open Component Model

This directory contains the configuration files required for the [Open Component Model](http://ocm.software) ([OCM](http://ocm.software)) integration.

## Overview

The [Open Component Model](http://ocm.software) (OCM) is your one-stop open-source Software Bill of Delivery (SBoD) for packaging, signing, transporting and deploying your artifacts – preserving end-to-end security, integrity and provenance.

This directory stores the necessary configuration and resources for orchestrating k8s workload through OCM.

## Preparation

It is possible to preload the docker images used for this demo by using
the `ocm/imagemgr.sh` script.

This requires to be authenticated to ghcr.io with a personal access
token that has the `read:packages` scope as some images are currently
private.

```bash
docker login ghcr.io -u <user>
```

To preload the images, run the following command:

```bash
bash ./ocm/imagemgr.sh pull
```

The images can be manually loaded into a cluster by running:

```bash
bash ./ocm/imagemgr.sh load <kind cluster name>
```

The `kind-cluster-setup.sh` will automatically use the image manager
script to load any available images into the cluster.

Note that this is entirely optional, it will only help in speeding up
the deployments.

## Getting Started

1. Ensure you have [kind cli](https://kind.sigs.k8s.io) installed
2. Create `kind` cluster on your local machine
```bash
kind version
# kind v0.27.0 go1.24.0 darwin/arm64
```
```bash
kind create cluster
```
```bash
Creating cluster "kind" ...
 ✓ Ensuring node image (kindest/node:v1.32.2) 🖼
 ✓ Preparing nodes 📦
 ✓ Writing configuration 📜
 ✓ Starting control-plane 🕹️
 ✓ Installing CNI 🔌
 ✓ Installing StorageClass 💾
Set kubectl context to "kind-kind"
You can now use your cluster with:

kubectl cluster-info --context kind-kind

Have a nice day! 👋
```
3. Install `flux` on your `kind` cluster
```bash
kubectl cluster-info --context kind-kind
# Kubernetes control plane is running at https://127.0.0.1:62133
# CoreDNS is running at https://127.0.0.1:62133/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy
```

```bash
kubectl config use-context kind-kind
# Switched to context "kind-kind".
```

```bash
kubectl apply -f https://github.com/fluxcd/flux2/releases/download/v2.6.2/install.yaml
```

4. Install `kro` on your `kind` cluster
Install the KRO (Kubernetes Resource Orchestrator) controller:

```bash
export KRO_VERSION=$(curl -sL \
    https://api.github.com/repos/kro-run/kro/releases/latest | \
    jq -r '.tag_name | ltrimstr("v")'
  )
```

```bash
echo $KRO_VERSION
# 0.3.0
```
```bash
helm install kro oci://ghcr.io/kro-run/kro/kro \
  --namespace kro \
  --create-namespace \
  --version=${KRO_VERSION}

# Pulled: ghcr.io/kro-run/kro/kro:0.3.0
# Digest: sha256:90e58aa85eb95c4df402c7104c48ebdc0e33393689cccf12ef99bb95caf99ef6
# NAME: kro
# LAST DEPLOYED: Mon Jun  2 13:47:01 2025
# NAMESPACE: kro
# STATUS: deployed
# REVISION: 1
# TEST SUITE: None
```

```bash
helm -n kro list
# NAME	NAMESPACE	REVISION	UPDATED                              	STATUS  	CHART    	APP VERSION
# kro 	kro      	1       	2025-06-02 13:47:01.641988 +0200 CEST	deployed	kro-0.3.0	0.3.0
```

5. Install `ocm-k8s-toolkit` on your `kind` cluster
Install the OCM Kubernetes toolkit following the [official documentation](https://github.com/open-component-model/ocm-k8s-toolkit/tree/main?tab=readme-ov-file#installation)

```bash
kubectl apply -k https://github.com/open-component-model/ocm-k8s-toolkit/config/default?ref=d1ea81fc8cb410274d57a46ab67f4435200ed7f6
kubectl --namespace ocm-k8s-toolkit-system rollout status deployment --timeout=5m
```

6. Private OCI Registry

If necessary, create a k8s secret which contains authentication credentials for your private OCI registries:

```bash
kubectl create secret docker-registry github-oci-pull-secret \
  --docker-username=sk31337 \
  --docker-password=github_..... \
  --docker-server=ghcr.io/platform-mesh
```
When using bash script `kind-cluster-setup.sh` you can put all of your k8s manifests for credentials into `ocm/k8s/credentials` folder which will be applied automatically.

7. OCM Bootstraping

Apply OCM Bootstrap Manifests:

```bash
kubectl k apply -f ocm/k8s/bootstrap.yaml
# ocmrepository.delivery.ocm.software/httpbin-operator-repository created
# component.delivery.ocm.software/httpbin-operator-component created
# resource.delivery.ocm.software/httpbin-operator-resource-rgd created
# deployer.delivery.ocm.software/httpbin-operator-deployer created
```

Verify resource creation
```bash
kubectl get repositories.delivery.ocm.software,components,resource,deployers.delivery.ocm.software,resourcegraphdefinitions.kro.run -owide
# NAME                                                              AGE
# repository.delivery.ocm.software/httpbin-operator-repository   2m

# NAME                                                         AGE
# component.delivery.ocm.software/httpbin-operator-component   2m

# NAME                                                           AGE
# resource.delivery.ocm.software/httpbin-operator-resource-rgd   2m

# NAME                                                       AGE
# deployer.delivery.ocm.software/httpbin-operator-deployer   2m

# NAME                                                   APIVERSION   KIND              STATE    TOPOLOGICALORDER                                                                                            AGE
# resourcegraphdefinition.kro.run/msp-httpbin-operator   v1alpha1     HttpbinOperator   Active   ["resourceChartCrds","ocirepositoryCrds","helmreleaseCrds","resourceChart","ocirepository","helmrelease"]   93s
```

8. Wait for KRO to have deployed the `HttpBinOperator` CRDs:

```bash
kubectl get crd httpbinoperators.kro.run
```

9. MSP http-bin deployment
Apply `kro` manifest to create an `HttpBinOperator` instance:

```bash
kubectl apply -f ocm/k8s/kro-instance.yaml
# httpbinoperator.kro.run/httpbin-operator-simple created
```

10. Verify deployment status:
```bash
kubectl get httpbinoperators.kro.run,pods -owide
# NAME                                              STATE    SYNCED   AGE
# httpbinoperator.kro.run/httpbin-operator-simple   ACTIVE   True     2m9s

# NAME                                   READY   STATUS    RESTARTS   AGE    IP            NODE                 NOMINATED NODE   READINESS GATES
# pod/httpbin-operator-664744dc5-xm7x5   1/1     Running   0          107s   10.244.0.44   kind-control-plane   <none>           <none>
```

11. Fulfilling orders with kcp-api-syncagent
See [official documentation](https://github.com/platform-mesh/httpbin-operator/tree/main/docs/kcp-api-syncagent).

## Additional Resources
- [Open Component Model Official Documentation](http://ocm.software)
- [kro Official Documentation](http://kro.run)
- [flux Official Documentation](http://fluxcd.io)
