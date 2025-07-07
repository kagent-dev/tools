DOCKER_REGISTRY ?= ghcr.io
BASE_IMAGE_REGISTRY ?= cgr.dev
DOCKER_REPO ?= kagent-dev/kagent

BUILD_DATE := $(shell date -u '+%Y-%m-%d')
GIT_COMMIT := $(shell git rev-parse --short HEAD || echo "unknown")
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/-dirty//' | grep v || echo "v0.0.0-$(GIT_COMMIT)")

# Version information for the build
LDFLAGS := -X github.com/kagent-dev/tools/internal/version.Version=$(VERSION) -X github.com/kagent-dev/tools/internal/version.GitCommit=$(GIT_COMMIT) -X github.com/kagent-dev/tools/internal/version.BuildDate=$(BUILD_DATE)

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

.PHONY: govulncheck
govulncheck:
	$(call go-install-tool,bin/govulncheck,golang.org/x/vuln/cmd/govulncheck,latest)
	./bin/govulncheck-latest ./...

.PHONY: tidy
tidy: ## Run go mod tidy to ensure dependencies are up to date.
	go mod tidy

.PHONY: test
test:
	go test -v -cover ./...

bin/kagent-tools-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-linux-amd64 ./cmd

bin/kagent-tools-linux-amd64.sha256: bin/kagent-tools-linux-amd64
	sha256sum bin/kagent-tools-linux-amd64 > bin/kagent-tools-linux-amd64.sha256

bin/kagent-tools-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-linux-arm64 ./cmd

bin/kagent-tools-linux-arm64.sha256: bin/kagent-tools-linux-arm64
	sha256sum bin/kagent-tools-linux-arm64 > bin/kagent-tools-linux-arm64.sha256

bin/kagent-tools-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-darwin-amd64 ./cmd

bin/kagent-tools-darwin-amd64.sha256: bin/kagent-tools-darwin-amd64
	sha256sum bin/kagent-tools-darwin-amd64 > bin/kagent-tools-darwin-amd64.sha256

bin/kagent-tools-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-darwin-arm64 ./cmd

bin/kagent-tools-darwin-arm64.sha256: bin/kagent-tools-darwin-arm64
	sha256sum bin/kagent-tools-darwin-arm64 > bin/kagent-tools-darwin-arm64.sha256

bin/kagent-tools-windows-amd64.exe:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-windows-amd64.exe ./cmd

bin/kagent-tools-windows-amd64.exe.sha256: bin/kagent-tools-windows-amd64.exe
	sha256sum bin/kagent-tools-windows-amd64.exe > bin/kagent-tools-windows-amd64.exe.sha256

.PHONY: build
build: $(LOCALBIN) tidy fmt lint bin/kagent-tools-linux-amd64.sha256 bin/kagent-tools-linux-arm64.sha256 bin/kagent-tools-darwin-amd64.sha256 bin/kagent-tools-darwin-arm64.sha256 bin/kagent-tools-windows-amd64.exe.sha256
build:
	@echo "Build complete. Binaries are available in the bin/ directory."
	ls -lt bin/kagent-tools-*

TOOLS_IMAGE_NAME ?= tools
TOOLS_IMAGE_TAG ?= $(VERSION)
TOOLS_IMG ?= $(DOCKER_REGISTRY)/$(DOCKER_REPO)/$(TOOLS_IMAGE_NAME):$(TOOLS_IMAGE_TAG)

RETAGGED_DOCKER_REGISTRY = cr.kagent.dev
RETAGGED_TOOLS_IMG = $(RETAGGED_DOCKER_REGISTRY)/$(DOCKER_REPO)/$(TOOLS_IMAGE_NAME):$(TOOLS_IMAGE_TAG)

LOCALARCH ?= $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

#buildx settings
BUILDKIT_VERSION = v0.23.0
BUILDX_NO_DEFAULT_ATTESTATIONS=1
BUILDX_BUILDER_NAME ?= kagent-builder-$(BUILDKIT_VERSION)

DOCKER_BUILDER ?= docker buildx
DOCKER_BUILD_ARGS ?= --pull --load --platform linux/$(LOCALARCH) --builder $(BUILDX_BUILDER_NAME)

# tools image build args
TOOLS_ISTIO_VERSION ?= 1.26.2
TOOLS_ARGO_ROLLOUTS_VERSION ?= 1.8.3
TOOLS_KUBECTL_VERSION ?= 1.33.2
TOOLS_HELM_VERSION ?= 3.18.3

# build args
TOOLS_IMAGE_BUILD_ARGS =  --build-arg VERSION=$(VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg LDFLAGS="$(LDFLAGS)"
TOOLS_IMAGE_BUILD_ARGS += --build-arg LOCALARCH=$(LOCALARCH)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_ISTIO_VERSION=$(TOOLS_ISTIO_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_ARGO_ROLLOUTS_VERSION=$(TOOLS_ARGO_ROLLOUTS_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_KUBECTL_VERSION=$(TOOLS_KUBECTL_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_HELM_VERSION=$(TOOLS_HELM_VERSION)

.PHONY: buildx-create
buildx-create:
	docker buildx inspect $(BUILDX_BUILDER_NAME) 2>&1 > /dev/null || \
	docker buildx create --name $(BUILDX_BUILDER_NAME) --platform linux/amd64,linux/arm64 --driver docker-container --use || true

.PHONY: docker-build  # build tools image
docker-build: fmt buildx-create
	$(DOCKER_BUILDER) build $(DOCKER_BUILD_ARGS) $(TOOLS_IMAGE_BUILD_ARGS) -t $(TOOLS_IMG) -f Dockerfile ./

.PHONY: docker-build  # build tools image for amd64 and arm64
docker-build-all: fmt buildx-create
docker-build-all: DOCKER_BUILD_ARGS = --progress=plain --builder $(BUILDX_BUILDER_NAME) --platform linux/amd64,linux/arm64 --output type=tar,dest=/dev/null
docker-build-all:
	$(DOCKER_BUILDER) build $(DOCKER_BUILD_ARGS) $(TOOLS_IMAGE_BUILD_ARGS) -f Dockerfile ./

## Tool Binaries
## Location to install dependencies t

.PHONY: $(LOCALBIN)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.63.4

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef