# Build stage - builds both manager and agent binaries
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG BINARY=manager

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source
# This layer will be cached unless go.mod/go.sum change
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build the specified binary (manager or agent)
# the GOARCH has not a default value to allow the binary be built according to the host
# For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -ldflags="-w -s" -o ${BINARY} cmd/${BINARY}/main.go

# Runtime stage - minimal distroless image
FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Copy the binary from builder stage
# Always copy as 'manager' to have a consistent entrypoint
ARG BINARY=manager
COPY --from=builder /workspace/${BINARY} /manager

# Run as non-root user 
# (agent will need CAP_NET_BIND_SERVICE added by Kubernetes securityContext)
USER 65532:65532

# Entrypoint is always /manager regardless of which binary was built
# This keeps the Dockerfile simple and works with distroless
ENTRYPOINT ["/manager"]
