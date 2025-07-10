package telemetry

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/config"
	"github.com/kagent-dev/tools/internal/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace/noop"
)

// Environment variable keys for telemetry configuration
const (
	OtelServiceName          = "OTEL_SERVICE_NAME"
	OtelServiceVersion       = "OTEL_SERVICE_VERSION"
	OtelEnvironment          = "OTEL_ENVIRONMENT"
	OtelExporterOtlpEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	OtelExporterOtlpProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"
	OtelTracesSamplerArg     = "OTEL_TRACES_SAMPLER_ARG"
	OtelExporterOtlpInsecure = "OTEL_EXPORTER_OTLP_TRACES_INSECURE"
	OtelSdkDisabled          = "OTEL_SDK_DISABLED"
	OtelExporterOtlpHeaders  = "OTEL_EXPORTER_OTLP_HEADERS"
)

// Protocol constants for OTLP exporters
const (
	ProtocolGRPC = "grpc"
	ProtocolHTTP = "http"
	ProtocolAuto = "auto"
)

// SetupOTelSDK initializes the OpenTelemetry SDK
func SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	log := logger.WithContext(ctx)
	cfg := config.Load()
	telemetryConfig := cfg.Telemetry

	// If tracing is disabled, set a no-op tracer provider and return.
	// This prevents further initialization and ensures no traces are exported.
	if cfg.Telemetry.Disabled {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithDetectors(), // Detectors for cloud provider, k8s, etc.
		resource.WithAttributes(
			semconv.ServiceNameKey.String(telemetryConfig.ServiceName),
			semconv.ServiceVersionKey.String(telemetryConfig.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(telemetryConfig.Environment),
		),
	)
	if err != nil {
		log.Error("failed to create resource", "error", err)
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Set up propagator
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(prop)

	exporter, err := createExporter(ctx, &telemetryConfig)
	if err != nil {
		log.Error("failed to create exporter", "error", err)
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Set up trace provider
	tracerProvider, err := newTracerProvider(ctx, &telemetryConfig, exporter, res)
	if err != nil {
		log.Error("failed to create tracer provider", "error", err)
		return nil, fmt.Errorf("failed to create tracer provider: %w", err)
	}
	otel.SetTracerProvider(tracerProvider)

	log.Info("OpenTelemetry SDK successfully initialized")
	return tracerProvider.Shutdown, nil
}

// newTracerProvider creates a new trace provider
func newTracerProvider(ctx context.Context, cfg *config.Telemetry, exporter sdktrace.SpanExporter, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sampler := sdktrace.TraceIDRatioBased(cfg.SamplingRatio)
	if cfg.Environment == "development" {
		// In development, always sample for better debugging
		sampler = sdktrace.AlwaysSample()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return tp, nil
}

// createExporter creates a OTLP exporter
func createExporter(ctx context.Context, cfg *config.Telemetry) (sdktrace.SpanExporter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Environment == "development" && cfg.Endpoint == "" {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	if cfg.Endpoint == "" {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	// Determine protocol
	protocol := cfg.Protocol
	if protocol == ProtocolAuto || protocol == "" {
		protocol = detectProtocol(cfg.Endpoint)
	}

	switch strings.ToLower(protocol) {
	case ProtocolGRPC:
		return createGRPCExporter(ctx, cfg)
	case ProtocolHTTP:
		return createHTTPExporter(ctx, cfg)
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
func createGRPCExporter(ctx context.Context, cfg *config.Telemetry) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(normalizeGRPCEndpoint(cfg.Endpoint)),
		otlptracegrpc.WithTimeout(30 * time.Second),
	}

	// Use insecure connection if explicitly configured
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if authToken := os.Getenv(OtelExporterOtlpHeaders); authToken != "" {
		opts = append(opts, otlptracegrpc.WithHeaders(parseHeaders(authToken)))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createHTTPExporter creates an HTTP OTLP exporter
func createHTTPExporter(ctx context.Context, cfg *config.Telemetry) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(normalizeHTTPEndpoint(cfg.Endpoint, cfg.Insecure)),
		otlptracehttp.WithTimeout(30 * time.Second),
	}

	// Use insecure connection if explicitly configured
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if authToken := os.Getenv(OtelExporterOtlpHeaders); authToken != "" {
		opts = append(opts, otlptracehttp.WithHeaders(parseHeaders(authToken)))
	}

	return otlptracehttp.New(ctx, opts...)
}

// normalizeGRPCEndpoint normalizes the endpoint for gRPC usage
func normalizeGRPCEndpoint(endpoint string) string {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint // Should not happen with the check above, but as a safeguard
	}

	return u.Host + u.Path
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

// parseHeaders parses a comma-separated string of headers into a map
func parseHeaders(headers string) map[string]string {
	headerMap := make(map[string]string)
	for _, h := range strings.Split(headers, ",") {
		if parts := strings.SplitN(h, "=", 2); len(parts) == 2 {
			headerMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headerMap
}
