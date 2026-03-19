#!/usr/bin/env bash
#
# Deploy the operator via OLM so it appears under "Installed Operators"
# in the OpenShift console.
#
# This creates:
# 1. A bundle image (from bundle/Containerfile)
# 2. A catalog image (file-based catalog)
# 3. A CatalogSource in openshift-marketplace
# 4. A Subscription that installs the operator
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(dirname "$SCRIPT_DIR")/ocp-support-web"
OPERATOR_NS="ocp-support-web-operator"
APP_NS="${APP_NAMESPACE:-ocp-support-web}"
VERSION="${VERSION:-0.1.0}"

echo "=== OCP Support Web Operator - OLM Deploy ==="
echo ""

if ! oc whoami &>/dev/null; then
    echo "ERROR: Not logged in to OpenShift. Run 'oc login' first."
    exit 1
fi
echo "Logged in as: $(oc whoami)"
echo ""

# --- Step 1: Build the application image ---
echo "=== Step 1: Building application image ==="
oc create namespace "$APP_NS" --dry-run=client -o yaml | oc apply -f -

oc apply -f - <<EOF
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: ocp-support-web
  namespace: $APP_NS
EOF

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

oc start-build ocp-support-web --from-dir="$APP_DIR" -n "$APP_NS" --follow
APP_IMAGE="image-registry.openshift-image-registry.svc:5000/${APP_NS}/ocp-support-web:latest"
echo "App image: $APP_IMAGE"

# --- Step 2: Build the operator image ---
echo ""
echo "=== Step 2: Building operator image ==="
oc create namespace "$OPERATOR_NS" --dry-run=client -o yaml | oc apply -f -

oc apply -f - <<EOF
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: ocp-support-web-operator
  namespace: $OPERATOR_NS
EOF

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

oc start-build ocp-support-web-operator --from-dir="$SCRIPT_DIR" -n "$OPERATOR_NS" --follow
OPERATOR_IMAGE="image-registry.openshift-image-registry.svc:5000/${OPERATOR_NS}/ocp-support-web-operator:latest"
echo "Operator image: $OPERATOR_IMAGE"

# --- Step 3: Prepare and build the bundle image ---
echo ""
echo "=== Step 3: Building OLM bundle image ==="

# Substitute real images into the CSV
BUNDLE_STAGE=$(mktemp -d)
cp -r "$SCRIPT_DIR/bundle/"* "$BUNDLE_STAGE/"

sed -i "s|OPERATOR_IMAGE_PLACEHOLDER|${OPERATOR_IMAGE}|g" \
    "$BUNDLE_STAGE/manifests/ocp-support-web-operator.clusterserviceversion.yaml"
sed -i "s|RELATED_IMAGE_APP_PLACEHOLDER|${APP_IMAGE}|g" \
    "$BUNDLE_STAGE/manifests/ocp-support-web-operator.clusterserviceversion.yaml"

oc apply -f - <<EOF
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: ocp-support-web-operator-bundle
  namespace: $OPERATOR_NS
EOF

oc apply -f - <<EOF
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: ocp-support-web-operator-bundle
  namespace: $OPERATOR_NS
spec:
  output:
    to:
      kind: ImageStreamTag
      name: ocp-support-web-operator-bundle:v${VERSION}
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      memory: 512Mi
EOF

oc start-build ocp-support-web-operator-bundle --from-dir="$BUNDLE_STAGE" -n "$OPERATOR_NS" --follow
rm -rf "$BUNDLE_STAGE"

BUNDLE_IMAGE="image-registry.openshift-image-registry.svc:5000/${OPERATOR_NS}/ocp-support-web-operator-bundle:v${VERSION}"
echo "Bundle image: $BUNDLE_IMAGE"

# --- Step 4: Build the catalog image ---
echo ""
echo "=== Step 4: Building OLM catalog image ==="

CATALOG_STAGE=$(mktemp -d)
mkdir -p "$CATALOG_STAGE/configs/ocp-support-web-operator"

cat > "$CATALOG_STAGE/configs/ocp-support-web-operator/catalog.yaml" <<CATEOF
---
schema: olm.package
name: ocp-support-web-operator
defaultChannel: alpha
---
schema: olm.channel
name: alpha
package: ocp-support-web-operator
entries:
  - name: ocp-support-web-operator.v${VERSION}
---
schema: olm.bundle
name: ocp-support-web-operator.v${VERSION}
package: ocp-support-web-operator
image: ${BUNDLE_IMAGE}
properties:
  - type: olm.package
    value:
      packageName: ocp-support-web-operator
      version: ${VERSION}
CATEOF

cat > "$CATALOG_STAGE/Containerfile" <<'CATDOCKERFILE'
FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:latest

COPY configs /configs

RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

EXPOSE 50051

ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]
CATDOCKERFILE

oc apply -f - <<EOF
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: ocp-support-web-operator-catalog
  namespace: $OPERATOR_NS
EOF

oc apply -f - <<EOF
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: ocp-support-web-operator-catalog
  namespace: $OPERATOR_NS
spec:
  output:
    to:
      kind: ImageStreamTag
      name: ocp-support-web-operator-catalog:latest
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      memory: 1Gi
EOF

oc start-build ocp-support-web-operator-catalog --from-dir="$CATALOG_STAGE" -n "$OPERATOR_NS" --follow
rm -rf "$CATALOG_STAGE"

CATALOG_IMAGE="image-registry.openshift-image-registry.svc:5000/${OPERATOR_NS}/ocp-support-web-operator-catalog:latest"
echo "Catalog image: $CATALOG_IMAGE"

# --- Step 5: Create CatalogSource ---
echo ""
echo "=== Step 5: Creating CatalogSource ==="
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ocp-support-web-operator
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: ${CATALOG_IMAGE}
  displayName: OCP Support Web Operator
  publisher: Community
  updateStrategy:
    registryPoll:
      interval: 10m
EOF

echo "--- Waiting for CatalogSource to be ready..."
for i in {1..30}; do
    STATE=$(oc get catalogsource ocp-support-web-operator -n openshift-marketplace -o jsonpath='{.status.connectionState.lastObservedState}' 2>/dev/null || echo "")
    if [[ "$STATE" == "READY" ]]; then
        echo "CatalogSource is READY"
        break
    fi
    echo "  Waiting... ($STATE)"
    sleep 5
done

# --- Step 6: Create OperatorGroup and Subscription ---
echo ""
echo "=== Step 6: Creating Subscription ==="

oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: ocp-support-web-operator
  namespace: $OPERATOR_NS
spec: {}
EOF

oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ocp-support-web-operator
  namespace: $OPERATOR_NS
spec:
  channel: alpha
  name: ocp-support-web-operator
  source: ocp-support-web-operator
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
EOF

echo "--- Waiting for operator to install via OLM..."
for i in {1..60}; do
    CSV_PHASE=$(oc get csv "ocp-support-web-operator.v${VERSION}" -n "$OPERATOR_NS" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    if [[ "$CSV_PHASE" == "Succeeded" ]]; then
        echo "CSV phase: Succeeded"
        break
    fi
    echo "  Waiting... (CSV phase: ${CSV_PHASE:-pending})"
    sleep 5
done

echo ""
echo "=== OLM Deployment complete ==="
echo ""
echo "The operator should now appear under Installed Operators in the"
echo "OpenShift console (namespace: $OPERATOR_NS)."
echo ""
echo "To deploy the application, create an OCPSupportWeb CR:"
echo ""
echo "  oc apply -f config/samples/support_v1alpha1_ocpsupportweb.yaml -n $APP_NS"
echo ""
