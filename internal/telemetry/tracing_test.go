package telemetry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Test protocol constants for additional test scenarios
const (
	ProtocolInvalid = "invalid"
)

func TestLoadConfig(t *testing.T) {
	// Test default config
	config := LoadConfig()

	assert.Equal(t, "kagent-tools", config.ServiceName)
	assert.Equal(t, "dev", config.ServiceVersion)
	assert.Equal(t, "development", config.Environment)
	assert.Equal(t, "", config.Endpoint)
	assert.Equal(t, 1.0, config.SamplingRatio) // development env sets to 1.0
	assert.False(t, config.Disabled)
}

func TestLoadConfigWithEnvVars(t *testing.T) {
	// Set environment variables
	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	os.Setenv("OTEL_SERVICE_VERSION", "1.0.0")
	os.Setenv("OTEL_ENVIRONMENT", "production")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	os.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")
	os.Setenv("OTEL_SDK_DISABLED", "true")

	defer func() {
		// Clean up
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("OTEL_SERVICE_VERSION")
		os.Unsetenv("OTEL_ENVIRONMENT")
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		os.Unsetenv("OTEL_TRACES_SAMPLER_ARG")
		os.Unsetenv("OTEL_SDK_DISABLED")
	}()

	config := LoadConfig()

	assert.Equal(t, "test-service", config.ServiceName)
	assert.Equal(t, "1.0.0", config.ServiceVersion)
	assert.Equal(t, "production", config.Environment)
	assert.Equal(t, "http://localhost:4317", config.Endpoint)
	assert.Equal(t, 0.5, config.SamplingRatio)
	assert.True(t, config.Disabled)
}

func TestLoadConfigProductionSampling(t *testing.T) {
	// Test that production environment doesn't override sampling ratio
	os.Setenv("OTEL_ENVIRONMENT", "production")
	os.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")

	defer func() {
		os.Unsetenv("OTEL_ENVIRONMENT")
		os.Unsetenv("OTEL_TRACES_SAMPLER_ARG")
	}()

	config := LoadConfig()

	assert.Equal(t, "production", config.Environment)
	assert.Equal(t, 0.1, config.SamplingRatio)
}

func TestSetupOTelSDKDisabled(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Disabled: true,
	}

	shutdown, err := SetupOTelSDK(ctx, config)

	require.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Should not return error when called
	err = shutdown(ctx)
	assert.NoError(t, err)
}

func TestSetupOTelSDKEnabled(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "",
		SamplingRatio:  1.0,
		Disabled:       false,
	}

	shutdown, err := SetupOTelSDK(ctx, config)

	require.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Clean up
	err = shutdown(ctx)
	assert.NoError(t, err)
}

func TestNewTracerProviderDevelopment(t *testing.T) {
	ctx := context.Background()

	// Create a resource for testing
	res, err := createTestResource(ctx, "test-service", "1.0.0", "development")
	require.NoError(t, err)

	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "",
		SamplingRatio:  1.0,
		Disabled:       false,
	}

	tp, err := newTracerProvider(ctx, res, config)
	require.NoError(t, err)
	assert.NotNil(t, tp)

	// Clean up
	err = tp.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestNewTracerProviderProduction(t *testing.T) {
	ctx := context.Background()

	// Create a resource for testing
	res, err := createTestResource(ctx, "test-service", "1.0.0", "production")
	require.NoError(t, err)

	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		Endpoint:       "",
		SamplingRatio:  0.1,
		Disabled:       false,
	}

	tp, err := newTracerProvider(ctx, res, config)
	require.NoError(t, err)
	assert.NotNil(t, tp)

	// Clean up
	err = tp.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestCreateExporterDevelopment(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Environment: "development",
		Endpoint:    "",
	}

	exporter, err := createExporter(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, exporter)

	// Clean up
	err = exporter.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestCreateExporterNoEndpoint(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Environment: "production",
		Endpoint:    "",
	}

	exporter, err := createExporter(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, exporter)

	// Clean up
	err = exporter.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestCreateExporterWithEndpoint(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Environment: "production",
		Endpoint:    "http://localhost:4317",
		Protocol:    ProtocolAuto,
	}

	exporter, err := createExporter(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, exporter)

	// Clean up
	err = exporter.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestCreateExporterWithAuthHeaders(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Environment: "production",
		Endpoint:    "http://localhost:4317",
		Protocol:    ProtocolAuto,
	}

	// Set auth header
	os.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer token123")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_HEADERS")

	exporter, err := createExporter(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, exporter)

	// Clean up
	err = exporter.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestGetEnv(t *testing.T) {
	// Test with existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	result := getEnv("TEST_VAR", "default")
	assert.Equal(t, "test_value", result)

	// Test with non-existing environment variable
	result = getEnv("NON_EXISTING_VAR", "default")
	assert.Equal(t, "default", result)
}

