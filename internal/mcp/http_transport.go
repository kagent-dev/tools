package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
	httpmodule "github.com/kagent-dev/tools/internal/mcp/http"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTPTransportImpl is an implementation of the Transport interface for HTTP mode.
// It provides an HTTP server for MCP protocol communication.
type HTTPTransportImpl struct {
	port            int
	mcpServer       *mcp.Server
	httpServer      *httpmodule.Server
	requestHandler  *httpmodule.RequestHandler
	toolRegistry    *ToolRegistry
	isRunning       bool
	shutdownTimeout time.Duration
	mu              sync.RWMutex
}

// registryExecutor adapts ToolRegistry to httpmodule.ToolExecutor interface
type registryExecutor struct {
	registry *ToolRegistry
}

// ExecuteTool executes a tool by name
func (re *registryExecutor) ExecuteTool(toolName string, args map[string]interface{}) (interface{}, error) {
	handler, exists := re.registry.GetHandler(toolName)
	if !exists {
		return nil, fmt.Errorf("tool not found")
	}

	// Convert args to JSON for MCP SDK format
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters")
	}

	// Create MCP request
	ctx := context.Background()
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      toolName,
			Arguments: argsJSON,
		},
	}

	// Call the tool handler
	result, err := handler(ctx, req)
	if err != nil {
		return nil, err
	}

	// Extract content from result
	if result.IsError {
		return nil, fmt.Errorf("tool execution failed: %s", extractContent(result))
	}

	return extractContent(result), nil
}

// extractContent extracts text content from CallToolResult
func extractContent(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}

	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			return textContent.Text
		}
	}

	return ""
}

// ListTools returns the list of available tools
func (re *registryExecutor) ListTools() ([]httpmodule.ToolInfo, error) {
	tools := re.registry.ListTools()
	result := make([]httpmodule.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		var schema map[string]interface{}
		if tool.InputSchema != nil {
			if s, ok := tool.InputSchema.(map[string]interface{}); ok {
				schema = s
			}
		}
		result = append(result, httpmodule.ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Schema:      schema,
		})
	}

	return result, nil
}

// NewHTTPTransport creates a new HTTP transport implementation.
func NewHTTPTransport(mcpServer *mcp.Server, port int, toolRegistry *ToolRegistry) *HTTPTransportImpl {
	return &HTTPTransportImpl{
		port:            port,
		mcpServer:       mcpServer,
		toolRegistry:    toolRegistry,
		isRunning:       false,
		shutdownTimeout: 10 * time.Second,
	}
}

// SetShutdownTimeout configures the graceful shutdown timeout.
func (h *HTTPTransportImpl) SetShutdownTimeout(timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shutdownTimeout = timeout
}

// Start initializes and starts the HTTP server.
func (h *HTTPTransportImpl) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.isRunning {
		h.mu.Unlock()
		return fmt.Errorf("HTTP transport is already running")
	}
	h.isRunning = true
	h.mu.Unlock()

	logger.Get().Info("Starting HTTP transport", "port", h.port)

	// Create HTTP server
	h.httpServer = httpmodule.NewServer(h.port)

	// Create tool executor from registry
	executor := &registryExecutor{registry: h.toolRegistry}
	logger.Get().Info("Initialized tool executor", "tool_count", h.toolRegistry.Count())

	// Create request handler and register handlers BEFORE starting server
	h.requestHandler = httpmodule.NewRequestHandler(h.httpServer, 100)
	h.requestHandler.SetToolExecutor(executor)

	// Register MCP protocol handlers BEFORE starting the server
	if err := h.requestHandler.RegisterHandlers(); err != nil {
		h.mu.Lock()
		h.isRunning = false
		h.mu.Unlock()
		return fmt.Errorf("failed to register MCP handlers: %w", err)
	}

	// Now start the HTTP server with all handlers registered
	startErr := make(chan error, 1)
	go func() {
		if err := h.httpServer.Start(ctx); err != nil && err != http.ErrServerClosed {
			logger.Get().Error("HTTP server error", "error", err)
			startErr <- err
			h.mu.Lock()
			h.isRunning = false
			h.mu.Unlock()
		}
	}()

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	// Check if server failed to start
	select {
	case err := <-startErr:
		return fmt.Errorf("failed to start HTTP server: %w", err)
	default:
		// Server started successfully
	}

	logger.Get().Info("HTTP transport started successfully", "port", h.port)
	logger.Get().Info("Running KAgent Tools Server", "port", h.port)
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (h *HTTPTransportImpl) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isRunning {
		return nil
	}

	logger.Get().Info("Stopping HTTP transport")

	if h.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
		defer cancel()

		if err := h.httpServer.Stop(shutdownCtx); err != nil {
			logger.Get().Error("Failed to stop HTTP server gracefully", "error", err)
			return fmt.Errorf("HTTP server shutdown error: %w", err)
		}
	}

	h.isRunning = false
	logger.Get().Info("HTTP transport stopped")
	return nil
}

// RegisterToolHandler is a no-op for HTTP transport since tools are registered via registry
func (h *HTTPTransportImpl) RegisterToolHandler(tool *mcp.Tool, handler mcp.ToolHandler) error {
	// Tools are registered via the shared registry, not directly with the transport
	return nil
}

// GetName returns the name of the transport.
func (h *HTTPTransportImpl) GetName() string {
	return "http"
}

// IsRunning returns whether the transport is currently running.
func (h *HTTPTransportImpl) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.isRunning
}
