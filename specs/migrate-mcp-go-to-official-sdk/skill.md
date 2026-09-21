# Migration Skill: mark3labs/mcp-go → modelcontextprotocol/go-sdk

> Reference document for migrating `github.com/mark3labs/mcp-go` to the official
> `github.com/modelcontextprotocol/go-sdk`. Use this as the authoritative lookup
> during implementation. All patterns use concrete Go struct types — no `map[any]any`.

---

## 1. Dependency Change

```diff
# go.mod
- github.com/mark3labs/mcp-go v0.43.2
+ github.com/modelcontextprotocol/go-sdk <latest>
```

```bash
go get github.com/modelcontextprotocol/go-sdk@latest
go mod tidy
```

---

## 2. Import Paths

| mark3labs | go-sdk |
|-----------|--------|
| `"github.com/mark3labs/mcp-go/mcp"` | `"github.com/modelcontextprotocol/go-sdk/mcp"` |
| `"github.com/mark3labs/mcp-go/server"` | _(removed — all under `mcp` package)_ |

---

## 3. Server Creation

### mark3labs
```go
import "github.com/mark3labs/mcp-go/server"

mcpServer := server.NewMCPServer(Name, Version)
```

### go-sdk
```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

mcpServer := mcp.NewServer(&mcp.Implementation{
    Name:    Name,
    Version: Version,
}, nil)
```

---

## 4. Tool Handler Signature

This is the most impactful change. Replace dynamic parsing with typed structs.

### mark3labs
```go
func handleMyTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    param := mcp.ParseString(request, "param_name", "")
    count := mcp.ParseInt(request, "count", 50)
    flag  := mcp.ParseString(request, "flag", "") == "true"
    // ...
}
```

### go-sdk (REQUIRED pattern — typed structs, no map[any]any)
```go
// 1. Define a params struct for every tool
type MyToolParams struct {
    ParamName string `json:"param_name" jsonschema:"description of param"`
    Count     int    `json:"count"      jsonschema:"number of lines,default=50"`
    Flag      bool   `json:"flag"       jsonschema:"enable flag"`
}

// 2. Handler receives populated, validated struct directly
func handleMyTool(ctx context.Context, req *mcp.CallToolRequest, args MyToolParams) (*mcp.CallToolResult, any, error) {
    // args.ParamName, args.Count, args.Flag are already set
    // ...
}
```

**Key rules:**
- Every tool MUST have a dedicated params struct.
- Fields validated as `required` in jsonschema will return a tool error automatically.
- Handler returns THREE values: `(*mcp.CallToolResult, any, error)` — the middle `any` is the structured output (return `nil` if unused).
- `req` is a pointer (`*mcp.CallToolRequest`), not a value.

---

## 5. Tool Definition & Registration

### mark3labs
```go
tool := mcp.NewTool("tool_name",
    mcp.WithDescription("description"),
    mcp.WithString("param",   mcp.Required(), mcp.Description("...")),
    mcp.WithBoolean("flag",   mcp.Description("...")),
    mcp.WithNumber("count",   mcp.Description("...")),
)
mcpServer.AddTool(tool, handler)
```

### go-sdk
```go
// Schema is auto-derived from the params struct — no need to list params manually.
mcp.AddTool(mcpServer, &mcp.Tool{
    Name:        "tool_name",
    Description: "description",
}, handleMyTool)
```

**Struct tags that drive schema generation:**

| Tag | Purpose |
|-----|---------|
| `json:"field_name"` | JSON key name (required) |
| `jsonschema:"description text"` | Field description shown in schema |
| `jsonschema:"description,required"` | Mark field as required |
| `jsonschema:"description,default=value"` | Provide default value |

---

## 6. Result Construction

### mark3labs → go-sdk

| Scenario | mark3labs | go-sdk |
|----------|-----------|--------|
| **Success** | `mcp.NewToolResultText("text")` | `&mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "text"}}}` |
| **Tool error** | `mcp.NewToolResultError("msg")` | `&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "msg"}}}` |
| **Protocol error** | `return nil, fmt.Errorf("...")` | `return nil, nil, fmt.Errorf("...")` |

### Helper functions to define (add to `pkg/utils/` or `internal/mcputil/`)

Since the official SDK has no `NewToolResultText`/`NewToolResultError` helpers,
define these once and reuse:

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

---

## 7. Transport / Server Startup

### Stdio transport

#### mark3labs
```go
stdioServer := server.NewStdioServer(mcpServer)
stdioServer.Listen(ctx, os.Stdin, os.Stdout)
```

#### go-sdk
```go
// Run blocks until client disconnects or ctx is cancelled
if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
    logger.Get().Info("Stdio server stopped", "error", err)
}
```

### HTTP/SSE transport

#### mark3labs
```go
sseServer := server.NewStreamableHTTPServer(mcpServer,
    server.WithHeartbeatInterval(30*time.Second),
)
mux.Handle("/", sseServer)
```

