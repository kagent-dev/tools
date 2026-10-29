---
name: helm-release-management
description: Inspect, upgrade, roll forward and uninstall Helm releases with the kagent-tools helm_* MCP tools. Use for questions about installed charts, release status, values, failed upgrades, or adding chart repositories.
---
# Helm release management

## Inspect (read-only)
1. `helm_list_releases` (namespace or all namespaces) to find the release and its STATUS (`deployed`, `failed`, `pending-*`).
2. `helm_get_release` for values, manifest, notes, or hooks of a single release.
3. To check what the release created, use `k8s_get_resources` in the release namespace.

## Change (mutating; confirm with the user first)
- Add or refresh chart sources: `helm_repo_add`, then `helm_repo_update`.
- Upgrade or install: `helm_upgrade` with the chart, version, and only the values that change. Restate the chart version and the value diff before running it.
- A release stuck in `pending-upgrade`/`pending-install` usually needs the previous operation to finish or be rolled back. Explain this before acting.
- Remove: `helm_uninstall`, only after explicit confirmation that names the release and namespace.

## Verify
After any change, run `helm_list_releases` and `k8s_get_resources` for the workload pods. Report the new revision and pod health.
