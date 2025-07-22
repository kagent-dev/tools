package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/internal/version"
	"github.com/kagent-dev/tools/pkg/argo"
	"github.com/kagent-dev/tools/pkg/cilium"
	"github.com/kagent-dev/tools/pkg/helm"
	"github.com/kagent-dev/tools/pkg/istio"
	"github.com/kagent-dev/tools/pkg/k8s"
	"github.com/kagent-dev/tools/pkg/prometheus"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/mark3labs/mcp-go/server"
)

var (
	port        int
	stdio       bool
	tools       []string
	kubeconfig  *string
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
	rootCmd.Flags().IntVarP(&port, "port", "p", 8084, "Port to run the server on")
	rootCmd.Flags().BoolVar(&stdio, "stdio", false, "Use stdio for communication instead of HTTP")
	rootCmd.Flags().StringSliceVar(&tools, "tools", []string{}, "List of tools to register. If empty, all tools are registered.")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information and exit")
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

	logger.Init()
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
	var httpServer *http.Server

	// Start server based on chosen mode
	wg.Add(1)
	if stdio {
		go func() {
			defer wg.Done()
			runStdioServer(ctx, mcp)
		}()
	} else {
		sseServer := server.NewStreamableHTTPServer(mcp,
			server.WithHeartbeatInterval(30*time.Second),
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

		// Add metrics endpoint (basic implementation for e2e tests)
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)

			// Generate real runtime metrics instead of hardcoded values
			metrics := generateRuntimeMetrics()
			if err := writeResponse(w, []byte(metrics)); err != nil {
				logger.Get().Error("Failed to write metrics response", "error", err)
			}
		})

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

// generateRuntimeMetrics generates real runtime metrics for the /metrics endpoint
func generateRuntimeMetrics() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	now := time.Now().Unix()

	// Build metrics in Prometheus format
	metrics := strings.Builder{}

	// Go runtime info
	metrics.WriteString("# HELP go_info Information about the Go environment.\n")
	metrics.WriteString("# TYPE go_info gauge\n")
	metrics.WriteString(fmt.Sprintf("go_info{version=\"%s\"} 1\n", runtime.Version()))

	// Process start time
	metrics.WriteString("# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.\n")
	metrics.WriteString("# TYPE process_start_time_seconds gauge\n")
	metrics.WriteString(fmt.Sprintf("process_start_time_seconds %d\n", now))

	// Memory metrics
	metrics.WriteString("# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.\n")
	metrics.WriteString("# TYPE go_memstats_alloc_bytes gauge\n")
	metrics.WriteString(fmt.Sprintf("go_memstats_alloc_bytes %d\n", m.Alloc))

	metrics.WriteString("# HELP go_memstats_total_alloc_bytes Total number of bytes allocated, even if freed.\n")
	metrics.WriteString("# TYPE go_memstats_total_alloc_bytes counter\n")
	metrics.WriteString(fmt.Sprintf("go_memstats_total_alloc_bytes %d\n", m.TotalAlloc))

	metrics.WriteString("# HELP go_memstats_sys_bytes Number of bytes obtained from system.\n")
	metrics.WriteString("# TYPE go_memstats_sys_bytes gauge\n")
	metrics.WriteString(fmt.Sprintf("go_memstats_sys_bytes %d\n", m.Sys))

	// Goroutine count
	metrics.WriteString("# HELP go_goroutines Number of goroutines that currently exist.\n")
	metrics.WriteString("# TYPE go_goroutines gauge\n")
	metrics.WriteString(fmt.Sprintf("go_goroutines %d\n", runtime.NumGoroutine()))

	return metrics.String()
}

func runStdioServer(ctx context.Context, mcp *server.MCPServer) {
	logger.Get().Info("Running KAgent Tools Server STDIO:", "tools", strings.Join(tools, ","))
	stdioServer := server.NewStdioServer(mcp)
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		logger.Get().Info("Stdio server stopped", "error", err)
	}
}

func registerMCP(mcp *server.MCPServer, enabledToolProviders []string, kubeconfig string) {
	// A map to hold tool providers and their registration functions
	toolProviderMap := map[string]func(*server.MCPServer){
		"argo":       argo.RegisterTools,
		"cilium":     cilium.RegisterTools,
		"helm":       helm.RegisterTools,
		"istio":      istio.RegisterTools,
		"k8s":        func(s *server.MCPServer) { k8s.RegisterTools(s, nil, kubeconfig) },
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
			registerFunc(mcp)
		} else {
			logger.Get().Error("Unknown tool specified", "provider", toolProviderName)
		}
	}
}
