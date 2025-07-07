ARG TOOLS_GO_VERSION
FROM mcr.microsoft.com/devcontainers/go:1-${TOOLS_GO_VERSION}-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates           \
    curl                      \
    git                       \
    gnupg                     \
    gnupg2                    \
    htop                      \
    jq                        \
    less                      \
    lsof                      \
    net-tools                 \
    openssh-client            \
    unzip                     \
    vim                       \
    zsh

ARG TARGETARCH
ARG TOOLS_HELM_VERSION
ARG TOOLS_ISTIO_VERSION
ARG TOOLS_ARGO_ROLLOUTS_VERSION
ARG TOOLS_KUBECTL_VERSION
ARG TOOLS_CILIUM_VERSION

WORKDIR /downloads

RUN curl -LO "https://dl.k8s.io/release/v$TOOLS_KUBECTL_VERSION/bin/linux/$TARGETARCH/kubectl" \
    && mv kubectl /usr/bin/kubectl \
    && chmod +x /usr/bin/kubectl   \
    && /usr/bin/kubectl version --client

RUN curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$TOOLS_ISTIO_VERSION TARGET_ARCH=$TARGETARCH sh - \
    && mv istio-*/bin/istioctl /usr/bin/                                                           \
    && rm -rf istio-* \
    && /usr/bin/istioctl --help

# Install Helm
RUN curl -Lo helm.tar.gz https://get.helm.sh/helm-v${TOOLS_HELM_VERSION}-linux-${TARGETARCH}.tar.gz  \
    && tar -xvf helm.tar.gz                                                                             \
    && mv linux-${TARGETARCH}/helm /usr/bin/helm                                           \
    && chmod +x /usr/bin/helm \
    && rm -rf helm.tar.gz linux-${TARGETARCH} \
    && /usr/bin/helm version

# Install kubectl-argo-rollouts
RUN curl -Lo /usr/bin/kubectl-argo-rollouts https://github.com/argoproj/argo-rollouts/releases/download/v${TOOLS_ARGO_ROLLOUTS_VERSION}/kubectl-argo-rollouts-linux-${TARGETARCH} \
    && chmod +x /usr/bin/kubectl-argo-rollouts \
    && /usr/bin/kubectl-argo-rollouts version

# Install Cilium CLI
RUN curl -Lo cilium.tar.gz https://github.com/cilium/cilium-cli/releases/download/v${TOOLS_CILIUM_VERSION}/cilium-linux-${TARGETARCH}.tar.gz \
    && tar -xvf cilium.tar.gz \
    && mv cilium /usr/bin/cilium \
    && chmod +x /usr/bin/cilium \
    && rm -rf cilium.tar.gz \
    && /usr/bin/cilium version

WORKDIR /tools

ENTRYPOINT ["zsh"]