DOCKER_REGISTRY ?= ghcr.io
BASE_IMAGE_REGISTRY ?= cgr.dev

DOCKER_REPO ?= kagent-dev/kagent

HELM_REPO ?= oci://ghcr.io/kagent-dev
HELM_ACTION=upgrade --install

KIND_CLUSTER_NAME ?= kagent
KIND_IMAGE_VERSION ?= 1.34.0
KIND_CREATE_CMD ?= "kind create cluster --name $(KIND_CLUSTER_NAME) --image kindest/node:v$(KIND_IMAGE_VERSION) --config ./scripts/kind/kind-config.yaml"

BUILD_DATE := $(shell date -u '+%Y-%m-%d')
GIT_COMMIT := $(shell git rev-parse --short HEAD || echo "unknown")
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/-dirty//' | grep v || echo "v0.0.0-$(GIT_COMMIT)")

# Version information for the build
LDFLAGS := -X github.com/kagent-dev/tools/internal/version.Version=$(VERSION) -X github.com/kagent-dev/tools/internal/version.GitCommit=$(GIT_COMMIT) -X github.com/kagent-dev/tools/internal/version.BuildDate=$(BUILD_DATE)

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
PATH := $(HOME)/local/bin:/opt/homebrew/bin/:$(LOCALBIN):$(PATH)
HELM_DIST_FOLDER ?= $(shell pwd)/dist

