# Implementation Plan: mark3labs/mcp-go → modelcontextprotocol/go-sdk

## Current Code Status

Last verified: 2026-09-21 from the current repository state.

This section is the single source of truth for migration progress. The original
step details below remain as historical implementation guidance, but the code no
longer matches the early plan exactly: the repository uses `internal/mcp` as the
SDK adapter/helper package instead of the planned `internal/mcputil` package.

### Completed

- [x] Step 1: Dependency is on `github.com/modelcontextprotocol/go-sdk`; no Go
  source imports `github.com/mark3labs/mcp-go`.
- [x] Step 2: SDK helper/adaptation layer exists as `internal/mcp`, including
  result constructors, typed `AddTool`, schema relaxation, and tool middleware.
- [x] Step 3: `internal/errors.ToolError.Context` is `map[string]string` and
  `WithContext` takes `(key, value string)`. Non-string callers converted at the
  call site: prometheus `status_code` (int -> decimal string), helm `helm_args`
  ([]string -> space-joined).
- [x] Step 4: Per-tool tracing wrappers are gone; server-level MCP middleware is
  centralized in `internal/mcp.ToolMiddleware()`.
- [x] Step 5: `pkg/utils` uses the go-sdk path through `internal/mcp`.
- [x] Step 6: `pkg/prometheus` uses the go-sdk path through `internal/mcp`.
- [x] Step 7: `pkg/argo` uses the go-sdk path through `internal/mcp`.
- [x] Step 8: `pkg/cilium` uses the go-sdk path through `internal/mcp`.
- [x] Step 9: `pkg/helm` uses the go-sdk path through `internal/mcp`.
- [x] Step 10: `pkg/istio` uses the go-sdk path through `internal/mcp`.
- [x] Step 11: `pkg/k8s` uses the go-sdk path through `internal/mcp`.
- [x] Step 12: `pkg/kubescape` builds its responses from concrete output structs,
  and the tests decode those structs.
- [x] Step 13: `cmd/main.go` creates a go-sdk server, registers provider tools,
  attaches MCP receiving middleware, and serves stdio plus Streamable HTTP.
- [x] Step 14: `test/e2e/helpers_test.go` uses the go-sdk client/session APIs.
- [x] Step 15: Final validation passes — see "Latest Verification".
- [x] Step 16: Every handler returns a typed `Out` instead of `any`. Raw CLI text
  uses the shared `mcp.TextOutput` wrapper via `mcp.TextResult` / `mcp.TextError`
  / `mcp.TextOf`; structured responses return their concrete DTO. `pkg/kubescape`
  DTOs gained `omitempty` on map fields, `CheckStatus.Details` is typed as
  `[]PodCheckEntry`, and `handleGetConfigurationScan` returns `mcp.TextOutput`
  because `v1beta1.WorkloadConfigurationScan` cannot infer an output schema.
  `pkg/prometheus` re-indents dynamic JSON with `json.Indent` instead of an
  `interface{}` round-trip. No production file registers `Out=any`, and no
  production file uses `interface{}` / untyped maps (verified by grep).

### Still Open

None.

### Latest Verification

Verified 2026-09-21 on `feature/mcp-sdk-migration` (typed-output pass).

- `go build ./...` and `go vet ./...` pass; `gofmt -l` is clean.
- `make lint` passes with `0 issues` (golangci-lint v2.13.2, pinned in the
  Makefile, with `.golangci.yml` for the go 1.27 directive).
- `go test ./pkg/... ./internal/... ./cmd/...` — 19/19 packages PASS, 0 failures.
- Coverage: every `pkg/` package is above the 80% gate (lowest `pkg/kubescape`
  at 86.9%). The two internals below 80% (`internal/commands`, `internal/cmd`)
  are pre-existing and unchanged by this work.
- `grep -rn "CallToolResult, any, error" pkg/ internal/ cmd/` (excluding tests)
  returns nothing — no handler registers `Out=any`.
- `grep -rn "interface{}|map[string]interface{}|[]interface{}|map[string]any|[]any" pkg/ internal/ cmd/`
  (excluding tests) returns nothing.
