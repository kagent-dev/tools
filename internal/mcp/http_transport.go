package mcp

import (
	"context"
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
	isRunning       bool
	shutdownTimeout time.Duration
	mu              sync.RWMutex
}

// NewHTTPTransport creates a new HTTP transport implementation.
func NewHTTPTransport(mcpServer *mcp.Server, port int) *HTTPTransportImpl {
	return &HTTPTransportImpl{
		port:            port,
		mcpServer:       mcpServer,
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

	// Start the HTTP server in a goroutine
	go func() {
		if err := h.httpServer.Start(ctx); err != nil && err != http.ErrServerClosed {
			logger.Get().Error("HTTP server error", "error", err)
			h.mu.Lock()
			h.isRunning = false
			h.mu.Unlock()
		}
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	logger.Get().Info("HTTP transport started successfully", "port", h.port)
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

// IsRunning returns true if the HTTP transport is currently running.
func (h *HTTPTransportImpl) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.isRunning
}

// GetName returns the human-readable name of the HTTP transport.
func (h *HTTPTransportImpl) GetName() string {
	return "http"
}
