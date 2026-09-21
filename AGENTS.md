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

Each provider implements a `RegisterTools` function that adds MCP tool handlers to the server. Registration goes through the wrapper in `internal/mcp`, which records the tool's provider for metrics and relaxes the inferred input schema so optional fields stay optional:

```go
func RegisterTools(s *mcp.Server, readOnly bool) {
    mcp.AddTool(s, "k8s", &mcp.Tool{
        Name:        "k8s_get_resources",
        Description: "Get Kubernetes resources",
    }, handleGetResources)

    if !readOnly {
        mcp.AddTool(s, "k8s", &mcp.Tool{
            Name:        "k8s_delete_resource",
            Description: "Delete a Kubernetes resource",
        }, handleDeleteResource)
    }
}
```

Handlers are registered with a typed input and a typed output: `func handleX(ctx context.Context, req *mcp.CallToolRequest, in xInput) (*mcp.CallToolResult, xOutput, error)`. Handler functions are prefixed with `handle` (e.g., `handleKubectlGetEnhanced`, `handleHelmList`).

### Typed MCP Inputs and Outputs

All MCP tool inputs and outputs must be strongly typed. The Go MCP SDK derives an input and output JSON schema from the handler's `In` and `Out` type parameters, populates `CallToolResult.StructuredContent` from the typed `Out` value, and validates that value against the inferred output schema on every call — so an untyped or wrongly-shaped `Out` is not merely untidy, it breaks the tool.

- Define a concrete input struct for every tool with `json` and `jsonschema` tags.
- Define a concrete output DTO for every structured response.
- Never register handlers with `Out=any`; typed outputs enable output schema inference and validation.
- Do not use `any`, `interface{}`, `map[string]any`, `map[string]interface{}`, `[]any`, or `[]interface{}` for handler inputs, handler outputs, public response DTOs, or tests.
- Handler signature: `func handleX(ctx, req, in xInput) (*mcp.CallToolResult, xOutput, error)`.

**Raw CLI text.** Most providers wrap CLI output in text. Use the shared `mcp.TextOutput` wrapper (`{"output": "..."}`) instead of inventing a per-tool shape, and return it through the helpers so the zero value on an error path still validates:

```go
// success — text is preserved in Content and mirrored in StructuredContent
return mcp.TextResult(output)

// tool-level failure — IsError=true, and the empty TextOutput keeps the
// inferred output schema satisfied
return mcp.TextError("resource_name is required")
```

When a helper builds the `*mcp.CallToolResult` itself (e.g. `runKubectlCommand` returning `(*mcp.CallToolResult, error)`), convert it with `mcp.TextOf(res)` and return `res, mcp.TextOf(res), err` so the typed value matches the returned result.

**Output-schema pitfalls.** These are enforced by the SDK at call time and are easy to trip:

- **Zero values are validated on every path, including errors.** A field whose zero value marshals to `null` but whose schema type is non-nullable (notably `map[K]V`) makes *all* error returns fail with `validating tool output`. Give such fields `omitempty`. Slices and pointers infer as nullable (`["null", ...]`) and are safe.
- **`json.RawMessage` does not mean "arbitrary JSON".** The schema inference treats it as a byte slice and validation then rejects real objects and arrays. For genuinely dynamic JSON, return the raw text through `mcp.TextOutput` rather than a `json.RawMessage` field, or re-indent it in place with `json.Indent` without decoding into `interface{}` (see `prettyJSONBody` in `pkg/prometheus`).
- **Not every type can be an `Out`.** Third-party structs with custom JSON marshallers can fail schema inference, which makes `mcp.AddTool` *panic* at registration (the server will not start). `v1beta1.WorkloadConfigurationScan` is one such type; such handlers return `mcp.TextOutput`. `cmd/tools_output_schema_test.go` registers every provider and fails if any `Out` type cannot produce a valid schema.
- **A third-party type may be used** as an `Out` field where inference succeeds (`[]v1beta1.Match`, `v1beta1.ExecCalls`, `metav1.LabelSelector` all work today); prefer a local summary DTO where it does not.

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
- Decode structured tool results into the same output DTOs used by production code. Avoid `map[string]interface{}` / `[]interface{}` assertions in tests.

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
- Do not use untyped maps or `any` for MCP tool input/output schemas or public response bodies.
- Do not register a handler with `Out=any` — the SDK cannot infer or validate an output schema, and the typed-output contract is what keeps the tool callable.
- Do not add a map-typed field to an output DTO without `omitempty`, and do not use `json.RawMessage` as a dynamic-JSON output field — both make the SDK reject valid results at call time (see the output-schema pitfalls above).
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
7. Define concrete typed input and output DTOs; avoid `any`, `interface{}`, and untyped maps. Return `mcp.TextResult(...)` / `mcp.TextError(...)` for raw CLI text, and a concrete DTO for a structured response. Watch the output-schema pitfalls above (`omitempty` on map fields, no `json.RawMessage` fields, no `Out` types that fail schema inference).
8. Return errors via `ToolError.ToMCPResult()`; remember the `Out` value must still validate on the error path, so return the zero value of the DTO (or `mcp.TextOutput{}`).
9. Write unit tests with mock shell executor (80% coverage minimum).
10. Add E2E tests if the tool interacts with a cluster.
11. Run `make fmt && make lint && make test` before submitting.

---

## Additional Resources

- [DEVELOPMENT.md](DEVELOPMENT.md) — Development setup and code standards
- [CONTRIBUTION.md](CONTRIBUTION.md) — Contribution process and PR guidelines
- [docs/quickstart.md](docs/quickstart.md) — Quick start guide
