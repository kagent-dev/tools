package mcp

import (
	"context"
	"fmt"

	"github.com/kagent-dev/tools/internal/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StdioTransportImpl is an implementation of the Transport interface for stdio mode.
// It wraps the MCP SDK's StdioTransport and provides a clean abstraction for transport management.
type StdioTransportImpl struct {
	stdioTransport *mcp.StdioTransport
	mcpServer      *mcp.Server
	isRunning      bool
}

// NewStdioTransport creates a new stdio transport implementation.
func NewStdioTransport(mcpServer *mcp.Server) *StdioTransportImpl {
	return &StdioTransportImpl{
		stdioTransport: &mcp.StdioTransport{},
		mcpServer:      mcpServer,
		isRunning:      false,
	}
}

// Start initializes and starts the stdio transport.
// This blocks until the transport is stopped or an error occurs.
func (s *StdioTransportImpl) Start(ctx context.Context) error {
	logger.Get().Info("Starting stdio transport")
	logger.Get().Info("Running KAgent Tools Server STDIO")
	s.isRunning = true
	defer func() { s.isRunning = false }()

	// Run the MCP server on the stdio transport
	// This is a blocking call that runs until context is cancelled
	if err := s.mcpServer.Run(ctx, s.stdioTransport); err != nil {
		// Context cancellation is expected during normal shutdown
		if err == context.Canceled {
			logger.Get().Info("Stdio transport cancelled")
			return nil
		}
		logger.Get().Error("Stdio transport error", "error", err)
		return fmt.Errorf("stdio transport error: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the stdio transport.
// For stdio transport, this is a no-op since shutdown is handled via context cancellation.
func (s *StdioTransportImpl) Stop(ctx context.Context) error {
	logger.Get().Info("Stopping stdio transport")
	s.isRunning = false
	return nil
}

// IsRunning returns true if the stdio transport is currently running.
func (s *StdioTransportImpl) IsRunning() bool {
	return s.isRunning
}

// GetName returns the human-readable name of the stdio transport.
func (s *StdioTransportImpl) GetName() string {
	return "stdio"
}

// RegisterToolHandler is a no-op for stdio transport since tools are registered with MCP server directly
func (s *StdioTransportImpl) RegisterToolHandler(tool *mcp.Tool, handler mcp.ToolHandler) error {
	// Stdio transport uses MCP SDK's built-in tool handling, so this is not needed
	return nil
}
