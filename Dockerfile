### STAGE 1: download-tools-cli
ARG BASE_IMAGE_REGISTRY=cgr.dev
FROM $BASE_IMAGE_REGISTRY/chainguard/wolfi-base:latest AS tools

ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

RUN apk update && apk add  \
    curl openssl bash git ca-certificates \
    && rm -rf /var/cache/apk/*

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

# Install kubectl-argo-rollouts
ARG TOOLS_ARGO_ROLLOUTS_VERSION
RUN curl -Lo /downloads/kubectl-argo-rollouts https://github.com/argoproj/argo-rollouts/releases/download/v${TOOLS_ARGO_ROLLOUTS_VERSION}/kubectl-argo-rollouts-linux-${TARGETARCH} \
    && chmod +x /downloads/kubectl-argo-rollouts \
    && /downloads/kubectl-argo-rollouts version

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
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -trimpath -ldflags "-s -w $LDFLAGS" -o tool-server ./cmd

### STAGE 3: assemble kagent Agent Plugin roots
# Only the small files and links are assembled here, on their own base so CLI
# downloads do not invalidate this stage. Binaries are copied in the final stage.
ARG BASE_IMAGE_REGISTRY=cgr.dev
FROM $BASE_IMAGE_REGISTRY/chainguard/wolfi-base:latest AS plugin
ARG VERSION=dev
COPY plugin /src/plugin
RUN set -eu; \
    root=/out/opt/kagent-tools; skills_root=/out/opt/kagent-tools-skills; \
    plugin_version="${VERSION#v}"; \
    mkdir -p "$root/bin" "$root/config" "$skills_root" /out/usr/bin; \
    sed "s/__VERSION__/${plugin_version}/" /src/plugin/plugin.json.tmpl > "$root/plugin.json"; \
    sed "s/__VERSION__/${plugin_version}/" /src/plugin/skills-plugin.json.tmpl > "$skills_root/plugin.json"; \
    install -m 0644 /src/plugin/mcp.json "$root/mcp.json"; \
    install -m 0644 /src/plugin/config/kubeconfig "$root/config/kubeconfig"; \
    cp -R /src/plugin/skills "$root/skills"; \
    cp -R /src/plugin/skills "$skills_root/skills"; \
    find /out/opt -type d -exec chmod 0755 {} +; \
    find /out/opt -type f ! -path '*/bin/*' -exec chmod 0644 {} +; \
    ln -s /opt/kagent-tools/bin/kagent-tools /out/tool-server; \
    for binary in kubectl helm istioctl kubectl-argo-rollouts cilium; do \
      ln -s "/opt/kagent-tools/bin/$binary" "/out/usr/bin/$binary"; \
    done; \
    chown -R 65532:65532 /out/opt

# Skills-only image (tools-skills): the skills root alone, so an Agent that
# mounts it does not pull the CLIs. The last stage stays the default target.
FROM scratch AS skills
ARG VERSION
COPY --from=plugin /out/opt/kagent-tools-skills /opt/kagent-tools-skills
LABEL org.opencontainers.image.source=https://github.com/kagent-dev/tools
LABEL org.opencontainers.image.description="kagent Agent Plugin with the kagent-tools skills only"
LABEL org.opencontainers.image.version="$VERSION"

FROM gcr.io/distroless/static:nonroot

WORKDIR /
USER 65532:65532
ENV PATH=/opt/kagent-tools/bin:$PATH:/bin

# /opt/kagent-tools        -> kagent Agent Plugin root (CLIs + stdio MCP server + skills)
# /opt/kagent-tools-skills -> skills-only Agent Plugin root
# /tool-server, /bin/*     -> backward-compatible symlinks (distroless /bin -> usr/bin)
# Three content layers, ordered from least to most frequently changed:
#   1. all CLIs (~500 MiB, change only on a version bump)
#   2. plugin manifests, skills and links (a few KiB)
#   3. the Go server (changes on every commit, ~70 MiB)
# so a Go or skill change re-ships only the small layers.
COPY --from=tools --chown=65532:65532 --chmod=0755 \
    /downloads/kubectl /downloads/helm /downloads/istioctl /downloads/kubectl-argo-rollouts /downloads/cilium \
    /opt/kagent-tools/bin/
COPY --from=plugin /out/ /
COPY --from=builder --chown=65532:65532 --chmod=0755 /workspace/tool-server /opt/kagent-tools/bin/kagent-tools

ARG VERSION

LABEL org.opencontainers.image.source=https://github.com/kagent-dev/tools
LABEL org.opencontainers.image.description="Kagent MCP tools server and kagent Agent Plugin (skills bundle)"
LABEL org.opencontainers.image.authors="Kagent Creators 🤖"
LABEL org.opencontainers.image.version="$VERSION"

ENTRYPOINT ["/opt/kagent-tools/bin/kagent-tools"]
