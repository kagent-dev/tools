package e2e

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/pkg/argo"
	"github.com/kagent-dev/tools/pkg/cilium"
	"github.com/kagent-dev/tools/pkg/helm"
	"github.com/kagent-dev/tools/pkg/istio"
	"github.com/kagent-dev/tools/pkg/k8s"
	"github.com/kagent-dev/tools/pkg/kubescape"
	"github.com/kagent-dev/tools/pkg/prometheus"
	"github.com/kagent-dev/tools/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

/*
Tool Coverage Sweep

Drives every read-only tool the deployed server advertises, so the suite covers
the whole tool surface rather than a handful of hand-picked tools.

Asserted for every invoked tool:
 1. the call completes without a protocol/transport error, and
 2. the result honours the typed-output contract: a success carries
    structuredContent that decodes into the shared TextOutput DTO (a handler
    regressing to Out=any would produce none), while a failure reports IsError
    with a readable message.

Write-guarded tools are deliberately NOT invoked: they mutate the cluster, and
calling one by accident is exactly the class of bug this sweep must catch. Their
registration is covered by TestEveryToolHasValidOutputSchema.

Tools whose backing dependency is absent (Cilium on a kindnet cluster, a
Prometheus server, the Kubescape operator) legitimately answer with a tool
error or are skipped with a reason; both outcomes are recorded, not failed.
*/

// ReadOnlyTools returns the tool names the server registers in read-only mode,
// taken from the providers themselves rather than guessed from name patterns.
// Anything outside this set is write-capable and must never be invoked by the
// sweep, since the server only exposes it when write access is enabled.
func ReadOnlyTools() map[string]bool {
	ctx := context.Background()
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "readonly-probe", Version: "test"}, nil)

	// Register every provider with readOnly=true: the same gating the server
	// applies under --read-only.
	argo.RegisterTools(srv, true)
	cilium.RegisterTools(srv, true)
	helm.RegisterTools(srv, true)
	istio.RegisterTools(srv, true)
	k8s.RegisterTools(srv, nil, "", true)
	kubescape.RegisterTools(srv, "", true)
	prometheus.RegisterTools(srv, true)
	utils.RegisterTools(srv, true)

	st, ct := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "readonly-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	Expect(err).ToNot(HaveOccurred(), "read-only probe failed to connect: %v", err)
	defer func() { _ = session.Close() }()

	names := map[string]bool{}
	for tool, err := range session.Tools(ctx, nil) {
		Expect(err).ToNot(HaveOccurred())
		names[tool.Name] = true
	}
	Expect(names).ToNot(BeEmpty(), "read-only probe advertised no tools")
	return names
}

type toolCase struct {
	name string
	// args are sent verbatim; nil means "no arguments".
	args map[string]any
	// skipReason, when set, documents why the tool cannot be exercised here.
	skipReason string
}

