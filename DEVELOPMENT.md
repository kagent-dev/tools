# Development Guide

This document provides comprehensive development guidelines for the KAgent Tools Go project.

## Prerequisites

### Required Tools

- **Go 1.24+** - Primary development language
- **Docker** - For containerization and testing
- **Make** - Build automation
- **Git** - Version control

### Optional External Tools

These tools enhance functionality but aren't required for basic development:

- `kubectl` - Kubernetes CLI for k8s tools
- `helm` - Helm package manager for helm tools  
- `istioctl` - Istio service mesh CLI for istio tools
- `cilium` - Cilium CLI for cilium tools

## Project Structure

```
.
├── cmd/
│   └── main.go              # Entry point and MCP server
├── internal/
│   └── version/             # Version and build metadata
├── pkg/
│   ├── k8s/                 # Kubernetes tools
│   ├── helm/                # Helm package manager tools
│   ├── istio/               # Istio service mesh tools
│   ├── cilium/              # Cilium CNI tools
│   ├── argo/                # Argo Rollouts tools
│   ├── prometheus/          # Prometheus monitoring tools
│   ├── utils/               # Common utilities
│   └── logger/              # Structured logging
├── tests/                   # Integration tests
├── Makefile                 # Build automation
├── go.mod                   # Go module definition
└── go.sum                   # Go module checksums
```

## Development Workflow

### 1. Environment Setup

```bash
# Clone the repository
git clone <repository-url>
cd kagent-tools

# Install dependencies
go mod download

# Verify setup
go version
make help
```

### 2. Local Development

```bash
# Build the project
make build

# Run tests
make test

# Run with verbose output
make test-verbose

# Format code
make fmt

# Lint code
make lint

# Fix linting issues
make lint-fix
```

### 3. Running the Server

```bash
# Build and run
make build
./bin/kagent-tools

# Or run directly
go run ./cmd
```

The server starts an MCP server using SSE (Server-Sent Events) transport.

## Code Quality Standards

### Linting and Formatting

```bash
# Format all Go files
go fmt ./...
make fmt

# Run comprehensive linting
make lint

# Fix auto-fixable lint issues
make lint-fix

# Run go vet
make vet

# Security vulnerability check
make govulncheck
```

### Testing Requirements

- **Minimum 80% test coverage** - Use `go test -cover` to verify
- **Unit tests** for all public functions
- **Integration tests** for complex workflows
- **Table-driven tests** for multiple scenarios
- **Mock external dependencies** appropriately

```bash
# Run tests with coverage
go test -v -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package tests
go test -v ./pkg/k8s
```

### Code Organization

- **Single responsibility** - Each package has one clear purpose
- **Interface segregation** - Keep interfaces small and focused
- **Dependency injection** - Use interfaces for testability
- **Error handling** - Always handle errors explicitly
- **Context usage** - Use `context.Context` for cancellation

## Architecture Guidelines

### Tool Implementation Pattern

Each tool category follows this pattern:

```go
// pkg/[category]/[category].go
package category

import (
    "context"
    "github.com/mark3labs/mcp-go/pkg/mcp"
)

type Tools struct {
    // dependencies
}

func NewTools() *Tools {
    return &Tools{}
}

func (t *Tools) RegisterTools(server *mcp.Server) {
    server.RegisterTool("tool_name", t.handleTool)
}

func (t *Tools) handleTool(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
    // implementation
}
```

### Error Handling Standards

```go
// Good: Wrap errors with context
if err != nil {
    return nil, fmt.Errorf("failed to execute kubectl command: %w", err)
}

// Good: Use custom error types when appropriate
type ValidationError struct {
    Field string
    Value interface{}
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("invalid %s: %v", e.Field, e.Value)
}
```

### Logging Standards

```go
import "github.com/go-logr/logr"

// Use structured logging
logger := logr.FromContextOrDiscard(ctx)
logger.Info("executing command", "command", cmd, "args", args)
logger.Error(err, "command failed", "command", cmd)
```

