# AGENTS.md - KAgent Tools Repository Guide for AI Agents

This document provides instructions and context for AI coding agents working in the kagent-dev/tools repository.

---

## Project Overview

**KAgent Tools** is a Go-based MCP (Model Context Protocol) server that provides Kubernetes and cloud-native management tools. It wraps CLI tools (kubectl, helm, istioctl, cilium, etc.) behind a standardized MCP interface for use by AI agents.

**Architecture:**

```
┌────────────────┐
│   MCP Client   │  (AI agent or kagent runtime)
└───────┬────────┘
        │ MCP Protocol (SSE / Streamable HTTP)
        ▼
┌────────────────┐
│   MCP Server   │  cmd/main.go
│  (mcp-go)      │
└───────┬────────┘
        │ Registers tool providers
        ▼
┌───────────────────────────────────────────────┐
│              Tool Providers (pkg/)             │
│  k8s │ helm │ istio │ argo │ cilium │ ...     │
└───────────────────────────────────────────────┘
        │ CommandBuilder (internal/commands/)
        ▼
┌───────────────────────────────────────────────┐
│           CLI Binaries                         │
│  kubectl │ helm │ istioctl │ cilium │ argoctl │
└───────────────────────────────────────────────┘
```

---

## Repository Structure

```
tools/
├── cmd/
│   └── main.go                  # MCP server entry point, tool registration
├── pkg/                         # Public tool provider packages
│   ├── argo/                    # Argo Rollouts tools
│   ├── cilium/                  # Cilium CNI networking tools
│   ├── helm/                    # Helm package management tools
│   ├── istio/                   # Istio service mesh tools
│   ├── k8s/                     # Kubernetes management tools
│   │   └── resources/           # K8s resource sub-packages
│   ├── kubescape/               # Security scanning tools
│   ├── prometheus/              # Prometheus query tools
│   └── utils/                   # DateTime and general utilities
├── internal/                    # Internal packages
│   ├── cache/                   # Thread-safe TTL cache with metrics
│   ├── cmd/                     # Shell command execution (with mock support)
│   ├── commands/                # CommandBuilder fluent CLI interface
│   ├── errors/                  # Structured ToolError with context
│   ├── logger/                  # Structured logging
│   ├── metrics/                 # Prometheus metrics definitions
│   ├── security/                # Input validation (K8s names, URLs, PromQL)
│   ├── telemetry/               # OpenTelemetry tracing and HTTP middleware
│   └── version/                 # Version info
├── test/
│   └── e2e/                     # End-to-end tests (Kind cluster)
├── scripts/                     # Build and setup scripts
│   └── kind/                    # Kind cluster configuration
├── helm/                        # Helm chart for deployment
├── docs/                        # Documentation
├── .github/workflows/           # CI/CD pipelines
│   ├── ci.yaml                  # Build, test, e2e, Helm tests
│   └── tag.yaml                 # Release tagging
├── Makefile                     # Build orchestration
├── Dockerfile                   # Multi-stage build (multi-arch)
├── go.mod                       # Go 1.25.6
├── DEVELOPMENT.md               # Development setup and standards
└── CONTRIBUTION.md              # Contribution process
```

---

## Tool Providers

Each provider lives in `pkg/` and registers MCP tools via a `RegisterTools(server, readOnly)` function:

| Provider | Package | Purpose |
|----------|---------|---------|
| **Kubernetes** | `pkg/k8s/` | kubectl get, describe, logs, exec, scale, patch, rollouts |
| **Helm** | `pkg/helm/` | List, install, upgrade, uninstall releases; repo management |
| **Istio** | `pkg/istio/` | VirtualService, Gateway, DestinationRule; proxy diagnostics |
| **Argo Rollouts** | `pkg/argo/` | Rollout promotion, pause/resume, canary/blue-green |
| **Cilium** | `pkg/cilium/` | BGP routing, cluster connectivity checks |
| **Kubescape** | `pkg/kubescape/` | Image scanning, configuration compliance |
| **Prometheus** | `pkg/prometheus/` | PromQL instant/range queries, scrape status |
| **Utils** | `pkg/utils/` | DateTime formatting/parsing |

---

## Build & Test Commands

| Task | Command |
|------|---------|
| Build all platforms | `make build` |
| Format code | `make fmt` |
| Lint | `make lint` |
| Lint with auto-fix | `make lint-fix` |
| Run tests with coverage | `make test` |
| Run tests only (no build/lint) | `make test-only` |
| Run E2E tests | `make e2e` |
| Build Docker image | `make docker-build` |
| Build multi-arch Docker | `make docker-build-all` |
| Helm chart tests | `make helm-test` |
| Install locally | `make tools-install` |
| Run MCP server locally | `make run` |
| Check tool version updates | `make check-releases` |
| Run Jaeger tracing | `make otel-local` |
| Show all targets | `make help` |

Before submitting changes, run `make fmt && make lint && make test`.

---

## Code Conventions

### Tool Registration Pattern

Each provider implements a `RegisterTools` function that adds MCP tool handlers to the server:

```go
func RegisterTools(server *server.MCPServer, readOnly bool) {
    server.AddTool(mcp.NewTool("tool_name", ...), handleToolName)
    if !readOnly {
        server.AddTool(mcp.NewTool("write_tool", ...), handleWriteTool)
    }
}
```

Handler functions are prefixed with `handle` (e.g., `handleKubectlGetEnhanced`, `handleHelmList`).

### CommandBuilder Pattern

Use the fluent `CommandBuilder` interface for executing CLI commands:

```go
result, err := commands.NewCommandBuilder("helm").
    WithArgs("list").
    WithNamespace("default").
    Execute(ctx)
```