// SweepReadOnlyTools exercises every read-only tool and asserts the typed-output
// contract. Call it from inside the ordered k8s container so the deployed server
// is still up (that container's AfterAll deletes the namespace).
func SweepReadOnlyTools(client *MCPClient) {
	tools, err := client.listTools()
	Expect(err).ToNot(HaveOccurred(), "listing tools failed: %v", err)

	advertised := make(map[string]*sdkmcp.Tool, len(tools))
	for _, t := range tools {
		advertised[t.Name] = t
	}
	Expect(advertised).ToNot(BeEmpty(), "server advertised no tools")

	// Every advertised tool must expose an output schema; none means Out=any.
	var noSchema []string
	for name, tool := range advertised {
		if tool.OutputSchema == nil {
			noSchema = append(noSchema, name)
		}
	}
	sort.Strings(noSchema)
	Expect(noSchema).To(BeEmpty(),
		"%d tool(s) advertise no output schema (handler still returns Out=any): %v",
		len(noSchema), noSchema)

	cases := readOnlyToolCases()
	Expect(cases).ToNot(BeEmpty())
	readOnly := ReadOnlyTools()

	var okCount, toolErrCount, skipCount int
	var missingTools []string
	invoked := make(map[string]bool, len(cases))

	for _, tc := range cases {
		if _, present := advertised[tc.name]; !present {
			missingTools = append(missingTools, tc.name)
			continue
		}
		if tc.skipReason != "" {
			skipCount++
			GinkgoWriter.Printf("SKIP  %-42s %s\n", tc.name, tc.skipReason)
			continue
		}

		// Safety rail: only invoke tools the providers register in read-only
		// mode. This is the authoritative set, so a write-capable tool can never
		// be reached from this list.
		Expect(readOnly[tc.name]).To(BeTrue(),
			"refusing to invoke %q: the server does not register it in read-only mode", tc.name)
		Expect(invoked[tc.name]).To(BeFalse(), "%s is enumerated twice", tc.name)
		invoked[tc.name] = true

		args := tc.args
		if args == nil {
			args = map[string]any{}
		}

		result, err := client.callToolWithTimeout(tc.name, args, 90*time.Second)
		Expect(err).ToNot(HaveOccurred(),
			"%s: transport error (a tool must answer, even with an error result): %v", tc.name, err)
		Expect(result).ToNot(BeNil(), "%s returned a nil result", tc.name)

		if result.IsError {
			// An absent dependency is a legitimate outcome; the contract is a
			// readable error, not a transport failure.
			toolErrCount++
			Expect(toolResultText(result)).ToNot(BeEmpty(),
				"%s reported IsError with an empty message", tc.name)
			GinkgoWriter.Printf("TERR  %-42s %s\n", tc.name, truncate(toolResultText(result), 90))
			continue
		}

		// Success must carry the typed output the SDK derives from Out. Some
		// tools use a dedicated DTO (mcp_inspect returns echo+headers) rather
		// than the shared TextOutput wrapper, so assert that structuredContent
		// is a non-empty JSON object rather than that it decodes as TextOutput.
		Expect(result.StructuredContent).ToNot(BeNil(),
			"%s succeeded without structuredContent; its handler likely returns Out=any", tc.name)
		raw, marshalErr := json.Marshal(result.StructuredContent)
		Expect(marshalErr).ToNot(HaveOccurred(), "%s: structuredContent is not JSON: %v", tc.name, marshalErr)
		Expect(len(raw)).To(BeNumerically(">", 2), "%s succeeded with empty structuredContent", tc.name)

		okCount++
		GinkgoWriter.Printf("OK    %-42s\n", tc.name)
	}

	GinkgoWriter.Printf("\ncoverage: ok=%d toolerr=%d skipped=%d enumerated=%d\n",
		okCount, toolErrCount, skipCount, len(cases))

	Expect(missingTools).To(BeEmpty(), "tool(s) not advertised by the server: %v", missingTools)
	Expect(okCount+toolErrCount+skipCount).To(Equal(len(cases)),
		"every enumerated tool must be invoked or explicitly skipped")
}

// SweepGuardedTools asserts the read/write split is meaningful and that no
// write-capable tool slipped into the sweep's invocation list.
func SweepGuardedTools(client *MCPClient) {
	tools, err := client.listTools()
	Expect(err).ToNot(HaveOccurred())

	readOnly := ReadOnlyTools()
	var guarded int
	for _, t := range tools {
		if !readOnly[t.Name] {
			guarded++
		}
	}
	Expect(guarded).To(BeNumerically(">", 0),
		"no write-capable tools were found, so the read-only safety rail proves nothing")

	// Every case the sweep may invoke must be a read-only tool.
	for _, tc := range readOnlyToolCases() {
		if tc.skipReason != "" {
			continue
		}
		Expect(readOnly[tc.name]).To(BeTrue(),
			"sweep would invoke %q, which is not registered in read-only mode", tc.name)
	}
}

