package telemetry

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvFloat(t *testing.T) {
	const key = "KAGENT_TEST_ENV_FLOAT"

	assert.Equal(t, 1.5, getEnvFloat(key, 1.5)) // unset -> fallback

	t.Setenv(key, "0.25")
	assert.Equal(t, 0.25, getEnvFloat(key, 1.5)) // parsed

	t.Setenv(key, "not-a-float")
	assert.Equal(t, 1.5, getEnvFloat(key, 1.5)) // parse error -> fallback
}

func TestLoad(t *testing.T) {
	// Reset singleton for testing
	once = sync.Once{}
	config = nil

	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	os.Setenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE", "true")
	defer func() {
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE")
	}()

	cfg := LoadOtelCfg()
	assert.Equal(t, "test-service", cfg.Telemetry.ServiceName)
	assert.True(t, cfg.Telemetry.Insecure)
}

func TestLoadDefaults(t *testing.T) {
	// Reset singleton for testing
	once = sync.Once{}
	config = nil

	cfg := LoadOtelCfg()
	assert.Equal(t, "kagent-tools", cfg.Telemetry.ServiceName)
	assert.False(t, cfg.Telemetry.Insecure)
	assert.Equal(t, 1.0, cfg.Telemetry.SamplingRatio)
}

func TestLoadDevelopmentSampling(t *testing.T) {
	// Reset singleton for testing
	once = sync.Once{}
	config = nil

	os.Setenv("OTEL_ENVIRONMENT", "development")
	defer os.Unsetenv("OTEL_ENVIRONMENT")

	cfg := LoadOtelCfg()
	assert.Equal(t, 1.0, cfg.Telemetry.SamplingRatio)
}
