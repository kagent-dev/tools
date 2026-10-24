---
name: prometheus-observability
description: Answer questions about cluster and application metrics (error rates, latency, saturation, scrape health) with the kagent-tools prometheus_* MCP tools. Use when investigating performance, alerts, or whether a target is scraped. Requires PROMETHEUS_URL to be configured.
---
# Prometheus observability

## 1. Check connectivity
`prometheus_targets_tool` lists scrape targets. If it fails, tell the user that `PROMETHEUS_URL` (and an EgressPolicy for it when running inside a kagent agent) must be configured.

## 2. Discover before querying
`prometheus_label_names_tool` shows the available labels, so queries use real label names.

## 3. Query
- Current value: `prometheus_query_tool`. Example: `sum(rate(http_requests_total{namespace="shop",code=~"5.."}[5m])) by (service)`.
- Trend: `prometheus_query_range_tool` with a range and step (for example, 1h with a 60s step).
- Draft complex PromQL with `prometheus_promql_tool`, then run it.

## 4. Interpret
Use RED for services (rate, errors, duration) and USE for resources (utilisation, saturation, errors). Always state the query you ran and the time window. Correlate spikes with `k8s_get_events` and rollouts.
