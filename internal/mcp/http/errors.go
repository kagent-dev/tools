package http

import (
	"errors"
	"net/http"
)

// Common error codes
const (
	// MCP Protocol Error Codes
	ErrorParseError       = -32700 // Parse error
	ErrorInvalidRequest   = -32600 // Invalid Request
	ErrorMethodNotFound   = -32601 // Method not found
	ErrorInvalidParams    = -32602 // Invalid params
	ErrorInternalError    = -32603 // Internal error
	ErrorServerErrorStart = -32099 // Server error (reserved for implementation-defined server errors)
	ErrorServerErrorEnd   = -32000

	// Custom error codes
	ErrorQueueFullCode       = -32000 // Request queue full
	ErrorTimeoutCode         = -32001 // Request timeout
	ErrorToolNotFound        = -32002 // Tool not found
	ErrorConnectionTimeout   = -32003 // Connection timeout
	ErrorClientDisconnect    = -32004 // Client disconnected
	ErrorServerShutdown      = -32005 // Server shutting down
	ErrorMalformedJSON       = -32700 // Malformed JSON (same as ParseError)
	ErrorMissingField        = -32602 // Missing required field
	ErrorInvalidFieldType    = -32602 // Invalid field type (same as InvalidParams)
	ErrorToolExecutionFailed = -32000 // Tool execution failed
)

var (
	// Custom errors
	ErrQueueFull         = errors.New("request queue is full")
	ErrTimeout           = errors.New("request timeout")
	ErrToolNotFound      = errors.New("tool not found")
	ErrConnectionTimeout = errors.New("connection timeout")
	ErrClientDisconnect  = errors.New("client disconnected")
	ErrServerShutdown    = errors.New("server is shutting down")
	ErrMalformedJSON     = errors.New("malformed JSON in request")
	ErrInvalidFieldType  = errors.New("invalid field type")
	ErrMissingField      = errors.New("missing required field")
)

// HTTPErrorResponse represents a structured HTTP error response.
// Complies with MCP JSONRPC 2.0 spec for error responses.
type HTTPErrorResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ErrorToHTTPStatus maps MCP error codes to HTTP status codes.
func ErrorToHTTPStatus(mcpErrorCode int) int {
	switch mcpErrorCode {
	case ErrorParseError:
		// ErrorMalformedJSON also maps to 400
		return http.StatusBadRequest // 400
	case ErrorInvalidRequest:
		return http.StatusBadRequest // 400
	case ErrorMethodNotFound:
		return http.StatusNotFound // 404
	case ErrorInvalidParams:
		// ErrorMissingField and ErrorInvalidFieldType also map to 400
		return http.StatusBadRequest // 400
	case ErrorInternalError:
		return http.StatusInternalServerError // 500
	case ErrorQueueFullCode:
		return http.StatusServiceUnavailable // 503
	case ErrorServerShutdown:
		return http.StatusServiceUnavailable // 503
	case ErrorTimeoutCode:
		return http.StatusRequestTimeout // 408
	case ErrorConnectionTimeout:
		return http.StatusRequestTimeout // 408
	case ErrorToolNotFound:
		return http.StatusNotFound // 404
	case ErrorClientDisconnect:
		return http.StatusBadRequest // 400
	default:
		// ErrorToolExecutionFailed and other server errors
		if mcpErrorCode >= ErrorServerErrorEnd && mcpErrorCode <= ErrorServerErrorStart {
			return http.StatusInternalServerError // 500
		}
		return http.StatusInternalServerError // 500
	}
}

// MCPErrorToHTTPStatus converts common Go errors to HTTP status codes.
func MCPErrorToHTTPStatus(err error) int {
	if errors.Is(err, ErrQueueFull) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, ErrTimeout) {
		return http.StatusRequestTimeout
	}
	if errors.Is(err, ErrToolNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrConnectionTimeout) {
		return http.StatusRequestTimeout
	}
	if errors.Is(err, ErrServerShutdown) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, ErrClientDisconnect) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// NewErrorResponse creates a new error response with code and message.
func NewErrorResponse(code int, message string) *HTTPErrorResponse {
	return &HTTPErrorResponse{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// AddDetail adds a detail field to the error response.
func (er *HTTPErrorResponse) AddDetail(key string, value interface{}) *HTTPErrorResponse {
	if er.Details == nil {
		er.Details = make(map[string]interface{})
	}
	er.Details[key] = value
	return er
}

// GetDetailMessage returns a formatted error message with details.
func (er *HTTPErrorResponse) GetDetailMessage() string {
	msg := er.Message
	if len(er.Details) > 0 {
		if suggestion, ok := er.Details["suggestion"]; ok {
			msg += ". Suggestion: " + suggestion.(string)
		}
	}
	return msg
}

// MalformedJSONResponse creates an error response for malformed JSON.
func MalformedJSONResponse(details string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorParseError, "Malformed JSON in request body").
		AddDetail("reason", details).
		AddDetail("suggestion", "Ensure the request body is valid JSON and properly formatted").
		AddDetail("error_type", "malformed_json")
}

// MissingFieldResponse creates an error response for missing required fields.
func MissingFieldResponse(fieldName string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorMissingField, "Missing required field").
		AddDetail("field", fieldName).
		AddDetail("error_type", "missing_field").
		AddDetail("suggestion", "Please provide the required field '"+fieldName+"' in the request")
}

