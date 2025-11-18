package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterHTTPFlags(t *testing.T) {
	cmd := &cobra.Command{}
	RegisterHTTPFlags(cmd)

	// Verify all flags are registered
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("http-port"))
	assert.NotNil(t, flags.Lookup("http-read-timeout"))
	assert.NotNil(t, flags.Lookup("http-write-timeout"))
	assert.NotNil(t, flags.Lookup("http-shutdown-timeout"))
}

func TestValidateHTTPConfig_Valid(t *testing.T) {
	tests := []struct {
		name   string
		config HTTPConfig
	}{
		{
			name: "default configuration",
			config: HTTPConfig{
				Port:            8080,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
		},
		{
			name: "custom port",
			config: HTTPConfig{
				Port:            9000,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
		},
		{
			name: "port at minimum range",
			config: HTTPConfig{
				Port:            1,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
		},
		{
			name: "port at maximum range",
			config: HTTPConfig{
				Port:            65535,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
		},
		{
			name: "port zero (disabled)",
			config: HTTPConfig{
				Port:            0,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTPConfig(tt.config)
			assert.NoError(t, err)
		})
	}
}

func TestValidateHTTPConfig_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		config HTTPConfig
		errMsg string
	}{
		{
			name: "port too high",
			config: HTTPConfig{
				Port:            65536,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
			errMsg: "http-port must be between 0-65535",
		},
		{
			name: "port negative",
			config: HTTPConfig{
				Port:            -1,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
			errMsg: "http-port must be between 0-65535",
		},
		{
			name: "read timeout zero",
			config: HTTPConfig{
				Port:            8080,
				ReadTimeout:     0,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
			errMsg: "http-read-timeout must be positive",
		},
		{
			name: "read timeout negative",
			config: HTTPConfig{
				Port:            8080,
				ReadTimeout:     -5,
				WriteTimeout:    30,
				ShutdownTimeout: 10,
			},
			errMsg: "http-read-timeout must be positive",
		},
		{
			name: "write timeout negative",
			config: HTTPConfig{
				Port:            8080,
				ReadTimeout:     30,
				WriteTimeout:    -1,
				ShutdownTimeout: 10,
			},
			errMsg: "http-write-timeout must be zero or positive",
		},
		{
			name: "shutdown timeout zero",
			config: HTTPConfig{
				Port:            8080,
				ReadTimeout:     30,
				WriteTimeout:    30,
				ShutdownTimeout: 0,
			},
			errMsg: "http-shutdown-timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTPConfig(tt.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestExtractHTTPConfig_Valid(t *testing.T) {
	cmd := &cobra.Command{}
	RegisterHTTPFlags(cmd)

	// Set some flag values
	require.NoError(t, cmd.Flags().Set("http-port", "9000"))
	require.NoError(t, cmd.Flags().Set("http-read-timeout", "45"))
	require.NoError(t, cmd.Flags().Set("http-write-timeout", "60"))
	require.NoError(t, cmd.Flags().Set("http-shutdown-timeout", "15"))

	cfg, err := ExtractHTTPConfig(cmd)
	require.NoError(t, err)

	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, 45, cfg.ReadTimeout)
	assert.Equal(t, 60, cfg.WriteTimeout)
	assert.Equal(t, 15, cfg.ShutdownTimeout)
}

func TestExtractHTTPConfig_DefaultValues(t *testing.T) {
	cmd := &cobra.Command{}
	RegisterHTTPFlags(cmd)

	// Don't set any flags - use defaults
	cfg, err := ExtractHTTPConfig(cmd)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, 30, cfg.ReadTimeout)
	assert.Equal(t, 0, cfg.WriteTimeout)
	assert.Equal(t, 10, cfg.ShutdownTimeout)
}

func TestExtractHTTPConfig_InvalidValues(t *testing.T) {
	cmd := &cobra.Command{}
	RegisterHTTPFlags(cmd)

	// Set invalid values
	require.NoError(t, cmd.Flags().Set("http-port", "99999"))

	_, err := ExtractHTTPConfig(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http-port must be between 0-65535")
}
