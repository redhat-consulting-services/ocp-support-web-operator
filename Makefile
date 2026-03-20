VERSION ?= 1.9.0
OPERATOR_IMG ?= quay.io/redhat-consulting-services/ocp-support-web-operator:v$(VERSION)
BUNDLE_IMG ?= quay.io/redhat-consulting-services/ocp-support-web-operator-bundle:v$(VERSION)
CATALOG_IMG ?= quay.io/redhat-consulting-services/rh-consulting-catalog:v$(VERSION)
APP_IMG ?= quay.io/redhat-consulting-services/ocp-support-web:v$(VERSION)

OPERATOR_REPO ?= quay.io/redhat-consulting-services/ocp-support-web-operator
APP_REPO ?= quay.io/redhat-consulting-services/ocp-support-web

BUILDER_IMAGE ?= registry.redhat.io/ubi9/go-toolset:latest
RUNTIME_IMAGE ?= registry.redhat.io/ubi9/ubi-micro:latest

.PHONY: build
build:
	CGO_ENABLED=0 go build -o bin/manager ./cmd/main.go

.PHONY: test
test:
	go test ./... -v

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: image-build
image-build:
	podman build \
		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
		--build-arg RUNTIME_IMAGE=$(RUNTIME_IMAGE) \
		-t $(OPERATOR_IMG) \
		-f Containerfile .

.PHONY: image-push
image-push:
	podman push $(OPERATOR_IMG)

.PHONY: bundle-pin-digests
bundle-pin-digests:
	@echo "Resolving digests for v$(VERSION)..."
	$(eval OPERATOR_DIGEST := $(shell skopeo inspect --format '{{.Digest}}' docker://$(OPERATOR_IMG)))
	$(eval APP_DIGEST := $(shell skopeo inspect --format '{{.Digest}}' docker://$(APP_IMG)))
	@echo "  Operator: $(OPERATOR_REPO)@$(OPERATOR_DIGEST)"
	@echo "  App:      $(APP_REPO)@$(APP_DIGEST)"
	sed -i 's|image: $(OPERATOR_REPO)[^ ]*|image: $(OPERATOR_REPO)@$(OPERATOR_DIGEST)|g' \
		bundle/manifests/ocp-support-web-operator.clusterserviceversion.yaml
	sed -i 's|containerImage: $(OPERATOR_REPO)[^ ]*|containerImage: $(OPERATOR_REPO)@$(OPERATOR_DIGEST)|g' \
		bundle/manifests/ocp-support-web-operator.clusterserviceversion.yaml
	sed -i '/RELATED_IMAGE_APP/{n;s|value: $(APP_REPO)[^ ]*|value: $(APP_REPO)@$(APP_DIGEST)|;}' \
		bundle/manifests/ocp-support-web-operator.clusterserviceversion.yaml
	sed -i '/name: app/{n;s|image: $(APP_REPO)[^ ]*|image: $(APP_REPO)@$(APP_DIGEST)|;}' \
		bundle/manifests/ocp-support-web-operator.clusterserviceversion.yaml
	@echo "Digests pinned in CSV."

.PHONY: bundle-build
bundle-build:
	podman build -t $(BUNDLE_IMG) -f bundle/Containerfile bundle/

.PHONY: bundle-push
bundle-push:
	podman push $(BUNDLE_IMG)

.PHONY: catalog-render
catalog-render:
	opm render $(BUNDLE_IMG) -o yaml > catalog/configs/operator.yaml

.PHONY: catalog-build
catalog-build:
	podman build -t $(CATALOG_IMG) -f catalog/Containerfile catalog/

.PHONY: catalog-push
catalog-push:
	podman push $(CATALOG_IMG)

.PHONY: install
install:
	oc apply -f config/crd/bases/

.PHONY: uninstall
uninstall:
	oc delete -f config/crd/bases/

.PHONY: deploy
deploy: install
	@echo "--- Creating namespace..."
	oc create namespace ocp-support-web-operator --dry-run=client -o yaml | oc apply -f -
	@echo "--- Creating RBAC..."
	cd config/rbac && \
		sed 's/namespace: system/namespace: ocp-support-web-operator/g' service_account.yaml | oc apply -f - && \
		oc apply -f role.yaml && \
		sed 's/namespace: system/namespace: ocp-support-web-operator/g' role_binding.yaml | oc apply -f -
	@echo "--- Creating manager deployment..."
	cd config/manager && \
		sed -e 's|OPERATOR_IMAGE_PLACEHOLDER|$(OPERATOR_IMG)|g' \
		    -e 's|RELATED_IMAGE_APP_PLACEHOLDER|$(APP_IMG)|g' \
		    -e 's/namespace: system/namespace: ocp-support-web-operator/g' \
		    manager.yaml | oc apply -f -
	@echo "--- Operator deployed to namespace ocp-support-web-operator"

.PHONY: undeploy
undeploy:
	oc delete deployment ocp-support-web-operator -n ocp-support-web-operator --ignore-not-found
	oc delete clusterrolebinding ocp-support-web-operator-rolebinding --ignore-not-found
	oc delete clusterrole ocp-support-web-operator-role --ignore-not-found
	oc delete serviceaccount ocp-support-web-operator -n ocp-support-web-operator --ignore-not-found
	oc delete namespace ocp-support-web-operator --ignore-not-found

.PHONY: run
run: build
	RELATED_IMAGE_APP=$(APP_IMG) ./bin/manager

.PHONY: help
help:
	@echo "Usage:"
	@echo "  make build              Build the operator binary"
	@echo "  make image-build        Build the operator container image"
	@echo "  make image-push         Push the operator container image"
	@echo "  make bundle-pin-digests  Pin sha256 digests in the CSV (run after image-push)"
	@echo "  make bundle-build       Build the OLM bundle image"
	@echo "  make bundle-push        Push the OLM bundle image"
	@echo "  make catalog-render     Render bundle into FBC catalog"
	@echo "  make catalog-build      Build the FBC catalog image"
	@echo "  make catalog-push       Push the FBC catalog image"
	@echo "  make install            Install CRD into cluster"
	@echo "  make deploy             Deploy operator to cluster (dev, without OLM)"
	@echo "  make undeploy           Remove operator from cluster"
	@echo "  make run                Run operator locally for development"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION           Operator version (default: $(VERSION))"
	@echo "  OPERATOR_IMG      Operator image (default: $(OPERATOR_IMG))"
	@echo "  APP_IMG           Application image (default: $(APP_IMG))"
	@echo "  BUILDER_IMAGE     Go builder image for disconnected builds"
	@echo "  RUNTIME_IMAGE     Runtime base image for disconnected builds"