- `grep -rn "interface{}" test/e2e/` returns nothing — the e2e helpers now return
  `*mcp.CallToolResult` and `[]*mcp.Tool`.
- `TestEveryToolHasValidOutputSchema` (`cmd/tools_output_schema_test.go`) registers
  every provider on an in-memory transport and asserts each advertised tool carries
  a JSON-serializable output schema; it passes, which also proves `mcp.AddTool` does
  not panic on any `Out` type.
- Tool parity: `TestNoToolNameRegressions` passes, so all 124 names from v0.2.1
  are still advertised.
- e2e: the suite compiles under `-tags=test`; running it needs the Kind cluster
  (see the note below), which is not available in this environment.

Note: the e2e suite needs the repo's kind `extraPortMappings` (30884/30885) and
helm 3. It fails locally on helm 4 (server-side apply rejects the duplicate
`containerPort: 8084` in the chart) and when the host cannot reach the NodePort;
both are environment/chart issues that predate this migration and affect `main`
identically.

---

## Step 1: Swap dependency and establish build baseline

**Objective:** Replace the mark3labs dependency with the official SDK so every
subsequent step compiles against the new API from the start.

**Implementation guidance:**
1. In `go.mod`, remove the `github.com/mark3labs/mcp-go` line.
2. Run `go get github.com/modelcontextprotocol/go-sdk@latest`.
3. Run `go mod tidy`.
4. The project will NOT compile at this point — that is expected. Every file
   that imports `mark3labs` will report errors.
5. Do NOT fix any files yet — just verify that `go mod` resolves the new SDK.

**Test requirements:**
- `go mod verify` passes (module graph is consistent).
- `go list -m github.com/modelcontextprotocol/go-sdk` prints the resolved version.

**Integration notes:**
- No code changes outside `go.mod`/`go.sum` in this step.
- Commit the `go.mod`/`go.sum` change independently for easy bisect.

**Demo:** `go list -m github.com/modelcontextprotocol/go-sdk` outputs the new version.

---

## Step 2: Create `internal/mcputil` helpers

**Objective:** Provide `TextResult` and `ErrorResult` helper functions that all
tool packages will use. Having these in place before migrating any package avoids
writing raw `&mcp.CallToolResult{Content: ...}` literals 40+ times.

**Implementation guidance:**
1. Create `internal/mcputil/mcputil.go`:
```go
package mcputil

import "github.com/modelcontextprotocol/go-sdk/mcp"

func TextResult(text string) *mcp.CallToolResult {
    return &mcp.CallToolResult{
        Content: []mcp.Content{&mcp.TextContent{Text: text}},
    }
}

func ErrorResult(msg string) *mcp.CallToolResult {
    return &mcp.CallToolResult{
        IsError: true,
        Content: []mcp.Content{&mcp.TextContent{Text: msg}},
    }
}
```
2. Create `internal/mcputil/mcputil_test.go` with table-driven tests covering
   both helpers (verify `IsError`, `Content[0].(*mcp.TextContent).Text`).

**Test requirements:**
- `go test ./internal/mcputil/...` passes with 100% coverage.

**Integration notes:**
- This package has no dependency on any `pkg/*` or other `internal` packages —
  it can be compiled independently even while the rest of the codebase has errors.

**Demo:** `go test ./internal/mcputil/...` reports PASS.

---

## Step 3: Migrate `internal/errors` — fix `ToolError`

**Objective:** Fix `ToMCPResult()` which calls `mcp.NewToolResultError` (does not
exist in go-sdk), and replace `map[string]interface{}` with `map[string]string`
in `ToolError.Context`.

**Implementation guidance:**
1. Update import: remove `mark3labs/mcp-go/mcp`, add `kagent-dev/tools/internal/mcputil`.
2. Change `ToolError.Context` field type: `map[string]interface{}` → `map[string]string`.
3. Update `WithContext(key string, value interface{})` → `WithContext(key, value string)`.
4. Update `NewToolError` constructor: `Context: make(map[string]string)`.
5. Replace `ToMCPResult()` body: `return mcp.NewToolResultError(message.String())` →
   `return mcputil.ErrorResult(message.String())`.
