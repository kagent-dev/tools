# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Reference

### Build & Test
```bash
make build          # Build all platform binaries
make test           # Run tests with coverage and linting
make lint           # Run golangci-lint
make lint-fix       # Auto-fix linting issues
make fmt            # Format code with go fmt
```

### Run Locally
```bash
go run ./cmd                    # Run directly
./bin/kagent-tools --stdio      # Stdio transport
./bin/kagent-tools --http --port 8084  # HTTP transport
```

### Test Specific Components
```bash
go test -v ./pkg/k8s       # Test specific package
go test -v -cover ./...    # All tests with coverage
```

## Architecture Overview

This is a Go-based MCP (Model Context Protocol) server that wraps Kubernetes and cloud-native tool CLIs. Rather than reimplementing functionality, it provides a unified MCP interface to existing command-line tools.

### Core Design
- **Single responsibility packages**: Each `pkg/` subdirectory handles one tool category (k8s, helm, istio, etc.)
- **CLI wrapper pattern**: Tools call external CLIs (kubectl, helm, istioctl, etc.) and return formatted results
- **MCP SDK integration**: Uses `github.com/modelcontextprotocol/go-sdk` for all tool registration and communication
- **Multiple transports**: Supports stdio (for direct client integration) and HTTP/SSE (for web integration)
- **Type-safe parameters**: All tool parameters validated using `request.RequireString()`, `request.RequireBool()`, etc.

### Package Structure
```
pkg/
├── k8s/        # Kubernetes operations via kubectl
├── helm/       # Helm package management
├── istio/      # Istio service mesh via istioctl
├── argo/       # Argo Rollouts via kubectl plugins
├── cilium/     # Cilium CNI operations
├── prometheus/ # Prometheus API queries
├── utils/      # Common shell command execution
├── logger/     # Structured logging
```

### Key Implementation Files
- `cmd/main.go`: MCP server setup, CLI flag handling, transport initialization
- `pkg/[category]/[category].go`: Tool registration and handler implementation
- Tool handlers follow: parse params → execute CLI → format result → return MCP result

## Development Practices

### MCP Tool Implementation Pattern
When adding a new tool, follow this structure:

1. **Define in RegisterTools()**: Use `mcp.NewTool()` with parameters
2. **Type-safe parsing**: Use `request.RequireString()`, `request.RequireBool()` for validation
3. **CLI execution**: Use `runCommand()` utility for consistent error handling
4. **Result formatting**: Return `mcp.NewToolResultText()` for success or `mcp.NewToolResultError()` for failures

Example from existing code (pkg/k8s/k8s.go style):
```go
func (t *Tools) RegisterTools(server *mcp.Server) error {
    tool := mcp.NewTool("tool_name",
        mcp.WithDescription("What this tool does"),
        mcp.WithString("param", mcp.Required(), mcp.Description("Parameter description")),
    )
    server.AddTool(tool, t.handleToolName)
    return nil
}

func (t *Tools) handleToolName(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    param, err := request.RequireString("param")
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    result, err := runCommand(ctx, "external-cli", []string{param})
    if err != nil {
        return mcp.NewToolResultError(fmt.Sprintf("failed: %v", err)), nil
    }
    return mcp.NewToolResultText(result), nil
}
```

### Testing Requirements ⚠️ 80% Coverage Required

**IMPORTANT**: This project enforces 80% test coverage. This is a hard requirement:
- **Overall threshold**: 80% coverage across entire codebase (CI enforces this)
- **Per-package minimum**: 70% for all packages
- **Critical packages**: 90% for k8s, helm, istio, argo (tool wrapper packages)
- **Unit tests only**: Coverage calculated from unit tests (integration tests supplementary)

**How to check coverage locally**:
```bash
make test                      # Runs tests with coverage
make coverage-report           # Generates HTML report (open coverage.html)
go test -cover ./pkg/example   # Check specific package
```

