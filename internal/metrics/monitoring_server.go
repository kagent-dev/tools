package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// kAgent Tools MCP Server metrics definition
var (
	KagentToolsMCPServerInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kagent_tools_mcp_server_info",
			Help: "Information about the MCP server including version and build details",
		},
		[]string{
			"server_name",
			"version",
			"git_commit",
			"build_date",
			"server_mode", // e.g., "read-only" or "read-write"
		},
	)

	KagentToolsMCPRegisteredTools = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kagent_tools_mcp_registered_tools",
			Help: "Set to 1 for each registered MCP tool provider",
		},
		[]string{
			"tool_name",
			"tool_provider",
		},
	)
)

func InitServer() *prometheus.Registry {
	// New registry for our custom metrics, separate from the default registry
	registry := prometheus.NewRegistry()

	// Add Go runtime metrics ( goroutines, GC stats, etc. )
	registry.MustRegister(collectors.NewGoCollector())

	// Add process metrics (CPU, memory, file descriptors, etc. )
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Register kAgent Tools MCP Server metrics
	registry.MustRegister(KagentToolsMCPServerInfo)
	registry.MustRegister(KagentToolsMCPRegisteredTools)

	return registry
}
