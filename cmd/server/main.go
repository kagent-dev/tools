package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/kagent-dev/tools/internal/logger"
	mcpinternal "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/internal/version"
	"github.com/kagent-dev/tools/pkg/argo"
	"github.com/kagent-dev/tools/pkg/cilium"
	"github.com/kagent-dev/tools/pkg/helm"
	"github.com/kagent-dev/tools/pkg/istio"
	"github.com/kagent-dev/tools/pkg/k8s"
	"github.com/kagent-dev/tools/pkg/prometheus"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	port        int
	httpPort    int
	stdio       bool = true // Default to stdio mode
	tools       []string
	kubeconfig  *string
	logLevel    string
	showVersion bool

	// These variables should be set during build time using -ldflags
	Name      = "kagent-tools-server"
	Version   = version.Version
	GitCommit = version.GitCommit
	BuildDate = version.BuildDate
)

var rootCmd = &cobra.Command{
	Use:   "tool-server",
	Short: "KAgent tool server",
	Run:   run,
}

func init() {
	rootCmd.Flags().IntVarP(&port, "port", "p", 8084, "Port to run the server on (deprecated, use --http-port)")
	rootCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level")
	rootCmd.Flags().BoolVar(&stdio, "stdio", true, "Use stdio for communication (default: true). Set --http-port to automatically use HTTP mode, or --stdio=false to force HTTP mode")
	rootCmd.Flags().StringSliceVar(&tools, "tools", []string{}, "List of tools to register. If empty, all tools are registered.")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information and exit")
	kubeconfig = rootCmd.Flags().String("kubeconfig", "", "kubeconfig file path (optional, defaults to in-cluster config)")

	// Register HTTP-specific flags
	cmd.RegisterHTTPFlags(rootCmd)

	// if found .env file, load it
	if _, err := os.Stat(".env"); err == nil {
		_ = godotenv.Load(".env")
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Use stderr directly for error before logger is initialized
		// This is safe because it's before any stdio transport is started
		fmt.Fprintf(os.Stderr, "Failed to start tools mcp server: %v\n", err)
		os.Exit(1)
	}
}

