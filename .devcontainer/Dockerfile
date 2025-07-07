FROM mcr.microsoft.com/devcontainers/go:1-1.24-bookworm

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

ENTRYPOINT ["zsh"]