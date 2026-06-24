package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	mcp "github.com/kagent-dev/tools/internal/mcp"
)

// KubeConfigManager manages kubeconfig path with thread safety
type KubeConfigManager struct {
	mu             sync.RWMutex
	kubeconfigPath string
}

// globalKubeConfigManager is the singleton instance
var globalKubeConfigManager = &KubeConfigManager{}

// SetKubeconfig sets the global kubeconfig path in a thread-safe manner
func SetKubeconfig(path string) {
	globalKubeConfigManager.mu.Lock()
	defer globalKubeConfigManager.mu.Unlock()

	globalKubeConfigManager.kubeconfigPath = path
	logger.Get().Info("Setting shared kubeconfig", "path", path)
}

// GetKubeconfig returns the global kubeconfig path in a thread-safe manner
func GetKubeconfig() string {
	globalKubeConfigManager.mu.RLock()
	defer globalKubeConfigManager.mu.RUnlock()

	return globalKubeConfigManager.kubeconfigPath
}

// AddKubeconfigArgs adds kubeconfig arguments to command args if configured
func AddKubeconfigArgs(args []string) []string {
	kubeconfigPath := GetKubeconfig()
	if kubeconfigPath != "" {
		return append([]string{"--kubeconfig", kubeconfigPath}, args...)
	}
	return args
}

// shellParams is the typed input for the shell tool.
type shellParams struct {
	Command string `json:"command" jsonschema:"The shell command to execute"`
}

func shellTool(ctx context.Context, params shellParams) (string, error) {
	// Split command into parts (basic implementation)
	parts := strings.Fields(params.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := parts[0]
	args := parts[1:]

	return commands.NewCommandBuilder(cmd).WithArgs(args...).Execute(ctx)
}

func handleShellTool(ctx context.Context, request *mcp.CallToolRequest, in shellParams) (*mcp.CallToolResult, any, error) {
	if in.Command == "" {
		return mcp.NewToolResultError("command parameter is required"), nil, nil
	}

	result, err := shellTool(ctx, in)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

// datetimeInput is the (empty) typed input for the datetime tool.
type datetimeInput struct{}

// handleGetCurrentDateTimeTool provides datetime functionality for both MCP and testing
func handleGetCurrentDateTimeTool(ctx context.Context, request *mcp.CallToolRequest, in datetimeInput) (*mcp.CallToolResult, any, error) {
	// Returns the current date and time in ISO 8601 format (RFC3339)
	// This matches the Python implementation: datetime.datetime.now().isoformat()
	now := time.Now()
	return mcp.NewToolResultText(now.Format(time.RFC3339)), nil, nil
}

func RegisterTools(s *mcp.Server, readOnly bool) {
	logger.Get().Info("RegisterTools initialized")

	// Register shell tool - disabled in read-only mode as it allows arbitrary command execution
	if !readOnly {
		mcp.AddTool(s, "utils", &mcp.Tool{
			Name:        "shell",
			Description: "Execute shell commands",
		}, handleShellTool)
	}

	// Register datetime tool
	mcp.AddTool(s, "utils", &mcp.Tool{
		Name:        "datetime_get_current_time",
		Description: "Returns the current date and time in ISO 8601 format.",
	}, handleGetCurrentDateTimeTool)

	// Note: LLM Tool implementation would go here if needed
}
