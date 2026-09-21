# Requirements Q&A

> This file captures requirements clarification questions and answers gathered during the PDD process.
> Questions and answers are appended in real time.

---

## Q1: What type safety requirements apply to the SDK migration?

**Q:** Should the migration use any dynamic/generic map types (e.g. `map[string]any`, `map[any]any`) for tool parameters or results, or should concrete Go struct types be used?

**A:** Use Go struct types throughout. Avoid `map[any]any` and prefer typed structs for all tool parameters, inputs, and outputs. This applies to parameter parsing, result construction, and any intermediate data structures introduced during the migration.

---

## Research findings appended

See `research/sdk-comparison.md` and `skill.md` for the full API mapping.

Key confirmed facts from official SDK examples and pkg.go.dev:
- `mcp.AddTool` is a generic function that auto-derives JSON schema from the typed `In` param struct.
- `ToolHandlerFor[In, Out any]` signature returns `(*CallToolResult, any, error)` — three values.
- No `NewToolResultText` / `NewToolResultError` helpers — must construct `CallToolResult` directly or add local helpers.
- Middleware uses `AddReceivingMiddleware` with `mcp.MethodHandler` / `mcp.Middleware` types.
- `ToolError.Context` field (`map[string]interface{}`) violates no-map-any-any rule and must be replaced.
