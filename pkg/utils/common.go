package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// shellTool provides shell command execution functionality
type shellParams struct {
	Command string `json:"command" description:"The shell command to execute"`
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

// handleGetCurrentDateTimeTool provides datetime functionality for both MCP and testing
func handleGetCurrentDateTimeTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Returns the current date and time in ISO 8601 format (RFC3339)
	// This matches the Python implementation: datetime.datetime.now().isoformat()
	now := time.Now()
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: now.Format(time.RFC3339)}},
	}, nil
}

// handleShellTool handles the shell tool MCP request
func handleShellTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	command, ok := args["command"].(string)
	if !ok || command == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "command parameter is required"}},
			IsError: true,
		}, nil
	}

	params := shellParams{Command: command}
	result, err := shellTool(ctx, params)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

func RegisterTools(s *mcp.Server) error {
	logger.Get().Info("RegisterTools initialized")

	// Register shell tool
	s.AddTool(&mcp.Tool{
		Name:        "shell",
		Description: "Execute shell commands",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"command": {
					Type:        "string",
					Description: "The shell command to execute",
				},
			},
			Required: []string{"command"},
		},
	}, handleShellTool)

	// Register datetime tool
	s.AddTool(&mcp.Tool{
		Name:        "datetime_get_current_time",
		Description: "Returns the current date and time in ISO 8601 format.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}, handleGetCurrentDateTimeTool)

	return nil
}
