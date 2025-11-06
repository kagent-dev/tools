package utils

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestKubeConfigManager(t *testing.T) {
	// Test setting and getting kubeconfig
	testPath := "/test/kubeconfig"
	SetKubeconfig(testPath)

	result := GetKubeconfig()
	if result != testPath {
		t.Errorf("Expected %s, got %s", testPath, result)
	}
}

func TestAddKubeconfigArgs(t *testing.T) {
	// Test with kubeconfig set
	testPath := "/test/kubeconfig"
	SetKubeconfig(testPath)

	args := []string{"get", "pods"}
	result := AddKubeconfigArgs(args)

	expected := []string{"--kubeconfig", testPath, "get", "pods"}
	if len(result) != len(expected) {
		t.Errorf("Expected length %d, got %d", len(expected), len(result))
	}

	for i, arg := range expected {
		if result[i] != arg {
			t.Errorf("Expected arg[%d] = %s, got %s", i, arg, result[i])
		}
	}

	// Test with empty kubeconfig
	SetKubeconfig("")
	result = AddKubeconfigArgs(args)

	if len(result) != len(args) {
		t.Errorf("Expected original args length %d, got %d", len(args), len(result))
	}

	for i, arg := range args {
		if result[i] != arg {
			t.Errorf("Expected arg[%d] = %s, got %s", i, arg, result[i])
		}
	}
}

func TestShellTool(t *testing.T) {
	ctx := context.Background()

	// Test basic command
	params := shellParams{Command: "echo hello"}
	result, err := shellTool(ctx, params)
	if err != nil {
		t.Fatalf("shellTool failed: %v", err)
	}

	if result != "hello\n" {
		t.Errorf("Expected 'hello\\n', got %q", result)
	}

	// Test empty command
	params = shellParams{Command: ""}
	_, err = shellTool(ctx, params)
	if err == nil {
		t.Error("Expected error for empty command")
	}
}

func TestShellToolHandler(t *testing.T) {
	ctx := context.Background()

	// Create a mock server to test tool registration
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	err := RegisterTools(server)
	if err != nil {
		t.Fatalf("RegisterTools failed: %v", err)
	}

	// We can test the underlying shellTool function directly
	params := shellParams{Command: "echo test"}
	result, err := shellTool(ctx, params)
	if err != nil {
		t.Fatalf("shellTool failed: %v", err)
	}

	if result != "test\n" {
		t.Errorf("Expected 'test\\n', got %q", result)
	}
}

func TestRegisterTools(t *testing.T) {
	// Test that RegisterTools doesn't return an error
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	err := RegisterTools(server)
	if err != nil {
		t.Fatalf("RegisterTools failed: %v", err)
	}

	// The server should now have tools registered, but we can't easily test
	// the internal state without more complex setup
}

