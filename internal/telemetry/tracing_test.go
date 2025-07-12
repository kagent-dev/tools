package telemetry

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace/noop"
)

// Test protocol constants for additional test scenarios
const (
	ProtocolInvalid = "invalid"
)

// resetConfig is a helper to reset the singleton config for tests
func resetConfig() {
	once = sync.Once{}
	config = nil
}

func TestSetupOTelSDK_Disabled(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	err := os.Setenv("OTEL_SDK_DISABLED", "true")
	require.NoError(t, err)
	defer func() {
		_ = os.Unsetenv("OTEL_SDK_DISABLED")
	}()
	resetConfig()

	err = SetupOTelSDK(ctx)
	require.NoError(t, err)

	// In a disabled state, the tracer provider should be a no-op provider
	tp := otel.GetTracerProvider()
	assert.IsType(t, noop.NewTracerProvider(), tp)

	// Shutdown should be a no-op function
	assert.NoError(t, err)
}

func TestSetupOTelSDKEnabled(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	err := os.Setenv(OtelSdkDisabled, "false")
	require.NoError(t, err)
	defer func() {
		_ = os.Unsetenv(OtelSdkDisabled)
	}()

	err = SetupOTelSDK(ctx)
	require.NoError(t, err)
}

func TestNewTracerProviderDevelopment(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	res := resource.NewSchemaless()
	cfg := &Telemetry{
		Environment: "development",
	}
	exporter, _ := stdouttrace.New()

	tp, err := newTracerProvider(ctx, cfg, exporter, res)
	require.NoError(t, err)
	assert.NotNil(t, tp)
}

func TestNewTracerProviderProduction(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	res := resource.NewSchemaless()
	cfg := &Telemetry{
		Environment:   "production",
		SamplingRatio: 0.5,
	}
	exporter, _ := stdouttrace.New()

	tp, err := newTracerProvider(ctx, cfg, exporter, res)
	require.NoError(t, err)
	assert.NotNil(t, tp)
}

