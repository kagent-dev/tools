package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Transport defines the interface that different MCP server transport implementations must implement.
// This enables clean separation between stdio, HTTP, and potentially other transport modes (gRPC, WebSocket, etc.).
type Transport interface {
	// Start initializes and starts the transport layer.
	// Returns an error if the transport cannot be started.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the transport layer.
	// Should close all connections and clean up resources.
	// Returns an error if graceful shutdown fails.
	Stop(ctx context.Context) error

	// IsRunning returns true if the transport is currently running.
	IsRunning() bool

	// GetName returns the human-readable name of this transport (e.g., "stdio", "http", "grpc").
	GetName() string

	// RegisterToolHandler registers a tool handler with the transport (optional for some transports).
	// For HTTP transport, this allows tools to be called directly via HTTP endpoints.
	// For stdio transport, this is a no-op since tools are registered with MCP server.
	RegisterToolHandler(tool *mcp.Tool, handler mcp.ToolHandler) error
}
