package utils

import (
	"context"
	"testing"

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
