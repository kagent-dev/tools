package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestInitServer_ReturnsRegistry(t *testing.T) {
	registry := InitServer()
	if registry == nil {
		t.Fatal("InitServer() returned nil registry")
	}
}

func TestInitServer_GathersMetrics(t *testing.T) {
	registry := InitServer()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(families) == 0 {
		t.Fatal("Expected at least one metric family from Go/process collectors, got none")
	}
}

func TestInitServer_RegistersCustomMetrics(t *testing.T) {
	registry := InitServer()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Build a set of metric names for easy lookup
	metricNames := make(map[string]bool)
	for _, family := range families {
		metricNames[family.GetName()] = true
	}

	// Go and process collectors should be present
	goMetrics := []string{
		"go_goroutines",
		"go_memstats_alloc_bytes",
	}
	for _, name := range goMetrics {
		if !metricNames[name] {
			t.Errorf("Expected Go collector metric %q to be registered", name)
		}
	}
}

func TestKagentToolsMCPServerInfo_SetAndGather(t *testing.T) {
	registry := InitServer()

	// Set the server info metric
	KagentToolsMCPServerInfo.WithLabelValues(
		"test-server",
		"v0.0.1",
		"abc123",
		"2026-02-12",
		"read-write",
	).Set(1)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := findMetricFamily(families, "kagent_tools_mcp_server_info")
	if found == nil {
		t.Fatal("Expected kagent_tools_mcp_server_info metric to be present")
	}

	metrics := found.GetMetric()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 time series, got %d", len(metrics))
	}

	// Verify label values
	expectedLabels := map[string]string{
		"server_name": "test-server",
		"version":     "v0.0.1",
		"git_commit":  "abc123",
		"build_date":  "2026-02-12",
		"server_mode": "read-write",
	}

	for _, label := range metrics[0].GetLabel() {
		expected, ok := expectedLabels[label.GetName()]
		if !ok {
			t.Errorf("Unexpected label %q", label.GetName())
			continue
		}
		if label.GetValue() != expected {
			t.Errorf("Label %q: expected %q, got %q", label.GetName(), expected, label.GetValue())
		}
	}

	// Verify gauge value is 1
	if metrics[0].GetGauge().GetValue() != 1 {
		t.Errorf("Expected gauge value 1, got %f", metrics[0].GetGauge().GetValue())
	}
}

func TestKagentToolsMCPRegisteredTools_SetAndGather(t *testing.T) {
	registry := InitServer()

	// Register a couple of tool providers
	KagentToolsMCPRegisteredTools.WithLabelValues("kubectl_get", "k8s").Set(1)
	KagentToolsMCPRegisteredTools.WithLabelValues("helm_list", "helm").Set(1)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := findMetricFamily(families, "kagent_tools_mcp_registered_tools")
	if found == nil {
		t.Fatal("Expected kagent_tools_mcp_registered_tools metric to be present")
	}

	metrics := found.GetMetric()
	if len(metrics) != 2 {
		t.Fatalf("Expected 2 time series (one per tool), got %d", len(metrics))
	}
}

// findMetricFamily finds a metric family by name from a gathered slice
func findMetricFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	return nil
}

// resetMetrics resets the global metric vectors so tests don't interfere with each other
func resetMetrics() {
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
			"server_mode",
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
}

func TestMain(m *testing.M) {
	// Reset metrics before each test run to avoid "duplicate registration" panics
	// since InitServer() registers the package-level vars into a new registry each time
	resetMetrics()
	m.Run()
}