Available builders: `KubectlBuilder()`, `HelmBuilder()`, `IstioCtlBuilder()`, `CiliumBuilder()`, `ArgoRolloutsBuilder()`.

### Error Handling

MCP handlers return `(*mcp.CallToolResult, error)`. Always return a `nil` Go error and use the structured `ToolError` type to format errors as MCP results:

```go
toolErr := errors.NewToolError("get_pods", "kubernetes", errors.ErrValidation).
    WithSuggestion("Check that the namespace exists").
    WithContext("namespace", namespace)
return toolErr.ToMCPResult(), nil
```

Never panic in tool handlers — always return a `*mcp.CallToolResult`.

### Security Validation

Always validate user inputs using the `internal/security` package before passing them to CLI commands:

- `security.ValidateK8sResourceName()` — Kubernetes resource names
- `security.ValidateNamespace()` — Kubernetes namespaces
- `security.ValidateURL()` — HTTP URLs
- `security.ValidatePromQLQuery()` — PromQL syntax
- `security.ValidateCommandInput()` — General input sanitization

### Caching

The `internal/cache` package provides a thread-safe generic `Cache[T]` with TTL:

- Cache is automatically invalidated on write operations (apply, delete, patch, scale)
- Metrics tracked: hits, misses, evictions, size
- Do not bypass caching for read-heavy operations

### Naming Conventions

- Use **descriptive variable and function names** throughout. Names should clearly convey purpose and intent.
- Avoid abbreviations and single-letter names (except loop counters). Use `resourceName` not `rn`, `kubeClient` not `kc`, `toolResult` not `tr`.
- Function names should describe what they do: `handleKubectlGetEnhanced` not `doGet`, `validatePromQLQuery` not `checkQ`.
- Handler functions: prefix with `handle`.
- Builder methods: prefix with `With`.
- Validation functions: prefix with `Validate`.

### Code Reuse

- Before writing new code, search for existing utilities in `internal/` that already solve the problem.
- Do not duplicate logic across tool providers. Shared functionality belongs in `internal/` packages:
  - Command execution → `internal/commands/`
  - Error formatting → `internal/errors/`
  - Input validation → `internal/security/`
  - Caching → `internal/cache/`
  - Logging → `internal/logger/`
- If you find duplicated code while working on a change, consolidate it as part of your PR when the scope is reasonable.

---

## Testing

### Framework

- **Ginkgo v2 + Gomega** for behavioral tests
- **testify** for assertions and mocking
- Table-driven tests for comprehensive coverage
- **Minimum 80% test coverage** enforced by CI

### Mock Infrastructure

Use the mock shell executor for unit tests instead of calling real CLI tools:

```go
mockExecutor := cmd.NewMockShellExecutor()
mockExecutor.AddResponse("kubectl get pods -n default", "NAME  READY  STATUS\npod1  1/1  Running", nil)
ctx := cmd.WithShellExecutor(context.Background(), mockExecutor)
```

### Test Files

- Unit tests: co-located `*_test.go` files in each package
- E2E tests: `test/e2e/` (requires Kind cluster)
- All public functions require unit tests

---

## CI/CD Pipeline

The main workflow (`.github/workflows/ci.yaml`) runs on pushes/PRs to `main`:

1. **Multi-arch Docker build** — amd64 + arm64
2. **Go unit tests** — `go test -v -cover`
3. **E2E tests** — Kind cluster-based integration tests
4. **Helm chart tests** — Chart unit tests

Additional workflow: `tag.yaml` for release tagging.

---

## Observability

The server includes built-in observability:

- **OpenTelemetry tracing** — gRPC, HTTP exporters, stdout
- **Prometheus metrics**:
  - `kagent_tools_mcp_invocations_total` — invocation count by tool/provider
  - `kagent_tools_mcp_invocations_failure_total` — failure count
  - `kagent_tools_mcp_registered_tools` — tool inventory
  - `kagent_tools_mcp_server_info` — server metadata
- **Structured logging** with context via `internal/logger/`

---

## Commit Messages

Use **Conventional Commits**:

```
<type>: <description>

[optional body]

Signed-off-by: Name <email>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`

---

## What Not to Do

- Do not call CLI tools directly — use the `CommandBuilder` from `internal/commands/`.
- Do not skip input validation — always use `internal/security/` validators.
- Do not return Go errors from MCP handlers — use `ToolError.ToMCPResult()` instead.
- Do not duplicate logic across providers — extract to `internal/` packages.
- Do not bypass the cache for read operations.
- Do not add new tool providers without a corresponding `RegisterTools` function.
- Do not commit without running `make fmt && make lint && make test`.

---

## Adding a New Tool

1. Create a new package under `pkg/<provider>/`.
2. Implement tool handlers (prefix with `handle`).
3. Implement `RegisterTools(server, readOnly)` — respect the `readOnly` flag for write operations.
4. Register the provider in `cmd/main.go` inside `registerMCP()`.
5. Add input validation using `internal/security/`.
6. Use `CommandBuilder` for CLI execution.
7. Return errors via `ToolError.ToMCPResult()`.
8. Write unit tests with mock shell executor (80% coverage minimum).
9. Add E2E tests if the tool interacts with a cluster.
10. Run `make fmt && make lint && make test` before submitting.

---

## Additional Resources

- [DEVELOPMENT.md](DEVELOPMENT.md) — Development setup and code standards
- [CONTRIBUTION.md](CONTRIBUTION.md) — Contribution process and PR guidelines
- [docs/quickstart.md](docs/quickstart.md) — Quick start guide
