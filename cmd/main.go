package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/internal/version"
	"github.com/kagent-dev/tools/pkg/utils"

	"github.com/kagent-dev/tools/pkg/argo"
	"github.com/kagent-dev/tools/pkg/cilium"
	"github.com/kagent-dev/tools/pkg/helm"
	"github.com/kagent-dev/tools/pkg/istio"
	"github.com/kagent-dev/tools/pkg/k8s"
	"github.com/kagent-dev/tools/pkg/prometheus"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	port       int
	stdio      bool
	tools      []string
	kubeconfig *string

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
	rootCmd.Flags().IntVarP(&port, "port", "p", 8084, "Port to run the server on")
	rootCmd.Flags().BoolVar(&stdio, "stdio", false, "Use stdio for communication instead of HTTP")
	rootCmd.Flags().StringSliceVar(&tools, "tools", []string{}, "List of tools to register. If empty, all tools are registered.")
	kubeconfig = rootCmd.Flags().String("kubeconfig", "", "kubeconfig file path (optional, defaults to in-cluster config)")

	// if found .env file, load it
	if _, err := os.Stat(".env"); err == nil {
		_ = godotenv.Load(".env")
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	logger.Init()
	defer logger.Sync()

	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OpenTelemetry tracing
	otelConfig := telemetry.LoadConfig()
	otelConfig.ServiceVersion = Version

	otelShutdown, err := telemetry.SetupOTelSDK(ctx, otelConfig)
	if err != nil {
		logger.Get().Error(err, "Failed to setup OpenTelemetry SDK")
		os.Exit(1)
	}
	defer func() {
		if err := otelShutdown(ctx); err != nil {
			logger.Get().Error(err, "Failed to shutdown OpenTelemetry SDK")
		}
	}()

	// Start root span for server lifecycle
	tracer := otel.Tracer("kagent-tools/server")
	ctx, rootSpan := tracer.Start(ctx, "server.lifecycle")
	defer rootSpan.End()

	rootSpan.SetAttributes(
		attribute.String("server.name", Name),
		attribute.String("server.version", Version),
		attribute.String("server.git_commit", GitCommit),
		attribute.String("server.build_date", BuildDate),
		attribute.Bool("server.stdio_mode", stdio),
		attribute.Int("server.port", port),
		attribute.StringSlice("server.tools", tools),
	)

	logger.Get().Info("Starting "+Name, "version", Version, "git_commit", GitCommit, "build_date", BuildDate)

	mcp := server.NewMCPServer(
		Name,
		Version,
	)

	// Register tools
	registerMCP(mcp, tools, *kubeconfig)

	// Create wait group for server goroutines
	var wg sync.WaitGroup

	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// HTTP server reference (only used when not in stdio mode)
	var sseServer *server.StreamableHTTPServer

	// Start server based on chosen mode
	wg.Add(1)
	if stdio {
		go func() {
			defer wg.Done()
			runStdioServer(ctx, mcp)
		}()
	} else {
		sseServer = server.NewStreamableHTTPServer(mcp)
		go func() {
			defer wg.Done()
			addr := fmt.Sprintf(":%d", port)
			logger.Get().Info("Running KAgent Tools Server", "port", addr, "tools", strings.Join(tools, ","))
			if err := sseServer.Start(addr); err != nil {
				if !errors.Is(err, http.ErrServerClosed) {
					logger.Get().Error(err, "Failed to start SSE server")
				} else {
					logger.Get().Info("SSE server closed gracefully.")
				}
			}
		}()
	}

	// Wait for termination signal
	go func() {
		<-signalChan
		logger.Get().Info("Received termination signal, shutting down server...")

		// Mark root span as shutting down
		rootSpan.AddEvent("server.shutdown.initiated")

		// Cancel context to notify any context-aware operations
		cancel()

		// Gracefully shutdown HTTP server if running
		if !stdio && sseServer != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := sseServer.Shutdown(shutdownCtx); err != nil {
				logger.Get().Error(err, "Failed to shutdown server gracefully")
				rootSpan.RecordError(err)
				rootSpan.SetStatus(codes.Error, "Server shutdown failed")
			} else {
				rootSpan.AddEvent("server.shutdown.completed")
			}
		}
	}()

	// Wait for all server operations to complete
	wg.Wait()
	logger.Get().Info("Server shutdown complete")
}

func runStdioServer(ctx context.Context, mcp *server.MCPServer) {
	logger.Get().Info("Running KAgent Tools Server STDIO:", "tools", strings.Join(tools, ","))
	stdioServer := server.NewStdioServer(mcp)
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		logger.Get().Info("Stdio server stopped", "error", err)
	}
}

func registerMCP(mcp *server.MCPServer, enabledToolProviders []string, kubeconfig string) {

	var toolProviderMap = map[string]func(*server.MCPServer){
		"utils":      utils.RegisterTools,
		"k8s":        k8s.RegisterTools,
		"prometheus": prometheus.RegisterTools,
		"helm":       helm.RegisterTools,
		"istio":      istio.RegisterTools,
		"argo":       argo.RegisterTools,
		"cilium":     cilium.RegisterTools,
	}

	// Set the shared kubeconfig
	if len(kubeconfig) > 0 {
		utils.SetKubeconfig(kubeconfig)
	}

	// If no tools specified, register all tools
	if len(enabledToolProviders) == 0 {
		logger.Get().Info("No specific tools provided, registering all tools")
		for toolProvider, registerFunc := range toolProviderMap {
			logger.Get().Info("Registering tools", "provider", toolProvider)
			registerFunc(mcp)
		}
		return
	}

	// Register only the specified tools
	logger.Get().Info("provider list", "tools", enabledToolProviders)
	for _, toolProviderName := range enabledToolProviders {
		if registerFunc, ok := toolProviderMap[strings.ToLower(toolProviderName)]; ok {
			logger.Get().Info("Registering tool", "provider", toolProviderName)
			registerFunc(mcp)
		} else {
			logger.Get().Error(nil, "Unknown tool specified", "provider", toolProviderName)
		}
	}
}
