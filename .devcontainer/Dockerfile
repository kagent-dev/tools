### STAGE 1: download-tools-cli
ARG BASE_IMAGE_REGISTRY=cgr.dev
FROM $BASE_IMAGE_REGISTRY/chainguard/wolfi-base:latest AS tools

RUN --mount=type=cache,target=/var/cache/apk,rw \
    echo "Installing on $BUILDPLATFORM" \
    && apk update \
    && apk add curl zsh go \
    && update-ca-certificates

ENTRYPOINT ["zsh"]