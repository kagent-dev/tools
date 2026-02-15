# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

kAgent Tools is a Go-based MCP (Model Context Protocol) server providing 118+ tools for Kubernetes, cloud-native, and observability operations. Originally migrated from Python, it offers comprehensive functionality for k8s, Helm, Istio, Argo, Cilium, Prometheus, and more.

## Architecture

### Core Structure

```
cmd/main.go          - Entry point, MCP server initialization, tool registration
internal/            - Shared utilities (logger, cache, telemetry, metrics, security)
pkg/                 - Tool providers (k8s, helm, istio, argo, cilium, prometheus, utils)
helm/kagent-tools/   - Kubernetes deployment via Helm
test/e2e/            - End-to-end tests using Ginkgo
```

### Tool Provider Pattern

Each `pkg/*/` directory follows this pattern:

1. **Tool struct** - Holds provider-specific state (e.g., kubeconfig, LLM model)
2. **Handler functions** - Named `handle<ToolName>`, parse MCP request parameters, execute operations
3. **RegisterTools function** - Signature: `RegisterTools(s *server.MCPServer, ...)` - Adds all tools from this provider to the MCP server

Example from `pkg/k8s/k8s.go`:
```go
func RegisterTools(s *server.MCPServer, llm llms.Model, kubeconfig string, readOnly bool) {
    k8sTool := NewK8sToolWithConfig(kubeconfig, llm)

    s.AddTool(mcp.Tool{...}, k8sTool.handleKubectlGetEnhanced)
    s.AddTool(mcp.Tool{...}, k8sTool.handleKubectlDescribe)
    // ...
}
```

**Key convention**: All tool providers export a `RegisterTools` function that gets called from `cmd/main.go:registerMCP()`.

### MCP Server Lifecycle

1. **Initialization** (`cmd/main.go:run()`):
   - Parse CLI flags (`--port`, `--metrics-port`, `--tools`, `--read-only`, `--kubeconfig`)
   - Create MCP server with `server.NewMCPServer()`
   - Initialize Prometheus metrics server

2. **Tool Registration** (`registerMCP()`):
   - Maps provider names to registration functions
   - Uses `ListTools()` diff technique to track which tools belong to which provider
   - Returns `map[string]string` of tool→provider for metrics

3. **Handler Wrapping** (`wrapToolHandlersWithMetrics()`):
   - Applies middleware pattern to ALL tool handlers
   - Increments Prometheus counters (`kagent_tools_mcp_invocations_total`, `kagent_tools_mcp_invocations_failure_total`)
   - Uses `SetTools()` to replace handlers - **zero changes to pkg/ files required**

4. **Server Start**:
   - HTTP mode: SSE transport on `--port`
   - STDIO mode: Direct stdin/stdout communication
   - Metrics server runs concurrently on `--metrics-port`
   - Both servers gracefully shutdown on SIGTERM/SIGINT

### Observability

**Prometheus Metrics** (`internal/metrics/`):
- `kagent_tools_mcp_server_info` - Server metadata (version, commit, build date)
- `kagent_tools_mcp_registered_tools` - Gauge per tool (tool_name, tool_provider)
- `kagent_tools_mcp_invocations_total` - Counter of all invocations
- `kagent_tools_mcp_invocations_failure_total` - Counter of failures

**ServiceMonitor** for Prometheus Operator is in `helm/kagent-tools/templates/servicemonitor.yaml`.

### Read-Only Mode

When `--read-only` flag is set, write operations are disabled. Tool providers check `readOnly` parameter in `RegisterTools()` and skip registering destructive tools (apply, delete, scale, etc.).

## Development Commands

### Build & Run

```bash
# Build binary
make build
# or manually:
go build -o kagent-tools ./cmd/main.go

# Run locally
./kagent-tools --port 8084 --metrics-port 9090

# Run with specific tools only
./kagent-tools --tools k8s,helm,utils

# Run in read-only mode
./kagent-tools --read-only

# Run with custom kubeconfig
./kagent-tools --kubeconfig ~/.kube/my-cluster-config
```

### Testing

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/metrics/
go test ./pkg/k8s/

# Run tests with verbose output
go test -v ./...

# Run E2E tests (requires kind cluster)
cd test/e2e && ginkgo -v
```

### Docker & Kubernetes

```bash
# Build Docker image
make docker-build

# Build with custom tag
make docker-build TOOLS_IMAGE_TAG=my-test-tag

# Generate Helm Chart.yaml (required before helm commands)
make helm-version

# Load image into kind cluster (adjust cluster name)
kind load docker-image ghcr.io/kagent-dev/kagent/tools:TAG --name CLUSTER_NAME

# Install via Helm
helm upgrade -i kagent-tools ./helm/kagent-tools \
  --namespace kagent \
  --create-namespace \
  --set tools.image.tag=TAG

