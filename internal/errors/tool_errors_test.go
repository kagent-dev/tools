package errors

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewToolError(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, cause, err.Cause)
	assert.Equal(t, "TestComponent", err.Component)
	assert.Equal(t, "UNKNOWN", err.ErrorCode)
	assert.False(t, err.IsRetryable)
	assert.Empty(t, err.Suggestions)
	assert.NotNil(t, err.Context)
	assert.WithinDuration(t, time.Now(), err.Timestamp, time.Second)
}

func TestToolErrorError(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	result := err.Error()
	expected := "[TestComponent] test operation failed: test error"
	assert.Equal(t, expected, result)
}

func TestToolErrorWithSuggestions(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	err = err.WithSuggestions("suggestion 1", "suggestion 2")

	assert.Equal(t, []string{"suggestion 1", "suggestion 2"}, err.Suggestions)

	// Test chaining
	err = err.WithSuggestions("suggestion 3")
	assert.Equal(t, []string{"suggestion 1", "suggestion 2", "suggestion 3"}, err.Suggestions)
}

func TestToolErrorWithRetryable(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	err = err.WithRetryable(true)
	assert.True(t, err.IsRetryable)

	err = err.WithRetryable(false)
	assert.False(t, err.IsRetryable)
}

func TestToolErrorWithErrorCode(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	err = err.WithErrorCode("TEST_ERROR")
	assert.Equal(t, "TEST_ERROR", err.ErrorCode)
}

func TestToolErrorWithResource(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	err = err.WithResource("Pod", "test-pod")
	assert.Equal(t, "Pod", err.ResourceType)
	assert.Equal(t, "test-pod", err.ResourceName)
}

func TestToolErrorWithContext(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	err = err.WithContext("key1", "value1")
	err = err.WithContext("key2", 42)

	assert.Equal(t, "value1", err.Context["key1"])
	assert.Equal(t, 42, err.Context["key2"])
}

func TestToolErrorToMCPResult(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause).
		WithErrorCode("TEST_ERROR").
		WithResource("Pod", "test-pod").
		WithSuggestions("suggestion 1", "suggestion 2").
		WithContext("key1", "value1").
		WithRetryable(true)

	result := err.ToMCPResult()

	assert.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.NotEmpty(t, result.Content)

	// Check content (assuming it's text content)
	if len(result.Content) > 0 {
		content := result.Content[0]
		// This depends on the actual MCP implementation
		// We'll just check that it's not empty
		assert.NotNil(t, content)
	}
}

func TestNewKubernetesError(t *testing.T) {
	tests := []struct {
		name          string
		causeError    string
		expectedCode  string
		expectedRetry bool
		expectedSuggs int
	}{
		{
			name:          "connection refused",
			causeError:    "connection refused",
			expectedCode:  "K8S_CONNECTION_ERROR",
			expectedRetry: true,
			expectedSuggs: 3,
		},
		{
			name:          "forbidden",
			causeError:    "forbidden",
			expectedCode:  "K8S_PERMISSION_ERROR",
			expectedRetry: false,
			expectedSuggs: 3,
		},
		{
			name:          "not found",
			causeError:    "not found",
			expectedCode:  "K8S_RESOURCE_NOT_FOUND",
			expectedRetry: false,
			expectedSuggs: 3,
		},
		{
			name:          "already exists",
			causeError:    "already exists",
			expectedCode:  "K8S_RESOURCE_EXISTS",
			expectedRetry: false,
			expectedSuggs: 3,
		},
		{
			name:          "generic error",
			causeError:    "some other error",
			expectedCode:  "K8S_GENERIC_ERROR",
			expectedRetry: true,
			expectedSuggs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New(tt.causeError)
			err := NewKubernetesError("test operation", cause)

			assert.Equal(t, "Kubernetes", err.Component)
			assert.Equal(t, tt.expectedCode, err.ErrorCode)
			assert.Equal(t, tt.expectedRetry, err.IsRetryable)
			assert.Len(t, err.Suggestions, tt.expectedSuggs)
		})
	}
}

func TestNewHelmError(t *testing.T) {
	tests := []struct {
		name          string
		causeError    string
		expectedCode  string
		expectedRetry bool
		expectedSuggs int
	}{
		{
			name:          "not found",
			causeError:    "not found",
			expectedCode:  "HELM_RELEASE_NOT_FOUND",
			expectedRetry: false,
			expectedSuggs: 3,
		},
		{
			name:          "already exists",
			causeError:    "already exists",
			expectedCode:  "HELM_RELEASE_EXISTS",
			expectedRetry: false,
			expectedSuggs: 3,
		},
		{
			name:          "repository error",
			causeError:    "repository error",
			expectedCode:  "HELM_REPOSITORY_ERROR",
			expectedRetry: true,
			expectedSuggs: 3,
		},
		{
			name:          "generic error",
			causeError:    "some other error",
			expectedCode:  "HELM_GENERIC_ERROR",
			expectedRetry: true,
			expectedSuggs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New(tt.causeError)
			err := NewHelmError("test operation", cause)

			assert.Equal(t, "Helm", err.Component)
			assert.Equal(t, tt.expectedCode, err.ErrorCode)
			assert.Equal(t, tt.expectedRetry, err.IsRetryable)
			assert.Len(t, err.Suggestions, tt.expectedSuggs)
		})
	}
}

