# OCP Support Web Operator

A Kubernetes operator that deploys and manages the [OCP Support Web](https://github.com/redhat-consulting-services/ocp-support-web) application on OpenShift clusters. Built with controller-runtime and distributed via OLM (Operator Lifecycle Manager).

## What It Does

The operator manages the full lifecycle of the OCP Support Web application:

- Creates a ServiceAccount with OAuth redirect annotations
- Creates a scoped ClusterRole for the app ServiceAccount with least-privilege access
- Generates and stores an OAuth cookie secret
- Deploys the application with an OpenShift OAuth proxy sidecar
- Creates a Route with TLS re-encryption
- Sets up a ServiceMonitor for Prometheus metrics scraping
- Auto-detects the cluster apps domain from `Ingress/cluster`
- Creates a `gather-common` ConfigMap for customizing gather definitions
- Cleans up cluster-scoped resources (ClusterRole, ClusterRoleBinding) on CR deletion via a finalizer

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
      memory: 512Mi
```

Check status:

```bash
oc get ocpsupportweb
```

The `URL` column shows the route where the application is accessible.

## CR Spec Reference

| Field | Description | Default |
|-------|-------------|---------|
| `spec.image` | Application container image (web UI, ACM agents, must-gather) | `RELATED_IMAGE_APP` env var |
| `spec.oauthProxyImage` | OAuth proxy sidecar image | `registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest` |
| `spec.clusterDomain` | Cluster apps domain | Auto-detected from `Ingress/cluster` |
| `spec.route.host` | Custom route hostname | Auto-generated |
| `spec.resources` | App container resource requirements | 50m CPU / 128Mi-512Mi memory |
| `spec.oauthProxyResources` | OAuth proxy resource requirements | 10m CPU / 32Mi-64Mi memory |
| `spec.allowedGroups` | OpenShift groups allowed to access the app | `["cluster-admins"]` |

## SupportGather CRD

The `SupportGather` custom resource lets you trigger must-gather operations declaratively via YAML. The operator watches for these resources and drives the gather through the OCP Support Web backend.

### Gather all detected operators

```yaml
apiVersion: support.openshift.io/v1alpha1
kind: SupportGather
metadata:
  name: full-gather
  namespace: ocp-support-web
spec:
  gatherTypes: ["all"]
```

### Targeted gather for specific operators

```yaml
apiVersion: support.openshift.io/v1alpha1
kind: SupportGather
metadata:
  name: cnv-odf-gather
  namespace: ocp-support-web
spec:
  gatherTypes: ["virtualization", "odf"]
  anonymize: true
  since: "24h"
```

### Namespace-scoped custom gather

```yaml
apiVersion: support.openshift.io/v1alpha1
kind: SupportGather
metadata:
  name: custom-gather
  namespace: ocp-support-web
spec:
  namespaces: ["openshift-cnv", "openshift-storage"]
  resourceTypes: ["pods", "events", "configmaps"]
  includeLogs: true
```

### Gather with automatic upload to Red Hat

```yaml
apiVersion: support.openshift.io/v1alpha1
kind: SupportGather
metadata:
  name: case-upload
  namespace: ocp-support-web
spec:
  gatherTypes: ["all"]
  upload:
    caseID: "03291543"
    secretRef:
      name: rh-upload-creds
```

The upload secret must contain `username` and `password` keys for Red Hat's SFTP server (`sftp.access.redhat.com`). Set `upload.internalUser: true` to route uploads to the internal Red Hat path (`/case-mgmt/`) instead of the external path (`/incoming/`).

### SupportGather Spec Reference

| Field | Description | Default |
|-------|-------------|---------|
| `spec.gatherTypes` | Operator profiles to gather (`["all"]`, `["virtualization", "odf"]`, etc.) | `["all"]` |
| `spec.namespaces` | Limit gather to specific namespaces (custom gather mode) | — |
| `spec.resourceTypes` | Resource types to collect in custom gather mode | — |
| `spec.includeLogs` | Collect pod logs in custom gather mode | — |
| `spec.anonymize` | Anonymize IPs, MACs, domains, secrets in the archive | `false` |
| `spec.since` | Time window for log collection (`"6h"`, `"24h"`, `"48h"`) | — |
| `spec.upload.caseID` | Red Hat support case number for automatic upload | — |
| `spec.upload.secretRef.name` | Secret with SFTP credentials (`username`/`password` keys) | — |
| `spec.upload.internalUser` | Use internal Red Hat upload path | `false` |

### Check status

```bash
oc get supportgathers
```

The output shows Phase, Progress, and Age columns. Phases progress through: Pending, Gathering, Uploading (if upload configured), Complete, or Failed.

## Disconnected / Air-Gapped Environments

Container images are configurable in the CR spec:

- `spec.image` — the application image (used for the web UI, ACM remote agents, and standalone must-gather)
- `spec.oauthProxyImage` — the OAuth proxy sidecar

The operator supports the OLM `RELATED_IMAGE_*` convention for automatic image mirroring via `ImageContentSourcePolicy`:

- `RELATED_IMAGE_APP` — application image
- `RELATED_IMAGE_OAUTH_PROXY` — OAuth proxy

For disconnected environments, set `spec.image` to the mirrored location of the application image in your internal registry.

## Standalone Must-Gather

The application image can also be used directly as a must-gather image with `oc adm must-gather`:

```bash
oc adm must-gather --image=quay.io/redhat-consulting-services/ocp-support-web:v3.0.0
```

This auto-detects installed operators and collects diagnostics for all of them using native Go API calls — no operator-specific must-gather images required.

## ConfigMap-Driven Gather Configuration

The operator creates a `gather-common` ConfigMap in the application namespace. Users can edit this ConfigMap to add custom resources, pod exec commands, node commands, and log specifications. Changes take effect on the next gather request without restarting the application.

See the [application README](https://github.com/redhat-consulting-services/ocp-support-web#configmap-driven-gather-configuration) for the full ConfigMap format and examples.

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
api/v1alpha1/                        CRD types (OCPSupportWeb, SupportGather)
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
