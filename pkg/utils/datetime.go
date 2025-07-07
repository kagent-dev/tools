package utils

import (
	"context"
	"github.com/kagent-dev/tools/pkg/logger"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var kubeConfig = ""

// DateTime tools using direct Go time package
// This implementation matches the Python version exactly
func handleGetCurrentDateTimeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Returns the current date and time in ISO 8601 format (RFC3339)
	// This matches the Python implementation: datetime.datetime.now().isoformat()
	now := time.Now()
	return mcp.NewToolResultText(now.Format(time.RFC3339)), nil
}

func RegisterDateTimeTools(s *server.MCPServer, kubeconfig string) {
	kubeConfig = kubeconfig
	logger.Get().Info("kubeConfig", kubeConfig)

	// Register the GetCurrentDateTime tool to match Python implementation exactly
	s.AddTool(mcp.NewTool("datetime_get_current_time",
		mcp.WithDescription("Returns the current date and time in ISO 8601 format."),
	), handleGetCurrentDateTimeTool)
}
