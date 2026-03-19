#!/usr/bin/env bash
#
# Development deployment script: builds the operator and app images using
# OpenShift binary builds and deploys everything to the current cluster.
#
# For disconnected environments, set these env vars before running:
#   BUILDER_IMAGE          - Go build image
#   RUNTIME_IMAGE          - UBI micro (operator) or ose-cli (app) runtime image
#   OAUTH_PROXY_IMAGE      - OAuth proxy sidecar image
#   MUST_GATHER_IMAGE_CNV  - CNV must-gather image
#   MUST_GATHER_IMAGE_ODF  - ODF must-gather image
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(dirname "$SCRIPT_DIR")/ocp-support-web"
OPERATOR_NS="ocp-support-web-operator"
APP_NS="${APP_NAMESPACE:-ocp-support-web}"

echo "=== OCP Support Web Operator - Dev Deploy ==="
echo ""

if ! oc whoami &>/dev/null; then
    echo "ERROR: Not logged in to OpenShift. Run 'oc login' first."
    exit 1
fi
echo "Logged in as: $(oc whoami)"
echo "Cluster: $(oc whoami --show-server)"
echo ""

# --- Step 1: Build the application image ---
echo "=== Building application image ==="
oc create namespace "$APP_NS" --dry-run=client -o yaml | oc apply -f -

# ImageStream for app
oc apply -f - <<EOF
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: ocp-support-web
  namespace: $APP_NS
EOF

# BuildConfig for app
oc apply -f - <<EOF
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: ocp-support-web
  namespace: $APP_NS
spec:
  output:
    to:
      kind: ImageStreamTag
      name: ocp-support-web:latest
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      memory: 2Gi
EOF

echo "--- Building app image..."
oc start-build ocp-support-web --from-dir="$APP_DIR" -n "$APP_NS" --follow

APP_IMAGE="image-registry.openshift-image-registry.svc:5000/${APP_NS}/ocp-support-web:latest"
echo "App image: $APP_IMAGE"

# --- Step 2: Build the operator image ---
echo ""
echo "=== Building operator image ==="
oc create namespace "$OPERATOR_NS" --dry-run=client -o yaml | oc apply -f -

# ImageStream for operator
oc apply -f - <<EOF
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: ocp-support-web-operator
  namespace: $OPERATOR_NS
EOF

# BuildConfig for operator
oc apply -f - <<EOF
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: ocp-support-web-operator
  namespace: $OPERATOR_NS
spec:
  output:
    to:
      kind: ImageStreamTag
      name: ocp-support-web-operator:latest
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      memory: 2Gi
EOF

echo "--- Building operator image..."
oc start-build ocp-support-web-operator --from-dir="$SCRIPT_DIR" -n "$OPERATOR_NS" --follow

OPERATOR_IMAGE="image-registry.openshift-image-registry.svc:5000/${OPERATOR_NS}/ocp-support-web-operator:latest"
echo "Operator image: $OPERATOR_IMAGE"

# --- Step 3: Deploy the operator ---
echo ""
echo "=== Deploying operator ==="

# CRD
echo "--- Installing CRD..."
oc apply -f "$SCRIPT_DIR/config/crd/bases/"

# RBAC
echo "--- Creating RBAC..."
cat "$SCRIPT_DIR/config/rbac/service_account.yaml" \
    | sed "s/namespace: system/namespace: $OPERATOR_NS/g" \
    | oc apply -f -
oc apply -f "$SCRIPT_DIR/config/rbac/role.yaml"
cat "$SCRIPT_DIR/config/rbac/role_binding.yaml" \
    | sed "s/namespace: system/namespace: $OPERATOR_NS/g" \
    | oc apply -f -

# Manager deployment
echo "--- Creating operator deployment..."
OAUTH_PROXY_IMAGE="${OAUTH_PROXY_IMAGE:-registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest}"
CNV_IMAGE="${MUST_GATHER_IMAGE_CNV:-registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0}"
ODF_IMAGE="${MUST_GATHER_IMAGE_ODF:-registry.redhat.io/odf4/ocs-must-gather-rhel9:latest}"

cat "$SCRIPT_DIR/config/manager/manager.yaml" \
    | sed "s|OPERATOR_IMAGE_PLACEHOLDER|${OPERATOR_IMAGE}|g" \
    | sed "s|RELATED_IMAGE_APP_PLACEHOLDER|${APP_IMAGE}|g" \
    | sed "s|registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest|${OAUTH_PROXY_IMAGE}|g" \
    | sed "s|registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0|${CNV_IMAGE}|g" \
    | sed "s|registry.redhat.io/odf4/ocs-must-gather-rhel9:latest|${ODF_IMAGE}|g" \
    | sed "s/namespace: system/namespace: $OPERATOR_NS/g" \
    | oc apply -f -

echo "--- Waiting for operator to be ready..."
oc rollout status deployment/ocp-support-web-operator -n "$OPERATOR_NS" --timeout=120s || true

# --- Step 4: Create the CR ---
echo ""
echo "=== Creating OCPSupportWeb CR ==="
oc apply -f - <<EOF
apiVersion: support.openshift.io/v1alpha1
kind: OCPSupportWeb
metadata:
  name: ocpsupportweb
  namespace: $APP_NS
spec:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      memory: 256Mi
EOF

echo "--- Waiting for application deployment..."
sleep 5
oc rollout status deployment/ocp-support-web -n "$APP_NS" --timeout=120s || true

ROUTE_URL=$(oc get route ocp-support-web -n "$APP_NS" -o jsonpath='{.spec.host}' 2>/dev/null || echo "pending")

echo ""
echo "=== Deployment complete ==="
echo ""
echo "  Operator NS: $OPERATOR_NS"
echo "  App NS:      $APP_NS"
echo "  Route:       https://$ROUTE_URL"
echo ""
echo "  oc get ocpsupportwebs -n $APP_NS"
echo ""
