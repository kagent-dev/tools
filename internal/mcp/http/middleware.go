package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
)

// Middleware represents an HTTP middleware function.
type Middleware func(http.Handler) http.Handler

// ValidationMiddleware validates incoming HTTP requests.
// It checks content-type and injects a request ID for tracing.
func ValidationMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only validate POST requests
			if r.Method != http.MethodPost && r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			// For POST requests, validate content-type
			if r.Method == http.MethodPost {
				contentType := r.Header.Get("Content-Type")
				// Be lenient with content-type checking
				if !strings.Contains(contentType, "application/json") && contentType != "" {
					logger.Get().Warn("Invalid content-type", "content-type", contentType)
					// Return structured error response for invalid content-type
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					errResp := BadRequestResponse("Content-Type header must be application/json").
						AddDetail("received_content_type", contentType)
					if err := json.NewEncoder(w).Encode(map[string]interface{}{
						"jsonrpc": "2.0",
						"error": map[string]interface{}{
							"code":    "invalid_content_type",
							"message": errResp.Message,
							"data": map[string]interface{}{
								"details": errResp.Details,
							},
						},
						"id": nil,
					}); err != nil {
						logger.Get().Error("failed to encode error response", "error", err)
					}
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestIDMiddleware injects a request ID for tracing and correlation.
func RequestIDMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				// Generate a request ID if not provided
				requestID = generateRequestID()
				r.Header.Set("X-Request-ID", requestID)
			}

			// Add request ID to response headers
			w.Header().Set("X-Request-ID", requestID)

			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs HTTP requests and responses.
func LoggingMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			requestID := r.Header.Get("X-Request-ID")

			// Create a wrapper to capture response status
			wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			logger.Get().Debug("HTTP request", "request_id", requestID, "method", r.Method, "path", r.RequestURI)

			// Call the next handler
			next.ServeHTTP(wrapper, r)

			// Log response
			duration := time.Since(startTime)
			logger.Get().Debug("HTTP response", "request_id", requestID, "status", wrapper.statusCode, "duration_ms", duration.Milliseconds())
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader captures the HTTP status code.
func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.written {
		w.statusCode = statusCode
		w.written = true
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

// Write wraps the ResponseWriter Write method.
func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.statusCode = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// ChainMiddleware chains multiple middleware functions together.
func ChainMiddleware(handler http.Handler, middlewares ...Middleware) http.Handler {
	// Apply middleware in reverse order so they execute in the expected order
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// generateRequestID generates a unique request ID for tracing.
func generateRequestID() string {
	return time.Now().Format("20060102150405000000")
}

// ErrorMiddleware handles panics and converts them to HTTP error responses.
func ErrorMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Get().Error("HTTP handler panic", "error", rec, "request_id", r.Header.Get("X-Request-ID"))
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware adds a timeout to HTTP request handling.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CORSMiddleware adds CORS headers to the response (if needed).
func CORSMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ValidateMCPRequest validates a parsed MCP request for required fields.
// Returns an error response if validation fails, nil if successful.
func ValidateMCPRequest(req map[string]interface{}) *HTTPErrorResponse {
	// Check for required JSONRPC field
	jsonrpc, ok := req["jsonrpc"].(string)
	if !ok {
		return MissingFieldResponse("jsonrpc")
	}
	if jsonrpc != "2.0" {
		return ProtocolErrorResponse("JSONRPC version must be 2.0, got: " + jsonrpc)
	}

	// Check for required id field
	if _, ok := req["id"]; !ok {
		return MissingFieldResponse("id")
	}

	// Check for required method field
	_, ok = req["method"].(string)
	if !ok {
		return MissingFieldResponse("method")
	}

	return nil
}

// ValidateToolCallRequest validates a tool call request parameters.
// Returns an error response if validation fails, nil if successful.
func ValidateToolCallRequest(req map[string]interface{}) *HTTPErrorResponse {
	params, ok := req["params"].(map[string]interface{})
	if !ok || params == nil {
		return MissingFieldResponse("params")
	}

	// Check for tool name
	toolName, ok := params["name"].(string)
	if !ok {
		return InvalidFieldTypeResponse("params.name", "string", "")
	}
	if toolName == "" {
		return ValidationErrorResponse("params.name", "Tool name cannot be empty")
	}

	return nil
}

// ValidateJSONRequest validates the JSON request body structure.
// Returns an error response with details if validation fails.
func ValidateJSONRequest(body interface{}, expectedType string) *HTTPErrorResponse {
	if body == nil {
		return MissingFieldResponse("request_body")
	}

	switch expectedType {
	case "object":
		if _, ok := body.(map[string]interface{}); !ok {
			return InvalidFieldTypeResponse("body", "object", "")
		}
	case "array":
		if _, ok := body.([]interface{}); !ok {
			return InvalidFieldTypeResponse("body", "array", "")
		}
	}

	return nil
}

// FieldTypeError creates a detailed error for field type mismatches.
func FieldTypeError(fieldName string, expectedType string, actualValue interface{}) *HTTPErrorResponse {
	actualType := "unknown"
	switch actualValue.(type) {
	case string:
		actualType = "string"
	case float64:
		actualType = "number"
	case bool:
		actualType = "boolean"
	case map[string]interface{}:
		actualType = "object"
	case []interface{}:
		actualType = "array"
	case nil:
		actualType = "null"
	}
	return InvalidFieldTypeResponse(fieldName, expectedType, actualType)
}
