package telemetry

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Telemetry holds all telemetry-related configuration.
type Telemetry struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
	Protocol       string
	SamplingRatio  float64
	Insecure       bool
	Disabled       bool
}

// Config holds all application configuration.
type Config struct {
	Telemetry Telemetry
}

var (
	once   sync.Once
	config *Config
)

// LoadOtelCfg initializes and returns the application configuration.
func LoadOtelCfg() *Config {
	once.Do(func() {
		config = &Config{
			Telemetry: Telemetry{
				ServiceName:    getEnv("OTEL_SERVICE_NAME", "kagent-tools"),
				ServiceVersion: getEnv("OTEL_SERVICE_VERSION", "dev"),
				Environment:    getEnv("OTEL_ENVIRONMENT", "development"),
				Endpoint:       getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
				Protocol:       getEnv("OTEL_EXPORTER_OTLP_PROTOCOL", "auto"),
				SamplingRatio:  getEnvFloat("OTEL_TRACES_SAMPLER_ARG", 1.0),
				Insecure:       getEnvBool("OTEL_EXPORTER_OTLP_TRACES_INSECURE", false),
				Disabled:       getEnvBool("OTEL_SDK_DISABLED", false),
			},
		}
	})
	return config
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if valueStr, ok := os.LookupEnv(key); ok {
		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			return value
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if valueStr, ok := os.LookupEnv(key); ok {
		if value, err := strconv.ParseBool(strings.ToLower(valueStr)); err == nil {
			return value
		}
	}
	return fallback
}
