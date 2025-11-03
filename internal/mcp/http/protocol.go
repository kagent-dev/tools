package http

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// MCPRequestAdapter translates HTTP requests to MCP protocol messages.
type MCPRequestAdapter struct {
	RequestID      string                 `json:"id"`
	JSONRPCVersion string                 `json:"jsonrpc"`
	Method         string                 `json:"method"`
	Params         map[string]interface{} `json:"params,omitempty"`
}

// MCPResponseAdapter translates MCP protocol messages to HTTP responses.
type MCPResponseAdapter struct {
	RequestID      string      `json:"id"`
	JSONRPCVersion string      `json:"jsonrpc"`
	Result         interface{} `json:"result,omitempty"`
	Error          interface{} `json:"error,omitempty"`
}

// ParseMCPRequest parses a raw JSON request into an MCP request structure.
func ParseMCPRequest(data []byte) (*MCPRequestAdapter, error) {
	var req MCPRequestAdapter
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MCP request: %w", err)
	}

	// Validate JSONRPC version
	if req.JSONRPCVersion != "2.0" {
		return nil, fmt.Errorf("invalid JSONRPC version: %s", req.JSONRPCVersion)
	}

	// Validate required fields
	if req.Method == "" {
		return nil, fmt.Errorf("MCP request method is required")
	}

	return &req, nil
}

// MarshalMCPRequest serializes an MCP request adapter to JSON.
func MarshalMCPRequest(adapter *MCPRequestAdapter) ([]byte, error) {
	// Ensure JSONRPC version is set
	if adapter.JSONRPCVersion == "" {
		adapter.JSONRPCVersion = "2.0"
	}

	return json.Marshal(adapter)
}

// NewMCPResponse creates a new MCP response adapter with the given request ID and result.
func NewMCPResponse(requestID string, result interface{}) *MCPResponseAdapter {
	return &MCPResponseAdapter{
		RequestID:      requestID,
		JSONRPCVersion: "2.0",
		Result:         result,
	}
}

// NewMCPErrorResponse creates a new MCP error response adapter.
func NewMCPErrorResponse(requestID string, errorCode int, errorMessage string) *MCPResponseAdapter {
	return &MCPResponseAdapter{
		RequestID:      requestID,
		JSONRPCVersion: "2.0",
		Error: map[string]interface{}{
			"code":    errorCode,
			"message": errorMessage,
		},
	}
}

// MarshalMCPResponse serializes an MCP response adapter to JSON.
func MarshalMCPResponse(adapter *MCPResponseAdapter) ([]byte, error) {
	// Ensure JSONRPC version is set
	if adapter.JSONRPCVersion == "" {
		adapter.JSONRPCVersion = "2.0"
	}

	// Clear irrelevant fields based on response type
	if adapter.Error != nil {
		adapter.Result = nil
	}

	return json.Marshal(adapter)
}

// ParameterMarshaler handles parameter type validation and conversion.
type ParameterMarshaler struct {
	params map[string]interface{}
}

// NewParameterMarshaler creates a new parameter marshaler.
func NewParameterMarshaler(params map[string]interface{}) *ParameterMarshaler {
	if params == nil {
		params = make(map[string]interface{})
	}
	return &ParameterMarshaler{params: params}
}

// GetString retrieves a string parameter by key.
func (pm *ParameterMarshaler) GetString(key string) (string, error) {
	value, exists := pm.params[key]
	if !exists {
		return "", fmt.Errorf("parameter %s not found", key)
	}

	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s is not a string, got %T", key, value)
	}

	return str, nil
}

// GetInt retrieves an integer parameter by key.
func (pm *ParameterMarshaler) GetInt(key string) (int, error) {
	value, exists := pm.params[key]
	if !exists {
		return 0, fmt.Errorf("parameter %s not found", key)
	}

	// Try to convert various numeric types to int
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("parameter %s is not a number, got %T", key, value)
	}
}

