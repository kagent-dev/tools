package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/mcpcompat/server"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
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

// shellInput provides shell command execution input.
type shellInput struct {
	Command string `json:"command" jsonschema:"required" jsonschema_description:"The shell command to execute"`
}

func shellTool(ctx context.Context, params shellInput) (string, error) {
	// Split command into parts (basic implementation)
	parts := strings.Fields(params.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := parts[0]
	args := parts[1:]

	return commands.NewCommandBuilder(cmd).WithArgs(args...).Execute(ctx)
}

// handleGetCurrentDateTimeTool provides datetime functionality for both MCP and testing
func handleGetCurrentDateTimeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	_ = request
	// Returns the current date and time in ISO 8601 format (RFC3339)
	now := time.Now()
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: now.Format(time.RFC3339)}}}, nil
}

func RegisterTools(s *server.MCPServer, readOnly bool) {
	logger.Get().Info("RegisterTools initialized")

	// Register shell tool - disabled in read-only mode as it allows arbitrary command execution
	if !readOnly {
		mcp.AddTool[shellInput, any](s.Inner(), &mcp.Tool{
			Name:        "shell",
			Description: "Execute shell commands",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input shellInput) (*mcp.CallToolResult, any, error) {
			if strings.TrimSpace(input.Command) == "" {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "command parameter is required"}}}, nil, nil
			}

			result, err := shellTool(ctx, input)
			if err != nil {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
			}

			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result}}}, nil, nil
		})
	}

	// Register datetime tool
	mcp.AddTool[struct{}, any](s.Inner(), &mcp.Tool{
		Name:        "datetime_get_current_time",
		Description: "Returns the current date and time in ISO 8601 format.",
	}, func(ctx context.Context, request *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		if request == nil {
			request = &mcp.CallToolRequest{}
		}
		result, err := handleGetCurrentDateTimeTool(ctx, *request)
		return result, nil, err
	})
}