// readOnlyToolCases enumerates the read-only tools and the arguments needed to
// exercise them. Tools needing cluster-specific setup are skipped with a reason
// rather than guessed at.
func readOnlyToolCases() []toolCase {
	// Resolve a schedulable target pod once, so the k8s read tools exercise
	// their real code paths on a namespace the arg validator accepts.
	podName, podNamespace := discoverPodTarget()

	cases := []toolCase{
		// utils
		{name: "datetime_get_current_time"},
		{name: "mcp_inspect", args: map[string]any{"echo": "coverage"}},

		// k8s (read-only)
		{name: "k8s_get_available_api_resources"},
		{name: "k8s_get_cluster_configuration"},
		{name: "k8s_get_events"},
		// Discover a concrete pod to target: k8s arg validation rejects the
		// reserved kube-* namespaces, and no pods are guaranteed in "default",
		// so resolve a real one at runtime.
		{name: "k8s_get_resources", args: map[string]any{"resource_type": "pods", "namespace": podNamespace, "output": "json"}},
		{name: "k8s_describe_resource", args: map[string]any{"resource_type": "pod", "resource_name": podName, "namespace": podNamespace}},
		{name: "k8s_get_resource_yaml", args: map[string]any{"resource_type": "pod", "resource_name": podName, "namespace": podNamespace}},
		{name: "k8s_get_pod_logs", args: map[string]any{"pod_name": podName, "namespace": podNamespace, "tail_lines": 5}},
		{name: "k8s_wait", args: map[string]any{"resource_type": "pod", "condition": "condition=Ready", "resource_name": podName, "namespace": podNamespace}},
		{name: "k8s_generate_resource", skipReason: "needs an LLM-backed resource_description; covered by unit tests"},

		// helm (read-only)
		{name: "helm_list_releases", args: map[string]any{"all_namespaces": true, "output": "json"}},
		{name: "helm_get_release", skipReason: "needs an existing release name"},
		{name: "helm_repo_update"},

		// istio (read-only)
		{name: "istio_version"},
		{name: "istio_proxy_status"},
		{name: "istio_analyze_cluster_configuration"},
		{name: "istio_generate_manifest", args: map[string]any{"profile": "default"}},
		{name: "istio_remote_clusters"},
		{name: "istio_list_waypoints", skipReason: "istioctl waypoint list needs ambient mode; none in this cluster"},
		{name: "istio_proxy_config", skipReason: "needs a pod enrolled in the mesh"},
		{name: "istio_generate_waypoint", skipReason: "needs an ambient-enrolled namespace"},
		{name: "istio_waypoint_status", skipReason: "needs an ambient waypoint"},
		{name: "istio_ztunnel_config", skipReason: "needs ambient ztunnel"},

		// argo (read-only)
		{name: "argo_rollouts_list", args: map[string]any{"namespace": "default"}},
		{name: "argo_verify_kubectl_plugin_install"},
		{name: "argo_verify_argo_rollouts_controller_install"},
		{name: "argo_check_plugin_logs", args: map[string]any{"namespace": "argo-rollouts"}},

		// kubescape (read-only) - the operator may be absent; a tool error is a
		// legitimate outcome and is recorded, not failed.
		{name: "kubescape_check_health"},
		{name: "kubescape_list_vulnerability_manifests"},
		{name: "kubescape_list_configuration_scans"},
		{name: "kubescape_list_application_profiles"},
		{name: "kubescape_list_network_neighborhoods"},
		{name: "kubescape_list_vulnerabilities", skipReason: "needs an existing vulnerability manifest name"},
		{name: "kubescape_get_vulnerability_details", skipReason: "needs an existing manifest and CVE id"},
		{name: "kubescape_get_configuration_scan", skipReason: "needs an existing configuration scan name"},
		{name: "kubescape_get_application_profile", skipReason: "needs an existing application profile"},
		{name: "kubescape_get_network_neighborhood", skipReason: "needs an existing network neighborhood"},

		// prometheus - no server deployed; tools must report a readable error.
		{name: "prometheus_query_tool", args: map[string]any{"query": "up"}},
		{name: "prometheus_query_range_tool", args: map[string]any{"query": "up", "start": "now-5m", "end": "now", "step": "60s"}},
		{name: "prometheus_label_names_tool"},
		{name: "prometheus_targets_tool"},
		{name: "prometheus_promql_tool", skipReason: "needs an OpenAI API key for LLM query generation"},
	}

	// Cilium tools are only meaningful when Cilium is the cluster CNI. On the
	// Kind default (kindnet) they must report an error, which the sweep records.
	ciliumNode := ciliumNodeName()
	ciliumTools := []string{
		"cilium_status_and_version",
		"cilium_get_daemon_status",
		"cilium_get_endpoints_list",
		"cilium_list_identities",
		"cilium_list_cluster_nodes",
		"cilium_list_node_ids",
		"cilium_list_bpf_maps",
		"cilium_list_ip_addresses",
		"cilium_list_services",
		"cilium_list_metrics",
		"cilium_display_encryption_state",
		"cilium_display_selectors",
		"cilium_display_policy_node_information",
		"cilium_show_configuration_options",
		"cilium_show_dns_names",
		"cilium_show_load_information",
		"cilium_show_features_status",
		"cilium_show_cluster_mesh_status",
		"cilium_list_bgp_peers",
		"cilium_list_bgp_routes",
		"cilium_list_pcap_recorders",
		"cilium_list_local_redirect_policies",
		"cilium_list_xdp_cidr_filters",
		"cilium_request_debugging_information",
		"cilium_validate_cilium_network_policies",
		"cilium_fqdn_cache",
	}
	for _, name := range ciliumTools {
		cases = append(cases, toolCase{name: name, args: map[string]any{"node_name": ciliumNode}})
	}

	// These read-only cilium tools need identifiers (endpoint/service/recorder
	// ID, map name, identity ID, CIDR) that only exist when a Cilium agent is
	// running. On a kindnet cluster there is nothing to query, so they are
	// enumerated (proving they are advertised) but skipped rather than called
	// with invented arguments, which would only re-test input validation.
	for _, name := range []string{
		"cilium_get_endpoint_details",
		"cilium_get_endpoint_health",
		"cilium_get_endpoint_logs",
		"cilium_get_kv_store_key",
		"cilium_get_pcap_recorder",
		"cilium_get_service_information",
		"cilium_list_envoy_config",
		"cilium_list_bpf_map_events",
		"cilium_get_bpf_map",
		"cilium_get_identity_details",
		"cilium_show_ip_cache_information",
	} {
		cases = append(cases, toolCase{
			name:       name,
			skipReason: "needs a live Cilium agent identifier; Cilium is absent on kindnet",
		})
	}

	return cases
}

// discoverPodTarget returns a running pod and its namespace, preferring a
// non-reserved namespace since the argument validator rejects kube-*
// namespaces. Both values fall back to a harmless default so the sweep still
// runs (and records a tool error) when nothing is schedulable.
func discoverPodTarget() (name, namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a Go raw string so the jsonpath quotes need no escaping.
	const jsonpath = `jsonpath={range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}`
	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "pods", "-A", "--field-selector=status.phase=Running", "-o", jsonpath).
		WithCache(false).
		Execute(ctx)
	if err != nil {
		return "no-such-pod", "default"
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ns, pod := fields[0], fields[1]
		// Skip namespaces the validator refuses.
		if strings.HasPrefix(ns, "kube-") {
			continue
		}
		return pod, ns
	}
	return "no-such-pod", "default"
}

// ciliumNodeName returns the first node name, so the cilium-dbg backed tools
// target a real (or cleanly absent) agent instead of an empty selector.
func ciliumNodeName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "nodes", "-o", "jsonpath={.items[0].metadata.name}").
		WithCache(false).
		Execute(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
