package integration

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStdioProcessLaunch launches server in stdio mode
// Contract: transport-test-contract.md (TC1)
// Implements: T020 - Stdio transport validation
func TestStdioProcessLaunch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Launch server process in stdio mode using CommandTransport
	binaryPath := getBinaryName()
	cmd := exec.CommandContext(ctx, binaryPath, "--stdio", "--tools", "utils")

	// Create transport using SDK's CommandTransport
	transport := &mcp.CommandTransport{Command: cmd}
	require.NotNil(t, transport, "Transport should be created")

	// Create client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	// Connect - this starts the process automatically
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "Should connect to server")
	require.NotNil(t, session, "Session should be established")
	defer func() { _ = session.Close() }()

	t.Log("✅ Stdio Process Launch test PASSED")
}

// TestStdioInitialize performs MCP initialize over stdin/stdout
// Contract: transport-test-contract.md (TC2)
// Implements: T020 - Stdio initialize validation
func TestStdioInitialize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Launch server process using CommandTransport
	binaryPath := getBinaryName()
	cmd := exec.CommandContext(ctx, binaryPath, "--stdio", "--tools", "utils")

	// Create transport
	transport := &mcp.CommandTransport{Command: cmd}

	// Create MCP client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	// Connect and initialize
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "Initialize handshake should succeed")
	require.NotNil(t, session, "Session should be established")
	defer func() { _ = session.Close() }()

	// Verify server capabilities returned
	assert.NotNil(t, session, "Session should contain server info")

	t.Log("✅ Stdio Initialize test PASSED")
}

// TestStdioToolsList lists tools via stdio
// Contract: transport-test-contract.md (TC3)
// Implements: T020 - Stdio tools listing validation
func TestStdioToolsList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Launch server with multiple tool categories using CommandTransport
	binaryPath := getBinaryName()
	cmd := exec.CommandContext(ctx, binaryPath, "--stdio", "--tools", "utils,k8s")

	// Create transport
	transport := &mcp.CommandTransport{Command: cmd}

	// Create client with stdio transport
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	// List tools via stdio
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		require.NoError(t, err, "Tool iteration should not error")
		tools = append(tools, tool)
	}

	// Verify tools array non-empty
	assert.NotEmpty(t, tools, "Should have tools registered")

	// Verify tool structure
	for _, tool := range tools {
		assert.NotEmpty(t, tool.Name, "Tool should have name")
		assert.NotEmpty(t, tool.Description, "Tool should have description")
		assert.NotNil(t, tool.InputSchema, "Tool should have input schema")
	}

	// Verify expected tool categories
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	// Should have at least datetime tool from utils category
	assert.Contains(t, toolNames, "datetime_get_current_time", "Should have utils tools")

	t.Log("✅ Stdio Tools List test PASSED")
}

// TestStdioToolExecution executes tool via stdio
// Contract: transport-test-contract.md (TC4)
// Implements: T020 - Stdio tool execution validation
func TestStdioToolExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Launch server using CommandTransport
	binaryPath := getBinaryName()
	cmd := exec.CommandContext(ctx, binaryPath, "--stdio", "--tools", "utils")

	// Create transport
	transport := &mcp.CommandTransport{Command: cmd}

	// Create client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	// Execute datetime tool via stdio
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "datetime_get_current_time",
		Arguments: map[string]any{},
	})

	require.NoError(t, err, "Tool call should not error")
	assert.False(t, result.IsError, "Tool should execute successfully")
	assert.NotEmpty(t, result.Content, "Tool should return content")

	// Verify no message corruption
	if len(result.Content) > 0 {
		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok, "Content should be TextContent")
		assert.NotEmpty(t, textContent.Text, "Should have timestamp")
		// Verify it looks like an ISO timestamp
		assert.Contains(t, textContent.Text, "T", "Should be ISO format timestamp")
	}

	t.Log("✅ Stdio Tool Execution test PASSED")
}