**How to improve coverage**:
1. Run `make coverage-report` and open `coverage.html`
2. Find red (uncovered) lines in your package
3. Write table-driven tests for uncovered functions
4. Run `make test` again to verify improvement
5. See coverage.md for detailed guidance

**Testing Patterns** (follow these strictly):
- **Table-driven tests**: Recommended for all scenarios (see examples in pkg/k8s/*_test.go)
- **Mock external dependencies**: Don't test kubectl/helm directly, test our wrappers
- **Test error paths**: Not just happy path (error handling must be covered)
- **Test edge cases**: Boundary conditions, empty inputs, etc.
- **Integration tests** in `test/integration/`: For testing actual tool execution

**CI Enforcement**: Coverage check is automated in CI pipeline:
- Build fails if overall coverage < 80%
- Build fails if any package < 70%
- Build fails if critical packages < 90%
- Cannot merge without passing coverage check

**See Also**: coverage.md (detailed coverage guide), quickstart.md (developer quick start)

### Code Quality
- Run `make lint` before submitting changes
- Use `go fmt ./...` for formatting (also: `make fmt`)
- Keep functions focused and testable
- Use context for cancellation in long-running operations

## Common Tasks

### Adding a New Tool
1. Create function in appropriate `pkg/[category]/` file
2. Register with MCP SDK using `mcp.NewTool()` in `RegisterTools()`
3. Parse params with `request.RequireString()`, `request.RequireBool()`, etc.
4. Execute using `runCommand()` utility
5. Return results using `mcp.NewToolResultText()` or `mcp.NewToolResultError()`
6. Add unit tests with 80%+ coverage
7. Update README.md tool list

### Debugging
```bash
LOG_LEVEL=debug go run ./cmd          # Debug logging
go run ./cmd --stdio                  # Stdio transport (easier to debug)
```

### Docker Testing
```bash
make docker-build    # Build Docker image
make run             # Run in Docker
```

### Integration with External Tools
Most tools depend on these being installed and in PATH:
- `kubectl` - for k8s tools
- `helm` - for helm tools
- `istioctl` - for istio tools
- `cilium` - for cilium tools

The `KUBECONFIG` environment variable is respected by k8s tools.

## Important Design Notes

### Why CLI Wrappers?
This approach allows:
- Minimal dependencies (no large Go SDK libraries)
- Feature parity with latest CLI versions
- Users can test locally without complex setup
- Easy to keep in sync with upstream tools

### Error Handling
- Always wrap errors with context: `fmt.Errorf("failed to do X: %w", err)`
- Return MCP errors using `mcp.NewToolResultError()` with descriptive messages
- External tool failures are caught and returned as readable errors

### Logging
Use structured logging via logr (see pkg/logger/):
```go
logger := logr.FromContextOrDiscard(ctx)
logger.Info("executing command", "command", cmd, "args", args)
logger.Error(err, "command failed", "command", cmd)
```

## Contribution Standards

From CONTRIBUTION.md - key principles:
- **Principle I**: Use official MCP SDK patterns
- **Principle II**: Type-safe input validation
- **Principle III**: Write tests BEFORE implementation (TDD)
- **Principle IV**: Modular packages under `pkg/`
- **Principle V**: Structured logging and input sanitization

Follow Conventional Commits:
- `feat(scope): description` - New feature
- `fix(scope): description` - Bug fix
- `test(scope): description` - Test changes
- `docs(scope): description` - Documentation

## Active Technologies
- Go 1.x (from go.mod and project setup) + Go standard library, testing libraries (built-in), MCP SDK from `github.com/modelcontextprotocol/go-sdk` (002-test-coverage)
- N/A (test coverage is metadata-only) (002-test-coverage)

## Recent Changes
- 002-test-coverage: Added Go 1.x (from go.mod and project setup) + Go standard library, testing libraries (built-in), MCP SDK from `github.com/modelcontextprotocol/go-sdk`
