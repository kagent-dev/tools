package http

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
)

// RequestHandler manages HTTP request handling for MCP protocol messages.
type RequestHandler struct {
	server         *Server
	requestQueue   chan *MCPRequest
	maxConcurrent  int
	requestTracker map[string]*MCPRequest
	trackerMutex   sync.RWMutex
	toolExecutor   ToolExecutor // Added: tool execution handler
}

// ToolExecutor interface allows injection of actual tool execution logic
type ToolExecutor interface {
	ExecuteTool(toolName string, args map[string]interface{}) (interface{}, error)
	ListTools() ([]ToolInfo, error)
}

// ToolInfo represents metadata about a tool
type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
}

// MCPRequest represents a queued MCP request.
type MCPRequest struct {
	ID        string
	Method    string
	Params    map[string]interface{}
	Timestamp int64
}

// NewRequestHandler creates a new request handler.
func NewRequestHandler(server *Server, maxConcurrent int) *RequestHandler {
	if maxConcurrent <= 0 {
		maxConcurrent = 100 // Default concurrent request limit
	}

	return &RequestHandler{
		server:         server,
		requestQueue:   make(chan *MCPRequest, maxConcurrent*2),
		maxConcurrent:  maxConcurrent,
		requestTracker: make(map[string]*MCPRequest),
		toolExecutor:   &DefaultToolExecutor{}, // Initialize with default executor
	}
}

// SetToolExecutor allows injection of a tool executor for testing
func (rh *RequestHandler) SetToolExecutor(executor ToolExecutor) {
	rh.toolExecutor = executor
}

// RegisterHandlers registers all MCP HTTP handlers with the server.
func (rh *RequestHandler) RegisterHandlers() error {
	handlers := []struct {
		path    string
		handler http.Handler
	}{
		{"/mcp/initialize", http.HandlerFunc(rh.handleInitialize)},
		{"/mcp/tools/list", http.HandlerFunc(rh.handleToolsList)},
		{"/mcp/tools/call", http.HandlerFunc(rh.handleToolsCall)},
	}

	for _, h := range handlers {
		if err := rh.server.RegisterHandler(h.path, h.handler); err != nil {
			logger.Get().Error("Failed to register handler", "path", h.path, "error", err)
			return err
		}
	}

	return nil
}

// handleInitialize handles MCP initialize requests.
func (rh *RequestHandler) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	rh.server.IncrementTotalRequests()

	// Parse request
	var req struct {
		JSONRPC string                 `json:"jsonrpc"`
		Method  string                 `json:"method"`
		Params  map[string]interface{} `json:"params,omitempty"`
		ID      string                 `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Failed to parse JSON")
		return
	}
	_ = r.Body.Close()

	// Validate request
	if req.JSONRPC != "2.0" {
		writeJSONError(w, http.StatusBadRequest, "invalid_jsonrpc_version", "JSONRPC version must be 2.0")
		return
	}

	if req.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_id", "Request ID is required")
		return
	}

	logger.Get().Debug("Initialize request received", "requestID", req.ID)

	// Return server capabilities
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "kagent-tools",
				"version": "1.0.0",
			},
		},
	}

	writeJSON(w, http.StatusOK, response)
}

// handleToolsList handles tool listing requests.
func (rh *RequestHandler) handleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	rh.server.IncrementTotalRequests()

	// Parse request
	var req struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		ID      string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Failed to parse JSON")
		return
	}
	_ = r.Body.Close()

	logger.Get().Debug("Tools list request received", "requestID", req.ID)

	// Get tools from executor
	tools := []interface{}{}
	if rh.toolExecutor != nil {
		if toolList, err := rh.toolExecutor.ListTools(); err == nil {
			for _, t := range toolList {
				tools = append(tools, map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"schema":      t.Schema,
				})
			}
		}
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]interface{}{
			"tools": tools,
		},
	}

	writeJSON(w, http.StatusOK, response)
}

// handleToolsCall handles tool invocation requests.
// T033 Implementation: Accept tool name and parameters, route to tool execution
func (rh *RequestHandler) handleToolsCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	rh.server.IncrementTotalRequests()

	// Parse request with timeout tracking
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		logger.Get().Debug("Tool call completed", "duration_ms", duration.Milliseconds())
	}()

	// Parse request
	var req struct {
		JSONRPC string                 `json:"jsonrpc"`
		Method  string                 `json:"method"`
		Params  map[string]interface{} `json:"params,omitempty"`
		ID      string                 `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Failed to parse JSON")
		return
	}
	_ = r.Body.Close()

	// Validate request
	if req.JSONRPC != "2.0" {
		writeJSONError(w, http.StatusBadRequest, "invalid_jsonrpc_version", "JSONRPC version must be 2.0")
		return
	}

	if req.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_id", "Request ID is required")
		return
	}

	logger.Get().Debug("Tools call request received", "requestID", req.ID, "params", req.Params)

	// T033: Extract tool name from params
	toolName, ok := req.Params["name"].(string)
	if !ok || toolName == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_tool_name", "Tool name is required in params.name")
		return
	}

	// T033: Extract tool arguments (everything except "name" is arguments)
	toolArgs := make(map[string]interface{})
	for k, v := range req.Params {
		if k != "name" {
			toolArgs[k] = v
		}
	}

	logger.Get().Debug("Executing tool", "tool", toolName, "args", toolArgs)

	// T033: Execute tool via executor (if available)
	if rh.toolExecutor == nil {
		writeJSONError(w, http.StatusInternalServerError, "no_executor", "Tool executor not configured")
		return
	}

	result, err := rh.toolExecutor.ExecuteTool(toolName, toolArgs)
	if err != nil {
		// Determine appropriate HTTP status code based on error
		errorMsg := err.Error()

		// Create detailed error response based on error type
		switch errorMsg {
		case "tool not found":
			writeToolErrorResponse(w, http.StatusNotFound, "tool_not_found", ToolNotFoundResponse(toolName))
		case "invalid parameters":
			writeToolErrorResponse(w, http.StatusBadRequest, "invalid_parameters", InvalidToolParametersResponse(toolName, errorMsg))
		default:
			// Generic tool execution error (500)
			writeToolErrorResponse(w, http.StatusInternalServerError, "tool_execution_failed", ToolExecutionFailedResponse(toolName, errorMsg))
		}
		return
	}

	// T035: Format response with proper serialization
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]interface{}{
			"tool":      toolName,
			"output":    result,
			"status":    "success",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	writeJSON(w, http.StatusOK, response)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Get().Error("Failed to write JSON response", "error", err)
	}
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, statusCode int, errorCode string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"jsonrpc": "2.0",
		"error": map[string]interface{}{
			"code":    errorCode,
			"message": message,
		},
		"id": nil,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		logger.Get().Error("Failed to write error response", "error", err)
	}
}

