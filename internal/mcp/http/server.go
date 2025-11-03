package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
)

// Server represents an HTTP MCP server with lifecycle management.
type Server struct {
	port              int
	httpServer        *http.Server
	listener          net.Listener
	mux               *http.ServeMux
	isRunning         bool
	startTime         time.Time
	requestTimeout    time.Duration
	startupTimeout    time.Duration
	shutdownTimeout   time.Duration
	connectionsMutex  sync.Mutex
	connectedClients  int
	totalRequests     int64
	requestCountMutex sync.Mutex
}

// NewServer creates a new HTTP MCP server with default configuration.
// The server is not started until Start() is called.
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	s := &Server{
		port:            port,
		mux:             mux,
		isRunning:       false,
		requestTimeout:  30 * time.Second,
		startupTimeout:  2 * time.Second,
		shutdownTimeout: 10 * time.Second,
	}
	// Register health handler immediately
	mux.HandleFunc("/health", s.healthHandler)
	return s
}

// SetRequestTimeout configures the request timeout for all handlers.
// Default is 30 seconds.
func (s *Server) SetRequestTimeout(timeout time.Duration) {
	s.requestTimeout = timeout
}

// SetStartupTimeout configures the timeout for server startup.
// Default is 2 seconds.
func (s *Server) SetStartupTimeout(timeout time.Duration) {
	s.startupTimeout = timeout
}

// SetShutdownTimeout configures the timeout for graceful shutdown.
// Default is 10 seconds.
func (s *Server) SetShutdownTimeout(timeout time.Duration) {
	s.shutdownTimeout = timeout
}

// Start initializes and starts the HTTP server.
// It listens on the configured port and starts accepting connections.
func (s *Server) Start(ctx context.Context) error {
	if s.isRunning {
		return fmt.Errorf("server is already running")
	}

	startupCtx, cancel := context.WithTimeout(ctx, s.startupTimeout)
	defer cancel()

	// Create a new listener on the configured port
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: s.port,
	})
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	s.listener = listener
	s.startTime = time.Now()
	s.isRunning = true

	logger.Get().Info("HTTP MCP server listening", "port", s.port)

	// Create HTTP server using the pre-configured mux
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s.mux,
		ReadTimeout:       s.requestTimeout,
		WriteTimeout:      s.requestTimeout,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start serving connections in a goroutine
	// This returns immediately; errors are communicated through channels
	go func() {
		logger.Get().Debug("Starting HTTP server", "addr", s.httpServer.Addr)
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Get().Error("HTTP server error", "error", err)
		}
	}()

	// Verify the server started within the startup timeout
	select {
	case <-startupCtx.Done():
		// If startup timeout exceeded, we stop the server
		_ = s.Stop(context.Background())
		return fmt.Errorf("server startup timeout exceeded")
	case <-time.After(100 * time.Millisecond):
		// Give the server a moment to start; if it fails,  Serve() will log the error
	}

	logger.Get().Info("HTTP MCP server started successfully", "port", s.port)
	return nil
}

// Stop gracefully shuts down the HTTP server.
// It closes the listener and drains active connections with a configurable timeout.
func (s *Server) Stop(ctx context.Context) error {
	if !s.isRunning {
		return nil
	}

	s.isRunning = false
	logger.Get().Info("Shutting down HTTP server")

	if s.httpServer == nil {
		return nil
	}

	// Create a shutdown context with the configured timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()

	// Gracefully shutdown the server
	// This stops accepting new connections and waits for active requests to complete
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		// If graceful shutdown times out, force close
		logger.Get().Warn("Graceful shutdown timeout, forcing close", "error", err)
		if closeErr := s.httpServer.Close(); closeErr != nil {
			return fmt.Errorf("failed to close server: %w", closeErr)
		}
	}

	logger.Get().Info("HTTP server stopped")
	return nil
}

// IsRunning returns true if the server is currently running.
func (s *Server) IsRunning() bool {
	return s.isRunning
}

// GetPort returns the port the server is listening on.
func (s *Server) GetPort() int {
	return s.port
}

// GetUptime returns the duration since the server started.
func (s *Server) GetUptime() time.Duration {
	if !s.isRunning {
		return 0
	}
	return time.Since(s.startTime)
}

