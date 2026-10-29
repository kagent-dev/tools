---
name: kubescape-security-posture
description: Review cluster security posture with Kubescape results (configuration scans, image vulnerabilities, application profiles, network neighborhoods) using the kagent-tools kubescape_* MCP tools. Use for security reviews, CVE questions, or hardening recommendations.
---
# Kubescape security posture

## 1. Preconditions
`kubescape_check_health` must report the operator and storage as healthy. Otherwise, explain that the Kubescape operator must be installed first.

## 2. Configuration risks
1. `kubescape_list_configuration_scans` in the namespace (or cluster-wide).
2. `kubescape_get_configuration_scan` for a workload. Group failed controls by severity and give the YAML fix for each.

## 3. Vulnerabilities
1. `kubescape_list_vulnerability_manifests` to find the manifest of each image or workload, then `kubescape_list_vulnerabilities` with its `manifest_name`. The tools do not filter by severity; report Critical and High first.
2. `kubescape_get_vulnerability_details` for a CVE: affected package, fixed version, and which workloads use the image.

## 4. Runtime behaviour
`kubescape_list_application_profiles` / `kubescape_get_application_profile` and `kubescape_list_network_neighborhoods` / `kubescape_get_network_neighborhood` show the observed syscalls, processes, and connections. Use them to propose least-privilege seccomp or NetworkPolicies.

Report findings as a prioritised table with severity, resource, issue, and fix. This skill is read-only; apply fixes only through other skills after confirmation.
