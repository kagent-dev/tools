// Package common provides shared MCP helper functions for all tool packages.
//
// This package centralizes argument parsing, validation, and result creation
// to reduce duplication across MCP tool implementations and ensure consistent
// error handling and response formatting.
package common

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ParseMCPArguments parses MCP tool request arguments into a map.
// Returns the parsed arguments, an error result (if parsing fails), and an error.
// If error result is not nil, the handler should return it immediately.
func ParseMCPArguments(request *mcp.CallToolRequest) (map[string]interface{}, *mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return nil, &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}
	return args, nil, nil
}

// GetStringArg safely extracts a string argument with default value.
func GetStringArg(args map[string]interface{}, key, defaultVal string) string {
	if val, ok := args[key].(string); ok {
		return val
	}
	return defaultVal
}

// GetBoolArg safely extracts a boolean argument from string representation.
// Accepts "true" as true, everything else as false.
func GetBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if val, ok := args[key].(string); ok {
		return val == "true"
	}
	return defaultVal
}

// GetIntArg safely extracts an integer argument.
func GetIntArg(args map[string]interface{}, key string, defaultVal int) int {
	switch v := args[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		// Try to parse string as int
		var result int
		if _, err := fmt.Sscanf(v, "%d", &result); err == nil {
			return result
		}
	}
	return defaultVal
}

// NewTextResult creates a success result with text content.
func NewTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// NewErrorResult creates an error result with text content.
func NewErrorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

// RequireStringArg validates that a required string argument exists.
// Returns error result if missing or empty.
func RequireStringArg(args map[string]interface{}, key string) (string, *mcp.CallToolResult) {
	val, ok := args[key].(string)
	if !ok || val == "" {
		return "", NewErrorResult(fmt.Sprintf("%s parameter is required", key))
	}
	return val, nil
}

// RequireArgs validates multiple required string arguments.
// Returns error result with missing parameters listed.
func RequireArgs(args map[string]interface{}, keys ...string) *mcp.CallToolResult {
	var missing []string
	for _, key := range keys {
		val, ok := args[key].(string)
		if !ok || val == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return NewErrorResult(fmt.Sprintf("required parameters missing: %v", missing))
	}
	return nil
}
