---
name: istio-traffic-debugging
description: Debug Istio service-mesh traffic (503s, mTLS failures, routing not applied, sidecar or ambient/waypoint issues) using the kagent-tools istio_* MCP tools. Use when requests between mesh services fail or routing rules do not take effect.
---
# Istio traffic debugging

## 1. Mesh health (read-only)
1. `istio_version` for control-plane and data-plane versions.
2. `istio_proxy_status` to check that every proxy is `SYNCED`. A `STALE` or `NOT SENT` proxy points to a control-plane push problem.
3. `istio_analyze_cluster_configuration` for the namespace. Fix reported errors first.

## 2. Follow one request
1. Identify the source pod and destination service.
2. `istio_proxy_config` on the source pod for `routes`, `clusters`, and `endpoints`. Confirm the destination host and subset exist and endpoints are `HEALTHY`.
3. For ambient mode, use `istio_ztunnel_config`, `istio_list_waypoints`, and `istio_waypoint_status` to confirm the waypoint is attached and programmed.
4. Read the VirtualService, DestinationRule, and AuthorizationPolicy objects with `k8s_get_resource_yaml`.

## 3. Common causes
| Symptom | Likely cause |
|---|---|
| 503 `UF`/`URX` | No healthy endpoints, or a DestinationRule subset with no matching pod labels |
| 503 `NR` | No route: a VirtualService host or gateway mismatch |
| `RBAC: access denied` | AuthorizationPolicy |
| TLS errors | PeerAuthentication or DestinationRule `tls.mode` mismatch |

## 4. Change (mutating; confirm first)
Use `istio_apply_waypoint`/`istio_delete_waypoint`, or `k8s_apply_manifest` for Istio CRs. Show the manifest first. Re-run step 2 to verify.
