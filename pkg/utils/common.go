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

// handleEchoTool handles the echo tool MCP request
func handleEchoTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	message, ok := args["message"].(string)
	if !ok {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "message parameter is required and must be a string"}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}, nil
}

// handleSleepTool handles the sleep tool MCP request
func handleSleepTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	// Handle both float64 and int types for duration
	var durationSeconds float64
	switch v := args["duration"].(type) {
	case float64:
		durationSeconds = v
	case int:
		durationSeconds = float64(v)
	case int64:
		durationSeconds = float64(v)
	default:
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "duration parameter is required and must be a number"}},
			IsError: true,
		}, nil
	}

	if durationSeconds < 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "duration must be non-negative"}},
			IsError: true,
		}, nil
	}

	// Convert to duration and sleep with context cancellation support
	duration := time.Duration(durationSeconds * float64(time.Second))

	// For short durations, just sleep without progress updates
	if durationSeconds < 1.0 {
		timer := time.NewTimer(duration)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "sleep cancelled after context cancellation"}},
				IsError: true,
			}, nil
		case <-timer.C:
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("slept for %.2f seconds", durationSeconds)}},
			}, nil
		}
	}

	// For longer durations, emit progress updates every second
	timer := time.NewTimer(duration)
	defer timer.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	log := logger.Get()

	for {
		select {
		case <-ctx.Done():
			elapsed := time.Since(startTime)
			log.Info("Sleep operation cancelled",
				"elapsed_seconds", elapsed.Seconds(),
				"total_seconds", durationSeconds)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{
					Text: fmt.Sprintf("sleep cancelled after %.2f seconds (requested %.2f seconds)", elapsed.Seconds(), durationSeconds),
				}},
				IsError: true,
			}, nil
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := duration - elapsed
			if remaining < 0 {
				remaining = 0
			}
			elapsedSeconds := int(elapsed.Seconds())
			totalSeconds := int(durationSeconds)

			if request.Session != nil {
				progressParams := &mcp.ProgressNotificationParams{
					Message:  fmt.Sprintf("Sleep progress: %d/%d seconds (%.1fs remaining)", elapsedSeconds, totalSeconds, remaining.Seconds()),
					Progress: elapsed.Seconds(),
					Total:    duration.Seconds(),
				}

				if err := request.Session.NotifyProgress(ctx, progressParams); err != nil {
					// Log the error but continue sleeping - progress notification failure shouldn't abort the operation
					log.Error("Failed to send progress notification",
						"error", err,
						"elapsed_seconds", elapsedSeconds,
						"total_seconds", totalSeconds)
				} else {
					log.Info("Progress notification sent",
						"elapsed_seconds", elapsedSeconds,
						"total_seconds", totalSeconds,
						"remaining_seconds", remaining.Seconds())
				}
			}
		case <-timer.C:
			actualDuration := time.Since(startTime).Seconds()
			log.Info("Sleep operation completed",
				"requested_seconds", durationSeconds,
				"actual_seconds", actualDuration)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("slept for %.2f seconds", durationSeconds)}},
			}, nil
		}
	}
}

// ToolRegistry is an interface for tool registration (to avoid import cycles)
type ToolRegistry interface {
	Register(tool *mcp.Tool, handler mcp.ToolHandler)
}

func RegisterTools(s *mcp.Server) error {
	return RegisterToolsWithRegistry(s, nil)
}

// RegisterToolsWithRegistry registers all utility tools with the MCP server and optionally with a tool registry
func RegisterToolsWithRegistry(s *mcp.Server, registry ToolRegistry) error {
	logger.Get().Info("Registering utility tools")

	// Define tools
	shellTool := &mcp.Tool{
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
	}

	datetimeTool := &mcp.Tool{
		Name:        "datetime_get_current_time",
		Description: "Returns the current date and time in ISO 8601 format.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}

	echoTool := &mcp.Tool{
		Name:        "echo",
		Description: "Echo back the provided message",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"message": {
					Type:        "string",
					Description: "The message to echo back",
				},
			},
			Required: []string{"message"},
		},
	}

	sleepTool := &mcp.Tool{
		Name:        "sleep",
		Description: "Sleep for the specified duration in seconds",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"duration": {
					Type:        "number",
					Description: "Duration to sleep in seconds (can be a decimal)",
					Minimum:     jsonschema.Ptr(0.0),
				},
			},
			Required: []string{"duration"},
		},
	}

	// Register shell tool
	s.AddTool(shellTool, handleShellTool)
	if registry != nil {
		registry.Register(shellTool, handleShellTool)
	}

	// Register datetime tool
	s.AddTool(datetimeTool, handleGetCurrentDateTimeTool)
	if registry != nil {
		registry.Register(datetimeTool, handleGetCurrentDateTimeTool)
	}

	// Register echo tool
	s.AddTool(echoTool, handleEchoTool)
	if registry != nil {
		registry.Register(echoTool, handleEchoTool)
	}

	// Register sleep tool
	s.AddTool(sleepTool, handleSleepTool)
	if registry != nil {
		registry.Register(sleepTool, handleSleepTool)
	}

	return nil
}
