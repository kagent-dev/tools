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
	stdio       bool
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
	rootCmd.Flags().BoolVar(&stdio, "stdio", false, "Use stdio for communication instead of HTTP")
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
		logger.Get().Error("Failed to start tools mcp server", "error", err)
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

func run(cmd *cobra.Command, args []string) {
	// Handle version flag early, before any initialization
	if showVersion {
		printVersion()
		return
	}

	// Extract HTTP configuration from flags
	httpCfg, err := cmd.Flags().GetInt("http-port")
	if err != nil {
		logger.Get().Error("Failed to get http-port flag", "error", err)
		os.Exit(1)
	}
	httpPort = httpCfg

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

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: Version,
	}, nil)

	// Register tools
	registerMCP(mcpServer, tools, *kubeconfig)

	// Create wait group for server goroutines
	var wg sync.WaitGroup

	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Select transport based on mode
	var transport mcpinternal.Transport

	if stdio {
		transport = mcpinternal.NewStdioTransport(mcpServer)
		logger.Get().Info("Using stdio transport")
	} else {
		transport = mcpinternal.NewHTTPTransport(mcpServer, httpPort)
		logger.Get().Info("Using HTTP transport", "port", httpPort)
	}

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

func registerMCP(mcpServer *mcp.Server, enabledToolProviders []string, kubeconfig string) {
	// A map to hold tool providers and their registration functions
	toolProviderMap := map[string]func(*mcp.Server) error{
		"argo":       argo.RegisterTools,
		"cilium":     cilium.RegisterTools,
		"helm":       helm.RegisterTools,
		"istio":      istio.RegisterTools,
		"k8s":        func(s *mcp.Server) error { return k8s.RegisterTools(s, nil, kubeconfig) },
		"prometheus": prometheus.RegisterTools,
		"utils":      utils.RegisterTools,
	}

	// If no specific tools are specified, register all available tools.
	if len(enabledToolProviders) == 0 {
		for name := range toolProviderMap {
			enabledToolProviders = append(enabledToolProviders, name)
		}
	}
	for _, toolProviderName := range enabledToolProviders {
		if registerFunc, ok := toolProviderMap[toolProviderName]; ok {
			if err := registerFunc(mcpServer); err != nil {
				logger.Get().Error("Failed to register tool provider", "provider", toolProviderName, "error", err)
			}
		} else {
			logger.Get().Error("Unknown tool specified", "provider", toolProviderName)
		}
	}
}
