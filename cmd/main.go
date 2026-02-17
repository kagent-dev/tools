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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

	mcp := server.NewMCPServer(
		Name,
		Version,
	)

	// Register tools and wrap handlers with metrics instrumentation.
	// registerMCP returns a map of tool_name -> tool_provider so that
	// wrapToolHandlersWithMetrics knows which provider each tool belongs to.
	toolProviders := registerMCP(mcp, tools, *kubeconfig, readOnly)
	wrapToolHandlersWithMetrics(mcp, toolProviders)

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

func runStdioServer(ctx context.Context, mcp *server.MCPServer) {
	logger.Get().Info("Running KAgent Tools Server STDIO:", "tools", strings.Join(tools, ","))
	stdioServer := server.NewStdioServer(mcp)
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		logger.Get().Info("Stdio server stopped", "error", err)
	}
}

// registerMCP registers tool providers with the MCP server and returns a mapping
// of tool_name -> tool_provider. This mapping is built using the ListTools() diff
// technique: we snapshot the tool list before and after each provider registers,
// so we know exactly which tools belong to which provider.
func registerMCP(mcp *server.MCPServer, enabledToolProviders []string, kubeconfig string, readOnly bool) map[string]string {
	// A map to hold tool providers and their registration functions
	toolProviderMap := map[string]func(*server.MCPServer){
		"argo":       func(s *server.MCPServer) { argo.RegisterTools(s, readOnly) },
		"cilium":     func(s *server.MCPServer) { cilium.RegisterTools(s, readOnly) },
		"helm":       func(s *server.MCPServer) { helm.RegisterTools(s, readOnly) },
		"istio":      func(s *server.MCPServer) { istio.RegisterTools(s, readOnly) },
		"k8s":        func(s *server.MCPServer) { k8s.RegisterTools(s, nil, kubeconfig, readOnly) },
		"kubescape":  func(s *server.MCPServer) { kubescape.RegisterTools(s, kubeconfig, readOnly) },
		"prometheus": func(s *server.MCPServer) { prometheus.RegisterTools(s, readOnly) },
		"utils":      func(s *server.MCPServer) { utils.RegisterTools(s, readOnly) },
	}

	// If no specific tools are specified, register all available tools.
	if len(enabledToolProviders) == 0 {
		for name := range toolProviderMap {
			enabledToolProviders = append(enabledToolProviders, name)
		}
	}

	// toolToProvider maps each tool name to its provider (e.g., "kubectl_get" -> "k8s").
	// This is used later by wrapToolHandlersWithMetrics to set the correct tool_provider label.
	toolToProvider := make(map[string]string)

	for _, toolProviderName := range enabledToolProviders {
		if registerFunc, ok := toolProviderMap[toolProviderName]; ok {
			// Snapshot the tool list before this provider registers its tools.
			// We need this because ListTools() returns ALL tools from ALL providers,
			// so the only way to know which tools belong to THIS provider is to compare
			// the list before and after registration.
			toolsBefore := mcp.ListTools()

			registerFunc(mcp)

			// Determine which tools were just registered by this provider
			// by finding tools that exist now but didn't exist before.
			// Record each one in Prometheus so we can observe the full tool inventory.
			for toolName := range mcp.ListTools() {
				if _, existed := toolsBefore[toolName]; !existed {
					metrics.KagentToolsMCPRegisteredTools.WithLabelValues(toolName, toolProviderName).Set(1)
					toolToProvider[toolName] = toolProviderName
				}
			}
		} else {
			logger.Get().Error("Unknown tool specified", "provider", toolProviderName)
		}
	}

	return toolToProvider
}

// wrapToolHandlersWithMetrics applies the wrapper/middleware pattern to instrument
// all registered MCP tool handlers with Prometheus invocation counters.
//
// How it works:
//  1. Grab all registered tools from the MCP server using ListTools()
//  2. For each tool, wrap its handler with a function that increments metrics
//  3. Replace all tools in the MCP server using SetTools()
//
// The wrapper function:
//   - Increments kagent_tools_mcp_invocations_total on every call
//   - Increments kagent_tools_mcp_invocations_failure_total only when the handler returns an error
//   - Calls the original handler unchanged - the tool's behaviour is not affected
//
// This uses the standard middleware/decorator pattern: the original handler and the
// wrapped handler have the same function signature, so they are interchangeable.
// No changes are required in any pkg/ file - all instrumentation happens centrally here.
func wrapToolHandlersWithMetrics(mcpServer *server.MCPServer, toolToProvider map[string]string) {
	allTools := mcpServer.ListTools()
	wrapped := make([]server.ServerTool, 0, len(allTools))

	for name, st := range allTools {
		originalHandler := st.Handler
		toolName := name // capture for closure
		provider := toolToProvider[toolName]

		wrapped = append(wrapped, server.ServerTool{
			Tool: st.Tool,
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				metrics.KagentToolsMCPInvocationsTotal.WithLabelValues(toolName, provider).Inc()

				result, err := originalHandler(ctx, req)

				if err != nil {
					metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues(toolName, provider).Inc()
				}

				return result, err
			},
		})
	}

	mcpServer.SetTools(wrapped...)
}
