# Platform Mesh Quickstart

This guide will help you run Platform Mesh locally and set up the KCP provider for development and testing.

---

## Prerequisites

- [Docker](https://www.docker.com/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [kind](https://kind.sigs.k8s.io/)
- [Go Task](https://taskfile.dev/) (`brew install go-task/tap/go-task` on Mac)
- [Helm](https://helm.sh/)
- [OCM CLI](https://ocm.software/)
- [KCP](https://github.com/kcp-dev/kcp) (optional, for advanced scenarios)

---

## 1. Install the platform-mesh on kind cluster

### Create OCI Pull Secret if using private

If you need to pull private charts from GHCR, export following variables:

```sh
export GITHUB_OCI_USER=your-gh-username
export GITHUB_OCI_PASS=your-ghcr-token
```

### Run the setup process

Run following make target:
```sh
make local-platform-mesh
```

This will setup everything needed for running whole setup locally by cloning helm-charts repo to local directory

```sh
git clone https://github.com/platform-mesh/helm-charts.git -b main .helm-charts
```

Adding the following to your `/etc/hosts` file:

```
127.0.0.1 demo.portal.dev.local default.portal.dev.local portal.dev.local kcp.api.portal.dev.local
```

Creating kind cluster named `platform-mesh`

And running task local-setup-cached from https://github.com/platform-mesh/helm-charts/local-setup
This will deploy the latest versions of platform-mesh components onto kind cluster.

#### Example-httpbin-operator deployment

Example-httpbin-operator is being deployed by first bootstrapping required OCM objects,
you can check their configuration under this path
```sh
platform-mesh/example-httpbin-operator/pullsecret/ocm-bootstrap.yaml
```

This will:
- Create the OCM Repository and Component resources
- Prepare your cluster for OCM-based delivery

As a second step it will deploy the operator with Flux, using these resources:

```sh
platform-mesh/example-httpbin-operator/pullsecret/helm.yaml
```

This will:
- Create an OCIRepository resource pointing to your chart in GHCR
- Create a HelmRelease resource to deploy the operator

## 2. Verify everything is Running

```sh
kubectl get pods -A
kubectl get helmreleases -A
kubectl get ocirepositories -A
kubectl get components -A
kubectl get resources -A
```

## 3. Setup the demo organization

Navigate to portal.dev.local:8443 and first create user, log in using your provided credentials
Setup the `demo` organization.

## 4. Setup KCP Provider

To define service provider for HTTPBin operator, we need to setup first the kcp provider workspace

1. **Export kcp kubeconfig**
	export KUBECONFIG=.helm-charts/.secret/kcp/admin.kubeconfig

2. **Configure your KCP workspace and context as needed:**
     ```sh
	kubectl ws create providers --type=root:providers
	kubectl ws create httpbin-provider --type=root:providers
     ```

3. **Deploy the KCP provider resources in your provider workspace:**

   ```sh
   kubectl ws httpbin-provider
   kubectl apply -f platform-mesh/provider-setup
   ```

## 5. Next steps

Set up the api-syncagent
Configure consumer workspace
Sync and deploy httpbin instance

## Troubleshooting

- Check pod logs with `kubectl logs <pod> -n <namespace>`
- Ensure your OCIRepository and HelmRelease resources are `Ready`
- If you see authentication errors, double-check your pull secret and registry permissions

---

## References

- [Platform Mesh Helm Charts](https://github.com/platform-mesh/helm-charts)
- [OCM Documentation](https://ocm.software/)
- [KCP Documentation](https://github.com/kcp-dev/kcp)