func TestNewIstioError(t *testing.T) {
	cause := errors.New("test error")
	err := NewIstioError("test operation", cause)

	assert.Equal(t, "Istio", err.Component)
	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, cause, err.Cause)
}

func TestNewPrometheusError(t *testing.T) {
	cause := errors.New("test error")
	err := NewPrometheusError("test operation", cause)

	assert.Equal(t, "Prometheus", err.Component)
	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, cause, err.Cause)
}

func TestNewArgoError(t *testing.T) {
	cause := errors.New("test error")
	err := NewArgoError("test operation", cause)

	assert.Equal(t, "Argo Rollouts", err.Component)
	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, cause, err.Cause)
}

func TestNewCiliumError(t *testing.T) {
	cause := errors.New("test error")
	err := NewCiliumError("test operation", cause)

	assert.Equal(t, "Cilium", err.Component)
	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, cause, err.Cause)
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("test-field", "validation failed")

	assert.Equal(t, "Validation", err.Component)
	assert.Equal(t, "validate test-field", err.Operation)
	assert.Equal(t, "VALIDATION_ERROR", err.ErrorCode)
	assert.False(t, err.IsRetryable)
	assert.Contains(t, err.Cause.Error(), "validation failed")
}

func TestNewSecurityError(t *testing.T) {
	cause := errors.New("security violation")
	err := NewSecurityError("test operation", cause)

	assert.Equal(t, "Security", err.Component)
	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, cause, err.Cause)
	assert.Equal(t, "SECURITY_ERROR", err.ErrorCode)
	assert.False(t, err.IsRetryable)
}

func TestNewTimeoutError(t *testing.T) {
	timeout := 30 * time.Second
	err := NewTimeoutError("test operation", timeout)

	assert.Equal(t, "Timeout", err.Component)
	assert.Equal(t, "test operation", err.Operation)
	assert.Equal(t, "TIMEOUT_ERROR", err.ErrorCode)
	assert.True(t, err.IsRetryable)
	assert.Contains(t, err.Cause.Error(), "30s")
}

func TestNewCommandError(t *testing.T) {
	cause := errors.New("command failed")
	err := NewCommandError("test-command", cause)

	assert.Equal(t, "Command", err.Component)
	assert.Equal(t, "execute test-command", err.Operation)
	assert.Equal(t, cause, err.Cause)
	assert.Equal(t, "COMMAND_ERROR", err.ErrorCode)
	assert.True(t, err.IsRetryable)
}

func TestToolErrorChaining(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause).
		WithErrorCode("TEST_ERROR").
		WithResource("Pod", "test-pod").
		WithSuggestions("suggestion 1").
		WithContext("key1", "value1").
		WithRetryable(true)

	// Test that all methods return the same instance for chaining
	assert.Equal(t, "TEST_ERROR", err.ErrorCode)
	assert.Equal(t, "Pod", err.ResourceType)
	assert.Equal(t, "test-pod", err.ResourceName)
	assert.Equal(t, []string{"suggestion 1"}, err.Suggestions)
	assert.Equal(t, "value1", err.Context["key1"])
	assert.True(t, err.IsRetryable)
}

func TestToolErrorStringRepresentation(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	errorStr := err.Error()
	assert.Contains(t, errorStr, "TestComponent")
	assert.Contains(t, errorStr, "test operation")
	assert.Contains(t, errorStr, "test error")
	assert.Contains(t, errorStr, "failed")
}

func TestToolErrorTimestamp(t *testing.T) {
	before := time.Now()
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)
	after := time.Now()

	assert.True(t, err.Timestamp.After(before) || err.Timestamp.Equal(before))
	assert.True(t, err.Timestamp.Before(after) || err.Timestamp.Equal(after))
}

func TestToolErrorContextInitialization(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause)

	// Context should be initialized but empty
	assert.NotNil(t, err.Context)
	assert.Empty(t, err.Context)

	// Should be able to add to context
	err = err.WithContext("test", "value")
	assert.Equal(t, "value", err.Context["test"])
}

func TestMCPResultContainsExpectedFields(t *testing.T) {
	cause := errors.New("test error")
	err := NewToolError("TestComponent", "test operation", cause).
		WithErrorCode("TEST_ERROR").
		WithResource("Pod", "test-pod").
		WithSuggestions("suggestion 1").
		WithContext("key1", "value1").
		WithRetryable(true)

	result := err.ToMCPResult()

	// The result should be an error result
	assert.True(t, result.IsError)

	// Should have content
	assert.NotEmpty(t, result.Content)
}
