# Official MCP Go SDK Migration Plan

## Objective
Remove all usage of `github.com/mark3labs/mcp-go` and migrate fully to `github.com/modelcontextprotocol/go-sdk` with strong typing, stable behavior, and passing tests.

## Definition of Done
- `go.mod` has **no** `mark3labs/mcp-go` dependency (direct or required by internal tests).
- `grep -R "mark3labs/mcp-go" .` returns only historical docs/changelog notes (or zero).
- `go test ./...` passes.
- Server runs in stdio and streamable HTTP modes with official SDK.
- Tool handlers use typed inputs (structs) for business logic.

## Current State
- Compatibility layer exists (`internal/mcpcompat`) and non-test build passes.
- Tests still depend on old request/content shapes.
- e2e helpers still use old mark3labs client APIs.

## Smart Migration Strategy (Two-Track)

### Track A: Stability Bridge (already started)
1. Keep runtime behavior via compatibility wrappers.
2. Migrate imports package-by-package with minimal breakage.
3. Keep incremental green builds while reducing blast radius.

### Track B: Typed Native Migration (target state)
1. Introduce typed input structs per tool.
2. Register tools with official `mcp.AddTool[In, Out]`.
3. Remove parse helpers and compatibility wrappers once all packages are native.

## Package Execution Order
1. `internal/telemetry` and `internal/errors`
2. `pkg/utils`
3. `pkg/istio`
4. `pkg/argo`
5. `pkg/cilium`
6. `pkg/helm`
7. `pkg/prometheus`
8. `pkg/kubescape`
9. `pkg/k8s` (largest, last for focused effort)
10. `test/e2e` client harness
11. Remove `internal/mcpcompat`

## Per-Package Work Template
For each package:
1. Replace raw request parsing with typed `In` structs.
2. Register tool with official `mcp.AddTool` typed handlers.
3. Replace content assertions to pointer form where needed.
4. Update tests to use `json.RawMessage` arguments and official request/result types.
5. Run package tests before moving forward.

## Internal Interfaces to Standardize
- Request args decode: typed structs only.
- Error conversion: centralized helper returning official `CallToolResult`.
- Trace middleware: preserve OTel context and request metadata with official request shape.

## Test Migration Notes
- Replace direct `map[string]any` assignment into `Arguments` with marshaled `json.RawMessage`.
- Update type assertions from value to pointer content (`*mcp.TextContent`).
- Rework e2e helpers to official SDK client/session APIs.

## Subagent Execution Plan (parallel by package)
- Subagent 1: `utils + telemetry + errors`
- Subagent 2: `istio + argo`
- Subagent 3: `cilium + helm + prometheus`
- Subagent 4: `kubescape + k8s`
- Subagent 5: `test/e2e + final cleanup`

Each subagent returns:
- changed files
- test results
- unresolved blockers
- rollback notes

## Integration Gates
- Gate 1: Build passes for cmd/internal/pkg.
- Gate 2: Unit tests pass for migrated package group.
- Gate 3: e2e tests pass.
- Gate 4: Remove compatibility layer and mark3labs dependency.

## Risk Controls
- Keep commits small and package-scoped.
- Never push PR automatically.
- Preserve old behavior and tool names during migration.
- If a package blocks, isolate and continue others in parallel.

## Immediate Next Actions
1. Finish migration of internal/test request constructors to official shapes.
2. Move `utils` and `istio` from compatibility parsing to typed handlers.
3. Start e2e harness migration to official SDK client.
