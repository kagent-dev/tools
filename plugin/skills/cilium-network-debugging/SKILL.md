---
name: cilium-network-debugging
description: Diagnose Cilium CNI networking (agent health, endpoint and identity state, network policy drops, services, BGP, cluster mesh) with the kagent-tools cilium_* MCP tools. Use when pods cannot reach each other or services, or a policy blocks traffic unexpectedly.
---
# Cilium network debugging

Most `cilium_*` tools run against the Cilium agent on one node and need its name as `node_name`; without it they fail. Only `cilium_status_and_version`, the BGP tools, and `cilium_show_cluster_mesh_status` are cluster-wide.

## 1. Health (read-only)
1. `cilium_status_and_version` for an overall status and version.
2. Find the node that hosts the affected pod with `k8s_get_resources` (`resource_type: pods`, `output: wide`); its NODE column is the `node_name` for the steps below.
3. `cilium_get_daemon_status` on that node.
4. `cilium_list_cluster_nodes` to confirm all nodes are known.

## 2. Endpoint and identity
Run these on the node of each pod; the source and destination pods may be on different nodes.
1. `cilium_get_endpoints_list` to find the endpoint ID and identity of the pod, then `cilium_get_endpoint_details` with that `endpoint_id`. Check that it is in the `ready` state.
2. `cilium_get_identity_details` with each `identity_id`. Check that the labels match what the policy selects.
3. `cilium_display_selectors` and `cilium_display_policy_node_information` to see which policies apply.
4. `cilium_validate_cilium_network_policies` to catch invalid policies.

## 3. Services and routing
- `cilium_list_services` / `cilium_get_service_information` (with `service_id`) for ClusterIP and backend mapping, on the client pod's node.
- `cilium_list_bgp_peers` / `cilium_list_bgp_routes` for BGP setups.
- `cilium_show_cluster_mesh_status` for multi-cluster setups.

## 4. Change (mutating; confirm first)
Prefer fixing the CiliumNetworkPolicy manifest through `k8s_apply_manifest` over imperative cilium state changes. Never run `cilium_uninstall_cilium` or `cilium_flush_ipsec_state` without explicit, specific confirmation.