func TestCreateExporterDevelopment(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	cfg := &Telemetry{
		Environment: "development",
	}

	exporter, err := createExporter(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.IsType(t, &stdouttrace.Exporter{}, exporter)
}

func TestCreateExporterNoEndpoint(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	cfg := &Telemetry{
		Environment: "production",
	}

	exporter, err := createExporter(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.IsType(t, &stdouttrace.Exporter{}, exporter)
}

func TestCreateExporterWithEndpoint(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	cfg := &Telemetry{
		Environment: "production",
		Endpoint:    "http://localhost:4317",
		Protocol:    ProtocolAuto,
	}

	exporter, err := createExporter(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, exporter)
}

func TestCreateExporterWithInsecure(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	cfg := &Telemetry{
		Environment: "production",
		Endpoint:    "localhost:4317",
		Insecure:    true,
	}

	// This should not fail, as insecure is handled by the exporters
	_, err := createExporter(ctx, cfg)
	require.NoError(t, err)
}

func TestCreateExporterWithAuthHeaders(t *testing.T) {
	resetConfig()
	ctx := context.Background()
	cfg := &Telemetry{
		Environment: "production",
		Endpoint:    "http://localhost:4317",
		Protocol:    ProtocolAuto,
	}

	// Set auth header
	err := os.Setenv(OtelExporterOtlpHeaders, "Authorization=Bearer token123")
	require.NoError(t, err)
	defer func() {
		_ = os.Unsetenv(OtelExporterOtlpHeaders)
	}()

	exporter, err := createExporter(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, exporter)

	// Clean up
	err = exporter.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestSetupOTelSDKWithCancellation(t *testing.T) {
	resetConfig()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel context immediately

	err := SetupOTelSDK(ctx)
	require.Error(t, err) // Expect an error due to context cancellation
}

func TestProtocolDetection(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{"gRPC port 4317", "localhost:4317", ProtocolGRPC},
		{"HTTP port 4318", "localhost:4318", ProtocolHTTP},
		{"gRPC port 4317 without scheme", "localhost:4317", ProtocolGRPC},
		{"HTTP port 4318 without scheme", "localhost:4318", ProtocolHTTP},
		{"gRPC with docker internal", "host.docker.internal:4317", ProtocolGRPC},
		{"HTTP with docker internal", "host.docker.internal:4318", ProtocolHTTP},
		{"No port specified", "localhost", ProtocolHTTP},
		{"Unknown port", "localhost:1234", ProtocolHTTP},
		{"HTTPS with gRPC port", "https://localhost:4317", ProtocolGRPC},
		{"HTTPS with HTTP port", "https://localhost:4318", ProtocolHTTP},
		{"gRPC with path", "localhost:4317/v1/traces", ProtocolGRPC},
		{"HTTP with path", "localhost:4318/v1/traces", ProtocolHTTP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectProtocol(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEndpointNormalization(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{"Basic gRPC endpoint", "localhost:4317", "localhost:4317"},
		{"gRPC with path", "localhost:4317/v1/traces", "localhost:4317/v1/traces"},
		{"gRPC without scheme", "localhost:4317", "localhost:4317"},
		{"gRPC with HTTPS", "https://localhost:4317", "localhost:4317"},
		{"Docker internal gRPC", "host.docker.internal:4317", "host.docker.internal:4317"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeGRPCEndpoint(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPEndpointNormalization(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		insecure bool
		expected string
	}{
		{"Basic HTTP endpoint", "http://localhost:4318", false, "http://localhost:4318/v1/traces"},
		{"HTTP with path", "http://localhost:4318/v1/traces", false, "http://localhost:4318/v1/traces"},
		{"HTTP without scheme - secure localhost", "localhost:4318", false, "http://localhost:4318/v1/traces"},
		{"HTTP without scheme - insecure localhost", "localhost:4318", true, "http://localhost:4318/v1/traces"},
		{"HTTP with trailing slash", "http://localhost:4318/", false, "http://localhost:4318/v1/traces"},
		{"Docker internal HTTP - secure", "host.docker.internal:4318", false, "http://host.docker.internal:4318/v1/traces"},
		{"Docker internal HTTP - insecure", "host.docker.internal:4318", true, "http://host.docker.internal:4318/v1/traces"},
		{"Remote endpoint - secure", "collector.example.com:4318", false, "https://collector.example.com:4318/v1/traces"},
		{"Remote endpoint - insecure", "collector.example.com:4318", true, "http://collector.example.com:4318/v1/traces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeHTTPEndpoint(tt.endpoint, tt.insecure)
			assert.Equal(t, tt.expected, result, "HTTP endpoint normalization failed for: %s", tt.endpoint)
		})
	}
}

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers string
		want    map[string]string
	}{
		{"Empty string", "", map[string]string{}},
		{"Single header", "key=value", map[string]string{"key": "value"}},
		{"Multiple headers", "key1=value1,key2=value2", map[string]string{"key1": "value1", "key2": "value2"}},
		{"Headers with spaces", " key1 = value1 ,  key2  =  value2  ", map[string]string{"key1": "value1", "key2": "value2"}},
		{"Invalid header format", "key-value,key2", map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHeaders(tt.headers)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateExporterWithProtocol(t *testing.T) {

	ctx := context.Background()

	tests := []struct {
		name        string
		config      *Telemetry
		shouldError bool
		description string
	}{
		{
			"gRPC protocol",
			&Telemetry{
				Environment: "development",
				Endpoint:    "localhost:4317",
				Protocol:    ProtocolGRPC,
			},
			false,
			"Should create gRPC exporter",
		},
		{
			"HTTP protocol",
			&Telemetry{
				Environment: "development",
				Endpoint:    "localhost:4318",
				Protocol:    ProtocolHTTP,
			},
			false,
			"Should create HTTP exporter",
		},
		{
			"Auto protocol with gRPC port",
			&Telemetry{
				Environment: "development",
				Endpoint:    "localhost:4317",
				Protocol:    ProtocolAuto,
			},
			false,
			"Should auto-detect gRPC",
		},
		{
			"Auto protocol with HTTP port",
			&Telemetry{
				Environment: "development",
				Endpoint:    "localhost:4318",
				Protocol:    ProtocolAuto,
			},
			false,
			"Should auto-detect HTTP",
		},
		{
			"gRPC protocol with insecure",
			&Telemetry{
				Environment: "production",
				Endpoint:    "localhost:4317",
				Protocol:    ProtocolGRPC,
				Insecure:    true,
			},
			false,
			"Should create gRPC exporter with insecure",
		},
		{
			"HTTP protocol with insecure",
			&Telemetry{
				Environment: "production",
				Endpoint:    "localhost:4318",
				Protocol:    ProtocolHTTP,
				Insecure:    true,
			},
			false,
			"Should create HTTP exporter with insecure",
		},
		{
			"Invalid protocol",
			&Telemetry{
				Environment: "development",
				Endpoint:    "localhost:1234",
				Protocol:    ProtocolInvalid,
			},
			true,
			"Should return error for invalid protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConfig()
			exporter, err := createExporter(ctx, tt.config)
			if tt.shouldError {
				require.Error(t, err, tt.description)
				assert.Nil(t, exporter, tt.description)
			} else {
				require.NoError(t, err, tt.description)
				assert.NotNil(t, exporter, tt.description)
				err = exporter.Shutdown(ctx)
				assert.NoError(t, err)
			}
		})
	}
}