# Render Helm templates (verify before install)
helm template kagent-tools ./helm/kagent-tools --namespace kagent
```

### Code Quality

```bash
# Format code
make fmt
# or:
go fmt ./...

# Run linter (if configured)
golangci-lint run

# Security scan
make scan
```

## Key Implementation Patterns

### MCP Tool Parameters

Use `mcp.Parse*` functions to extract typed parameters from `CallToolRequest`:

```go
func handleMyTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    requiredParam := mcp.ParseString(request, "param_name", "")
    optionalParam := mcp.ParseString(request, "optional", "default_value")
    boolParam := mcp.ParseBool(request, "flag", false)

    if requiredParam == "" {
        return mcp.NewToolResultError("param_name is required"), nil
    }

    // ... execute operation ...

    return mcp.NewToolResultText(output), nil
}
```

### Command Execution

Use `internal/commands` package for shell commands:

```go
import "github.com/kagent-dev/tools/internal/commands"

result, err := commands.RunCommand(ctx, "kubectl", "get", "pods")
if err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}
return mcp.NewToolResultText(result), nil
```

### Cache Invalidation

Kubernetes operations use `internal/cache` for performance. Invalidate after write operations:

```go
import "github.com/kagent-dev/tools/internal/cache"

// After kubectl apply/delete/patch/etc:
cache.InvalidateKubernetesCache()
```

### Security Validation

Use `internal/security` for dangerous operation checks:

```go
import "github.com/kagent-dev/tools/internal/security"

if err := security.ValidateKubernetesOperation(args); err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}
```

## Helm Chart Architecture

The Helm chart (`helm/kagent-tools/`) deploys the tools server to Kubernetes:

- **Chart.yaml** - Generated from `Chart-template.yaml` via `make helm-version`
- **values.yaml** - Configuration (image, resources, enabled tools, metrics, ServiceMonitor)
- **templates/deployment.yaml** - Main container with `--port` and `--metrics-port` args
- **templates/service.yaml** - Two services: main (`kagent-tools`) and metrics (`kagent-tools-metrics`)
- **templates/servicemonitor.yaml** - Prometheus Operator integration (conditional via `.Values.tools.metrics.servicemonitor.enabled`)

**Important**: The instance label is typically `kagent`, not `kagent-tools`, due to nameOverride in production values.

## Version Management

Version information is injected at build time via `-ldflags`:

```go
// internal/version/version.go
var (
    Version   = "dev"
    GitCommit = "none"
    BuildDate = "unknown"
)
```

Build with version info:
```bash
# Automatic via Makefile (uses git describe)
make docker-build

# Manual
go build -ldflags "-X github.com/kagent-dev/tools/internal/version.Version=v1.0.0 ..." ./cmd/main.go
```

## Adding a New Tool Provider

1. Create `pkg/newprovider/newprovider.go`
2. Implement tool struct and handlers
3. Export `RegisterTools(s *server.MCPServer, ...)` function
4. Add to `toolProviderMap` in `cmd/main.go:registerMCP()`
5. Add tests in `pkg/newprovider/newprovider_test.go`
6. Update `--tools` flag documentation in README.md

**No metrics code needed** - the handler wrapper automatically instruments all tools.

## Testing Strategy

- **Unit tests**: Each `pkg/*/` has `*_test.go` files
- **E2E tests**: `test/e2e/` uses Ginkgo/Gomega, deploys to kind cluster
- **Manual testing**: Build binary, run locally, invoke tools via MCP client

E2E tests require:
- kind cluster running
- Chart.yaml generated (`make helm-version`)
- Docker image built and loaded into kind

## Git Workflow

**Branch naming**: `feature/description`, `fix/description`, `observability/prometheus`

**Commit signatures**:
```
Signed-off-by: Name <email>
Co-authored-by: Claude <noreply@anthropic.com>
```

**Commit message format** (from git log):
```
feat(scope): short description

Longer explanation of what changed and why.

Signed-off-by: ...
Co-authored-by: ...
```

Common scopes: `metrics`, `cli`, `deps`, `helm`, `k8s`, `prometheus`

## Troubleshooting

**Port conflicts**: Metrics and main server can share the same port (both serve on 8084 by default, `/metrics` endpoint vs MCP endpoints).

**Helm Chart.yaml missing**: Run `make helm-version` to generate from template.

**E2E test failures**: Helm installs fail if Chart.yaml doesn't exist or if custom values override instance names inconsistently.

**ServiceMonitor not discovered**: Ensure `release: prometheus` label matches your Prometheus Operator's `serviceMonitorSelector`.

**Metrics not working**: Verify deployment has `--metrics-port` arg and container exposes the port. Check service selector matches pod labels.