## Testing Guidelines

### Unit Testing

```go
func TestToolFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    interface{}
        expected interface{}
        wantErr  bool
    }{
        {
            name:     "valid input",
            input:    validInput,
            expected: expectedOutput,
            wantErr:  false,
        },
        {
            name:    "invalid input",
            input:   invalidInput,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := ToolFunction(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Integration Testing

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    
    // Setup test environment
    ctx := context.Background()
    tools := NewTools()
    
    // Test actual functionality
    result, err := tools.ExecuteCommand(ctx, "kubectl", []string{"version"})
    assert.NoError(t, err)
    assert.Contains(t, result, "Client Version")
}
```

## Build and Deployment

### Local Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Build specific platform
make bin/kagent-tools-linux-amd64
```

### Docker

```bash
# Build Docker image
make docker-build

# Run in Docker
make run
```

## Environment Configuration

### Environment Variables

The application respects these environment variables:

- `KUBECONFIG` - Kubernetes configuration file path
- `PROMETHEUS_URL` - Prometheus server URL
- `GRAFANA_URL` - Grafana server URL  
- `GRAFANA_API_KEY` - Grafana API authentication key
- `LOG_LEVEL` - Logging level (debug, info, warn, error)

### Configuration Files

- `go.mod` - Go module dependencies
- `Makefile` - Build automation
- `.golangci.yml` - Linting configuration
- `Dockerfile` - Container build instructions

## Debugging

### Local Debugging

```bash
# Run with debug logging
LOG_LEVEL=debug go run ./cmd

# Use delve debugger
dlv debug ./cmd

# Profile the application
go tool pprof http://localhost:6060/debug/pprof/profile
```

### Docker Debugging

```bash
# Run container with debug shell
docker run -it --entrypoint /bin/sh kagent-tools:latest

# Check container logs
docker logs <container-id>
```

## Performance Considerations

### Optimization Guidelines

- **Avoid unnecessary allocations** in hot paths
- **Use connection pooling** for external services
- **Implement caching** for expensive operations
- **Use context timeouts** for external calls
- **Profile regularly** to identify bottlenecks

### Memory Management

```go
// Good: Reuse slices when possible
var buf []byte
if cap(buf) < needed {
    buf = make([]byte, needed)
}
buf = buf[:needed]

// Good: Use sync.Pool for expensive objects
var pool = sync.Pool{
    New: func() interface{} {
        return &ExpensiveObject{}
    },
}
```

## Security Guidelines

### Input Validation

```go
// Always validate inputs
func validateInput(input string) error {
    if input == "" {
        return errors.New("input cannot be empty")
    }
    if len(input) > maxLength {
        return errors.New("input too long")
    }
    return nil
}
```

### Secure Defaults

- Use HTTPS for all external communications
- Validate all user inputs
- Handle sensitive data appropriately
- Keep dependencies updated
- Use secure random number generation

## Contributing

### Code Review Checklist

- [ ] Code follows Go best practices
- [ ] Tests are included and passing
- [ ] Code coverage meets minimum requirements
- [ ] Linting passes without errors
- [ ] Documentation is updated
- [ ] Security considerations addressed
- [ ] Performance impact considered

### Commit Guidelines

```bash
# Format: <type>(<scope>): <description>
git commit -m "feat(k8s): add pod scaling functionality"
git commit -m "fix(helm): handle missing repository error"
git commit -m "docs(readme): update installation instructions"
```

## Troubleshooting

### Common Issues

1. **Build failures**: Check Go version and dependencies
2. **Test failures**: Verify external tool availability
3. **Linting errors**: Run `make lint-fix` for auto-fixes
4. **Import errors**: Run `go mod tidy` to clean dependencies

### Getting Help

- Check existing issues in the repository
- Review the CLAUDE.md file for project-specific guidance
- Consult Go documentation and best practices
- Ask questions in code reviews or team discussions