6. Update `WithContext` call sites in the same file (all callers pass string values).

**Test requirements:**
- `go test ./internal/errors/...` passes.
- Existing tests updated to pass string values to `WithContext`.
- Coverage ≥ 70%.

**Integration notes:**
- `internal/errors` depends only on `internal/mcputil` (already done in Step 2).
- `pkg/*` packages that call `WithContext` will need their call sites updated when
  each package is migrated (Steps 5–12) — not required here.

**Demo:** `go test ./internal/errors/... ./internal/mcputil/...` reports PASS.

---

## Step 4: Migrate `internal/telemetry` — rewrite to `mcp.Middleware`

**Objective:** Remove the per-handler `WithTracing` wrapper and the `AdaptToolHandler`
adapter. Replace with a single server-level `NewTracingMiddleware()` factory that
returns an `mcp.Middleware` and intercepts all tool calls.

**Implementation guidance:**
1. In `middleware.go`:
   - Remove `import "github.com/mark3labs/mcp-go/server"`.
   - Change import to `"github.com/modelcontextprotocol/go-sdk/mcp"`.
   - Delete type `ToolHandler`.
   - Delete functions `WithTracing` and `AdaptToolHandler`.
   - Add:
```go
// NewTracingMiddleware returns an mcp.Middleware that records an OTel span
// for every MCP method call, with richer attributes for tools/call.
func NewTracingMiddleware() mcp.Middleware {
    return func(next mcp.MethodHandler) mcp.MethodHandler {
        return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
            tracer := otel.Tracer("kagent-tools/mcp")
            spanName := fmt.Sprintf("mcp.method.%s", method)

            // Enrich span name and attributes for tool calls
            toolName := ""
            if ctr, ok := req.(*mcp.CallToolRequest); ok {
                toolName = ctr.Params.Name
                spanName = fmt.Sprintf("mcp.tool.%s", toolName)
            }

            ctx, span := tracer.Start(ctx, spanName)
            defer span.End()

            headers := ExtractHTTPHeaders(ctx)
            for k, v := range headers {
                span.SetAttributes(attribute.String(fmt.Sprintf("http.header.%s", k), v))
            }
            if toolName != "" {
                span.SetAttributes(attribute.String("mcp.tool.name", toolName))
            }
            span.AddEvent("mcp.method.start")
            start := time.Now()

            result, err := next(ctx, method, req)

            span.SetAttributes(attribute.Float64("mcp.tool.duration_seconds", time.Since(start).Seconds()))
            if err != nil {
                span.RecordError(err)
                span.SetStatus(codes.Error, err.Error())
            } else {
                span.SetStatus(codes.Ok, "completed")
                if ctr, ok := result.(*mcp.CallToolResult); ok {
                    span.SetAttributes(attribute.Bool("mcp.result.is_error", ctr.IsError))
                    span.SetAttributes(attribute.Int("mcp.result.content_count", len(ctr.Content)))
                }
            }
            return result, err
        }
    }
}
```

2. Update `middleware_test.go`:
   - Remove test for `WithTracing` and `AdaptToolHandler`.
   - Add test for `NewTracingMiddleware` using `mcp.NewInMemoryTransports()` to
     create a real client-server pair with middleware applied; verify span is recorded.

**Test requirements:**
- `go test ./internal/telemetry/...` passes.
- Coverage ≥ 70%.
- `WithTracing` and `AdaptToolHandler` are not referenced anywhere.

**Integration notes:**
- `cmd/main.go` will call `mcpServer.AddReceivingMiddleware(telemetry.NewTracingMiddleware())`
  in Step 13.
- Until Step 13, `NewTracingMiddleware` is defined but not yet wired.

**Demo:** `go test ./internal/telemetry/...` reports PASS.

---

## Step 5: Migrate `pkg/utils`

**Objective:** Update `pkg/utils/common.go` (and any related files) to use
go-sdk types. `pkg/utils` is a leaf package with no dependencies on other `pkg/*`
packages, making it the safest starting point.

