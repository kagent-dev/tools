package telemetry

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Protocol constants for OTLP exporters
const (
	ProtocolGRPC = "grpc"
	ProtocolHTTP = "http"
	ProtocolAuto = "auto"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
	Protocol       string // ProtocolGRPC, ProtocolHTTP, or ProtocolAuto (default)
	SamplingRatio  float64
	Insecure       bool
	Disabled       bool
}

func LoadConfig() *Config {
	config := &Config{
		ServiceName:    getEnv("OTEL_SERVICE_NAME", "kagent-tools"),
		ServiceVersion: getEnv("OTEL_SERVICE_VERSION", "dev"),
		Environment:    getEnv("OTEL_ENVIRONMENT", "development"),
		Endpoint:       getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		Protocol:       getEnv("OTEL_EXPORTER_OTLP_PROTOCOL", ProtocolAuto),
		SamplingRatio:  getEnvFloat("OTEL_TRACES_SAMPLER_ARG", 1),
		Insecure:       getEnvBool("OTEL_EXPORTER_OTLP_TRACES_INSECURE", false),
		Disabled:       getEnvBool("OTEL_SDK_DISABLED", false),
	}

	if config.Environment == "development" {
		config.SamplingRatio = 1.0
	}

	return config
}

func SetupOTelSDK(ctx context.Context, config *Config) (shutdown func(context.Context) error, err error) {
	if config.Disabled {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	tracerProvider, err := newTracerProvider(ctx, res, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create tracer provider: %w", err)
	}
	otel.SetTracerProvider(tracerProvider)

	return tracerProvider.Shutdown, nil
}

func newTracerProvider(ctx context.Context, res *resource.Resource, config *Config) (*trace.TracerProvider, error) {
	exporter, err := createExporter(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	sampler := trace.TraceIDRatioBased(config.SamplingRatio)
	if config.Environment == "development" {
		sampler = trace.AlwaysSample()
	}

	batchTimeout := time.Second * 5
	maxExportBatchSize := 512
	maxQueueSize := 2048

	if config.Environment == "development" {
		batchTimeout = time.Second * 1
		maxExportBatchSize = 256
		maxQueueSize = 1024
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			trace.WithBatchTimeout(batchTimeout),
			trace.WithMaxExportBatchSize(maxExportBatchSize),
			trace.WithMaxQueueSize(maxQueueSize),
		),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	return tp, nil
}

func createExporter(ctx context.Context, config *Config) (trace.SpanExporter, error) {
	if config.Environment == "development" && config.Endpoint == "" {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	if config.Endpoint == "" {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	// Determine protocol
	protocol := config.Protocol
	if protocol == ProtocolAuto || protocol == "" {
		protocol = detectProtocol(config.Endpoint)
	}

	switch strings.ToLower(protocol) {
	case ProtocolGRPC:
		return createGRPCExporter(ctx, config)
	case ProtocolHTTP:
		return createHTTPExporter(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s (supported: %s, %s)", protocol, ProtocolGRPC, ProtocolHTTP)
	}
}

// detectProtocol determines the protocol based on the endpoint URL
func detectProtocol(endpoint string) string {
	// Parse URL to extract port
	if parsedURL, err := url.Parse(endpoint); err == nil {
		port := parsedURL.Port()
		if port == "" {
			// Check for default ports in hostname
			if strings.Contains(parsedURL.Host, ":4317") {
				return ProtocolGRPC
			}
			if strings.Contains(parsedURL.Host, ":4318") {
				return ProtocolHTTP
			}
		} else {
			switch port {
			case "4317":
				return ProtocolGRPC
			case "4318":
				return ProtocolHTTP
			}
		}
	}

	// Check if endpoint contains port info directly
	if strings.Contains(endpoint, ":4317") {
		return ProtocolGRPC
	}
	if strings.Contains(endpoint, ":4318") {
		return ProtocolHTTP
	}

	// Default to HTTP for backward compatibility
	return ProtocolHTTP
}

// createGRPCExporter creates a gRPC OTLP exporter
func createGRPCExporter(ctx context.Context, config *Config) (trace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(normalizeGRPCEndpoint(config.Endpoint)),
		otlptracegrpc.WithTimeout(30 * time.Second),
	}

	// Use insecure connection if explicitly configured or for development/localhost
	if config.Insecure || config.Environment == "development" || strings.Contains(config.Endpoint, "localhost") || strings.Contains(config.Endpoint, "127.0.0.1") {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if authToken := getEnv("OTEL_EXPORTER_OTLP_HEADERS", ""); authToken != "" {
		opts = append(opts, otlptracegrpc.WithHeaders(parseHeaders(authToken)))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createHTTPExporter creates an HTTP OTLP exporter
func createHTTPExporter(ctx context.Context, config *Config) (trace.SpanExporter, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(normalizeHTTPEndpoint(config.Endpoint, config.Insecure)),
		otlptracehttp.WithTimeout(30 * time.Second),
	}

	// Use insecure connection if explicitly configured
	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if authToken := getEnv("OTEL_EXPORTER_OTLP_HEADERS", ""); authToken != "" {
		opts = append(opts, otlptracehttp.WithHeaders(parseHeaders(authToken)))
	}

	return otlptracehttp.New(ctx, opts...)
}

// normalizeGRPCEndpoint normalizes the endpoint for gRPC usage
func normalizeGRPCEndpoint(endpoint string) string {
	// Remove http:// or https:// prefix for gRPC
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Remove /v1/traces suffix if present
	endpoint = strings.TrimSuffix(endpoint, "/v1/traces")

	return endpoint
}

// normalizeHTTPEndpoint normalizes the endpoint for HTTP usage
func normalizeHTTPEndpoint(endpoint string, insecure bool) string {
	// Ensure we have a proper HTTP URL
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		// Use HTTP if insecure is true or if endpoint contains localhost/127.0.0.1/docker.internal
		if insecure || strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1") || strings.Contains(endpoint, "docker.internal") {
			endpoint = "http://" + endpoint
		} else {
			endpoint = "https://" + endpoint
		}
	}

	// Add /v1/traces suffix if not present
	if !strings.HasSuffix(endpoint, "/v1/traces") {
		endpoint = strings.TrimSuffix(endpoint, "/") + "/v1/traces"
	}

	return endpoint
}

// parseHeaders parses header string into map
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}

	// Simple parsing - expect "key=value,key2=value2" format
	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		if parts := strings.SplitN(strings.TrimSpace(pair), "=", 2); len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return headers
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
