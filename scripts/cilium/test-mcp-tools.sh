#!/usr/bin/env bash
# Test Cilium MCP tools against a running kagent-tools instance.
#
# Prerequisites:
#   - Kind cluster "kagent" running with Cilium installed
#   - kagent-tools deployed and accessible on NodePort 30884
#
# Usage:
#   ./scripts/cilium/test-mcp-tools.sh [MCP_URL] [NODE_NAME]
#
# Defaults:
#   MCP_URL   = http://127.0.0.1:30884/mcp
#   NODE_NAME = kagent-control-plane

set -euo pipefail

MCP_URL="${1:-http://127.0.0.1:30884/mcp}"
NODE_NAME="${2:-kagent-control-plane}"

PASS=0
FAIL=0
SKIP=0

# Initialize MCP session
SESSION_ID=$(curl -sf -D - -X POST "$MCP_URL" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"cilium-test","version":"1.0"}}}' \
  2>&1 | grep -i 'mcp-session-id' | tr -d '\r' | awk '{print $2}')

if [ -z "$SESSION_ID" ]; then
  echo "ERROR: Failed to initialize MCP session at $MCP_URL"
  exit 1
fi
echo "MCP session: $SESSION_ID"
echo ""

ID=2

call_tool() {
  local name=$1
  local args=$2

  RESULT=$(curl -sf -X POST "$MCP_URL" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -H "Mcp-Session-Id: $SESSION_ID" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":$ID,\"method\":\"tools/call\",\"params\":{\"name\":\"$name\",\"arguments\":$args}}" 2>&1 || true)
  ID=$((ID + 1))

  if [ -z "$RESULT" ]; then
    echo "SKIP: $name (no response)"
    SKIP=$((SKIP + 1))
  elif echo "$RESULT" | grep -q '"isError":true'; then
    echo "FAIL: $name"
    FAIL=$((FAIL + 1))
  else
    echo "OK:   $name"
    PASS=$((PASS + 1))
  fi
}

NODE_ARGS="{\"node_name\":\"$NODE_NAME\"}"

echo "=== Cilium CLI tools ==="
call_tool "cilium_status_and_version" "{}"

echo ""
echo "=== Cilium-dbg read-only tools ==="
call_tool "cilium_get_endpoints_list"              "$NODE_ARGS"
call_tool "cilium_get_daemon_status"               "$NODE_ARGS"
call_tool "cilium_list_identities"                 "$NODE_ARGS"
call_tool "cilium_display_encryption_state"        "$NODE_ARGS"
call_tool "cilium_list_services"                   "$NODE_ARGS"
call_tool "cilium_list_metrics"                    "$NODE_ARGS"
call_tool "cilium_fqdn_cache"                      "$NODE_ARGS"
call_tool "cilium_show_dns_names"                  "$NODE_ARGS"
call_tool "cilium_show_configuration_options"       "$NODE_ARGS"
call_tool "cilium_list_cluster_nodes"              "$NODE_ARGS"
call_tool "cilium_list_node_ids"                   "$NODE_ARGS"
call_tool "cilium_list_bpf_maps"                   "$NODE_ARGS"
call_tool "cilium_list_ip_addresses"               "$NODE_ARGS"
call_tool "cilium_display_selectors"               "$NODE_ARGS"
call_tool "cilium_list_local_redirect_policies"    "$NODE_ARGS"
call_tool "cilium_request_debugging_information"   "$NODE_ARGS"
call_tool "cilium_show_load_information"           "$NODE_ARGS"
call_tool "cilium_display_policy_node_information" "{\"node_name\":\"$NODE_NAME\",\"labels\":\"\"}"
call_tool "cilium_list_xdp_cidr_filters"           "$NODE_ARGS"

echo ""
echo "=== Results ==="
echo "PASS: $PASS  FAIL: $FAIL  SKIP: $SKIP  TOTAL: $((PASS + FAIL + SKIP))"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