**Implementation guidance:**
1. Replace imports: remove `mark3labs/mcp-go/mcp` and `mark3labs/mcp-go/server`,
   add `modelcontextprotocol/go-sdk/mcp` and `kagent-dev/tools/internal/mcputil`.
2. For each tool handler:
   - Define a `<Action>Params` struct with `json` and `jsonschema` tags.
   - Change handler signature to `func(ctx, *mcp.CallToolRequest, <Action>Params) (*mcp.CallToolResult, any, error)`.
   - Replace `mcp.ParseString(request, ...)` with struct field access.
   - Replace `mcp.NewToolResultText(...)` with `mcputil.TextResult(...)`.
   - Replace `mcp.NewToolResultError(...)` with `mcputil.ErrorResult(...)`.
3. Update `RegisterTools` signature: `func RegisterTools(s *mcp.Server, readOnly bool)`.
4. Replace `s.AddTool(mcp.NewTool(...), handler)` with `mcp.AddTool(s, &mcp.Tool{...}, handler)`.
5. Update `*_test.go`: call handlers directly with typed args structs.

**Test requirements:**
- `go test ./pkg/utils/...` passes.
- Coverage ≥ 70%.
- No references to `mark3labs` in package.

**Demo:** `go test ./pkg/utils/...` PASS.

---

## Step 6: Migrate `pkg/prometheus`

**Objective:** Migrate the Prometheus query tools. This package also includes
`promql.go` which uses MCP types for result construction.

**Implementation guidance:**
1. Same handler migration pattern as Step 5.
2. Key params structs to define:
   - `PrometheusQueryParams` (query string, time range, step)
   - `PrometheusQueryRangeParams`
   - `PrometheusInstantQueryParams`
3. Update `promql.go` if it constructs `mcp.CallToolResult` directly — replace
   with `mcputil.TextResult` / `mcputil.ErrorResult`.
4. Update `prometheus_test.go` to use typed args.

**Test requirements:**
- `go test ./pkg/prometheus/...` passes.
- Coverage ≥ 70%.

**Demo:** `go test ./pkg/prometheus/...` PASS.

---

## Step 7: Migrate `pkg/argo`

**Objective:** Migrate the 8 Argo Rollouts tool handlers.

**Implementation guidance:**
1. Define params structs:
   - `VerifyArgoControllerParams` (namespace, label)
   - `VerifyKubectlPluginParams` (no params — empty struct `struct{}`)
   - `ListRolloutsParams` (namespace, type)
   - `CheckPluginLogsParams` (namespace, timeout)
   - `PromoteRolloutParams` (rollout_name, namespace, full bool)
   - `PauseRolloutParams` (rollout_name, namespace)
   - `SetRolloutImageParams` (rollout_name, container_image, namespace)
   - `VerifyGatewayPluginParams` (version, namespace, should_install bool)
2. For handlers with no parameters (e.g., `handleVerifyKubectlPluginInstall`),
   use an empty struct: `type VerifyKubectlPluginParams struct{}`.
3. Remove `WithTracing` wrapping from `RegisterTools` — tracing is now server-wide.
4. Update `argo_test.go`.

**Test requirements:**
- `go test ./pkg/argo/...` passes.
- Coverage ≥ 90% (critical package per CLAUDE.md).

**Demo:** `go test ./pkg/argo/...` PASS with ≥ 90% coverage shown.

---

## Step 8: Migrate `pkg/cilium`

**Objective:** Migrate the 12 Cilium tool handlers.

**Implementation guidance:**
1. Define params structs for each handler:
   - `CiliumStatusParams` — empty struct
   - `UpgradeCiliumParams` (cluster_name, datapath_mode)
   - `InstallCiliumParams` (cluster_name, cluster_id, datapath_mode)
   - `UninstallCiliumParams` — empty struct
   - `ConnectRemoteClusterParams` (cluster_name required, context)
   - `DisconnectRemoteClusterParams` (cluster_name required)
   - `ListBGPPeersParams` — empty struct
   - `ListBGPRoutesParams` — empty struct
   - `ClusterMeshStatusParams` — empty struct
   - `FeaturesStatusParams` — empty struct
   - `ToggleHubbleParams` (enable bool, default=true)
   - `ToggleClusterMeshParams` (enable bool, default=true)
