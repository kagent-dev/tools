package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// HTTPConfig holds HTTP transport-specific configuration
type HTTPConfig struct {
	Port            int
	ReadTimeout     int
	WriteTimeout    int
	ShutdownTimeout int
}

// RegisterHTTPFlags adds HTTP transport-specific flags to the root command
func RegisterHTTPFlags(cmd *cobra.Command) {
	cmd.Flags().IntP("http-port", "", 8080,
		"Port to run HTTP server on (1-65535). Set to 0 to disable HTTP mode. Default: 8080")

	cmd.Flags().IntP("http-read-timeout", "", 30,
		"HTTP request read timeout in seconds. Default: 30")

	cmd.Flags().IntP("http-write-timeout", "", 30,
		"HTTP response write timeout in seconds. Default: 30")

	cmd.Flags().IntP("http-shutdown-timeout", "", 10,
		"HTTP server graceful shutdown timeout in seconds. Default: 10")
}

// ValidateHTTPConfig validates HTTP configuration values
func ValidateHTTPConfig(cfg HTTPConfig) error {
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("http-port must be between 0-65535, got %d", cfg.Port)
	}

	if cfg.ReadTimeout <= 0 {
		return fmt.Errorf("http-read-timeout must be positive, got %d", cfg.ReadTimeout)
	}

	if cfg.WriteTimeout <= 0 {
		return fmt.Errorf("http-write-timeout must be positive, got %d", cfg.WriteTimeout)
	}

	if cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("http-shutdown-timeout must be positive, got %d", cfg.ShutdownTimeout)
	}

	return nil
}

// ExtractHTTPConfig extracts HTTP configuration from command flags
func ExtractHTTPConfig(cmd *cobra.Command) (*HTTPConfig, error) {
	httpPort, err := cmd.Flags().GetInt("http-port")
	if err != nil {
		return nil, fmt.Errorf("failed to get http-port flag: %w", err)
	}

	readTimeout, err := cmd.Flags().GetInt("http-read-timeout")
	if err != nil {
		return nil, fmt.Errorf("failed to get http-read-timeout flag: %w", err)
	}

	writeTimeout, err := cmd.Flags().GetInt("http-write-timeout")
	if err != nil {
		return nil, fmt.Errorf("failed to get http-write-timeout flag: %w", err)
	}

	shutdownTimeout, err := cmd.Flags().GetInt("http-shutdown-timeout")
	if err != nil {
		return nil, fmt.Errorf("failed to get http-shutdown-timeout flag: %w", err)
	}

	cfg := &HTTPConfig{
		Port:            httpPort,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ShutdownTimeout: shutdownTimeout,
	}

	if err := ValidateHTTPConfig(*cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
