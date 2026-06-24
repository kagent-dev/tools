package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kagent-dev/tools/internal/logger"
	mcpserver "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/internal/metrics"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/internal/version"
	"github.com/kagent-dev/tools/pkg/argo"
	"github.com/kagent-dev/tools/pkg/cilium"
	"github.com/kagent-dev/tools/pkg/helm"
	"github.com/kagent-dev/tools/pkg/istio"
	"github.com/kagent-dev/tools/pkg/k8s"
	"github.com/kagent-dev/tools/pkg/kubescape"
	"github.com/kagent-dev/tools/pkg/prometheus"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	port        int
	metricsPort int
	stdio       bool
	tools       []string
	kubeconfig  *string
	showVersion bool
	readOnly    bool

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
	rootCmd.Flags().IntVarP(&metricsPort, "metrics-port", "m", 0, "Port to run the metrics server on (default 0: same as --port)")
	rootCmd.Flags().BoolVar(&stdio, "stdio", false, "Use stdio for communication instead of HTTP")
	rootCmd.Flags().StringSliceVar(&tools, "tools", []string{}, "List of tools to register. If empty, all tools are registered.")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information and exit")
	rootCmd.Flags().BoolVar(&readOnly, "read-only", false, "Run in read-only mode (disable tools that perform write operations)")
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

	// 0 means "same as --port" - resolve it before any server logic uses it
	if metricsPort == 0 {
		metricsPort = port
	}

	logger.Init(stdio)
	defer logger.Sync()

	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OpenTelemetry tracing
	cfg := telemetry.LoadOtelCfg()

	err := telemetry.SetupOTelSDK(ctx)
	if err != nil {
		logger.Get().Error("Failed to setup OpenTelemetry SDK", "error", err)
		os.Exit(1)
	}

	// Start root span for server lifecycle
	tracer := otel.Tracer("kagent-tools/server")
	ctx, rootSpan := tracer.Start(ctx, "server.lifecycle")
	defer rootSpan.End()

	rootSpan.SetAttributes(
		attribute.String("server.name", Name),
		attribute.String("server.version", cfg.Telemetry.ServiceVersion),
		attribute.String("server.git_commit", GitCommit),
		attribute.String("server.build_date", BuildDate),
		attribute.Bool("server.stdio_mode", stdio),
		attribute.Int("server.port", port),
		attribute.StringSlice("server.tools", tools),
		attribute.Bool("server.read_only", readOnly),
	)

	logger.Get().Info("Starting "+Name, "version", Version, "git_commit", GitCommit, "build_date", BuildDate)
	if readOnly {
		logger.Get().Info("Running in read-only mode - write operations are disabled")
	}

	mcpSrv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: Name, Version: Version}, nil)

	// Attach a single receiving middleware that instruments every tools/call
	// with an OTel span and Prometheus invocation counters. Per-tool provider
	// labels are recorded as each provider registers its tools.
	mcpSrv.AddReceivingMiddleware(mcpserver.ToolMiddleware())

	registerMCP(mcpSrv, tools, *kubeconfig, readOnly)

	// Create wait group for server goroutines
	var wg sync.WaitGroup

	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// HTTP server reference (only used when not in stdio mode)
	var httpServer *http.Server
	var metricsServer *http.Server // Separate server for metrics if metricsPort is different from main port

	// Start server based on chosen mode
	wg.Add(1)
	if stdio {
		go func() {
			defer wg.Done()
			runStdioServer(ctx, mcpSrv)
		}()
	} else {
		sseServer := sdkmcp.NewStreamableHTTPHandler(
			func(*http.Request) *sdkmcp.Server { return mcpSrv },
			nil,
		)

		// Create a mux to handle different routes
		mux := http.NewServeMux()

		// Add health endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			if err := writeResponse(w, []byte("OK")); err != nil {
				logger.Get().Error("Failed to write health response", "error", err)
			}
		})

		// Add metrics endpoint
		registry := metrics.InitServer() // Initialize Prometheus metrics before starting the server

		if metricsPort != port { // Only start a separate metrics server if the metrics port is different from the main server port
			// Create the metrics server outside the goroutine to avoid a race condition
			// between the goroutine assigning metricsServer and the shutdown handler reading it
			metricsMux := http.NewServeMux()
			metricsMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
			metricsServer = &http.Server{
				Addr:    fmt.Sprintf(":%d", metricsPort),
				Handler: metricsMux,
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				logger.Get().Info("Starting Prometheus metrics endpoint on /metrics", "port", strconv.Itoa(metricsPort))
				if err := metricsServer.ListenAndServe(); err != nil {
					if !errors.Is(err, http.ErrServerClosed) {
						logger.Get().Error("Metrics endpoint failed", "error", err)
					} else {
						logger.Get().Info("Metrics server closed gracefully.")
					}
				}
			}()
		} else {
			logger.Get().Info("Starting Prometheus metrics endpoint on /metrics", "port", strconv.Itoa(port))
			mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		}
		serverMode := "read-write"
		if readOnly {
			serverMode = "read-only"
		}
		metrics.KagentToolsMCPServerInfo.WithLabelValues(Name, Version, GitCommit, BuildDate, serverMode).Set(1)

		// Handle all other routes with the MCP server wrapped in telemetry middleware
		mux.Handle("/", telemetry.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sseServer.ServeHTTP(w, r)
		})))

		httpServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		}

		go func() {
			defer wg.Done()
			logger.Get().Info("Running KAgent Tools Server", "port", fmt.Sprintf(":%d", port), "tools", strings.Join(tools, ","))
			if err := httpServer.ListenAndServe(); err != nil {
				if !errors.Is(err, http.ErrServerClosed) {
					logger.Get().Error("Failed to start HTTP server", "error", err)
				} else {
					logger.Get().Info("HTTP server closed gracefully.")
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
		if !stdio && httpServer != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				logger.Get().Error("Failed to shutdown server gracefully", "error", err)
				rootSpan.RecordError(err)
				rootSpan.SetStatus(codes.Error, "Server shutdown failed")
			} else {
				rootSpan.AddEvent("server.shutdown.completed")
			}
		}

		// Gracefully shutdown metrics server if running separately
		if !stdio && metricsServer != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				logger.Get().Error("Failed to shutdown metrics server gracefully", "error", err)
				rootSpan.RecordError(err)
			} else {
				logger.Get().Info("Metrics server shutdown completed")
			}
		}
	}()

	// Wait for all server operations to complete
	wg.Wait()
	logger.Get().Info("Server shutdown complete")
}