2. For boolean-toggle handlers, note that bool default in jsonschema tag must be
   specified: `jsonschema:"enable Hubble,default=true"`.
3. Update `cilium_test.go`.

**Test requirements:**
- `go test ./pkg/cilium/...` passes.
- Coverage ≥ 90%.

**Demo:** `go test ./pkg/cilium/...` PASS.

---

## Step 9: Migrate `pkg/helm`

**Objective:** Migrate the 6 Helm tool handlers.

**Implementation guidance:**
1. Define params structs:
   - `HelmListParams` (namespace, all_namespaces, all, uninstalled, failed, deployed, pending, filter, output)
   - `HelmGetReleaseParams` (name required, namespace required, output)
   - `HelmUpgradeParams` (name required, chart required, namespace, version, values_file, set, wait bool, timeout, create_namespace bool, install bool)
   - `HelmUninstallParams` (name required, namespace required, keep_history bool)
   - `HelmRepoAddParams` (name required, url required, username, password, force_update bool)
   - `HelmRepoUpdateParams` — empty struct
2. Update `helm_test.go`.
3. Verify security validation calls (`security.ValidateName`, etc.) still occur
   after struct population — these are business-logic checks that remain.

**Test requirements:**
- `go test ./pkg/helm/...` passes.
- Coverage ≥ 90%.

**Demo:** `go test ./pkg/helm/...` PASS.

---

## Step 10: Migrate `pkg/istio`

**Objective:** Migrate all Istio tool handlers.

**Implementation guidance:**
1. Define params structs for each istio handler (proxy-status, analyze, install,
   upgrade, verify-install, etc.) — follow the same struct pattern.
2. Update `istio_test.go`.

**Test requirements:**
- `go test ./pkg/istio/...` passes.
- Coverage ≥ 90%.

**Demo:** `go test ./pkg/istio/...` PASS.

---

## Step 11: Migrate `pkg/k8s`

**Objective:** Migrate the largest and most critical package — all kubectl-based
Kubernetes tool handlers.

**Implementation guidance:**
1. Define params structs for all handlers:
   - `KubectlGetParams`, `KubectlLogsParams`, `ScaleDeploymentParams`,
     `PatchResourceParams`, `ApplyManifestParams`, `DeleteResourceParams`,
     `CheckServiceConnectivityParams`, `GetEventsParams`, `ExecCommandParams`,
     `GetAvailableAPIResourcesParams`, `DescribeResourceParams`,
     `ManageAnnotationParams`, `ManageLabelParams`, `SetAnnotationsParams`,
     and any others present.
2. Required fields identified from current `if param == "" { return error }` guards:
   use `jsonschema:"...,required"` for these, then remove the redundant guard.
3. Keep non-trivial business-logic guards (e.g., security validation).
4. Update `k8s_test.go` — this is the largest test file; use table-driven tests
   for all params variations.

**Test requirements:**
- `go test ./pkg/k8s/...` passes.
- Coverage ≥ 90%.

**Demo:** `go test ./pkg/k8s/...` PASS with ≥ 90% coverage.

---

## Step 12: Migrate `pkg/kubescape`

**Objective:** Migrate all Kubescape scan and report tool handlers.

**Implementation guidance:**
1. Define params structs for each handler (scan, get vulnerability manifests,
   get configuration scans, get application profiles, etc.).
2. Update `kubescape_test.go`.

**Test requirements:**
- `go test ./pkg/kubescape/...` passes.
- Coverage ≥ 90%.

**Demo:** `go test ./pkg/kubescape/...` PASS.

---

## Step 13: Migrate `cmd/main.go` — wire everything together

**Objective:** Update the entry point to use the new SDK server, transports,
and middleware. This is the integration step that makes the full binary compile
and run end-to-end.

