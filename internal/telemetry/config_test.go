package telemetry

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Reset singleton for testing
	once = sync.Once{}
	config = nil

	_ = os.Setenv("OTEL_SERVICE_NAME", "test-service")
	_ = os.Setenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE", "true")
	defer func() {
		_ = os.Unsetenv("OTEL_SERVICE_NAME")
		_ = os.Unsetenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE")
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

	_ = os.Setenv("OTEL_ENVIRONMENT", "development")
	defer func() { _ = os.Unsetenv("OTEL_ENVIRONMENT") }()

	cfg := LoadOtelCfg()
	assert.Equal(t, 1.0, cfg.Telemetry.SamplingRatio)
}
