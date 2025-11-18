package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultReadTimeout       = 5 * time.Minute
	defaultIdleTimeout       = 5 * time.Minute
	defaultReadHeaderTimeout = 10 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
)

// HTTPTransportConfig captures configuration parameters for the HTTP transport.
// Durations are expected to be fully resolved (e.g. seconds converted to time.Duration).
type HTTPTransportConfig struct {
	Port              int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

// HTTPTransportImpl is an implementation of the Transport interface for HTTP mode.
// It provides an HTTP server for MCP protocol communication using SSE (Server-Sent Events).
type HTTPTransportImpl struct {
	configuredPort int
	port           int

	mcpServer  *mcp.Server
	httpServer *http.Server

	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	readHeaderTimeout time.Duration
	shutdownTimeout   time.Duration

	isRunning bool
	mu        sync.Mutex
}

// NewHTTPTransport creates a new HTTP transport implementation.
// The mcpServer parameter is the MCP server instance that will handle requests.
func NewHTTPTransport(mcpServer *mcp.Server, cfg HTTPTransportConfig) (*HTTPTransportImpl, error) {
	if mcpServer == nil {
		return nil, fmt.Errorf("mcp server must not be nil")
	}

	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d (must be 0-65535)", cfg.Port)
	}

	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}

	if cfg.WriteTimeout < 0 {
		return nil, fmt.Errorf("write timeout must be zero or positive")
	}

	if cfg.IdleTimeout <= 0 {
		if cfg.ReadTimeout > 0 {
			cfg.IdleTimeout = cfg.ReadTimeout
		} else {
			cfg.IdleTimeout = defaultIdleTimeout
		}
	}

	if cfg.ReadHeaderTimeout <= 0 {
		cfg.ReadHeaderTimeout = defaultReadHeaderTimeout
	}

	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}

	return &HTTPTransportImpl{
		configuredPort:    cfg.Port,
		port:              cfg.Port,
		mcpServer:         mcpServer,
		readTimeout:       cfg.ReadTimeout,
		writeTimeout:      cfg.WriteTimeout,
		idleTimeout:       cfg.IdleTimeout,
		readHeaderTimeout: cfg.ReadHeaderTimeout,
		shutdownTimeout:   cfg.ShutdownTimeout,
	}, nil
}

// Start initializes and starts the HTTP server.
// It returns an error if the transport is already running or if the server fails to start.
// The method validates the port, sets up routes, and starts the HTTP server in a goroutine.
// Context cancellation is respected, and the server will shut down gracefully if the context is cancelled.
func (h *HTTPTransportImpl) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.isRunning {
		h.mu.Unlock()
		return fmt.Errorf("HTTP transport is already running")
	}

	configuredPort := h.configuredPort
	h.mu.Unlock()

	logger.Get().Info("Starting HTTP transport", "port", configuredPort)

	mux := http.NewServeMux()

	sseHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return h.mcpServer
	}, nil)
	mux.Handle("/mcp", sseHandler)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok"}`)
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "# HELP go_info Information about the Go environment.\n")
		_, _ = fmt.Fprintf(w, "# TYPE go_info gauge\n")
		_, _ = fmt.Fprintf(w, "go_info{version=\"%s\"} 1\n", runtime.Version())
		_, _ = fmt.Fprintf(w, "# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.\n")
		_, _ = fmt.Fprintf(w, "# TYPE process_start_time_seconds gauge\n")
		_, _ = fmt.Fprintf(w, "process_start_time_seconds %d\n", time.Now().Unix())
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{
		"service": "kagent-tools",
		"version": "mcp-server",
		"endpoints": {
			"/mcp": "MCP protocol endpoint (SSE)",
			"/health": "Health check endpoint"
		}
	}`)
	})

	h.httpServer = &http.Server{
		Handler:           mux,
		ReadTimeout:       h.readTimeout,
		WriteTimeout:      h.writeTimeout,
		IdleTimeout:       h.idleTimeout,
		ReadHeaderTimeout: h.readHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", configuredPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", configuredPort, err)
	}

	actualPort := 0
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
		actualPort = tcpAddr.Port
	}

	logger.Get().Info("Registered MCP SSE handler", "endpoint", "/mcp")

	serverErrChan := make(chan error, 1)

	go func() {
		if err := h.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Get().Error("HTTP server error", "error", err)
			select {
			case serverErrChan <- err:
			default:
			}
		}
	}()

	select {
	case err := <-serverErrChan:
		_ = listener.Close()
		return fmt.Errorf("HTTP server failed to start: %w", err)
	case <-time.After(100 * time.Millisecond):
		h.mu.Lock()
		h.port = actualPort
		h.isRunning = true
		h.mu.Unlock()
	case <-ctx.Done():
		_ = listener.Close()
		return fmt.Errorf("HTTP transport start cancelled: %w", ctx.Err())
	}

	logger.Get().Info("HTTP transport started successfully", "configured_port", configuredPort, "port", h.port)
	logger.Get().Info("Running KAgent Tools Server", "port", h.port, "endpoint", fmt.Sprintf("http://localhost:%d/mcp", h.port))
	return nil
}

// Stop gracefully shuts down the HTTP server.
// It waits for active connections to finish within the shutdown timeout.
// Returns an error if the shutdown fails or times out.
func (h *HTTPTransportImpl) Stop(ctx context.Context) error {
	h.mu.Lock()
	if !h.isRunning {
		h.mu.Unlock()
		return nil
	}

	server := h.httpServer
	shutdownTimeout := h.shutdownTimeout
	h.mu.Unlock()

	logger.Get().Info("Stopping HTTP transport")

	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Get().Error("Failed to stop HTTP server gracefully", "error", err)
			h.mu.Lock()
			h.isRunning = false
			h.httpServer = nil
			h.port = h.configuredPort
			h.mu.Unlock()
			return fmt.Errorf("HTTP server shutdown error: %w", err)
		}
	}

	h.mu.Lock()
	h.isRunning = false
	h.httpServer = nil
	h.port = h.configuredPort
	h.mu.Unlock()
	logger.Get().Info("HTTP transport stopped")
	return nil
}

// RegisterToolHandler is a no-op for HTTP transport since tools are registered with MCP server.
// Tools are registered directly with the MCP server instance during initialization.
func (h *HTTPTransportImpl) RegisterToolHandler(tool *mcp.Tool, handler mcp.ToolHandler) error {
	return nil
}

// GetName returns the name of the transport.
// This is used for logging and identification purposes.
func (h *HTTPTransportImpl) GetName() string {
	return "http"
}

// IsRunning returns whether the transport is currently running.
// This method is thread-safe and can be called concurrently.
func (h *HTTPTransportImpl) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.isRunning
}