#### go-sdk
```go
// StreamableHTTPHandler (MCP spec 2025-03-26+)
handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    return mcpServer
}, nil)
mux.Handle("/", handler)

// OR legacy SSEHandler (MCP spec 2024-11-05)
handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
    return mcpServer
}, nil)
mux.Handle("/", handler)
```

> **Note:** `WithHeartbeatInterval` has no direct equivalent — check
> `StreamableHTTPOptions` for any keepalive options in the installed version.

---

## 8. Middleware / Telemetry

The telemetry `WithTracing` wrapper currently adapts `ToolHandler` → `server.ToolHandlerFunc`.
With go-sdk, use `AddReceivingMiddleware` instead.

### go-sdk middleware signature
```go
type MethodHandler func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error)
type Middleware     func(next mcp.MethodHandler) mcp.MethodHandler

mcpServer.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
    return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
        // Intercept tool calls
        if ctr, ok := req.(*mcp.CallToolRequest); ok {
            toolName := ctr.Params.Name
            _ = toolName // use for spans
        }
        result, err := next(ctx, method, req)
        // Inspect tool results
        if ctr, ok := result.(*mcp.CallToolResult); ok {
            _ = ctr.IsError
        }
        return result, err
    }
})
```

### Migrating `internal/telemetry/middleware.go`

1. Remove `ToolHandler` type alias (no longer needed).
2. Remove `AdaptToolHandler` function.
3. Expose a `NewTracingMiddleware(tracer) mcp.Middleware` function instead.
4. The `WithTracing(toolName, handler)` wrapper pattern is replaced by a single
   server-level middleware that extracts tool name from `req.(*mcp.CallToolRequest).Params.Name`.

### Accessing request context in middleware
```go
// Tool name
ctr.Params.Name

// Arguments (json.RawMessage, not map — use json.Unmarshal to read)
ctr.Params.Arguments

// Session ID
req.GetSession().ID()
```

---

## 9. internal/errors/tool_errors.go

`ToMCPResult()` calls `mcp.NewToolResultError(...)` which does not exist in go-sdk.

### Fix
```go
// Before (mark3labs)
return mcp.NewToolResultError(message.String())

// After (go-sdk)
return &mcp.CallToolResult{
    IsError: true,
    Content: []mcp.Content{&mcp.TextContent{Text: message.String()}},
}
```

Also replace `map[string]interface{}` in `ToolError.Context` with a concrete struct
or `map[string]string` to honour the "no map[any]any" requirement.

---

## 10. RegisterTools Function Signature

All `pkg/*/` packages export a `RegisterTools` function. Signature changes from:

```go
// mark3labs
func RegisterTools(s *server.MCPServer, readOnly bool)
```

to:

```go
// go-sdk
func RegisterTools(s *mcp.Server, readOnly bool)
```

`cmd/main.go` `registerMCP` function and its `toolProviderMap` closures update accordingly:

```go
// Before
toolProviderMap := map[string]func(*server.MCPServer){...}

// After
toolProviderMap := map[string]func(*mcp.Server){...}
```

---

## 11. Params Struct Reference (per package)

Define one `*Params` struct per tool handler. Name it `<ToolAction>Params`.

### Example: k8s package

```go
// kubectl_get
type KubectlGetParams struct {
    ResourceType  string `json:"resource_type"  jsonschema:"type of K8s resource (pod/deploy/svc..),required"`
    ResourceName  string `json:"resource_name"  jsonschema:"name of the resource"`
    Namespace     string `json:"namespace"      jsonschema:"namespace to query"`
    AllNamespaces bool   `json:"all_namespaces" jsonschema:"query all namespaces"`
    Output        string `json:"output"         jsonschema:"output format (wide/json/yaml),default=wide"`
}

// kubectl_logs
type KubectlLogsParams struct {
    PodName   string `json:"pod_name"   jsonschema:"name of the pod,required"`
    Namespace string `json:"namespace"  jsonschema:"namespace,default=default"`
    Container string `json:"container"  jsonschema:"container name"`
    TailLines int    `json:"tail_lines" jsonschema:"number of log lines,default=50"`
}

// scale_deployment
type ScaleDeploymentParams struct {
    Name      string `json:"name"      jsonschema:"deployment name,required"`
    Namespace string `json:"namespace" jsonschema:"namespace,default=default"`
    Replicas  int    `json:"replicas"  jsonschema:"desired replica count,default=1"`
}
```

### Example: helm package

```go
type HelmListParams struct {
    Namespace     string `json:"namespace"      jsonschema:"filter by namespace"`
    AllNamespaces bool   `json:"all_namespaces" jsonschema:"list across all namespaces"`
    All           bool   `json:"all"            jsonschema:"show all releases"`
    Uninstalled   bool   `json:"uninstalled"    jsonschema:"show uninstalled releases"`
    Failed        bool   `json:"failed"         jsonschema:"show failed releases"`
    Deployed      bool   `json:"deployed"       jsonschema:"show deployed releases"`
    Pending       bool   `json:"pending"        jsonschema:"show pending releases"`
    Filter        string `json:"filter"         jsonschema:"regex filter for release names"`
    Output        string `json:"output"         jsonschema:"output format"`
}
```

