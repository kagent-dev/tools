#!/usr/bin/env bash
# Verifies that a built kagent-tools image is a valid kagent Agent Plugin and,
# when given, that the skills-only image holds the same skills root.
set -euo pipefail

image="${1:?usage: $0 <image> [<skills-image>]}"
skills_image="${2:-}"
workdir="$(mktemp -d)"
container=""
cleanup() {
  [[ -n "$container" ]] && docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$workdir"
}
trap cleanup EXIT

export_image() {
  # The skills image has no entrypoint; docker create needs a command but never runs it.
  container="$(docker create "$1" /unused)"
  mkdir -p "$2"
  docker export "$container" | tar -x -C "$2"
  docker rm "$container" >/dev/null
  container=""
}

export_image "$image" "$workdir/tools"
export KAGENT_TOOLS_IMAGE_ROOT="$workdir/tools"
tests=TestPluginImageLayout
if [[ -n "$skills_image" ]]; then
  export_image "$skills_image" "$workdir/skills"
  export KAGENT_TOOLS_SKILLS_IMAGE_ROOT="$workdir/skills"
  tests="$tests|TestSkillsImageMatchesToolsImage"
fi
go test ./cmd -run "^($tests)\$" -count=1 -v

echo "--- legacy entrypoints"
docker run --rm --entrypoint /tool-server "$image" --version
docker run --rm --entrypoint /bin/kubectl "$image" version --client
docker run --rm --entrypoint /bin/helm "$image" version --short

echo "--- stdio smoke test"
# Keep stdin open briefly: the server exits on EOF before answering.
response="$({ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 5; } \
  | docker run -i --rm --entrypoint /opt/kagent-tools/bin/kagent-tools "$image" --stdio 2>/dev/null)"

for tool in k8s_get_resources helm_list_releases istio_proxy_status argo_rollouts_list cilium_status_and_version; do
  grep -q "\"name\":\"$tool\"" <<<"$response" || { echo "missing tool $tool in stdio tools/list"; exit 1; }
done
if grep -qv '^{"jsonrpc"' <<<"$response"; then
  echo "non JSON-RPC output on stdout:"; grep -v '^{"jsonrpc"' <<<"$response" | head -5; exit 1
fi
echo "plugin image OK: $image${skills_image:+, skills image OK: $skills_image}"
