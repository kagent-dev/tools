package security

import (
	"testing"
)

func TestValidateK8sResourceName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid name", "my-service", false},
		{"valid name with numbers", "service-123", false},
		{"empty name", "", true},
		{"too long name", "this-is-a-very-long-name-that-exceeds-the-maximum-allowed-length-of-63-characters", true},
		{"invalid characters", "my_service", true},
		{"starts with dash", "-service", true},
		{"ends with dash", "service-", true},
		{"uppercase", "Service", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateK8sResourceName(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid namespace", "my-namespace", false},
		{"empty namespace", "", false}, // Empty is allowed
		{"reserved namespace", "kube-system", true},
		{"valid default", "default", false},
		{"invalid characters", "my_namespace", true},
		{"too long", "this-is-a-very-long-namespace-name-that-exceeds-the-maximum-allowed-length-of-63-characters", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNamespace(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateContainerImage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid image", "nginx:latest", false},
		{"valid image with registry", "docker.io/nginx:1.21", false},
		{"valid image with tag", "nginx:1.21.0", false},
		{"empty image", "", true},
		{"too long image", "this-is-a-very-long-image-name-that-exceeds-the-maximum-allowed-length-of-255-characters-and-should-fail-validation-because-it-is-way-too-long-for-a-container-image-name-and-this-should-definitely-trigger-an-error-condition-in-our-validation-logic", true},
		{"invalid characters", "nginx:latest!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerImage(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid path", "/tmp/file.txt", false},
		{"valid relative path", "config/app.yaml", false},
		{"empty path", "", true},
		{"path traversal", "../../../etc/passwd", true},
		{"invalid characters", "file<script>", true},
		{"too long path", string(make([]byte, 5000)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateCommandInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid input", "my-service", false},
		{"empty input", "", true},
		{"command injection", "test; rm -rf /", true},
		{"pipe injection", "test | cat /etc/passwd", true},
		{"backtick injection", "test`whoami`", true},
		{"variable expansion", "test${USER}", true},
		{"too long input", string(make([]byte, 2000)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommandInput(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"clean input", "hello world", "hello world"},
		{"with newlines", "hello\nworld", "hello world"},
		{"with tabs", "hello\tworld", "hello world"},
		{"with carriage returns", "hello\rworld", "hello world"},
		{"with spaces", "  hello world  ", "hello world"},
		{"mixed whitespace", "\n\t  hello world  \r\n", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeInput(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestValidateK8sLabel(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		expectError bool
	}{
		{"valid label", "app", "nginx", false},
		{"valid label with dash", "app-version", "1.0", false},
		{"valid label with underscore", "app_name", "nginx", false},
		{"empty key", "", "value", true},
		{"empty value", "key", "", false}, // Empty value is allowed
		{"too long key", string(make([]byte, 70)), "value", true},
		{"too long value", "key", string(make([]byte, 70)), true},
		{"invalid key characters", "app/name", "nginx", true},
		{"invalid value characters", "app", "nginx/web", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateK8sLabel(tt.key, tt.value)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for key %q, value %q, but got none", tt.key, tt.value)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for key %q, value %q: %v", tt.key, tt.value, err)
			}
		})
	}
}

func TestValidatePromQLQuery(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid query", "up{job=\"prometheus\"}", false},
		{"valid aggregation", "sum(rate(http_requests_total[5m]))", false},
		{"empty query", "", true},
		{"command injection", "up; rm -rf /", true},
		{"backtick injection", "up`whoami`", true},
		{"variable expansion", "up${USER}", true},
		{"too long query", string(make([]byte, 10000)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePromQLQuery(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateYAMLContent(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid YAML", "apiVersion: v1\nkind: Pod", false},
		{"empty content", "", true},
		{"python object", "!!python/object/apply", true},
		{"python import", "__import__('os').system('rm -rf /')", true},
		{"eval injection", "eval('print(1)')", true},
		{"too large content", string(make([]byte, 2*1024*1024)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateYAMLContent(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateHelmReleaseName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid release name", "my-release", false},
		{"valid with numbers", "release-123", false},
		{"empty name", "", true},
		{"too long name", "this-is-a-very-long-release-name-that-exceeds-the-maximum-allowed-length-of-53-characters", true},
		{"invalid characters", "my_release", true},
		{"starts with dash", "-release", true},
		{"ends with dash", "release-", true},
		{"uppercase", "Release", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHelmReleaseName(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid http URL", "http://example.com", false},
		{"valid https URL", "https://example.com/path", false},
		{"empty URL", "", true},
		{"invalid protocol", "ftp://example.com", true},
		{"javascript injection", "javascript:alert('xss')", true},
		{"data URL", "data:text/html,<script>alert('xss')</script>", true},
		{"too long URL", "https://" + string(make([]byte, 3000)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.input)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for input %q, but got none", tt.input)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError{
		Field:   "test_field",
		Message: "test message",
	}

	expected := "validation error in field 'test_field': test message"
	if err.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, err.Error())
	}
}