func TestKubeConfigManagerConcurrency(t *testing.T) {
	// Test concurrent access to kubeconfig manager
	const goroutines = 10
	done := make(chan bool)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Set kubeconfig
			testPath := "/test/path" + string(rune(id))
			SetKubeconfig(testPath)

			// Get kubeconfig
			_ = GetKubeconfig()

			// Add kubeconfig args
			args := []string{"get", "pods"}
			_ = AddKubeconfigArgs(args)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestShellToolWithMultipleArgs(t *testing.T) {
	ctx := context.Background()

	// Test command with multiple arguments
	params := shellParams{Command: "echo arg1 arg2 arg3"}
	result, err := shellTool(ctx, params)
	if err != nil {
		t.Fatalf("shellTool failed: %v", err)
	}

	if result != "arg1 arg2 arg3\n" {
		t.Errorf("Expected 'arg1 arg2 arg3\\n', got %q", result)
	}
}

func TestShellToolWithInvalidCommand(t *testing.T) {
	ctx := context.Background()

	// Test with non-existent command
	params := shellParams{Command: "nonexistentcommand12345"}
	_, err := shellTool(ctx, params)
	if err == nil {
		t.Error("Expected error for non-existent command")
	}
}

func TestAddKubeconfigArgsWithEmptyArgs(t *testing.T) {
	testPath := "/test/kubeconfig"
	SetKubeconfig(testPath)

	// Test with empty args slice
	args := []string{}
	result := AddKubeconfigArgs(args)

	expected := []string{"--kubeconfig", testPath}
	if len(result) != len(expected) {
		t.Errorf("Expected length %d, got %d", len(expected), len(result))
	}

	// Test with nil args
	result = AddKubeconfigArgs(nil)
	if len(result) != len(expected) {
		t.Errorf("Expected length %d for nil args, got %d", len(expected), len(result))
	}
}

// TestRegisterToolsUtils verifies that RegisterTools correctly registers all utility tools
func TestRegisterToolsUtils(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

	err := RegisterTools(server)
	if err != nil {
		t.Errorf("RegisterTools should not return an error, got: %v", err)
	}

	// Note: In the actual implementation, we can't easily verify tool registration
	// without accessing internal server state. This test verifies the function
	// runs without errors, which covers the registration logic paths.
}

// TestShellToolMCPHandler tests the shell tool MCP handler function
func TestShellToolMCPHandler(t *testing.T) {
	ctx := context.Background()

	t.Run("valid command", func(t *testing.T) {
		params := shellParams{Command: "echo hello"}
		result, err := shellTool(ctx, params)
		if err != nil {
			t.Errorf("shell tool failed: %v", err)
		}
		if result != "hello\n" {
			t.Errorf("expected 'hello\\n', got %q", result)
		}
	})

	t.Run("command with multiple arguments", func(t *testing.T) {
		params := shellParams{Command: "echo multiple args"}
		result, err := shellTool(ctx, params)
		if err != nil {
			t.Errorf("shell tool failed: %v", err)
		}
		if result != "multiple args\n" {
			t.Errorf("expected 'multiple args\\n', got %q", result)
		}
	})

	t.Run("failing command", func(t *testing.T) {
		params := shellParams{Command: "false"}
		_, err := shellTool(ctx, params)
		if err == nil {
			t.Error("expected error for 'false' command")
		}
	})
}

// TestHandleShellTool tests the MCP shell tool handler with JSON arguments
func TestHandleShellTool(t *testing.T) {
	ctx := context.Background()

	t.Run("valid command via handler", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"command": "echo test"}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleShellTool(ctx, request)
		if err != nil {
			t.Errorf("handleShellTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			if len(result.Content) == 0 {
				t.Error("expected content in result")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "test\n" {
					t.Errorf("expected 'test\\n', got %q", textContent.Text)
				}
			}
		} else {
			t.Error("expected non-nil result")
		}
	})

	t.Run("invalid JSON arguments", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("invalid json"),
			},
		}

		result, err := handleShellTool(ctx, request)
		if err != nil {
			t.Errorf("handleShellTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for invalid JSON")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "failed to parse arguments" {
					t.Errorf("expected 'failed to parse arguments', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("missing command parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleShellTool(ctx, request)
		if err != nil {
			t.Errorf("handleShellTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for missing command")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "command parameter is required" {
					t.Errorf("expected 'command parameter is required', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("empty command parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"command": ""}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleShellTool(ctx, request)
		if err != nil {
			t.Errorf("handleShellTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for empty command")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "command parameter is required" {
					t.Errorf("expected 'command parameter is required', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("non-string command parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"command": 123}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleShellTool(ctx, request)
		if err != nil {
			t.Errorf("handleShellTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for non-string command")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "command parameter is required" {
					t.Errorf("expected 'command parameter is required', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("command execution error", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"command": "nonexistentcommand12345"}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleShellTool(ctx, request)
		if err != nil {
			t.Errorf("handleShellTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for non-existent command")
			}
			if len(result.Content) == 0 {
				t.Error("expected error content in result")
			}
		}
	})
}

// TestHandleEchoTool tests the MCP echo tool handler with JSON arguments
func TestHandleEchoTool(t *testing.T) {
	ctx := context.Background()

	t.Run("valid message via handler", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"message": "Hello, World!"}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			if len(result.Content) == 0 {
				t.Error("expected content in result")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "Hello, World!" {
					t.Errorf("expected 'Hello, World!', got %q", textContent.Text)
				}
			}
		} else {
			t.Error("expected non-nil result")
		}
	})

	t.Run("empty message", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"message": ""}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool should not return Go error: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result for empty message")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "" {
					t.Errorf("expected empty string, got %q", textContent.Text)
				}
			}
		}
	})

	t.Run("message with special characters", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"message": "Hello\nWorld\tTest"}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "Hello\nWorld\tTest" {
					t.Errorf("expected 'Hello\\nWorld\\tTest', got %q", textContent.Text)
				}
			}
		}
	})

	t.Run("invalid JSON arguments", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("invalid json"),
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for invalid JSON")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "failed to parse arguments" {
					t.Errorf("expected 'failed to parse arguments', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("missing message parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for missing message")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "message parameter is required and must be a string" {
					t.Errorf("expected 'message parameter is required and must be a string', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("non-string message parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"message": 123}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for non-string message")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "message parameter is required and must be a string" {
					t.Errorf("expected 'message parameter is required and must be a string', got %q", textContent.Text)
				}
			} else {
				t.Error("expected error content in result")
			}
		}
	})

	t.Run("message with unicode characters", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"message": "Hello 🌍 世界"}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleEchoTool(ctx, request)
		if err != nil {
			t.Errorf("handleEchoTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "Hello 🌍 世界" {
					t.Errorf("expected 'Hello 🌍 世界', got %q", textContent.Text)
				}
			}
		}
	})
}