---

## 12. Test Migration

Tests using mark3labs types must be updated:

```go
// Before (mark3labs)
req := mcp.CallToolRequest{}
req.Params.Arguments = map[string]interface{}{"param": "value"}

// After (go-sdk — construct the typed params struct directly in tests)
args := MyToolParams{ParamName: "value", Count: 10}
// Call handler directly with args, bypassing request parsing:
result, _, err := handleMyTool(ctx, &mcp.CallToolRequest{}, args)
```

For mock-based tests in `pkg/*/`, inject args directly into the typed handler —
no need to construct `CallToolRequest` params at all for unit tests.

---

## 13. Files to Modify (complete list)

| File | Change |
|------|--------|
| `go.mod` / `go.sum` | Replace dependency |
| `cmd/main.go` | Server creation, transports, `registerMCP` signature |
| `internal/telemetry/middleware.go` | Replace `ToolHandler` type, remove `AdaptToolHandler`, add `mcp.Middleware` factory |
| `internal/telemetry/middleware_test.go` | Update test types |
| `internal/errors/tool_errors.go` | Fix `ToMCPResult()`, fix `Context` map type |
| `pkg/k8s/k8s.go` | Params structs, handler signatures, registration |
| `pkg/k8s/k8s_test.go` | Update test helpers |
| `pkg/helm/helm.go` | Params structs, handler signatures, registration |
| `pkg/helm/helm_test.go` | Update test helpers |
| `pkg/istio/istio.go` | Params structs, handler signatures, registration |
| `pkg/istio/istio_test.go` | Update test helpers |
| `pkg/argo/argo.go` | Params structs, handler signatures, registration |
| `pkg/argo/argo_test.go` | Update test helpers |
| `pkg/cilium/cilium.go` | Params structs, handler signatures, registration |
| `pkg/cilium/cilium_test.go` | Update test helpers |
| `pkg/prometheus/prometheus.go` | Params structs, handler signatures, registration |
| `pkg/prometheus/prometheus_test.go` | Update test helpers |
| `pkg/prometheus/promql.go` | Update MCP types |
| `pkg/kubescape/kubescape.go` | Params structs, handler signatures, registration |
| `pkg/kubescape/kubescape_test.go` | Update test helpers |
| `pkg/utils/common.go` | Update MCP types |
| `pkg/utils/datetime_test.go` | Update test types |
| `test/e2e/helpers_test.go` | Update client/server setup |

---

## 14. Migration Order (recommended)

1. **`go.mod`** — swap dependency, run `go mod tidy`
2. **`internal/mcputil/`** — create `TextResult` / `ErrorResult` helpers (new file)
3. **`internal/errors/tool_errors.go`** — fix `ToMCPResult()` and `Context` field type
4. **`internal/telemetry/middleware.go`** — rewrite to `mcp.Middleware` pattern
5. **`pkg/utils/`** — update types (least dependent)
6. **`pkg/prometheus/`** — update types
7. **`pkg/argo/`**, **`pkg/cilium/`**, **`pkg/helm/`**, **`pkg/istio/`**, **`pkg/k8s/`**, **`pkg/kubescape/`** — update each package (params structs + handler signatures + registration)
8. **`cmd/main.go`** — update server creation and transport wiring
9. **All `*_test.go`** — update test helpers per package
10. **`test/e2e/`** — update integration test helpers

Run `make test` and `make lint` after each package to catch regressions early.

---

## 15. Quick Reference Card

```
REMOVED (mark3labs)          →  REPLACEMENT (go-sdk)
─────────────────────────────────────────────────────────────────
server.NewMCPServer(n,v)     →  mcp.NewServer(&mcp.Implementation{Name:n,Version:v}, nil)
server.NewStdioServer(s)     →  s.Run(ctx, &mcp.StdioTransport{})
server.NewStreamableHTTP(s)  →  mcp.NewStreamableHTTPHandler(func(r)*mcp.Server{return s}, nil)
server.ToolHandlerFunc       →  mcp.ToolHandlerFor[In,Out] or mcp.ToolHandler
mcp.CallToolRequest (value)  →  *mcp.CallToolRequest (pointer)
mcp.ParseString(req,k,d)     →  struct field (typed params)
mcp.ParseInt(req,k,d)        →  struct field (typed params)
mcp.NewTool(name, opts...)   →  &mcp.Tool{Name:"...", Description:"..."}
s.AddTool(tool, handler)     →  mcp.AddTool(s, &mcp.Tool{...}, typedHandler)
mcp.NewToolResultText(t)     →  mcputil.TextResult(t)  [local helper]
mcp.NewToolResultError(t)    →  mcputil.ErrorResult(t) [local helper]
handler returns (res, err)   →  handler returns (res, any, err)
─────────────────────────────────────────────────────────────────
```