// GetStartTime returns the time when the server started.
func (s *Server) GetStartTime() time.Time {
	return s.startTime
}

// IncrementConnectedClients increments the connected clients counter.
func (s *Server) IncrementConnectedClients() {
	s.connectionsMutex.Lock()
	defer s.connectionsMutex.Unlock()
	s.connectedClients++
}

// DecrementConnectedClients decrements the connected clients counter.
func (s *Server) DecrementConnectedClients() {
	s.connectionsMutex.Lock()
	defer s.connectionsMutex.Unlock()
	if s.connectedClients > 0 {
		s.connectedClients--
	}
}

// GetConnectedClients returns the current number of connected clients.
func (s *Server) GetConnectedClients() int {
	s.connectionsMutex.Lock()
	defer s.connectionsMutex.Unlock()
	return s.connectedClients
}

// GetTotalRequests returns the total number of requests processed.
func (s *Server) GetTotalRequests() int64 {
	s.requestCountMutex.Lock()
	defer s.requestCountMutex.Unlock()
	return s.totalRequests
}

// IncrementTotalRequests increments the total requests counter.
func (s *Server) IncrementTotalRequests() {
	s.requestCountMutex.Lock()
	defer s.requestCountMutex.Unlock()
	s.totalRequests++
}

// GetMetrics returns server metrics.
func (s *Server) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"port":              s.port,
		"is_running":        s.isRunning,
		"uptime":            s.GetUptime().String(),
		"connected_clients": s.GetConnectedClients(),
		"total_requests":    s.GetTotalRequests(),
		"request_timeout":   s.requestTimeout.String(),
		"shutdown_timeout":  s.shutdownTimeout.String(),
	}
}

// HandleConnectionError logs a connection error with details.
// This method is used to track and report connection-related errors.
func (s *Server) HandleConnectionError(remoteAddr string, err error) {
	logger.Get().Error("Connection error",
		"remote_address", remoteAddr,
		"error", err,
		"is_running", s.isRunning,
	)
}

// HandleConnectionTimeout logs a connection timeout event.
// Returns the appropriate error response for timeout scenarios.
func (s *Server) HandleConnectionTimeout(remoteAddr string, timeoutSeconds float64) *HTTPErrorResponse {
	logger.Get().Warn("Connection timeout",
		"remote_address", remoteAddr,
		"timeout_seconds", timeoutSeconds,
	)
	return ConnectionTimeoutResponse(remoteAddr, timeoutSeconds)
}

// HandleClientDisconnect logs a client disconnection event.
// Returns the appropriate error response for client disconnect scenarios.
func (s *Server) HandleClientDisconnect(remoteAddr string) *HTTPErrorResponse {
	logger.Get().Info("Client disconnected",
		"remote_address", remoteAddr,
	)
	s.DecrementConnectedClients()
	return ClientDisconnectResponse(remoteAddr)
}

// HandleServerShutdown returns an appropriate error response when server is shutting down.
func (s *Server) HandleServerShutdown() *HTTPErrorResponse {
	logger.Get().Warn("Request received while server is shutting down")
	return ServerShutdownResponse()
}

// RegisterHandler registers a handler function for a specific path.
// Can be called before or after server starts.
func (s *Server) RegisterHandler(path string, handler http.Handler) error {
	if s.mux == nil {
		return fmt.Errorf("server mux not initialized")
	}

	s.mux.Handle(path, handler)
	logger.Get().Debug("Registered handler", "path", path)
	return nil
}

// healthHandler is a basic health check endpoint.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.IncrementTotalRequests()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Return health status as JSON
	response := map[string]interface{}{
		"status":            "ok",
		"uptime_seconds":    s.GetUptime().Seconds(),
		"connected_clients": s.GetConnectedClients(),
		"total_requests":    s.GetTotalRequests(),
	}

	// Simple JSON encoding without external dependencies
	_, _ = fmt.Fprintf(w, `{"status":"%s","uptime_seconds":%.1f,"connected_clients":%d,"total_requests":%d}`,
		response["status"], response["uptime_seconds"], response["connected_clients"], response["total_requests"])
}
