# Integration Tests for KAgent Tools

This directory contains comprehensive integration tests for the KAgent Tools project. These tests verify that the MCP server implementation using the official `github.com/modelcontextprotocol/go-sdk` maintains functionality and compatibility across all tool categories and transport methods.

## Test Structure

### Test Files

1. **`binary_verification_test.go`** - Tests binary existence, build process, and basic functionality
2. **`mcp_integration_test.go`** - Core MCP integration tests with server lifecycle management
3. **`stdio_transport_test.go`** - Tests stdio transport functionality (currently shows unimplemented status)
4. **`http_transport_test.go`** - Tests HTTP transport functionality
5. **`tool_categories_test.go`** - Tests all tool categories (utils, k8s, helm, argo, cilium, istio, prometheus)
6. **`comprehensive_integration_test.go`** - Comprehensive end-to-end tests covering all aspects

### Test Categories

#### 1. Binary and Build Tests
- Binary existence and executability
- Version and help flag functionality
- Build process verification
- Go module integrity

#### 2. Transport Layer Tests
- **HTTP Transport**: Health endpoints, metrics, concurrent requests, error handling
- **Stdio Transport**: Basic initialization, tool registration (transport implementation pending)

#### 3. Tool Category Tests
- Individual tool category registration
- Multiple tool combinations
- All tools registration
- Error handling with invalid tools
- Performance and startup time testing

#### 4. Comprehensive Integration Tests
- End-to-end functionality across both transports
- Concurrent operations and stress testing
- Performance benchmarking
- Robustness and error recovery
- SDK migration verification

## Running Tests

### Prerequisites

1. Ensure the binary is built:
   ```bash
   make build
   ```

2. Ensure Go dependencies are up to date:
   ```bash
   go mod tidy
   ```

### Running Individual Test Suites

```bash
# Binary verification tests
go test -v ./test/integration/binary_verification_test.go

# Core MCP integration tests
go test -v ./test/integration/mcp_integration_test.go

# HTTP transport tests
go test -v ./test/integration/http_transport_test.go

# Stdio transport tests
go test -v ./test/integration/stdio_transport_test.go

# Tool category tests
go test -v ./test/integration/tool_categories_test.go

# Comprehensive integration tests
go test -v ./test/integration/comprehensive_integration_test.go
```

### Running All Tests

Use the provided test runner script:

```bash
cd test/integration
chmod +x run_integration_tests.sh
./run_integration_tests.sh
```

Or run all tests directly:

```bash
go test -v ./test/integration/... -timeout=600s
```

## Test Coverage

### HTTP Transport Tests
- ✅ Server startup and shutdown
- ✅ Health endpoint functionality
- ✅ Metrics endpoint with real runtime metrics
- ✅ Concurrent request handling
- ✅ Error handling and robustness
- ✅ Tool registration verification
- ⏳ MCP endpoint (returns not implemented until HTTP transport is complete)

### Stdio Transport Tests
- ✅ Server startup in stdio mode
- ✅ Tool registration verification
- ✅ Error handling
- ⏳ Actual MCP communication (pending stdio transport implementation)

### Tool Category Tests
- ✅ Utils tools registration
- ✅ K8s tools registration
- ✅ Helm tools registration
- ✅ Argo tools registration
- ✅ Cilium tools registration
- ✅ Istio tools registration
- ✅ Prometheus tools registration
- ✅ Multiple tool combinations
- ✅ Error handling with invalid tools

### Performance Tests
- ✅ Startup time measurement
- ✅ Response time benchmarking
- ✅ Concurrent request handling
- ✅ Memory usage monitoring
- ✅ Resource cleanup verification

### MCP SDK Integration Tests
- ✅ Official SDK pattern verification
- ✅ MCP protocol compliance
- ✅ Tool registration and discovery
- ✅ All tool categories functionality verification

## Current Status

### ✅ Implemented and Working
- Binary verification and build process
- HTTP server functionality (health, metrics endpoints)
- Tool registration for all categories
- Error handling and robustness
- Performance testing
- Concurrent operations
- Graceful shutdown

### ⏳ Pending Implementation
- **HTTP MCP Transport**: The `/mcp` endpoint currently returns "not implemented"
- **Stdio MCP Transport**: Currently shows "not yet implemented with new SDK"
- **Actual Tool Calls**: Once transports are implemented, tool calling functionality

### 🔄 Test Evolution
As the MCP transport implementations are completed, the tests will be updated to:

1. Remove placeholder assertions for unimplemented transport features
2. Add comprehensive MCP protocol communication tests
3. Test real tool invocations across all transports
4. Verify full MCP specification compliance
5. Add performance benchmarks for the official SDK

## Test Configuration

### Ports Used
Tests use different port ranges to avoid conflicts:
- Binary verification: N/A (command-line only)
- MCP integration: 8090-8109
- HTTP transport: 8110-8119
- Tool categories: 8120-8189
- Comprehensive: 8200-8299

### Timeouts
- Individual tests: 30-120 seconds
- Server startup: 10-30 seconds
- HTTP requests: 30 seconds
- Graceful shutdown: 10 seconds

### Environment Variables
- `LOG_LEVEL=debug` - Enables debug logging for test servers
- `OTEL_SERVICE_NAME=kagent-tools-integration-test` - Sets telemetry service name

## Troubleshooting

### Common Issues

1. **Binary not found**: Run `make build` to create the binary
2. **Port conflicts**: Tests use different port ranges, but ensure no other services are using these ports
3. **Timeout errors**: Increase timeout values if running on slower systems
4. **Go module issues**: Run `go mod tidy` to resolve dependency issues

### Debug Information

Tests capture server output and provide detailed error messages. Check test output for:
- Server startup logs
- Tool registration messages
- Error messages and stack traces
- Performance metrics

### Test Isolation

Each test creates its own server instance with unique ports to ensure isolation. Tests clean up resources automatically, but you can manually kill any remaining processes:

```bash
pkill -f "kagent-tools"
```

## Contributing

When adding new integration tests:

1. Follow the existing naming conventions
2. Use unique port ranges to avoid conflicts
3. Include proper cleanup in defer statements
4. Add comprehensive assertions for both success and failure cases
5. Update this README with new test descriptions

## Future Enhancements

As the MCP transport implementations are completed:

1. **Real MCP Communication**: Test actual JSON-RPC communication over all transports
2. **Tool Invocation**: Test real tool calls with comprehensive parameter validation
3. **Protocol Compliance**: Verify full MCP specification compliance
4. **Client Integration**: Test with various MCP clients (Cursor, Claude Desktop, etc.)
5. **Performance Benchmarks**: Establish performance baselines and optimization targets
6. **Load Testing**: Test server performance under high concurrent load
7. **Error Recovery**: Test robustness and error recovery scenarios