// printVersion displays version information in a formatted way
func printVersion() {
	fmt.Printf("%s\n", Name)
	fmt.Printf("Version:    %s\n", Version)
	fmt.Printf("Git Commit: %s\n", GitCommit)
	fmt.Printf("Build Date: %s\n", BuildDate)
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func run(command *cobra.Command, args []string) {
	// Handle version flag early, before any initialization
	if showVersion {
		printVersion()
		return
	}

	// Extract HTTP configuration from flags
	httpConfig, err := cmd.ExtractHTTPConfig(command)
	if err != nil {
		// Use stderr directly for error before logger is initialized
		fmt.Fprintf(os.Stderr, "Failed to parse HTTP configuration: %v\n", err)
		os.Exit(1)
	}
	httpPort = httpConfig.Port

	// Determine transport mode:
	// 1. If --stdio is explicitly set to false, use HTTP mode
	// 2. If --http-port is explicitly set to a non-zero value, use HTTP mode
	// 3. Otherwise, use stdio mode (default)
	if command.Flags().Changed("stdio") && !stdio {
		// User explicitly set --stdio=false, use HTTP mode
		stdio = false
	} else if command.Flags().Changed("http-port") && httpPort > 0 {
		// User explicitly set --http-port to a non-zero value, use HTTP mode
		stdio = false
	} else {
		// Default to stdio mode (even if http-port has default value)
		stdio = true
	}

	// Initialize logger FIRST, before any logging calls
	// This ensures all log.Info calls use stderr when stdio mode is enabled
	logger.Init(stdio, logLevel)
	defer logger.Sync()

	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OpenTelemetry tracing
	cfg := telemetry.LoadOtelCfg()

	if err = telemetry.SetupOTelSDK(ctx); err != nil {
		logger.Get().Error("Failed to setup OpenTelemetry SDK", "error", err)
		os.Exit(1)
	}

	// Start root span for server lifecycle
	tracer := otel.Tracer("kagent-tools/server")
	ctx, rootSpan := tracer.Start(ctx, "server.lifecycle")
	defer rootSpan.End()

	// Determine effective port (httpPort takes precedence if HTTP mode is used)
	effectivePort := httpPort
	if stdio {
		effectivePort = port
	}

	rootSpan.SetAttributes(
		attribute.String("server.name", Name),
		attribute.String("server.version", cfg.Telemetry.ServiceVersion),
		attribute.String("server.git_commit", GitCommit),
		attribute.String("server.build_date", BuildDate),
		attribute.Bool("server.stdio_mode", stdio),
		attribute.Int("server.port", effectivePort),
		attribute.StringSlice("server.tools", tools),
	)

	logger.Get().Info("Starting "+Name, "version", Version, "git_commit", GitCommit, "build_date", BuildDate, "mode", map[bool]string{true: "stdio", false: "http"}[stdio])

	// Create shared tool registry
	toolRegistry := mcpinternal.NewToolRegistry()

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: Version,
	}, nil)

	// Register tools with both MCP server and tool registry
	registerMCP(mcpServer, toolRegistry, tools, *kubeconfig)
	logger.Get().Info("Registered tools", "count", toolRegistry.Count())

	// Select transport based on mode
	var transport mcpinternal.Transport

	if stdio {
		transport = mcpinternal.NewStdioTransport(mcpServer)
		logger.Get().Info("Using stdio transport")
	} else {
		httpTransport, err := mcpinternal.NewHTTPTransport(mcpServer, mcpinternal.HTTPTransportConfig{
			Port:              httpConfig.Port,
			ReadTimeout:       time.Duration(httpConfig.ReadTimeout) * time.Second,
			WriteTimeout:      time.Duration(httpConfig.WriteTimeout) * time.Second,
			IdleTimeout:       0, // use default behaviour inside transport
			ReadHeaderTimeout: 0,
			ShutdownTimeout:   time.Duration(httpConfig.ShutdownTimeout) * time.Second,
		})
		if err != nil {
			logger.Get().Error("Failed to configure HTTP transport", "error", err)
			os.Exit(1)
		}
		transport = httpTransport
		logger.Get().Info("Using HTTP transport", "port", httpConfig.Port)
	}

	// Create wait group for server goroutines
	var wg sync.WaitGroup

	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Channel to track when transport has started
	transportErrorChan := make(chan error, 1)

	// Start transport in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := transport.Start(ctx); err != nil {
			logger.Get().Error("Transport error", "error", err, "transport", transport.GetName())
			rootSpan.RecordError(err)
			rootSpan.SetStatus(codes.Error, fmt.Sprintf("Transport error: %v", err))
			transportErrorChan <- err
			cancel()
		}
	}()

	// Wait for termination signal
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-signalChan
		logger.Get().Info("Received termination signal, shutting down server...")

		// Mark root span as shutting down
		rootSpan.AddEvent("server.shutdown.initiated")

		// Cancel context to initiate graceful shutdown
		cancel()

		// Give transport time to gracefully shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := transport.Stop(shutdownCtx); err != nil {
			logger.Get().Error("Failed to shutdown transport gracefully", "error", err, "transport", transport.GetName())
			rootSpan.RecordError(err)
			rootSpan.SetStatus(codes.Error, "Transport shutdown failed")
		} else {
			rootSpan.AddEvent("server.shutdown.completed")
		}
	}()

	// Wait for all server operations to complete
	wg.Wait()
	logger.Get().Info("Server shutdown complete")
}

func registerMCP(mcpServer *mcp.Server, toolRegistry *mcpinternal.ToolRegistry, enabledToolProviders []string, kubeconfig string) {
	// A map to hold tool providers and their registration functions
	toolProviderMap := map[string]func(*mcp.Server) error{
		"argo":       func(s *mcp.Server) error { return argo.RegisterToolsWithRegistry(s, toolRegistry) },
		"cilium":     func(s *mcp.Server) error { return cilium.RegisterToolsWithRegistry(s, toolRegistry) },
		"helm":       func(s *mcp.Server) error { return helm.RegisterToolsWithRegistry(s, toolRegistry) },
		"istio":      func(s *mcp.Server) error { return istio.RegisterToolsWithRegistry(s, toolRegistry) },
		"k8s":        func(s *mcp.Server) error { return k8s.RegisterToolsWithRegistry(s, toolRegistry, nil, kubeconfig) },
		"prometheus": func(s *mcp.Server) error { return prometheus.RegisterToolsWithRegistry(s, toolRegistry) },
		"utils":      func(s *mcp.Server) error { return utils.RegisterToolsWithRegistry(s, toolRegistry) },
	}

	// If no specific tools are specified, register all available tools.
	if len(enabledToolProviders) == 0 {
		for name := range toolProviderMap {
			enabledToolProviders = append(enabledToolProviders, name)
		}
	}

	// Register tools with MCP server (and registry for providers that support it)
	for _, toolProviderName := range enabledToolProviders {
		if registerFunc, ok := toolProviderMap[toolProviderName]; ok {
			if err := registerFunc(mcpServer); err != nil {
				logger.Get().Error("Failed to register tool provider", "provider", toolProviderName, "error", err)
			}
		} else {
			logger.Get().Error("Unknown tool specified", "provider", toolProviderName)
		}
	}

	// All tool providers now support ToolRegistry for full HTTP transport support
}
