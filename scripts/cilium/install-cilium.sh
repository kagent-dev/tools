#!/usr/bin/env bash
# Install Cilium into a Kind cluster via the kagent-tools pod.
#
# Prerequisites:
#   - Kind cluster running with kagent-tools deployed
#
# Usage:
#   ./scripts/cilium/install-cilium.sh [NAMESPACE] [RELEASE_NAME] [KUBE_CONTEXT]
#
# Defaults:
#   NAMESPACE    = kagent
#   RELEASE_NAME = kagent-tools
#   KUBE_CONTEXT = kind-kagent

set -euo pipefail

NAMESPACE="${1:-kagent}"
RELEASE_NAME="${2:-kagent-tools}"
KUBE_CONTEXT="${3:-kind-kagent}"

echo "Installing Cilium via kagent-tools pod in namespace=$NAMESPACE context=$KUBE_CONTEXT"

# Find the kagent-tools pod
POD=$(kubectl --context "$KUBE_CONTEXT" get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/instance=$RELEASE_NAME" \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$POD" ]; then
  echo "ERROR: No kagent-tools pod found in namespace $NAMESPACE"
  exit 1
fi

echo "Using pod: $POD"

# Install Cilium
kubectl --context "$KUBE_CONTEXT" exec -n "$NAMESPACE" "$POD" -- \
  cilium install \
    --set routingMode=native \
    --set ipv4NativeRoutingCIDR=10.244.0.0/16 \
    --set bpf.masquerade=false

echo ""
echo "Waiting for Cilium pods to be ready..."
kubectl --context "$KUBE_CONTEXT" wait \
  --for=condition=ready pod \
  -l k8s-app=cilium \
  -n kube-system \
  --timeout=120s

echo ""
echo "Cilium status:"
kubectl --context "$KUBE_CONTEXT" exec -n "$NAMESPACE" "$POD" -- cilium status
