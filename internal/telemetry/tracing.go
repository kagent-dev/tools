package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
	SamplingRatio  float64
	Disabled       bool
}

func LoadConfig() *Config {
	config := &Config{
		ServiceName:    getEnv("OTEL_SERVICE_NAME", "kagent-tools"),
		ServiceVersion: getEnv("OTEL_SERVICE_VERSION", "dev"),
		Environment:    getEnv("OTEL_ENVIRONMENT", "development"),
		Endpoint:       getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		SamplingRatio:  getEnvFloat("OTEL_TRACES_SAMPLER_ARG", 0.1),
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

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.Endpoint),
		otlptracehttp.WithTimeout(30 * time.Second),
	}

	if authToken := getEnv("OTEL_EXPORTER_OTLP_HEADERS", ""); authToken != "" {
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": authToken,
		}))
	}

	return otlptracehttp.New(ctx, opts...)
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