// TestHandleSleepTool tests the MCP sleep tool handler with JSON arguments
func TestHandleSleepTool(t *testing.T) {
	ctx := context.Background()

	t.Run("valid duration integer", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"duration": 1}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		start := time.Now()
		result, err := handleSleepTool(ctx, request)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("handleSleepTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			if len(result.Content) == 0 {
				t.Error("expected content in result")
			}
			// Verify we slept approximately 1 second (allow some tolerance)
			if duration < 900*time.Millisecond || duration > 1100*time.Millisecond {
				t.Errorf("expected sleep duration ~1s, got %v", duration)
			}
		} else {
			t.Error("expected non-nil result")
		}
	})

	t.Run("valid duration float", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"duration": 0.1}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		start := time.Now()
		result, err := handleSleepTool(ctx, request)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("handleSleepTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			// Verify we slept approximately 0.1 seconds
			if duration < 80*time.Millisecond || duration > 150*time.Millisecond {
				t.Errorf("expected sleep duration ~100ms, got %v", duration)
			}
		}
	})

	t.Run("zero duration", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"duration": 0}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleSleepTool(ctx, request)
		if err != nil {
			t.Errorf("handleSleepTool should not return Go error: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result for zero duration")
			}
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"duration": -1}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleSleepTool(ctx, request)
		if err != nil {
			t.Errorf("handleSleepTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for negative duration")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "duration must be non-negative" {
					t.Errorf("expected 'duration must be non-negative', got %q", textContent.Text)
				}
			}
		}
	})

	t.Run("invalid JSON arguments", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("invalid json"),
			},
		}

		result, err := handleSleepTool(ctx, request)
		if err != nil {
			t.Errorf("handleSleepTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for invalid JSON")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "failed to parse arguments" {
					t.Errorf("expected 'failed to parse arguments', got %q", textContent.Text)
				}
			}
		}
	})

	t.Run("missing duration parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleSleepTool(ctx, request)
		if err != nil {
			t.Errorf("handleSleepTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for missing duration")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "duration parameter is required and must be a number" {
					t.Errorf("expected 'duration parameter is required and must be a number', got %q", textContent.Text)
				}
			}
		}
	})

	t.Run("non-number duration parameter", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"duration": "invalid"}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleSleepTool(ctx, request)
		if err != nil {
			t.Errorf("handleSleepTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for non-number duration")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok && textContent.Text != "duration parameter is required and must be a number" {
					t.Errorf("expected 'duration parameter is required and must be a number', got %q", textContent.Text)
				}
			}
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		cmdArgs := map[string]interface{}{"duration": 10}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleSleepTool(ctx, request)
		if err != nil {
			t.Errorf("handleSleepTool should not return Go error: %v", err)
		}
		if result != nil {
			if !result.IsError {
				t.Error("expected error result for cancelled context")
			}
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
					// The improved error message includes actual elapsed time
					if !strings.Contains(textContent.Text, "sleep cancelled after") || !strings.Contains(textContent.Text, "requested 10.00 seconds") {
						t.Errorf("expected cancellation message with timing info, got %q", textContent.Text)
					}
				}
			}
		}
	})

	t.Run("decimal duration", func(t *testing.T) {
		cmdArgs := map[string]interface{}{"duration": 0.5}
		argsJSON, _ := json.Marshal(cmdArgs)
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		start := time.Now()
		result, err := handleSleepTool(ctx, request)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("handleSleepTool failed: %v", err)
		}
		if result != nil {
			if result.IsError {
				t.Error("expected success result")
			}
			// Verify we slept approximately 0.5 seconds
			if duration < 400*time.Millisecond || duration > 600*time.Millisecond {
				t.Errorf("expected sleep duration ~500ms, got %v", duration)
			}
		}
	})
}