// writeResponse writes data to an HTTP response writer with proper error handling
func writeResponse(w http.ResponseWriter, data []byte) error {
	_, err := w.Write(data)
	return err
}

func runStdioServer(ctx context.Context, mcpSrv *sdkmcp.Server) {
	logger.Get().Info("Running KAgent Tools Server STDIO:", "tools", strings.Join(tools, ","))
	if err := mcpSrv.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
		logger.Get().Info("Stdio server stopped", "error", err)
	}
}

// registerMCP registers the enabled tool providers with the MCP server. Each
// provider's RegisterTools call records tool->provider mappings and the tool
// inventory metric centrally (see internal/mcp.AddTool); invocation metrics and
// tracing are applied by the receiving middleware installed in run().
func registerMCP(mcpSrv *sdkmcp.Server, enabledToolProviders []string, kubeconfig string, readOnly bool) {
	toolProviderMap := map[string]func(*sdkmcp.Server){
		"argo":       func(s *sdkmcp.Server) { argo.RegisterTools(s, readOnly) },
		"cilium":     func(s *sdkmcp.Server) { cilium.RegisterTools(s, readOnly) },
		"helm":       func(s *sdkmcp.Server) { helm.RegisterTools(s, readOnly) },
		"istio":      func(s *sdkmcp.Server) { istio.RegisterTools(s, readOnly) },
		"k8s":        func(s *sdkmcp.Server) { k8s.RegisterTools(s, nil, kubeconfig, readOnly) },
		"kubescape":  func(s *sdkmcp.Server) { kubescape.RegisterTools(s, kubeconfig, readOnly) },
		"prometheus": func(s *sdkmcp.Server) { prometheus.RegisterTools(s, readOnly) },
		"utils":      func(s *sdkmcp.Server) { utils.RegisterTools(s, readOnly) },
	}

	// If no specific tools are specified, register all available tools.
	if len(enabledToolProviders) == 0 {
		for name := range toolProviderMap {
			enabledToolProviders = append(enabledToolProviders, name)
		}
	}

	for _, toolProviderName := range enabledToolProviders {
		if registerFunc, ok := toolProviderMap[toolProviderName]; ok {
			registerFunc(mcpSrv)
		} else {
			logger.Get().Error("Unknown tool specified", "provider", toolProviderName)
		}
	}
}
