package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

type inspectInput struct {
	Echo string `json:"echo" jsonschema:"Optional value to echo back in the inspect response"`
}

type inspectHeader struct {
	Name   string   `json:"name" jsonschema:"HTTP header name"`
	Values []string `json:"values" jsonschema:"All values received for this HTTP header"`
}

type inspectOutput struct {
	Echo    string          `json:"echo" jsonschema:"The echo value supplied by the caller"`
	Headers []inspectHeader `json:"headers" jsonschema:"HTTP headers received with the MCP request"`
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

func handleMCPInspectTool(_ context.Context, request *mcp.CallToolRequest, in inspectInput) (*mcp.CallToolResult, *inspectOutput, error) {
	output := &inspectOutput{
		Echo:    in.Echo,
		Headers: inspectHeaders(mcp.Header(request)),
	}

	payload, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to render inspect output: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(string(payload)), output, nil
}

func inspectHeaders(headers http.Header) []inspectHeader {
	if len(headers) == 0 {
		return []inspectHeader{}
	}

	canonicalHeaders := make(http.Header, len(headers))
	for name, values := range headers {
		canonicalName := http.CanonicalHeaderKey(name)
		canonicalHeaders[canonicalName] = append(canonicalHeaders[canonicalName], values...)
	}

	names := make([]string, 0, len(canonicalHeaders))
	for name := range canonicalHeaders {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]inspectHeader, 0, len(names))
	for _, name := range names {
		values := append([]string(nil), canonicalHeaders[name]...)
		result = append(result, inspectHeader{Name: name, Values: values})
	}
	return result
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

	mcp.AddTool(s, "utils", &mcp.Tool{
		Name:        "mcp_inspect",
		Description: "Echo input and return all HTTP headers received with the MCP request for debugging.",
	}, handleMCPInspectTool)

	// Note: LLM Tool implementation would go here if needed
}
