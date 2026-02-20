# MCP SDK Migration (mark3labs -> official go-sdk)

## Status
In progress (Gate B started).

## Completed in this pass

1. Added official SDK dependency: `github.com/modelcontextprotocol/go-sdk`.
2. Introduced compatibility layer:
   - `internal/mcpcompat/compat.go`
   - `internal/mcpcompat/server/server.go`
3. Switched project imports from:
   - `github.com/mark3labs/mcp-go/mcp` -> `github.com/kagent-dev/tools/internal/mcpcompat`
   - `github.com/mark3labs/mcp-go/server` -> `github.com/kagent-dev/tools/internal/mcpcompat/server`
4. Provider/server code now compiles against official SDK through compatibility wrappers.
5. Non-test build is green:
   - `go build ./cmd/... ./internal/... ./pkg/...` ✅

## Key compatibility decisions

- Keep existing tool handler signatures for now:
  - `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`
- Bridge to official SDK typed registration via:
  - `mcp.RegisterLegacyTool(...)` -> `sdkmcp.AddTool(...)`
- Keep old tool schema builder ergonomics (`NewTool`, `WithString`, `Required`, etc.)
  while emitting official SDK `Tool` definitions.
- Keep old `server.MCPServer` shape via wrapper to reduce churn in provider packages.

## Remaining work

1. **Tests migration (major):**
   - Existing tests construct old-style request payloads (`map` directly in `Arguments`),
     and assert old content concrete types.
   - Update tests to official request encoding (`json.RawMessage`) and pointer content assertions.
2. Remove residual direct dependency on `mark3labs/mcp-go` (currently retained by e2e tests).
3. Introduce strongly-typed input structs per tool/provider (current layer still parses dynamic args).
4. Gradually replace compatibility wrappers with native official SDK handlers.
5. Validate HTTP streamable behavior parity and update docs.

## Next execution wave

- Wave C1: migrate `utils` + `istio` tests and typed inputs.
- Wave C2: migrate `k8s` typed requests and tests.
- Wave C3: migrate remaining providers and e2e harness.
