---
name: k8s-troubleshooting
description: Diagnose unhealthy Kubernetes workloads (CrashLoopBackOff, Pending, ImagePullBackOff, OOMKilled, failing readiness, unreachable Services) using the kagent-tools k8s_* MCP tools. Use when a pod, deployment or service is not working as expected.
---
# Kubernetes troubleshooting

Work from symptoms to cause. Gather facts before proposing changes.

## 1. Locate the failing object (read-only)
1. `k8s_get_resources` with `resource_type: pods` and the namespace (`all_namespaces: true` if unknown). Note STATUS, RESTARTS, and READY.
2. `k8s_describe_resource` on the failing pod. Read the **Events** section and the container `State`/`Last State`.
3. `k8s_get_events` for the namespace to find scheduling, pull, or probe failures.

## 2. Classify
| Symptom | Next step |
|---|---|
| `CrashLoopBackOff`, `Error` | `k8s_get_pod_logs` (also with `previous: true` if supported) for the crashing container |
| `OOMKilled` | Compare limits in `k8s_get_resource_yaml` with usage. Recommend new limits |
| `Pending` | Describe the pod: insufficient CPU/memory, taints, node selectors, or unbound PVCs |
| `ImagePullBackOff` | Check the image name/tag and `imagePullSecrets` in `k8s_get_resource_yaml` |
| Readiness failing | Compare the probe path/port in YAML with the container port. Check logs |
| Service unreachable | `k8s_get_resources` for `endpoints`/`endpointslices`, compare Service selector with pod labels, then `k8s_check_service_connectivity` |

## 3. Remediate (mutating; confirm with the user first)
Only after the user agrees: `k8s_patch_resource`, `k8s_scale`, `k8s_rollout` (restart/undo), or `k8s_apply_manifest`. Show the exact change before applying it. Afterwards, re-run step 1 and report the result.

## Notes
- Mutating tools are absent when the server runs read-only. In that case, provide the patch as YAML for the user to apply.
- Do not use `k8s_delete_resource` to "restart" pods; use `k8s_rollout` restart.
