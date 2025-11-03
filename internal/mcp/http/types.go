package http

import (
	"time"
)

// HTTPRequest represents an MCP request received via HTTP.
type HTTPRequest struct {
	RequestID      string                 `json:"request_id"`
	Method         string                 `json:"method"`
	Path           string                 `json:"path"`
	JSONRPCVersion string                 `json:"jsonrpc"`
	Params         map[string]interface{} `json:"params,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// HTTPResponse represents an MCP response to be sent via HTTP.
type HTTPResponse struct {
	StatusCode     int            `json:"status_code"`
	RequestID      string         `json:"request_id"`
	JSONRPCVersion string         `json:"jsonrpc"`
	Result         interface{}    `json:"result,omitempty"`
	Error          *ErrorResponse `json:"error,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
}

// ErrorResponse represents an error returned by the HTTP MCP server.
type ErrorResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ServerState represents the current state of the HTTP MCP server.
type ServerState struct {
	IsRunning        bool      `json:"is_running"`
	Port             int       `json:"port"`
	ConnectedClients int       `json:"connected_clients"`
	ActiveRequests   int       `json:"active_requests"`
	TotalRequests    int64     `json:"total_requests"`
	StartTime        time.Time `json:"start_time"`
}

// ToolOperation represents a KAgent tool operation invocation.
type ToolOperation struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
	Timeout    time.Duration          `json:"timeout,omitempty"`
	ClientID   string                 `json:"client_id,omitempty"`
}

// NewHTTPRequest creates a new HTTP request with current timestamp.
func NewHTTPRequest(requestID, method, path string, params map[string]interface{}) *HTTPRequest {
	return &HTTPRequest{
		RequestID:      requestID,
		Method:         method,
		Path:           path,
		JSONRPCVersion: "2.0",
		Params:         params,
		Timestamp:      time.Now().UTC(),
	}
}

// NewHTTPResponse creates a new HTTP response with current timestamp.
func NewHTTPResponse(requestID string, statusCode int, result interface{}) *HTTPResponse {
	return &HTTPResponse{
		StatusCode:     statusCode,
		RequestID:      requestID,
		JSONRPCVersion: "2.0",
		Result:         result,
		Timestamp:      time.Now().UTC(),
	}
}
