---
name: argo-rollouts-progressive-delivery
description: Operate Argo Rollouts canary and blue-green releases (status, promote, pause, change image) with the kagent-tools argo_* MCP tools. Use when a Rollout is progressing, paused, degraded, or needs a new image.
---
# Argo Rollouts progressive delivery

## Inspect (read-only)
1. `argo_verify_argo_rollouts_controller_install` and `argo_verify_kubectl_plugin_install` once per cluster.
2. `argo_rollouts_list` in the namespace for phase, step, and weight.
3. If a Rollout is `Degraded`, describe it with `k8s_describe_resource` (`resource_type: rollout`) and check AnalysisRun results with `k8s_get_resources` (`resource_type: analysisrun`).

## Act (mutating; confirm with the user first)
- Start a release: `argo_set_rollout_image` with the container name and the full image reference.
- Hold: `argo_pause_rollout`.
- Advance one step or fully promote: `argo_promote_rollout`. State which applies before running it.
- Gateway API traffic routing: `argo_verify_gateway_plugin` before the first release.

## Verify
After each action, run `argo_rollouts_list` again. Report the step and weight, and stop if analysis fails.