func TestGetEnvFloat(t *testing.T) {
	// Test with valid float
	os.Setenv("TEST_FLOAT", "3.14")
	defer os.Unsetenv("TEST_FLOAT")

	result := getEnvFloat("TEST_FLOAT", 1.0)
	assert.Equal(t, 3.14, result)

	// Test with invalid float
	os.Setenv("TEST_INVALID_FLOAT", "not_a_float")
	defer os.Unsetenv("TEST_INVALID_FLOAT")

	result = getEnvFloat("TEST_INVALID_FLOAT", 1.0)
	assert.Equal(t, 1.0, result)

	// Test with non-existing environment variable
	result = getEnvFloat("NON_EXISTING_FLOAT", 2.0)
	assert.Equal(t, 2.0, result)
}

func TestGetEnvBool(t *testing.T) {
	// Test with valid true
	os.Setenv("TEST_BOOL_TRUE", "true")
	defer os.Unsetenv("TEST_BOOL_TRUE")

	result := getEnvBool("TEST_BOOL_TRUE", false)
	assert.True(t, result)

	// Test with valid false
	os.Setenv("TEST_BOOL_FALSE", "false")
	defer os.Unsetenv("TEST_BOOL_FALSE")

	result = getEnvBool("TEST_BOOL_FALSE", true)
	assert.False(t, result)

	// Test with invalid bool
	os.Setenv("TEST_INVALID_BOOL", "not_a_bool")
	defer os.Unsetenv("TEST_INVALID_BOOL")

	result = getEnvBool("TEST_INVALID_BOOL", true)
	assert.True(t, result)

	// Test with non-existing environment variable
	result = getEnvBool("NON_EXISTING_BOOL", false)
	assert.False(t, result)
}

func TestConfigDefaults(t *testing.T) {
	// Clear all relevant environment variables
	envVars := []string{
		"OTEL_SERVICE_NAME",
		"OTEL_SERVICE_VERSION",
		"OTEL_ENVIRONMENT",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_TRACES_SAMPLER_ARG",
		"OTEL_SDK_DISABLED",
	}

	originalValues := make(map[string]string)
	for _, envVar := range envVars {
		originalValues[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	defer func() {
		// Restore original values
		for _, envVar := range envVars {
			if originalValues[envVar] != "" {
				os.Setenv(envVar, originalValues[envVar])
			}
		}
	}()

	config := LoadConfig()

	assert.Equal(t, "kagent-tools", config.ServiceName)
	assert.Equal(t, "dev", config.ServiceVersion)
	assert.Equal(t, "development", config.Environment)
	assert.Equal(t, "", config.Endpoint)
	assert.Equal(t, ProtocolAuto, config.Protocol)
	assert.Equal(t, 1.0, config.SamplingRatio) // development env sets to 1.0
	assert.False(t, config.Disabled)
}

func TestConfigEnvironmentOverride(t *testing.T) {
	// Test that development environment overrides sampling ratio
	os.Setenv("OTEL_ENVIRONMENT", "development")
	os.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")

	defer func() {
		os.Unsetenv("OTEL_ENVIRONMENT")
		os.Unsetenv("OTEL_TRACES_SAMPLER_ARG")
	}()

	config := LoadConfig()

	assert.Equal(t, "development", config.Environment)
	assert.Equal(t, 1.0, config.SamplingRatio) // should be overridden to 1.0
}

func TestGetEnvFloatEdgeCases(t *testing.T) {
	// Test with zero
	os.Setenv("TEST_ZERO", "0")
	defer os.Unsetenv("TEST_ZERO")

	result := getEnvFloat("TEST_ZERO", 1.0)
	assert.Equal(t, 0.0, result)

	// Test with negative
	os.Setenv("TEST_NEGATIVE", "-1.5")
	defer os.Unsetenv("TEST_NEGATIVE")

	result = getEnvFloat("TEST_NEGATIVE", 1.0)
	assert.Equal(t, -1.5, result)
}

func TestGetEnvBoolEdgeCases(t *testing.T) {
	// Test with "1"
	os.Setenv("TEST_BOOL_1", "1")
	defer os.Unsetenv("TEST_BOOL_1")

	result := getEnvBool("TEST_BOOL_1", false)
	assert.True(t, result)

	// Test with "0"
	os.Setenv("TEST_BOOL_0", "0")
	defer os.Unsetenv("TEST_BOOL_0")

	result = getEnvBool("TEST_BOOL_0", true)
	assert.False(t, result)

	// Test with empty string
	os.Setenv("TEST_BOOL_EMPTY", "")
	defer os.Unsetenv("TEST_BOOL_EMPTY")

	result = getEnvBool("TEST_BOOL_EMPTY", true)
	assert.True(t, result) // should use default
}

// Helper function to create a test resource
func createTestResource(ctx context.Context, serviceName, serviceVersion, environment string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.DeploymentEnvironment(environment),
		),
	)
}

// Integration test with context cancellation
func TestSetupOTelSDKWithCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "",
		SamplingRatio:  1.0,
		Disabled:       false,
	}

	// This should still work even with short timeout since we're not making network calls
	shutdown, err := SetupOTelSDK(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Clean up
	err = shutdown(context.Background())
	assert.NoError(t, err)
}

