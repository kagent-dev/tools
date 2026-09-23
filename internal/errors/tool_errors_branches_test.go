package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// errorCodeCase exercises one keyword-driven branch of a component error
// constructor and asserts the resulting error code and retryability.
type errorCodeCase struct {
	cause         string
	expectedCode  string
	expectedRetry bool
}

func runErrorCodeCases(t *testing.T, component string, ctor func(string, error) *ToolError, cases []errorCodeCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.expectedCode, func(t *testing.T) {
			err := ctor("op", errors.New(c.cause))
			assert.Equal(t, component, err.Component)
			assert.Equal(t, c.expectedCode, err.ErrorCode)
			assert.Equal(t, c.expectedRetry, err.IsRetryable)
			assert.NotEmpty(t, err.Suggestions)
		})
	}
}

func TestNewIstioErrorBranches(t *testing.T) {
	runErrorCodeCases(t, "Istio", NewIstioError, []errorCodeCase{
		{"resource not found", "ISTIO_RESOURCE_NOT_FOUND", false},
		{"connection refused", "ISTIO_CONNECTION_ERROR", true},
		{"boom", "ISTIO_GENERIC_ERROR", true},
	})
}

func TestNewPrometheusErrorBranches(t *testing.T) {
	runErrorCodeCases(t, "Prometheus", NewPrometheusError, []errorCodeCase{
		{"connection refused", "PROMETHEUS_CONNECTION_ERROR", true},
		{"parse error", "PROMETHEUS_QUERY_ERROR", false},
		{"boom", "PROMETHEUS_GENERIC_ERROR", true},
	})
}

func TestNewArgoErrorBranches(t *testing.T) {
	runErrorCodeCases(t, "Argo Rollouts", NewArgoError, []errorCodeCase{
		{"rollout not found", "ARGO_ROLLOUT_NOT_FOUND", false},
		{"plugin missing", "ARGO_PLUGIN_ERROR", true},
		{"boom", "ARGO_GENERIC_ERROR", true},
	})
}

func TestNewCiliumErrorBranches(t *testing.T) {
	runErrorCodeCases(t, "Cilium", NewCiliumError, []errorCodeCase{
		{"cilium not found", "CILIUM_NOT_FOUND", false},
		{"connection lost", "CILIUM_CONNECTION_ERROR", true},
		{"boom", "CILIUM_GENERIC_ERROR", true},
	})
}

func TestNewKubescapeErrorBranches(t *testing.T) {
	// "not found" branches further specialize by operation keyword.
	notFoundOps := []string{"vulnerability scan", "sbom build", "configuration scan", "application_profile get", "network_neighborhood get", "other op"}
	for _, op := range notFoundOps {
		t.Run("not_found/"+op, func(t *testing.T) {
			err := NewKubescapeError(op, errors.New("resource not found"))
			assert.Equal(t, "Kubescape", err.Component)
			assert.Equal(t, "KUBESCAPE_RESOURCE_NOT_FOUND", err.ErrorCode)
			assert.False(t, err.IsRetryable)
			assert.NotEmpty(t, err.Suggestions)
		})
	}

	runErrorCodeCases(t, "Kubescape", NewKubescapeError, []errorCodeCase{
		{"connection refused", "KUBESCAPE_CONNECTION_ERROR", true},
		{"timeout exceeded", "KUBESCAPE_CONNECTION_ERROR", true},
		{"forbidden", "KUBESCAPE_PERMISSION_ERROR", false},
		{"boom", "KUBESCAPE_GENERIC_ERROR", true},
	})
}

// TestToMCPResultRendersAllSections ensures the optional resource/context
// sections of ToMCPResult are exercised.
func TestToMCPResultRendersAllSections(t *testing.T) {
	res := NewKubernetesError("op", errors.New("not found")).
		WithResource("Pod", "web").
		WithContext("namespace", "default").
		ToMCPResult()
	assert.True(t, res.IsError)
	assert.NotEmpty(t, res.Content)
}
