package integration

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBinaryExists verifies the server binary exists and is executable
func TestBinaryExists(t *testing.T) {
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()

	// Check if server binary exists
	_, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		// Try to build the binary
		t.Log("Binary not found, attempting to build...")
		cmd := exec.Command("make", "build")
		cmd.Dir = "../.."
		output, buildErr := cmd.CombinedOutput()
		if buildErr != nil {
			t.Logf("Build output: %s", string(output))
			t.Skipf("Server binary not found and build failed: %v. Run 'make build' first.", buildErr)
		}

		// Check again after build
		_, err = os.Stat(binaryPath)
	}
	require.NoError(t, err, "Server binary should exist at %s", binaryPath)

	// Test --help flag
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Server should respond to --help flag")

	outputStr := string(output)
	assert.Contains(t, outputStr, "KAgent tool server")
	assert.Contains(t, outputStr, "--port")
	assert.Contains(t, outputStr, "--stdio")
	assert.Contains(t, outputStr, "--tools")
	assert.Contains(t, outputStr, "--kubeconfig")
}

// TestVersionFlag tests the version flag functionality
func TestVersionFlag(t *testing.T) {
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()

	// Check if server binary exists
	_, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		t.Skip("Server binary not found, skipping test. Run 'make build' first.")
	}
	require.NoError(t, err, "Server binary should exist")

	// Test --version flag
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Server should respond to --version flag")

	outputStr := string(output)
	assert.Contains(t, outputStr, "kagent-tools-server")
	assert.Contains(t, outputStr, "Version:")
	assert.Contains(t, outputStr, "Git Commit:")
	assert.Contains(t, outputStr, "Build Date:")
	assert.Contains(t, outputStr, "Go Version:")
	assert.Contains(t, outputStr, "OS/Arch:")
}

// TestBinaryExecutable tests that the binary is executable and starts correctly
func TestBinaryExecutable(t *testing.T) {
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()

	// Check if server binary exists
	_, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		t.Skip("Server binary not found, skipping test. Run 'make build' first.")
	}
	require.NoError(t, err, "Server binary should exist")

	// Test that binary starts and exits gracefully with invalid flag
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--invalid-flag")
	output, err := cmd.CombinedOutput()

	// Should exit with error due to invalid flag, but should not crash
	assert.Error(t, err, "Should exit with error for invalid flag")

	outputStr := string(output)
	// Should show help or error message, not crash
	assert.True(t,
		len(outputStr) > 0,
		"Should produce some output, not crash silently")
}

// TestBuildProcess tests the build process if binary doesn't exist
func TestBuildProcess(t *testing.T) {
	// This test ensures the build process works correctly
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()

	// If binary doesn't exist, try building it
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Log("Binary not found, testing build process...")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "make", "build")
		cmd.Dir = "../.."
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Logf("Build output: %s", string(output))
			t.Errorf("Build process failed: %v", err)
			return
		}

		t.Log("Build process completed successfully")

		// Verify binary was created
		_, err = os.Stat(binaryPath)
		assert.NoError(t, err, "Binary should exist after build")
	} else {
		t.Log("Binary already exists, skipping build test")
	}
}

// TestGoModIntegrity tests that go.mod is properly configured
func TestGoModIntegrity(t *testing.T) {
	// Check that go.mod exists
	_, err := os.Stat("../../go.mod")
	require.NoError(t, err, "go.mod should exist")

	// Test go mod tidy
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("go mod tidy output: %s", string(output))
	}
	assert.NoError(t, err, "go mod tidy should succeed")

	// Test go mod verify
	cmd = exec.CommandContext(ctx, "go", "mod", "verify")
	cmd.Dir = "../.."
	output, err = cmd.CombinedOutput()

	if err != nil {
		t.Logf("go mod verify output: %s", string(output))
	}
	assert.NoError(t, err, "go mod verify should succeed")
}

// TestDependencyVersions tests that required dependencies are present
func TestDependencyVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Check for MCP SDK dependency
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "github.com/modelcontextprotocol/go-sdk")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	require.NoError(t, err, "MCP SDK dependency should be present")

	outputStr := string(output)
	assert.Contains(t, outputStr, "github.com/modelcontextprotocol/go-sdk")
	assert.Contains(t, outputStr, "v0.") // Should have a version

	// Check for other critical dependencies
	dependencies := []string{
		"github.com/spf13/cobra",
		"github.com/stretchr/testify",
		"go.opentelemetry.io/otel",
	}

	for _, dep := range dependencies {
		cmd = exec.CommandContext(ctx, "go", "list", "-m", dep)
		cmd.Dir = "../.."
		output, err = cmd.CombinedOutput()

		assert.NoError(t, err, "Dependency %s should be present", dep)
		if err == nil {
			assert.Contains(t, string(output), dep)
		}
	}
}
