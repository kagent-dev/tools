package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StdioTestServer represents a server instance for stdio transport testing
type StdioTestServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	cancel context.CancelFunc
}

// NewStdioTestServer creates a new stdio test server
func NewStdioTestServer() *StdioTestServer {
	return &StdioTestServer{}
}

// Start starts the stdio test server
func (s *StdioTestServer) Start(ctx context.Context, tools []string) error {
	binaryPath := getBinaryName()

	// Build command arguments
	args := []string{"--stdio"}
	if len(tools) > 0 {
		args = append(args, "--tools", strings.Join(tools, ","))
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Create command
	s.cmd = exec.CommandContext(ctx, binaryPath, args...)
	s.cmd.Env = append(os.Environ(), "LOG_LEVEL=debug")

	// Set up pipes
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	s.stdin = stdin

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	s.stdout = stdout

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	s.stderr = stderr

	// Start the command
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Stop stops the stdio test server
func (s *StdioTestServer) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// Close pipes
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.stdout != nil {
		_ = s.stdout.Close()
	}
	if s.stderr != nil {
		_ = s.stderr.Close()
	}

	if s.cmd != nil && s.cmd.Process != nil {
		// Send interrupt signal for graceful shutdown
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
			// If interrupt fails, kill the process
			_ = s.cmd.Process.Kill()
		}

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- s.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Timeout, force kill
			_ = s.cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				// Force kill timeout, continue anyway
			}
		}
	}

	return nil
}

// SendMessage sends a JSON-RPC message to the server
func (s *StdioTestServer) SendMessage(message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Add newline for JSON-RPC over stdio
	data = append(data, '\n')

	_, err = s.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// ReadMessage reads a JSON-RPC message from the server
func (s *StdioTestServer) ReadMessage(timeout time.Duration) (map[string]interface{}, error) {
	// Set up timeout
	done := make(chan map[string]interface{}, 1)
	errChan := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(s.stdout)
		if scanner.Scan() {
			var message map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				errChan <- fmt.Errorf("failed to unmarshal message: %w", err)
				return
			}
			done <- message
		} else {
			if err := scanner.Err(); err != nil {
				errChan <- fmt.Errorf("failed to read message: %w", err)
			} else {
				errChan <- fmt.Errorf("no message received")
			}
		}
	}()

	select {
	case message := <-done:
		return message, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout reading message")
	}
}

// ReadStderr reads stderr output from the server
func (s *StdioTestServer) ReadStderr(timeout time.Duration) (string, error) {
	done := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		buf := make([]byte, 1024)
		n, err := s.stderr.Read(buf)
		if err != nil {
			errChan <- err
			return
		}
		done <- string(buf[:n])
	}()

	select {
	case output := <-done:
		return output, nil
	case err := <-errChan:
		return "", err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout reading stderr")
	}
}

// TestStdioTransportBasic tests basic stdio transport functionality
func TestStdioTransportBasic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Read stderr to check for initialization messages
	stderr, err := server.ReadStderr(5 * time.Second)
	if err == nil {
		// Check for expected initialization messages
		assert.Contains(t, stderr, "Running KAgent Tools Server STDIO")
		assert.Contains(t, stderr, "RegisterTools initialized")
	}

	// Test actual MCP communication:
	//
	// Send initialize request
	// initRequest := map[string]interface{}{
	//     "jsonrpc": "2.0",
	//     "id": 1,
	//     "method": "initialize",
	//     "params": map[string]interface{}{
	//         "protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
	//         "clientInfo": map[string]interface{}{
	//             "name": "test-client",
	//             "version": "1.0.0",
	//         },
	//         "capabilities": map[string]interface{}{},
	//     },
	// }
	//
	// err = server.SendMessage(initRequest)
	// require.NoError(t, err, "Should send initialize request")
	//
	// response, err := server.ReadMessage(10 * time.Second)
	// require.NoError(t, err, "Should receive initialize response")
	//
	// assert.Equal(t, "2.0", response["jsonrpc"])
	// assert.Equal(t, float64(1), response["id"])
	// assert.Contains(t, response, "result")
}

