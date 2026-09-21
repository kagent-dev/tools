# SDK Comparison: mark3labs/mcp-go vs modelcontextprotocol/go-sdk

## Sources
- https://github.com/modelcontextprotocol/go-sdk
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
- https://github.com/modelcontextprotocol/go-sdk/tree/main/examples

---

## Current dependency (mark3labs/mcp-go v0.43.2)

### Imports used in this project
```
"github.com/mark3labs/mcp-go/mcp"
"github.com/mark3labs/mcp-go/server"
```

### Server lifecycle
```go
// Create server
mcpServer := server.NewMCPServer(name, version)

// Stdio mode
stdioServer := server.NewStdioServer(mcpServer)
stdioServer.Listen(ctx, os.Stdin, os.Stdout)

// HTTP/SSE mode
sseServer := server.NewStreamableHTTPServer(mcpServer,
    server.WithHeartbeatInterval(30*time.Second),
)
sseServer.ServeHTTP(w, r)
```

### Tool definition & registration
```go
// Define tool with option-function pattern
tool := mcp.NewTool("tool_name",
    mcp.WithDescription("description"),
    mcp.WithString("param",
        mcp.Required(),
        mcp.Description("param description"),
    ),
    mcp.WithBoolean("flag",
        mcp.Description("flag description"),
    ),
    mcp.WithNumber("count",
        mcp.Description("count description"),
    ),
)
// Register on server
mcpServer.AddTool(tool, handler)
```

### Handler signature
```go
type ToolHandlerFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
// Note: CallToolRequest is a value type (not pointer) in mark3labs
```

### Parameter parsing
```go
// String with default
val := mcp.ParseString(request, "param_name", "default")
// Int with default
count := mcp.ParseInt(request, "count", 50)
// Bool equivalent (parsed as string)
flag := mcp.ParseString(request, "flag", "") == "true"
```

### Result construction
```go
// Success
return mcp.NewToolResultText("output text"), nil
// Error (tool-level, not protocol error)
return mcp.NewToolResultError("error message"), nil
```

### Middleware / telemetry adapter
```go
// Adapter wraps a typed ToolHandler into server.ToolHandlerFunc
func AdaptToolHandler(th ToolHandler) server.ToolHandlerFunc {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        return th(ctx, req)
    }
}
```

### request.Params access (used in telemetry)
```go
request.Params.Name       // tool name string
request.Params.Arguments  // map[string]interface{} or nil
```

---

## Target dependency (modelcontextprotocol/go-sdk, latest)

### Import
```go
"github.com/modelcontextprotocol/go-sdk/mcp"
```

### Server lifecycle
```go
// Create server
server := mcp.NewServer(&mcp.Implementation{Name: "name", Version: "v1.0"}, nil)

// Stdio mode (blocks until client disconnects)
server.Run(ctx, &mcp.StdioTransport{})

// HTTP/SSE mode (legacy SSE, spec 2024-11-05)
handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
    return server
}, nil)
http.ListenAndServe(addr, handler)

// HTTP Streamable mode (spec 2025-03-26+)
handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    return server
}, nil)
http.ListenAndServe(addr, handler)
```

### Tool definition & registration (typed — PREFERRED)
```go
// Define typed params struct
type MyToolParams struct {
    Param  string `json:"param"  jsonschema:"description of param,required"`
    Flag   bool   `json:"flag"   jsonschema:"flag description"`
    Count  int    `json:"count"  jsonschema:"count description"`
}

// Register — schema auto-derived from struct tags
mcp.AddTool(server, &mcp.Tool{
    Name:        "tool_name",
    Description: "description",
}, func(ctx context.Context, req *mcp.CallToolRequest, args MyToolParams) (*mcp.CallToolResult, any, error) {
    // args.Param, args.Flag, args.Count are already populated and validated
    return &mcp.CallToolResult{
        Content: []mcp.Content{&mcp.TextContent{Text: "output"}},
    }, nil, nil
})
```

### Tool definition & registration (low-level — avoid if possible)
```go
// Low-level: handler receives raw CallToolRequest, no auto-validation
server.AddTool(&mcp.Tool{
    Name:        "tool_name",
    Description: "description",
    InputSchema: &jsonschema.Schema{ /* ... */ },
}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // manual parsing required
    return &mcp.CallToolResult{...}, nil
})
```

### Handler signatures
```go
// Typed (preferred) — ToolHandlerFor[In, Out any]
func(ctx context.Context, req *mcp.CallToolRequest, args MyParams) (*mcp.CallToolResult, any, error)

// Low-level — ToolHandler
func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)
```

### Result construction
```go
// Success
return &mcp.CallToolResult{
    Content: []mcp.Content{&mcp.TextContent{Text: "output text"}},
}, nil, nil

// Tool-level error (IsError=true, not a protocol error)
return &mcp.CallToolResult{
    IsError: true,
    Content: []mcp.Content{&mcp.TextContent{Text: "error message"}},
}, nil, nil

// Protocol-level error (returns as Go error)
return nil, nil, fmt.Errorf("protocol error: %w", err)
```

### Middleware
```go
type MethodHandler func(ctx context.Context, method string, req Request) (Result, error)
type Middleware func(next MethodHandler) MethodHandler

server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
    return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
        // pre-processing
        result, err := next(ctx, method, req)
        // post-processing
        return result, err
    }
})

// Access tool info inside middleware:
if ctr, ok := req.(*mcp.CallToolRequest); ok {
    _ = ctr.Params.Name       // tool name
    _ = ctr.Params.Arguments  // json.RawMessage
}
// Access tool result in middleware:
if ctr, ok := result.(*mcp.CallToolResult); ok {
    _ = ctr.IsError
    _ = ctr.StructuredContent
}
```

### Key types
```go
mcp.Implementation{Name string; Version string}
mcp.ServerOptions{}
mcp.Tool{Name string; Description string; InputSchema *jsonschema.Schema; OutputSchema *jsonschema.Schema}
mcp.CallToolRequest   // = ServerRequest[*CallToolParamsRaw]
mcp.CallToolResult{Content []Content; IsError bool; StructuredContent any}
mcp.Content           // interface
mcp.TextContent{Text string; Meta Meta; Annotations *Annotations}
mcp.StdioTransport{}
mcp.SSEHandler        // http.Handler for SSE
mcp.StreamableHTTPHandler // http.Handler for streamable HTTP
```
