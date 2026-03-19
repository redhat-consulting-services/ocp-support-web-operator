# OCP Support Web Operator

A Kubernetes operator that deploys and manages the [OCP Support Web](https://github.com/redhat-consulting-services/ocp-support-web) application on OpenShift clusters. Built with controller-runtime and distributed via OLM (Operator Lifecycle Manager).

## What It Does

The operator manages the full lifecycle of the OCP Support Web application:

- Creates a ServiceAccount with OAuth redirect annotations
- Binds cluster-admin to the app ServiceAccount (required for must-gather and etcd operations)
- Generates and stores an OAuth cookie secret
- Deploys the application with an OpenShift OAuth proxy sidecar
- Creates a Route with TLS re-encryption
- Sets up a ServiceMonitor for Prometheus metrics scraping
- Auto-detects the cluster apps domain from `Ingress/cluster`
- Cleans up cluster-scoped resources (ClusterRoleBinding) on CR deletion via a finalizer

## Prerequisites

- OpenShift 4.12+
- OLM installed (included by default on OpenShift)
- User Workload Monitoring enabled (for metrics)

## Installation

### Via GitOps (recommended)

Copy [`deploy/gitops-install.yaml`](deploy/gitops-install.yaml) into your GitOps repository. It contains everything needed to deploy the operator and application:

- Namespace
- CatalogSource (pulls the operator catalog from quay.io)
- OperatorGroup (scoped to a single namespace)
- Subscription (installs the operator via OLM)
- OCPSupportWeb CR (deploys the application)

```bash
oc apply -f deploy/gitops-install.yaml
```

### Via OLM (manual)

```bash
make bundle-build bundle-push
oc apply -f bundle/
```

### Direct deployment (development)

```bash
make image-build image-push
make deploy OPERATOR_IMG=quay.io/youruser/ocp-support-web-operator:v0.1.0 APP_IMG=quay.io/youruser/ocp-support-web:v0.1.0
```

## Usage

Create an `OCPSupportWeb` custom resource:

```yaml
apiVersion: support.openshift.io/v1alpha1
kind: OCPSupportWeb
metadata:
  name: ocpsupportweb
  namespace: ocp-support-web
spec:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      memory: 256Mi
```

Check status:

```bash
oc get ocpsupportweb
```

The `URL` column shows the route where the application is accessible.

## CR Spec Reference

| Field | Description | Default |
|-------|-------------|---------|
| `spec.image` | Application container image | `RELATED_IMAGE_APP` env var |
| `spec.oauthProxyImage` | OAuth proxy sidecar image | `registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest` |
| `spec.mustGatherImages.standard` | Standard must-gather image | Cluster release payload |
| `spec.mustGatherImages.cnv` | CNV must-gather image | `registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0` |
| `spec.mustGatherImages.odf` | ODF must-gather image | `registry.redhat.io/odf4/ocs-must-gather-rhel9:latest` |
| `spec.clusterDomain` | Cluster apps domain | Auto-detected from `Ingress/cluster` |
| `spec.route.host` | Custom route hostname | Auto-generated |
| `spec.resources` | App container resource requirements | 50m CPU / 64Mi-256Mi memory |
| `spec.oauthProxyResources` | OAuth proxy resource requirements | 10m CPU / 32Mi-64Mi memory |

## Disconnected / Air-Gapped Environments

All container images are configurable. The operator supports the OLM `RELATED_IMAGE_*` convention for automatic image mirroring via `ImageContentSourcePolicy`:

- `RELATED_IMAGE_APP` — application image
- `RELATED_IMAGE_OAUTH_PROXY` — OAuth proxy
- `RELATED_IMAGE_MUST_GATHER_STANDARD` — standard must-gather
- `RELATED_IMAGE_MUST_GATHER_CNV` — CNV must-gather
- `RELATED_IMAGE_MUST_GATHER_ODF` — ODF must-gather

Override in the CR spec for direct configuration.

## Development

```bash
make build           # Build operator binary
make test            # Run tests
make run             # Run operator locally (requires kubeconfig)
make image-build     # Build container image
make image-push      # Push container image
make deploy          # Deploy to cluster without OLM
make undeploy        # Remove from cluster
```

## Project Structure

```
cmd/main.go                          Entry point
api/v1alpha1/                        CRD types (OCPSupportWeb)
internal/controller/                 Reconciliation logic
config/crd/bases/                    CRD YAML
config/rbac/                         RBAC for the operator itself
config/manager/                      Operator Deployment manifest
bundle/                              OLM bundle (CSV, CRD, metadata)
deploy/                              GitOps-ready install manifests
```

## Metrics

The operator exposes controller-runtime metrics on port 8080 and the application exposes custom metrics on port 8081. Both are scraped via ServiceMonitors using OpenShift User Workload Monitoring.

Operator metrics include reconciliation counts and duration. Application metrics include HTTP request counts/duration, active must-gather jobs, and etcd diagnostic job counts.

---

*Assisted by: Claude*