// GetBool retrieves a boolean parameter by key.
func (pm *ParameterMarshaler) GetBool(key string) (bool, error) {
	value, exists := pm.params[key]
	if !exists {
		return false, fmt.Errorf("parameter %s not found", key)
	}

	bool_, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("parameter %s is not a boolean, got %T", key, value)
	}

	return bool_, nil
}

// GetMap retrieves a map parameter by key.
func (pm *ParameterMarshaler) GetMap(key string) (map[string]interface{}, error) {
	value, exists := pm.params[key]
	if !exists {
		return nil, fmt.Errorf("parameter %s not found", key)
	}

	mapValue, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("parameter %s is not a map, got %T", key, value)
	}

	return mapValue, nil
}

// GetArray retrieves an array parameter by key.
func (pm *ParameterMarshaler) GetArray(key string) ([]interface{}, error) {
	value, exists := pm.params[key]
	if !exists {
		return nil, fmt.Errorf("parameter %s not found", key)
	}

	array, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("parameter %s is not an array, got %T", key, value)
	}

	return array, nil
}

// Get retrieves any parameter by key.
func (pm *ParameterMarshaler) Get(key string) (interface{}, error) {
	value, exists := pm.params[key]
	if !exists {
		return nil, fmt.Errorf("parameter %s not found", key)
	}

	return value, nil
}

// ValidateRequired validates that required parameters are present.
func (pm *ParameterMarshaler) ValidateRequired(requiredParams ...string) error {
	for _, param := range requiredParams {
		if _, exists := pm.params[param]; !exists {
			return fmt.Errorf("required parameter %s is missing", param)
		}
	}
	return nil
}

// ResponseSerializer handles response serialization with type safety.
type ResponseSerializer struct {
	data      interface{}
	timestamp time.Time
}

// NewResponseSerializer creates a new response serializer.
func NewResponseSerializer(data interface{}) *ResponseSerializer {
	return &ResponseSerializer{
		data:      data,
		timestamp: time.Now().UTC(),
	}
}

// ToJSON serializes the response data to JSON.
func (rs *ResponseSerializer) ToJSON() ([]byte, error) {
	// Wrap the data with metadata
	response := map[string]interface{}{
		"data":      rs.data,
		"timestamp": rs.timestamp.Format(time.RFC3339),
	}

	return json.Marshal(response)
}

// ToMap returns the response as a map for further processing.
func (rs *ResponseSerializer) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"data":      rs.data,
		"timestamp": rs.timestamp.Format(time.RFC3339),
	}
}

// MarshalComplex handles complex type marshaling for tool results.
func MarshalComplex(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string, float64, bool, nil:
		// Primitives are fine as-is
		return v, nil
	case map[string]interface{}:
		// Maps need deep marshaling
		result := make(map[string]interface{})
		for k, val := range v {
			marshaled, err := MarshalComplex(val)
			if err != nil {
				return nil, err
			}
			result[k] = marshaled
		}
		return result, nil
	case []interface{}:
		// Arrays need deep marshaling
		result := make([]interface{}, len(v))
		for i, val := range v {
			marshaled, err := MarshalComplex(val)
			if err != nil {
				return nil, err
			}
			result[i] = marshaled
		}
		return result, nil
	default:
		// For custom types, try to marshal to map via reflection
		if reflect.TypeOf(v).Kind() == reflect.Struct {
			data, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil, err
			}
			return result, nil
		}
		return nil, fmt.Errorf("unsupported type for marshaling: %T", value)
	}
}

// GetArrayString retrieves an array of strings parameter by key.
func (pm *ParameterMarshaler) GetArrayString(key string) ([]string, error) {
	array, err := pm.GetArray(key)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(array))
	for _, item := range array {
		if str, ok := item.(string); ok {
			result = append(result, str)
		} else {
			return nil, fmt.Errorf("array item in %s is not a string, got %T", key, item)
		}
	}

	return result, nil
}