func TestProtocolDetection(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{"gRPC port 4317", "http://localhost:4317", ProtocolGRPC},
		{"HTTP port 4318", "http://localhost:4318", ProtocolHTTP},
		{"gRPC port 4317 without scheme", "localhost:4317", ProtocolGRPC},
		{"HTTP port 4318 without scheme", "localhost:4318", ProtocolHTTP},
		{"gRPC with docker internal", "http://host.docker.internal:4317", ProtocolGRPC},
		{"HTTP with docker internal", "http://host.docker.internal:4318", ProtocolHTTP},
		{"No port specified", "http://localhost", ProtocolHTTP},
		{"Unknown port", "http://localhost:9090", ProtocolHTTP},
		{"HTTPS with gRPC port", "https://otel-collector.example.com:4317", ProtocolGRPC},
		{"HTTPS with HTTP port", "https://otel-collector.example.com:4318", ProtocolHTTP},
		{"gRPC with path", "http://localhost:4317/v1/traces", ProtocolGRPC},
		{"HTTP with path", "http://localhost:4318/v1/traces", ProtocolHTTP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectProtocol(tt.endpoint)
			assert.Equal(t, tt.expected, result, "Protocol detection failed for endpoint: %s", tt.endpoint)
		})
	}
}

func TestEndpointNormalization(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{"Basic gRPC endpoint", "http://localhost:4317", "localhost:4317"},
		{"gRPC with path", "http://localhost:4317/v1/traces", "localhost:4317"},
		{"gRPC without scheme", "localhost:4317", "localhost:4317"},
		{"gRPC with HTTPS", "https://otel.example.com:4317", "otel.example.com:4317"},
		{"Docker internal gRPC", "http://host.docker.internal:4317", "host.docker.internal:4317"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeGRPCEndpoint(tt.endpoint)
			assert.Equal(t, tt.expected, result, "gRPC endpoint normalization failed for: %s", tt.endpoint)
		})
	}
}

func TestHTTPEndpointNormalization(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{"Basic HTTP endpoint", "http://localhost:4318", "http://localhost:4318/v1/traces"},
		{"HTTP with path", "http://localhost:4318/v1/traces", "http://localhost:4318/v1/traces"},
		{"HTTP without scheme", "localhost:4318", "http://localhost:4318/v1/traces"},
		{"HTTP with trailing slash", "http://localhost:4318/", "http://localhost:4318/v1/traces"},
		{"Docker internal HTTP", "host.docker.internal:4318", "http://host.docker.internal:4318/v1/traces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeHTTPEndpoint(tt.endpoint)
			assert.Equal(t, tt.expected, result, "HTTP endpoint normalization failed for: %s", tt.endpoint)
		})
	}
}

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			"Empty string",
			"",
			map[string]string{},
		},
		{
			"Single header",
			"Authorization=Bearer token123",
			map[string]string{"Authorization": "Bearer token123"},
		},
		{
			"Multiple headers",
			"Authorization=Bearer token123,Content-Type=application/json",
			map[string]string{"Authorization": "Bearer token123", "Content-Type": "application/json"},
		},
		{
			"Headers with spaces",
			"Authorization = Bearer token123 , Content-Type = application/json",
			map[string]string{"Authorization": "Bearer token123", "Content-Type": "application/json"},
		},
		{
			"Invalid header format",
			"InvalidHeader",
			map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHeaders(tt.input)
			assert.Equal(t, tt.expected, result, "Header parsing failed for: %s", tt.input)
		})
	}
}

func TestCreateExporterWithProtocol(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		config      *Config
		shouldError bool
		description string
	}{
		{
			"gRPC protocol",
			&Config{
				Environment: "development",
				Endpoint:    "localhost:4317",
				Protocol:    ProtocolGRPC,
			},
			false,
			"Should create gRPC exporter",
		},
		{
			"HTTP protocol",
			&Config{
				Environment: "development",
				Endpoint:    "localhost:4318",
				Protocol:    ProtocolHTTP,
			},
			false,
			"Should create HTTP exporter",
		},
		{
			"Auto protocol with gRPC port",
			&Config{
				Environment: "development",
				Endpoint:    "localhost:4317",
				Protocol:    ProtocolAuto,
			},
			false,
			"Should auto-detect gRPC",
		},
		{
			"Auto protocol with HTTP port",
			&Config{
				Environment: "development",
				Endpoint:    "localhost:4318",
				Protocol:    ProtocolAuto,
			},
			false,
			"Should auto-detect HTTP",
		},
		{
			"Invalid protocol",
			&Config{
				Environment: "development",
				Endpoint:    "localhost:4317",
				Protocol:    ProtocolInvalid,
			},
			true,
			"Should fail with unsupported protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := createExporter(ctx, tt.config)

			if tt.shouldError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, exporter)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, exporter)
				if exporter != nil {
					_ = exporter.Shutdown(ctx)
				}
			}
		})
	}
}
