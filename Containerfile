# Build stage
ARG BUILDER_IMAGE=registry.redhat.io/ubi9/go-toolset:latest
ARG RUNTIME_IMAGE=registry.redhat.io/ubi9/ubi-micro:latest

FROM ${BUILDER_IMAGE} AS builder

COPY . .
RUN CGO_ENABLED=0 go build -o manager ./cmd/main.go

# Runtime stage
FROM ${RUNTIME_IMAGE}

COPY --from=builder /opt/app-root/src/manager /manager

USER 65532:65532

ENTRYPOINT ["/manager"]