// GetStringOrDefault retrieves a string parameter or returns a default if not found.
func (pm *ParameterMarshaler) GetStringOrDefault(key string, defaultValue string) string {
	value, err := pm.GetString(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetIntOrDefault retrieves an int parameter or returns a default if not found.
func (pm *ParameterMarshaler) GetIntOrDefault(key string, defaultValue int) int {
	value, err := pm.GetInt(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetBoolOrDefault retrieves a bool parameter or returns a default if not found.
func (pm *ParameterMarshaler) GetBoolOrDefault(key string, defaultValue bool) bool {
	value, err := pm.GetBool(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// HasKey checks if a parameter exists.
func (pm *ParameterMarshaler) HasKey(key string) bool {
	_, exists := pm.params[key]
	return exists
}

// Keys returns all parameter keys.
func (pm *ParameterMarshaler) Keys() []string {
	keys := make([]string, 0, len(pm.params))
	for k := range pm.params {
		keys = append(keys, k)
	}
	return keys
}

// GetAll returns all parameters as a map.
func (pm *ParameterMarshaler) GetAll() map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range pm.params {
		result[k] = v
	}
	return result
}

// MergeMaps recursively merges two maps for nested parameter handling.
func MergeMaps(target, source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy target
	for k, v := range target {
		result[k] = v
	}

	// Merge source
	for k, v := range source {
		if existing, ok := result[k]; ok {
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if sourceMap, ok := v.(map[string]interface{}); ok {
					result[k] = MergeMaps(existingMap, sourceMap)
					continue
				}
			}
		}
		result[k] = v
	}

	return result
}

// ResponseSerializer helpers for T035

// AddMetadata adds additional metadata to the response serializer.
func (rs *ResponseSerializer) AddMetadata(key string, value interface{}) map[string]interface{} {
	response := rs.ToMap()
	response[key] = value
	return response
}

// WithStatus creates a response with a status field.
func (rs *ResponseSerializer) WithStatus(status string) map[string]interface{} {
	return map[string]interface{}{
		"data":      rs.data,
		"status":    status,
		"timestamp": rs.timestamp.Format(time.RFC3339),
	}
}

// WithStatusAndMeta creates a response with status and additional metadata.
func (rs *ResponseSerializer) WithStatusAndMeta(status string, meta map[string]interface{}) map[string]interface{} {
	response := rs.WithStatus(status)
	for k, v := range meta {
		response[k] = v
	}
	return response
}

// FormatToolResult formats tool execution results for HTTP response
// T035 Implementation: Standardize tool result serialization
func FormatToolResult(toolName string, output interface{}, executionTime int64) map[string]interface{} {
	return map[string]interface{}{
		"tool":            toolName,
		"output":          output,
		"executionTimeMs": executionTime,
		"success":         true,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
}

// FormatToolError formats tool execution errors for HTTP response
// T035 Implementation: Standardize error serialization
func FormatToolError(toolName string, errMsg string, errorCode string) map[string]interface{} {
	return map[string]interface{}{
		"tool":      toolName,
		"error":     errMsg,
		"errorCode": errorCode,
		"success":   false,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// SerializeToolOutput handles serialization of complex tool output types
// T035 Implementation: Support various tool output formats
func SerializeToolOutput(output interface{}) (interface{}, error) {
	// Handle nil
	if output == nil {
		return nil, nil
	}

	// For strings, numbers, booleans - return as-is
	switch v := output.(type) {
	case string, float64, float32, int, int64, int32, bool:
		return v, nil
	}

	// For slices/arrays
	switch v := output.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(v))
		for _, item := range v {
			serialized, err := SerializeToolOutput(item)
			if err != nil {
				return nil, err
			}
			result = append(result, serialized)
		}
		return result, nil
	}

	// For maps
	if mapV, ok := output.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for k, v := range mapV {
			serialized, err := SerializeToolOutput(v)
			if err != nil {
				return nil, err
			}
			result[k] = serialized
		}
		return result, nil
	}

	// For other types, try JSON marshaling as fallback
	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("unable to serialize output of type %T: %w", output, err)
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unable to unmarshal serialized output: %w", err)
	}

	return result, nil
}
