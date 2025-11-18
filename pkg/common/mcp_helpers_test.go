package common

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParseMCPArguments(t *testing.T) {
	tests := []struct {
		name        string
		jsonArgs    string
		expectError bool
	}{
		{
			name:        "valid arguments",
			jsonArgs:    `{"key": "value", "number": 42}`,
			expectError: false,
		},
		{
			name:        "empty arguments",
			jsonArgs:    `{}`,
			expectError: false,
		},
		{
			name:        "invalid json",
			jsonArgs:    `{invalid}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: json.RawMessage(tt.jsonArgs),
				},
			}

			args, errResult, err := ParseMCPArguments(request)

			if tt.expectError {
				if errResult == nil {
					t.Errorf("expected error result, got nil")
				}
			} else {
				if errResult != nil {
					t.Errorf("expected no error result, got %v", errResult)
				}
				if args == nil {
					t.Errorf("expected args map, got nil")
				}
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetStringArg(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defVal   string
		expected string
	}{
		{
			name:     "existing key",
			args:     map[string]interface{}{"name": "test"},
			key:      "name",
			defVal:   "default",
			expected: "test",
		},
		{
			name:     "missing key",
			args:     map[string]interface{}{},
			key:      "name",
			defVal:   "default",
			expected: "default",
		},
		{
			name:     "non-string value",
			args:     map[string]interface{}{"name": 123},
			key:      "name",
			defVal:   "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStringArg(tt.args, tt.key, tt.defVal)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetBoolArg(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defVal   bool
		expected bool
	}{
		{
			name:     "true string",
			args:     map[string]interface{}{"flag": "true"},
			key:      "flag",
			defVal:   false,
			expected: true,
		},
		{
			name:     "false string",
			args:     map[string]interface{}{"flag": "false"},
			key:      "flag",
			defVal:   true,
			expected: false,
		},
		{
			name:     "missing key uses default",
			args:     map[string]interface{}{},
			key:      "flag",
			defVal:   true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBoolArg(tt.args, tt.key, tt.defVal)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetIntArg(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defVal   int
		expected int
	}{
		{
			name:     "int value",
			args:     map[string]interface{}{"count": 42},
			key:      "count",
			defVal:   0,
			expected: 42,
		},
		{
			name:     "float64 value",
			args:     map[string]interface{}{"count": 42.0},
			key:      "count",
			defVal:   0,
			expected: 42,
		},
		{
			name:     "string int value",
			args:     map[string]interface{}{"count": "42"},
			key:      "count",
			defVal:   0,
			expected: 42,
		},
		{
			name:     "invalid string value",
			args:     map[string]interface{}{"count": "not-a-number"},
			key:      "count",
			defVal:   99,
			expected: 99,
		},
		{
			name:     "missing key",
			args:     map[string]interface{}{},
			key:      "count",
			defVal:   10,
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetIntArg(tt.args, tt.key, tt.defVal)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestNewTextResult(t *testing.T) {
	result := NewTextResult("test message")

	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.IsError {
		t.Error("expected IsError to be false")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content, got empty")
	}

	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if text.Text != "test message" {
		t.Errorf("expected 'test message', got %q", text.Text)
	}
}

func TestNewErrorResult(t *testing.T) {
	result := NewErrorResult("error message")

	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content, got empty")
	}

	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if text.Text != "error message" {
		t.Errorf("expected 'error message', got %q", text.Text)
	}
}

func TestRequireStringArg(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		key         string
		expectValue string
		expectError bool
	}{
		{
			name:        "valid string",
			args:        map[string]interface{}{"name": "test"},
			key:         "name",
			expectValue: "test",
			expectError: false,
		},
		{
			name:        "missing key",
			args:        map[string]interface{}{},
			key:         "name",
			expectValue: "",
			expectError: true,
		},
		{
			name:        "empty string",
			args:        map[string]interface{}{"name": ""},
			key:         "name",
			expectValue: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, errResult := RequireStringArg(tt.args, tt.key)

			if tt.expectError {
				if errResult == nil {
					t.Errorf("expected error result, got nil")
				} else if !errResult.IsError {
					t.Error("expected IsError to be true")
				}
			} else {
				if errResult != nil {
					t.Errorf("expected no error, got %v", errResult)
				}
				if val != tt.expectValue {
					t.Errorf("expected %q, got %q", tt.expectValue, val)
				}
			}
		})
	}
}

func TestRequireArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		keys        []string
		expectError bool
	}{
		{
			name:        "all required present",
			args:        map[string]interface{}{"name": "test", "namespace": "default"},
			keys:        []string{"name", "namespace"},
			expectError: false,
		},
		{
			name:        "missing one required",
			args:        map[string]interface{}{"name": "test"},
			keys:        []string{"name", "namespace"},
			expectError: true,
		},
		{
			name:        "empty required string",
			args:        map[string]interface{}{"name": "", "namespace": "default"},
			keys:        []string{"name", "namespace"},
			expectError: true,
		},
		{
			name:        "all required missing",
			args:        map[string]interface{}{},
			keys:        []string{"name", "namespace"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errResult := RequireArgs(tt.args, tt.keys...)

			if tt.expectError {
				if errResult == nil {
					t.Errorf("expected error result, got nil")
				} else if !errResult.IsError {
					t.Error("expected IsError to be true")
				}
			} else {
				if errResult != nil {
					t.Errorf("expected no error, got %v", errResult)
				}
			}
		})
	}
}