// writeToolErrorResponse writes a detailed tool error response.
func writeToolErrorResponse(w http.ResponseWriter, statusCode int, errorCode string, errResp *HTTPErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"jsonrpc": "2.0",
		"error": map[string]interface{}{
			"code":    errorCode,
			"message": errResp.Message,
			"data": map[string]interface{}{
				"details": errResp.Details,
			},
		},
		"id": nil,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		logger.Get().Error("Failed to write tool error response", "error", err)
	}
}

// AddRequest adds a request to the processing queue.
func (rh *RequestHandler) AddRequest(req *MCPRequest) error {
	select {
	case rh.requestQueue <- req:
		rh.trackerMutex.Lock()
		rh.requestTracker[req.ID] = req
		rh.trackerMutex.Unlock()
		return nil
	default:
		return ErrQueueFull
	}
}

// RemoveRequest removes a request from tracking.
func (rh *RequestHandler) RemoveRequest(requestID string) {
	rh.trackerMutex.Lock()
	defer rh.trackerMutex.Unlock()
	delete(rh.requestTracker, requestID)
}

// GetActiveRequests returns the number of active requests being processed.
func (rh *RequestHandler) GetActiveRequests() int {
	rh.trackerMutex.RLock()
	defer rh.trackerMutex.RUnlock()
	return len(rh.requestTracker)
}

// DefaultToolExecutor provides basic tool execution capabilities
type DefaultToolExecutor struct{}

// ExecuteTool executes a tool by name with given arguments
func (dte *DefaultToolExecutor) ExecuteTool(toolName string, args map[string]interface{}) (interface{}, error) {
	// This is a placeholder - will be overridden with actual tool execution
	// For now, return success with echo of input
	return map[string]interface{}{
		"tool":      toolName,
		"arguments": args,
		"message":   "Tool execution placeholder - override with actual tool logic",
	}, nil
}

// ListTools returns available tools
func (dte *DefaultToolExecutor) ListTools() ([]ToolInfo, error) {
	return []ToolInfo{
		{Name: "k8s", Description: "Kubernetes operations"},
		{Name: "helm", Description: "Helm package management"},
		{Name: "istio", Description: "Istio service mesh operations"},
		{Name: "argo", Description: "Argo Workflows"},
		{Name: "cilium", Description: "Cilium networking"},
		{Name: "prometheus", Description: "Prometheus monitoring"},
	}, nil
}
