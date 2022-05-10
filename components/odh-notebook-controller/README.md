# ODH Notebook Controller

The controller will watch the **Kubeflow Notebook** custom resource events to
extend the Kubeflow notebook controller behavior with the following
capabilities:

- Openshift ingress controller integration.
- Openshift OAuth sidecar injection.

It has been developed using **Golang** and
**[Kubebuilder](https://book.kubebuilder.io/quick-start.html)**.

## Implementation detail

By default, when the ODH notebook controller is deployed along with the
Kubeflow notebook controller, it will expose the notebook in the Openshift
ingress by creating a TLS `Route` object.

If the notebook annotation `notebooks.opendatahub.io/inject-oauth` is set to
true, the OAuth proxy will be injected as a sidecar proxy in the notebook
deployment to provide authN and authZ capabilities:

```yaml
apiVersion: kubeflow.org/v1
kind: Notebook
metadata:
  name: example
  annotations:
    notebooks.opendatahub.io/inject-oauth: "true"
```

A [mutating webhook](./controllers/notebook_webhook.go) is part of the ODH
notebook controller, it will add the sidecar to the notebook deployment. The
controller will create all the objects needed by the proxy as explained in the
follow diagram:

![ODH Notebook Controller OAuth injection
diagram](./assets/odh-notebook-controller-oauth-diagram.png)

When accessing the notebook, you will have to authenticate with your Openshift
user, and you will only be able to access it if you have the necessary
permissions.

The authorization is delegated to Openshift RBAC through the `--openshfit-sar`
flag in the OAuth proxy:

```json
--openshift-sar=
{
    "verb":"get",
    "resource":"notebooks",
    "resourceAPIGroup":"kubeflow.org",
    "name":"example",
    "namespace":"opendatahub"
}
```

That is, you will only be able to access the notebook if you can perform a `GET`
notebook operation on the cluster:

```shell
oc get notebook example -n <YOUR_NAMESPACE>
```

## Developer docs

Follow the instructions below if you want to extend the controller
functionality:

### Run unit tests

Unit tests have been developed using the [**Kubernetes envtest
framework**](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest).

Run the following command to execute them:

```shell
make test
```

### Run locally

Install the `notebooks.kubeflow.org` CRD from the [Kubeflow notebook
controller](../notebook-controller) repository as a requirement.

When running the controller locally, the [admission webhook](./config/webhook)
will be running in your local machine. The requests made by the Openshift API
have to be redirected to the local port.

This will be solved by deploying the [Ktunnel
application](https://github.com/omrikiei/ktunnel) in your cluster instead of the
controller manager, it will create a reverse tunnel between the cluster and your
local machine:

```shell
make deploy-dev -e K8S_NAMESPACE=<YOUR_NAMESPACE>
```

Run the controller locally:

```shell
make run -e K8S_NAMESPACE=<YOUR_NAMESPACE>
```

### Deploy local changes

Build a new image with your local changes and push it to `<YOUR_IMAGE>` (by
default `quay.io/opendatahub/odh-notebook-controller`).

```shell
make image -e IMG=<YOUR_IMAGE>
```

Deploy the manager using the image in your registry:

```shell
make deploy -e K8S_NAMESPACE=<YOUR_NAMESPACE> -e IMG=<YOUR_IMAGE>
```