// InvalidFieldTypeResponse creates an error response for invalid field types.
func InvalidFieldTypeResponse(fieldName string, expectedType string, actualType string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorInvalidFieldType, "Invalid field type").
		AddDetail("field", fieldName).
		AddDetail("expected_type", expectedType).
		AddDetail("actual_type", actualType).
		AddDetail("error_type", "invalid_field_type").
		AddDetail("suggestion", "Please provide a "+expectedType+" value for field '"+fieldName+"'")
}

// ValidationErrorResponse creates an error response for validation failures.
func ValidationErrorResponse(fieldName, reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorInvalidParams, "Validation failed").
		AddDetail("field", fieldName).
		AddDetail("reason", reason).
		AddDetail("error_type", "validation_failed").
		AddDetail("suggestion", "Please check the request parameters and try again")
}

// ToolErrorResponse creates an error response for tool-related errors.
func ToolErrorResponse(toolName, reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorToolNotFound, "Tool execution failed").
		AddDetail("tool", toolName).
		AddDetail("reason", reason).
		AddDetail("error_type", "tool_error").
		AddDetail("suggestion", "Please verify the tool name and parameters are correct")
}

// ToolNotFoundResponse creates an error response when a tool doesn't exist.
func ToolNotFoundResponse(toolName string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorToolNotFound, "Tool not found").
		AddDetail("tool", toolName).
		AddDetail("error_type", "tool_not_found").
		AddDetail("suggestion", "Please verify the tool name exists and is correctly spelled")
}

// InvalidToolParametersResponse creates an error response for invalid tool parameters.
func InvalidToolParametersResponse(toolName string, reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorInvalidParams, "Invalid tool parameters").
		AddDetail("tool", toolName).
		AddDetail("reason", reason).
		AddDetail("error_type", "invalid_tool_parameters").
		AddDetail("suggestion", "Please check the tool's parameter requirements and try again")
}

// ToolExecutionFailedResponse creates an error response for tool execution failures.
func ToolExecutionFailedResponse(toolName string, reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorToolExecutionFailed, "Tool execution failed").
		AddDetail("tool", toolName).
		AddDetail("reason", reason).
		AddDetail("error_type", "tool_execution_failed").
		AddDetail("suggestion", "Please check the tool's requirements and retry or contact support")
}

// TimeoutErrorResponse creates an error response for timeout errors.
func TimeoutErrorResponse(operationName string, timeoutSeconds float64) *HTTPErrorResponse {
	return NewErrorResponse(ErrorTimeoutCode, "Request timeout").
		AddDetail("operation", operationName).
		AddDetail("timeout_seconds", timeoutSeconds).
		AddDetail("error_type", "request_timeout").
		AddDetail("suggestion", "Please increase timeout or check if the server is overloaded")
}

// ConnectionTimeoutResponse creates an error response for connection timeouts.
func ConnectionTimeoutResponse(remoteAddr string, timeoutSeconds float64) *HTTPErrorResponse {
	return NewErrorResponse(ErrorConnectionTimeout, "Connection timeout").
		AddDetail("remote_address", remoteAddr).
		AddDetail("timeout_seconds", timeoutSeconds).
		AddDetail("error_type", "connection_timeout").
		AddDetail("suggestion", "Please check your network connection and retry")
}

// ClientDisconnectResponse creates an error response for client disconnections.
func ClientDisconnectResponse(remoteAddr string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorClientDisconnect, "Client disconnected").
		AddDetail("remote_address", remoteAddr).
		AddDetail("error_type", "client_disconnect").
		AddDetail("suggestion", "The client unexpectedly disconnected; reconnect and retry")
}

// ServerShutdownResponse creates an error response when the server is shutting down.
func ServerShutdownResponse() *HTTPErrorResponse {
	return NewErrorResponse(ErrorServerShutdown, "Server is shutting down").
		AddDetail("error_type", "server_shutdown").
		AddDetail("suggestion", "Please retry your request after the server is back online")
}

// QueueFullResponse creates an error response when the request queue is full.
func QueueFullResponse() *HTTPErrorResponse {
	return NewErrorResponse(ErrorQueueFullCode, "Request queue is full").
		AddDetail("error_type", "queue_full").
		AddDetail("suggestion", "Please retry your request after a short delay")
}

// ProtocolErrorResponse creates an error response for protocol violations.
func ProtocolErrorResponse(reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorInvalidRequest, "Protocol error").
		AddDetail("reason", reason).
		AddDetail("error_type", "protocol_error").
		AddDetail("suggestion", "Please ensure the request follows the MCP JSONRPC 2.0 specification")
}

// ServerErrorResponse creates an error response for internal server errors.
func ServerErrorResponse(reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorInternalError, "Internal server error").
		AddDetail("reason", reason).
		AddDetail("error_type", "internal_server_error").
		AddDetail("suggestion", "Please retry the request or contact support")
}

// BadRequestResponse creates a generic bad request error response.
func BadRequestResponse(reason string) *HTTPErrorResponse {
	return NewErrorResponse(ErrorInvalidRequest, "Bad request").
		AddDetail("reason", reason).
		AddDetail("error_type", "bad_request").
		AddDetail("suggestion", "Please check your request and try again")
}
