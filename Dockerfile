### STAGE 1: download-tools-cli
ARG BASE_IMAGE_REGISTRY=cgr.dev
FROM $BASE_IMAGE_REGISTRY/chainguard/wolfi-base:latest AS tools

ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

RUN apk update && apk add --no-cache \
    curl openssl bash git ca-certificates go

ARG TARGETARCH
WORKDIR /downloads

ARG TOOLS_KUBECTL_VERSION
RUN curl -LO "https://dl.k8s.io/release/v$TOOLS_KUBECTL_VERSION/bin/linux/$TARGETARCH/kubectl" \
    && chmod +x kubectl \
    && /downloads/kubectl version --client

# Install Helm
ARG TOOLS_HELM_VERSION
RUN curl -Lo helm.tar.gz https://get.helm.sh/helm-v${TOOLS_HELM_VERSION}-linux-${TARGETARCH}.tar.gz  \
    && tar -xvf helm.tar.gz                                                                             \
    && mv linux-${TARGETARCH}/helm /downloads/helm                                           \
    && chmod +x /downloads/helm \
    && /downloads/helm version

ARG TOOLS_ISTIO_VERSION
RUN curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$TOOLS_ISTIO_VERSION TARGET_ARCH=$TARGETARCH sh - \
    && mv istio-*/bin/istioctl /downloads/ \
    && rm -rf istio-* \
    && /downloads/istioctl --help

# Install kubectl-argo-rollouts from source and fix CVE's
ARG TOOLS_ARGO_ROLLOUTS_VERSION
RUN git clone --depth 1 https://github.com/argoproj/argo-rollouts.git -b v${TOOLS_ARGO_ROLLOUTS_VERSION}
RUN cd argo-rollouts \
    && go mod edit -replace=golang.org/x/net=golang.org/x/net@v0.43.0 \
    && go mod edit -replace=golang.org/x/crypto=golang.org/x/crypto@v0.35.0 \
    && go mod edit -replace=k8s.io/kubernetes=k8s.io/kubernetes@v1.34.1 \
    && go mod edit -replace=k8s.io/apimachinery=k8s.io/apimachinery@v0.34.1 \
    && go mod edit -replace=k8s.io/client-go=k8s.io/client-go@v0.34.1 \
    && go mod edit -replace=k8s.io/api=k8s.io/api@v0.34.1 \
    && go mod edit -replace=k8s.io/apiserver=k8s.io/apiserver@v0.34.1 \
    && go mod edit -replace=k8s.io/apiextensions-apiserver=k8s.io/apiextensions-apiserver@v0.34.1 \
    && go mod edit -replace=k8s.io/cli-runtime=k8s.io/cli-runtime@v0.34.1 \
    && go mod edit -replace=k8s.io/kubectl=k8s.io/kubectl@v0.34.1 \
    && go mod edit -replace=k8s.io/code-generator=k8s.io/code-generator@v0.34.1 \
    && go mod edit -replace=github.com/argoproj/notifications-engine=github.com/argoproj/notifications-engine@v0.5.0 \
    && go mod edit -replace=github.com/expr-lang/expr=github.com/expr-lang/expr@v1.17.0 \
    && sed -i 's/v0.30.14/v0.34.1/g' go.mod \
    && sed -i 's/ValidatePodTemplateSpecForReplicaSet(&template, nil, selector,/ValidatePodTemplateSpecForReplicaSet(\&template, selector,/g' pkg/apis/rollouts/validation/validation.go \
    && go mod tidy \
    && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags "-s -w" -o /downloads/kubectl-argo-rollouts ./cmd/kubectl-argo-rollouts \
    && /downloads/kubectl-argo-rollouts version

# Install Argo CLI
ARG TOOLS_ARGO_CLI_VERSION
RUN curl -sSL -o /downloads/argocd https://github.com/argoproj/argo-cd/releases/download/v${TOOLS_ARGO_CLI_VERSION}/argocd-linux-${TARGETARCH} \
    && chmod +x /downloads/argocd \
    && /downloads/argocd version --client

# Install Cilium CLI
ARG TOOLS_CILIUM_VERSION
RUN curl -Lo cilium.tar.gz https://github.com/cilium/cilium-cli/releases/download/v${TOOLS_CILIUM_VERSION}/cilium-linux-${TARGETARCH}.tar.gz \
    && tar -xvf cilium.tar.gz \
    && mv cilium /downloads/cilium \
    && chmod +x /downloads/cilium \
    && rm -rf cilium.tar.gz \
    && /downloads/cilium version

### STAGE 2: build-tools MCP
ARG BASE_IMAGE_REGISTRY=cgr.dev
ARG BUILDARCH=amd64
FROM --platform=linux/$BUILDARCH $BASE_IMAGE_REGISTRY/chainguard/go:latest AS builder
ARG TARGETPLATFORM
ARG TARGETARCH
ARG BUILDARCH
ARG LDFLAGS

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/root/go/pkg/mod,rw      \
    --mount=type=cache,target=/root/.cache/go-build,rw \
     go mod download

# Copy the go source
COPY cmd cmd
COPY internal internal
COPY pkg pkg

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN --mount=type=cache,target=/root/go/pkg/mod,rw      \
    --mount=type=cache,target=/root/.cache/go-build,rw \
    echo "Building tool-server for $TARGETARCH on $BUILDARCH" && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags "$LDFLAGS" -o tool-server cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
USER 65532:65532
ENV HOME=/home/nonroot
ENV PATH=$PATH:/bin

# Copy the tools
COPY --from=tools --chown=65532:65532 /downloads/kubectl               /bin/kubectl
COPY --from=tools --chown=65532:65532 /downloads/istioctl              /bin/istioctl
COPY --from=tools --chown=65532:65532 /downloads/helm                  /bin/helm
COPY --from=tools --chown=65532:65532 /downloads/cilium                /bin/cilium
COPY --from=tools --chown=65532:65532 /downloads/argocd                /bin/argocd
COPY --from=tools --chown=65532:65532 /downloads/kubectl-argo-rollouts /bin/kubectl-argo-rollouts

# Copy the tool-server binary
COPY --from=builder --chown=65532:65532 /workspace/tool-server           /tool-server

ARG VERSION

LABEL org.opencontainers.image.source=https://github.com/kagent-dev/tools
LABEL org.opencontainers.image.description="Kagent MCP tools server"
LABEL org.opencontainers.image.authors="Kagent Creators 🤖"
LABEL org.opencontainers.image.version="$VERSION"

ENTRYPOINT ["/tool-server"]