.PHONY: clean
clean:
	rm -rf ./*.out ./coverage.out ./coverage.html ./*.test
	rm -rf ./bin/kagent-tools-*
	rm -rf $(HOME)/.local/bin/kagent-tools-*

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run --build-tags=test

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --build-tags=test --fix

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
test: build lint ## Run all tests with build, lint, and coverage
	go test -tags=test -v -cover -coverprofile=coverage.out ./pkg/... || true
	@echo ""
	@echo "Coverage Report:"
	@./scripts/check-coverage.sh coverage.out || true
	@echo ""

.PHONY: test-coverage
test-coverage: ## Run tests with coverage output
	go test -tags=test -v -cover -coverprofile=coverage.out ./pkg/... ./internal/...

.PHONY: coverage-report
coverage-report: test-coverage ## Generate HTML coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report generated: coverage.html"

.PHONY: test-only
test-only: ## Run tests only (without build/lint for faster iteration)
	go test -tags=test -v -cover ./pkg/... ./internal/...

.PHONY: e2e
e2e: test retag
	pkill -f "kagent-tools.*--http-port" 2>/dev/null || true
	go test -v -tags=test -cover ./test/e2e/ -timeout 5m

bin/kagent-tools-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-linux-amd64 ./cmd/server

bin/kagent-tools-linux-amd64.sha256: bin/kagent-tools-linux-amd64
	sha256sum bin/kagent-tools-linux-amd64 > bin/kagent-tools-linux-amd64.sha256

bin/kagent-tools-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-linux-arm64 ./cmd/server

bin/kagent-tools-linux-arm64.sha256: bin/kagent-tools-linux-arm64
	sha256sum bin/kagent-tools-linux-arm64 > bin/kagent-tools-linux-arm64.sha256

bin/kagent-tools-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-darwin-amd64 ./cmd/server

bin/kagent-tools-darwin-amd64.sha256: bin/kagent-tools-darwin-amd64
	sha256sum bin/kagent-tools-darwin-amd64 > bin/kagent-tools-darwin-amd64.sha256

bin/kagent-tools-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-darwin-arm64 ./cmd/server

bin/kagent-tools-darwin-arm64.sha256: bin/kagent-tools-darwin-arm64
	sha256sum bin/kagent-tools-darwin-arm64 > bin/kagent-tools-darwin-arm64.sha256

bin/kagent-tools-windows-amd64.exe:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/kagent-tools-windows-amd64.exe ./cmd/server

bin/kagent-tools-windows-amd64.exe.sha256: bin/kagent-tools-windows-amd64.exe
	sha256sum bin/kagent-tools-windows-amd64.exe > bin/kagent-tools-windows-amd64.exe.sha256

.PHONY: build
build: $(LOCALBIN) clean bin/kagent-tools-linux-amd64.sha256 bin/kagent-tools-linux-arm64.sha256 bin/kagent-tools-darwin-amd64.sha256 bin/kagent-tools-darwin-arm64.sha256 bin/kagent-tools-windows-amd64.exe.sha256
build:
	@echo "Build complete. Binaries are available in the bin/ directory."
	ls -lt bin/kagent-tools-*

.PHONY: run
run: docker-build
	@echo "Running tool server on http://localhost:8084/mcp ..."
	@echo "Use:  npx @modelcontextprotocol/inspector to connect to the tool server"
	@docker run --rm --net=host -p 8084:8084 -e OPENAI_API_KEY=$(OPENAI_API_KEY) -v $(HOME)/.kube:/home/nonroot/.kube -e KAGENT_TOOLS_PORT=8084 $(TOOLS_IMG) -- --kubeconfig /root/.kube/config

.PHONY: retag
retag: docker-build helm-version
	@echo "Check Kind cluster $(KIND_CLUSTER_NAME) exists"
	kind get clusters | grep -q $(KIND_CLUSTER_NAME) || bash -c $(KIND_CREATE_CMD)
	bash ./scripts/kind/setup-kind.sh
	@echo "Retagging tools image to $(RETAGGED_TOOLS_IMG)"
	docker tag $(TOOLS_IMG) $(RETAGGED_TOOLS_IMG)
	kind load docker-image --name $(KIND_CLUSTER_NAME) $(RETAGGED_TOOLS_IMG)

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
TOOLS_ISTIO_VERSION ?= 1.27.3
TOOLS_ARGO_ROLLOUTS_VERSION ?= 1.8.3
TOOLS_KUBECTL_VERSION ?= 1.34.1
TOOLS_HELM_VERSION ?= 3.19.0
TOOLS_CILIUM_VERSION ?= 0.18.8
TOOLS_ARGO_CLI_VERSION ?= 3.1.9

# build args
TOOLS_IMAGE_BUILD_ARGS =  --build-arg VERSION=$(VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg LDFLAGS="$(LDFLAGS)"
TOOLS_IMAGE_BUILD_ARGS += --build-arg LOCALARCH=$(LOCALARCH)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_ISTIO_VERSION=$(TOOLS_ISTIO_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_ARGO_CLI_VERSION=$(TOOLS_ARGO_CLI_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_ARGO_ROLLOUTS_VERSION=$(TOOLS_ARGO_ROLLOUTS_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_KUBECTL_VERSION=$(TOOLS_KUBECTL_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_HELM_VERSION=$(TOOLS_HELM_VERSION)
TOOLS_IMAGE_BUILD_ARGS += --build-arg TOOLS_CILIUM_VERSION=$(TOOLS_CILIUM_VERSION)

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

.PHONY: helm-version
helm-version:
	VERSION=$(VERSION) envsubst < helm/kagent-tools/Chart-template.yaml > helm/kagent-tools/Chart.yaml
	mkdir -p $(HELM_DIST_FOLDER)
	helm package -d $(HELM_DIST_FOLDER) helm/kagent-tools

.PHONY: helm-uninstall
helm-uninstall:
	helm uninstall kagent --namespace kagent --kube-context kind-$(KIND_CLUSTER_NAME) --wait

.PHONY: helm-install
helm-install: helm-version retag
	export ARGOCD_PASSWORD=$$(kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath="{.data.password}" | base64 -d) || true
	helm template kagent-tools ./helm/kagent-tools --namespace kagent | kubectl --namespace kagent delete -f - || :
	helm $(HELM_ACTION) kagent-tools ./helm/kagent-tools \
		--namespace kagent \
		--create-namespace \
		--history-max 2    \
		--timeout 5m       \
		-f ./scripts/kind/test-values.yaml \
		--set tools.image.registry=$(RETAGGED_DOCKER_REGISTRY) \
		--set argocd.apiToken=$$ARGOCD_PASSWORD \
		--wait

.PHONY: helm-publish
helm-publish: helm-version
	helm push $(HELM_DIST_FOLDER)/kagent-tools-$(VERSION).tgz $(HELM_REPO)/tools/helm

.PHONY: create-kind-cluster
create-kind-cluster:
	docker pull kindest/node:v$(KIND_IMAGE_VERSION) || true
	bash -c $(KIND_CREATE_CMD)

.PHONY: delete-kind-cluster
delete-kind-cluster:
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-update-kagent
kind-update-kagent:  retag
	kubectl patch --namespace kagent deployment/kagent --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/3/image", "value": "$(RETAGGED_TOOLS_IMG)"}]'

.PHONY: otel-local
otel-local:
	docker rm -f jaeger-desktop || true
	docker run -d --name jaeger-desktop --restart=always -p 16686:16686 -p 4317:4317 -p 4318:4318 jaegertracing/jaeger:2.7.0
	open http://localhost:16686/

.PHONY: install/argocd
install/argocd:
	kubectl get namespace argocd || kubectl create namespace argocd || true
	kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	@echo "Waiting for ArgoCD deployments to be created..."
	kubectl wait --for=condition=available --timeout=5m deployment/argocd-applicationset-controller -n argocd
	@echo "ArgoCD is ready!"
	@ARGOCD_PASSWORD=$$(kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath="{.data.password}" | base64 -d); \
	echo "ArgoCD Admin Password: $$ARGOCD_PASSWORD"

.PHONY: install/istio
install/istio:
	istioctl install --set profile=demo -y

.PHONY: install/kagent
install/kagent:
	@echo "Installing kagent in namespace 'kagent' ..."
	which kagent || curl https://raw.githubusercontent.com/kagent-dev/kagent/refs/heads/main/scripts/get-kagent | bash
	kagent install   -n kagent

.PHONY: install/tools
install/tools: clean
	mkdir -p $(HOME)/.local/bin
	echo "Building go-mcp-client..."
	go build -ldflags "$(LDFLAGS)" -o $(LOCALBIN)/go-mcp-client ./cmd/client
	go build -ldflags "$(LDFLAGS)" -o $(HOME)/.local/bin/go-mcp-client ./cmd/client
	echo "Building kagent-tools..."
	go build -ldflags "$(LDFLAGS)" -o $(LOCALBIN)/kagent-tools  ./cmd/server
	go build -ldflags "$(LDFLAGS)" -o $(HOME)/.local/bin/kagent-tools ./cmd/server
	$(HOME)/.local/bin/kagent-tools --version

.PHONY: docker-build install
install: install/tools install/kagent install/istio install/argocd helm-install

.PHONY: dashboard/kagent
dashboard/kagent:
	kagent dashboard -n kagent

.PHONY: run-agentgateway
run-agentgateway: install/tools
	open http://localhost:15000/ui
	cd scripts \
	&& agentgateway -f agentgateway-config-tools.yaml

.PHONY: report/image-cve
report/image-cve: docker-build govulncheck
	echo "Running CVE scan :: CVE -> CSV ... reports/$(SEMVER)/"
	grype docker:$(TOOLS_IMG) -o template -t reports/cve-report.tmpl --file reports/$(SEMVER)/tools-cve.csv

## Tool Binaries
## Location to install dependencies t

# check-release-version checks if a tool version matches the latest GitHub release
# $1 - variable name (e.g., TOOLS_ISTIO_VERSION)
# $2 - current version value
# $3 - GitHub repo (e.g., istio/istio)
define check-release-version
@LATEST=$$(gh release list --repo $(3) --json tagName,isLatest | jq -r '.[] | select(.isLatest==true) | .tagName'); \
if [ "$(2)" = "$${LATEST#v}" ]; then \
	echo "✅ $(1)=$(2) == $$LATEST"; \
else \
	echo "❌ $(1)=$(2) != $$LATEST"; \
fi
endef

.PHONY: check-releases
check-releases:
	@echo "Checking tool versions against latest releases..."
	@echo ""
	$(call check-release-version,TOOLS_ARGO_ROLLOUTS_VERSION,$(TOOLS_ARGO_ROLLOUTS_VERSION),argoproj/argo-rollouts)
	$(call check-release-version,TOOLS_ARGO_CLI_VERSION,$(TOOLS_ARGO_CLI_VERSION),argoproj/argo-cd)
	$(call check-release-version,TOOLS_CILIUM_VERSION,$(TOOLS_CILIUM_VERSION),cilium/cilium-cli)
	$(call check-release-version,TOOLS_ISTIO_VERSION,$(TOOLS_ISTIO_VERSION),istio/istio)
	$(call check-release-version,TOOLS_HELM_VERSION,$(TOOLS_HELM_VERSION),helm/helm)
	$(call check-release-version,TOOLS_KUBECTL_VERSION,$(TOOLS_KUBECTL_VERSION),kubernetes/kubernetes)

.PHONY: $(LOCALBIN)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.5.0

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

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