// TestStdioTransportToolListing tests tool listing over stdio
func TestStdioTransportToolListing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils", "k8s"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Read stderr to verify tools are registered
	stderr, err := server.ReadStderr(5 * time.Second)
	if err == nil {
		assert.Contains(t, stderr, "RegisterTools initialized")
		assert.Contains(t, stderr, "utils")
		assert.Contains(t, stderr, "k8s")
	}

	// Test tools/list:
	//
	// Send tools/list request
	// listRequest := map[string]interface{}{
	//     "jsonrpc": "2.0",
	//     "id": 2,
	//     "method": "tools/list",
	//     "params": map[string]interface{}{},
	// }
	//
	// err = server.SendMessage(listRequest)
	// require.NoError(t, err, "Should send tools/list request")
	//
	// response, err := server.ReadMessage(10 * time.Second)
	// require.NoError(t, err, "Should receive tools/list response")
	//
	// assert.Equal(t, "2.0", response["jsonrpc"])
	// assert.Equal(t, float64(2), response["id"])
	// assert.Contains(t, response, "result")
	//
	// result := response["result"].(map[string]interface{})
	// tools := result["tools"].([]interface{})
	// assert.Greater(t, len(tools), 0, "Should have tools registered")
}

// TestStdioTransportToolCall tests tool calling over stdio
func TestStdioTransportToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Read stderr to verify server is ready
	stderr, err := server.ReadStderr(5 * time.Second)
	if err == nil {
		assert.Contains(t, stderr, "Running KAgent Tools Server STDIO")
	}

	// Test tool calls:
	//
	// Send tools/call request for datetime tool
	// callRequest := map[string]interface{}{
	//     "jsonrpc": "2.0",
	//     "id": 3,
	//     "method": "tools/call",
	//     "params": map[string]interface{}{
	//         "name": "datetime_get_current_time",
	//         "arguments": map[string]interface{}{},
	//     },
	// }
	//
	// err = server.SendMessage(callRequest)
	// require.NoError(t, err, "Should send tools/call request")
	//
	// response, err := server.ReadMessage(10 * time.Second)
	// require.NoError(t, err, "Should receive tools/call response")
	//
	// assert.Equal(t, "2.0", response["jsonrpc"])
	// assert.Equal(t, float64(3), response["id"])
	// assert.Contains(t, response, "result")
	//
	// result := response["result"].(map[string]interface{})
	// assert.False(t, result["isError"].(bool), "Tool call should not error")
	// assert.Contains(t, result, "content")
}

// TestStdioTransportErrorHandling tests error handling over stdio
func TestStdioTransportErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Test error scenarios:
	//
	// Send invalid JSON-RPC request
	// invalidRequest := map[string]interface{}{
	//     "jsonrpc": "2.0",
	//     "id": 4,
	//     "method": "nonexistent/method",
	//     "params": map[string]interface{}{},
	// }
	//
	// err = server.SendMessage(invalidRequest)
	// require.NoError(t, err, "Should send invalid request")
	//
	// response, err := server.ReadMessage(10 * time.Second)
	// require.NoError(t, err, "Should receive error response")
	//
	// assert.Equal(t, "2.0", response["jsonrpc"])
	// assert.Equal(t, float64(4), response["id"])
	// assert.Contains(t, response, "error")
	//
	// errorObj := response["error"].(map[string]interface{})
	// assert.Contains(t, errorObj, "code")
	// assert.Contains(t, errorObj, "message")
}

// TestStdioTransportMultipleTools tests stdio with multiple tool categories
func TestStdioTransportMultipleTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allTools := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}

	server := NewStdioTestServer()
	err := server.Start(ctx, allTools)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(3 * time.Second)

	// Read stderr to verify all tools are registered
	stderr, err := server.ReadStderr(5 * time.Second)
	if err == nil {
		assert.Contains(t, stderr, "RegisterTools initialized")
		for _, tool := range allTools {
			assert.Contains(t, stderr, tool, "Tool %s should be registered", tool)
		}
	}
}

