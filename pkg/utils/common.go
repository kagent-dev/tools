// Package utils provides common utility functions and tools.
//
// This package implements MCP tools for general utilities, providing operations such as:
//   - Shell command execution
//   - DateTime queries
//   - Echo and message utilities
//   - Sleep operations with progress tracking
//
// Tools provide foundational capabilities for integration with other systems.
// Kubeconfig management and multi-cluster support are provided through global configuration.
//
// Example usage:
//
//	server := mcp.NewServer(...)
//	err := RegisterTools(server)
package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/pkg/common"
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
	args, errResult, _ := common.ParseMCPArguments(request)
	if errResult != nil {
		return errResult, nil
	}

	command, errResult := common.RequireStringArg(args, "command")
	if errResult != nil {
		return errResult, nil
	}

	params := shellParams{Command: command}
	result, err := shellTool(ctx, params)
	if err != nil {
		return common.NewErrorResult(err.Error()), nil
	}

	return common.NewTextResult(result), nil
}

// handleEchoTool handles the echo tool MCP request
func handleEchoTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, errResult, _ := common.ParseMCPArguments(request)
	if errResult != nil {
		return errResult, nil
	}

	message, ok := args["message"].(string)
	if !ok {
		return common.NewErrorResult("message parameter is required and must be a string"), nil
	}

	return common.NewTextResult(message), nil
}

// handleSleepTool handles the sleep tool MCP request
func handleSleepTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, errResult, _ := common.ParseMCPArguments(request)
	if errResult != nil {
		return errResult, nil
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
		return common.NewErrorResult("duration parameter is required and must be a number"), nil
	}

	if durationSeconds < 0 {
		return common.NewErrorResult("duration must be non-negative"), nil
	}

	// Convert to duration and sleep with context cancellation support
	duration := time.Duration(durationSeconds * float64(time.Second))

	// For short durations, just sleep without progress updates
	if durationSeconds < 1.0 {
		timer := time.NewTimer(duration)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return common.NewErrorResult("sleep cancelled after context cancellation"), nil
		case <-timer.C:
			return common.NewTextResult(fmt.Sprintf("slept for %.2f seconds", durationSeconds)), nil
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
			return common.NewErrorResult(fmt.Sprintf("sleep cancelled after %.2f seconds (requested %.2f seconds)", elapsed.Seconds(), durationSeconds)), nil
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := duration - elapsed
			if remaining < 0 {
				remaining = 0
			}
			elapsedSeconds := int(elapsed.Seconds())
			totalSeconds := int(durationSeconds)

			if request.Session != nil {

				// Send progress notification
				progressParams := &mcp.ProgressNotificationParams{
					ProgressToken: request.Params.GetProgressToken(),
					Message:       fmt.Sprintf("Sleep progress: %d/%d seconds (%.1fs remaining)", elapsedSeconds, totalSeconds, remaining.Seconds()),
					Progress:      elapsed.Seconds(),
					Total:         duration.Seconds(),
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
			return common.NewTextResult(fmt.Sprintf("slept for %.2f seconds", durationSeconds)), nil
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