**Implementation guidance:**
1. Remove `import "github.com/mark3labs/mcp-go/server"`.
2. Add `import "github.com/modelcontextprotocol/go-sdk/mcp"`.
3. Replace server creation:
```go
mcpServer := mcp.NewServer(&mcp.Implementation{
    Name:    Name,
    Version: Version,
}, nil)
```
4. Add telemetry middleware:
```go
mcpServer.AddReceivingMiddleware(telemetry.NewTracingMiddleware())
```
5. Update `toolProviderMap` type: `map[string]func(*mcp.Server)`.
6. Replace `runStdioServer`:
```go
func runStdioServer(ctx context.Context, s *mcp.Server) {
    logger.Get().Info("Running KAgent Tools Server STDIO:", "tools", strings.Join(tools, ","))
    if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
        logger.Get().Info("Stdio server stopped", "error", err)
    }
}
```
7. Replace HTTP server setup:
```go
httpHandler := mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return mcpServer },
    nil,
)
mux.Handle("/", telemetry.HTTPMiddleware(http.HandlerFunc(
    func(w http.ResponseWriter, r *http.Request) {
        httpHandler.ServeHTTP(w, r)
    },
)))
```
8. Remove the `server.WithHeartbeatInterval` option (no equivalent in go-sdk
   StreamableHTTPHandler; rely on HTTP keep-alive).
9. Verify `registerMCP(mcpServer, ...)` compiles with `*mcp.Server` argument.

**Test requirements:**
- `go build ./cmd/...` succeeds with zero errors.
- `go run ./cmd -- --stdio` starts and responds to `tools/list`.
- `make lint` passes.

**Integration notes:**
- This is the first step where `grep -r "mark3labs" .` should return zero results.

**Demo:**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | go run ./cmd -- --stdio
```
Returns a JSON response listing all tools.

---

## Step 14: Update E2E test helpers

**Objective:** Update `test/e2e/helpers_test.go` to use go-sdk client types for
integration test scaffolding.

**Implementation guidance:**
1. Replace `mark3labs` client types with go-sdk equivalents:
```go
// Before: mark3labs client construction
// After:
client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
transport := &mcp.CommandTransport{Command: exec.Command("./bin/kagent-tools", "--stdio")}
session, err := client.Connect(ctx, transport, nil)
```
2. Update tool invocations:
```go
res, err := session.CallTool(ctx, &mcp.CallToolParams{
    Name:      "kubectl_get",
    Arguments: map[string]any{"resource_type": "pod"},
})
```
3. Replace result assertions:
```go
// Check IsError flag
if res.IsError { t.Fatalf(...) }
text := res.Content[0].(*mcp.TextContent).Text
```

**Test requirements:**
- `go test ./test/e2e/...` passes (or is skipped gracefully when cluster unavailable).

**Demo:** `go test ./test/e2e/... -run TestToolsList` PASS.

---

## Step 15: Final validation

**Objective:** Confirm all quality gates pass, no mark3labs references remain,
and the binary behaves identically to before migration.

**Implementation guidance:**
1. Run full test suite:
```bash
make test
```
2. Verify zero mark3labs references:
```bash
grep -r "mark3labs" . --include="*.go" --include="go.mod"
# must return: no output
```
3. Verify no `map[any]any` or `map[string]interface{}` in tool params:
```bash
grep -r "map\[string\]interface{}\|map\[string\]any\|map\[any\]" pkg/ internal/ --include="*.go"
# must return: no output
```
4. Run linter:
```bash
make lint
```
5. Build all platform binaries:
```bash
make build
```
6. Smoke test both transports:
```bash
# Stdio
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | ./bin/kagent-tools --stdio

# HTTP
./bin/kagent-tools --port 8085 &
sleep 1
curl -s http://localhost:8085/health
kill %1
```

**Test requirements:**
- `make test` exits 0.
- `make lint` exits 0.
- `make build` exits 0.
- Both smoke tests return expected responses.
- `grep -r "mark3labs" .` returns no matches.

**Demo:** CI pipeline passes (or equivalent local `make test && make lint && make build`).