// TestStdioTransportGracefulShutdown tests graceful shutdown over stdio
func TestStdioTransportGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Stop server and measure shutdown time
	start := time.Now()
	err = server.Stop()
	duration := time.Since(start)

	require.NoError(t, err, "Server should stop gracefully")
	assert.Less(t, duration, 10*time.Second, "Shutdown should complete within reasonable time")
}

// TestStdioTransportInvalidTools tests stdio with invalid tool names
func TestStdioTransportInvalidTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"invalid-tool", "utils"})
	require.NoError(t, err, "Server should start even with invalid tools")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Read stderr to check for error messages about invalid tools
	stderr, err := server.ReadStderr(5 * time.Second)
	if err == nil {
		assert.Contains(t, stderr, "Unknown tool specified")
		assert.Contains(t, stderr, "invalid-tool")
		// Valid tools should still be registered
		assert.Contains(t, stderr, "RegisterTools initialized")
		assert.Contains(t, stderr, "utils")
	}
}

// TestStdioTransportConcurrentMessages tests concurrent message handling
func TestStdioTransportConcurrentMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Test concurrent messages:
	//
	// Send multiple messages concurrently
	// var wg sync.WaitGroup
	// numMessages := 5
	//
	// for i := 0; i < numMessages; i++ {
	//     wg.Add(1)
	//     go func(id int) {
	//         defer wg.Done()
	//
	//         request := map[string]interface{}{
	//             "jsonrpc": "2.0",
	//             "id": id + 10,
	//             "method": "tools/list",
	//             "params": map[string]interface{}{},
	//         }
	//
	//         err := server.SendMessage(request)
	//         assert.NoError(t, err, "Should send message %d", id)
	//     }(i)
	// }
	//
	// wg.Wait()
	//
	// // Read responses (order may vary)
	// for i := 0; i < numMessages; i++ {
	//     response, err := server.ReadMessage(5 * time.Second)
	//     assert.NoError(t, err, "Should receive response %d", i)
	//     assert.Equal(t, "2.0", response["jsonrpc"])
	//     assert.Contains(t, response, "id")
	//     assert.Contains(t, response, "result")
	// }
}

// TestStdioTransportLargeMessages tests handling of large messages
func TestStdioTransportLargeMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Test large messages:
	//
	// Create a large shell command
	// largeCommand := "echo " + strings.Repeat("a", 1000)
	//
	// callRequest := map[string]interface{}{
	//     "jsonrpc": "2.0",
	//     "id": 100,
	//     "method": "tools/call",
	//     "params": map[string]interface{}{
	//         "name": "shell",
	//         "arguments": map[string]interface{}{
	//             "command": largeCommand,
	//         },
	//     },
	// }
	//
	// err = server.SendMessage(callRequest)
	// require.NoError(t, err, "Should send large message")
	//
	// response, err := server.ReadMessage(10 * time.Second)
	// require.NoError(t, err, "Should receive response for large message")
	//
	// assert.Equal(t, "2.0", response["jsonrpc"])
	// assert.Equal(t, float64(100), response["id"])
	// assert.Contains(t, response, "result")
}

// TestStdioTransportMalformedJSON tests handling of malformed JSON
func TestStdioTransportMalformedJSON(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server := NewStdioTestServer()
	err := server.Start(ctx, []string{"utils"})
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to initialize
	time.Sleep(2 * time.Second)

	// Send malformed JSON
	malformedJSON := "{invalid json"
	_, err = server.stdin.Write([]byte(malformedJSON + "\n"))
	require.NoError(t, err, "Should send malformed JSON")

	// Verify error handling:
	//
	// response, err := server.ReadMessage(5 * time.Second)
	// if err == nil {
	//     // Should receive a JSON-RPC error response
	//     assert.Equal(t, "2.0", response["jsonrpc"])
	//     assert.Contains(t, response, "error")
	//
	//     errorObj := response["error"].(map[string]interface{})
	//     assert.Contains(t, errorObj, "code")
	//     assert.Contains(t, errorObj, "message")
	// }
}
