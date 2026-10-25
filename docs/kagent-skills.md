# Using kagent-tools as a kagent 1.x skills bundle

The kagent-tools image is also a kagent 1.x **Agent Plugin**: an agent can mount it read-only
as an OCI image volume and get the MCP tools server, all bundled CLIs, and a set of operational
skills, with no separate deployment.

## What you get

The image contains two plugin roots. Select **one** of them per agent.

| Root (`source.path`) | Contents |
|---|---|
| `opt/kagent-tools` | `plugin.json`, `mcp.json` (stdio MCP server), `bin/` (`kagent-tools`, `kubectl`, `helm`, `istioctl`, `kubectl-argo-rollouts`, `cilium`), `config/kubeconfig`, `skills/` |
| `opt/kagent-tools-skills` | `plugin.json` and `skills/` only (use with a `RemoteMCPServer`) |

Skills (identical in both roots):

| Skill | Use it for |
|---|---|
| `k8s-troubleshooting` | Unhealthy workloads: CrashLoopBackOff, Pending, ImagePullBackOff, OOMKilled, failing readiness, unreachable Services |
| `helm-release-management` | Installed charts, release status, values, failed upgrades, chart repositories |
| `istio-traffic-debugging` | Mesh 503s, mTLS failures, routing not applied, sidecar or ambient/waypoint issues |
| `argo-rollouts-progressive-delivery` | Canary and blue-green Rollouts: status, promote, pause, new image |
| `cilium-network-debugging` | Cilium agent health, endpoints and identities, policy drops, services, BGP, cluster mesh |
| `kubescape-security-posture` | Configuration scans, image CVEs, application profiles, network neighborhoods |
| `prometheus-observability` | Error rates, latency, saturation, scrape health (needs `PROMETHEUS_URL`) |

Skills are playbooks that call the MCP tools by name (`k8s_get_resources`, `helm_list_releases`,
...). They work the same whether the tools come from the bundled stdio server or a
`RemoteMCPServer`.

## Requirements

- kagent 1.x (`api.kagent.dev/v1alpha3`) with the **Go ADK (`kagent`) harness**. Only that
  harness starts plugin `mcp.json` servers; Codex and Claude harnesses get the skills only.
- For the full root (`opt/kagent-tools`): a kagent release that does not apply the 100 MiB
  package-size limit to platform-mounted OCI packages. The full root is ~550–650 MiB because it
  contains the CLIs. Older kagent releases must use [skills-only mode](#skills-only-mode).
- The image pinned by digest. Print it with:

  ```bash
  docker buildx imagetools inspect ghcr.io/kagent-dev/kagent/tools:<tag>
  ```

## Cluster access

Inside the agent, the bundled server uses `config/kubeconfig`. It points at
`https://kubernetes.default.svc`, trusts the kagent egress CA
(`/run/kagent/egress/trust-bundle.pem`) and carries the placeholder token
`kagent-credential-injected`. The kagent egress gateway replaces that token with a real
ServiceAccount token from an `EgressPolicy`, so no credential ever enters the agent sandbox.
RBAC on the ServiceAccount is the authorization boundary.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kagent-tools-agent
  namespace: kagent
---
apiVersion: v1
kind: Secret
metadata:
  name: kagent-tools-agent-token
  namespace: kagent
  annotations:
    kubernetes.io/service-account.name: kagent-tools-agent
type: kubernetes.io/service-account-token
---
# "view" for read-only agents; bind a custom role if the agent may use write tools.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kagent-tools-agent-view
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
  - kind: ServiceAccount
    name: kagent-tools-agent
    namespace: kagent
---
apiVersion: api.kagent.dev/v1alpha3
kind: EgressPolicy
metadata:
  name: kube-api
  namespace: kagent
spec:
  destinations:
    - origin: https://kubernetes.default.svc
      authentication:
        header: Authorization
        prefix: "Bearer "
        secretKeyRef:
          name: kagent-tools-agent-token
          key: token
```

## Agent

```yaml
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: k8s-operator
  namespace: kagent
spec:
  systemPrompt: Use the kagent-tools skills and tools to operate the cluster.
  plugins:
    - source:
        oci: ghcr.io/kagent-dev/kagent/tools@sha256:<digest>
        path: opt/kagent-tools
      skills:
        - k8s-troubleshooting
        - helm-release-management
---
apiVersion: api.kagent.dev/v1alpha3
kind: Agent
metadata:
  name: k8s-operator
  namespace: kagent
spec:
  template:
    ref:
      name: k8s-operator
  harnessRef:
    name: kagent # a Harness with spec.kagent (Go ADK)
  egressPolicyRefs:
    - name: kube-api
```

kagent starts `bin/kagent-tools --stdio` from the plugin root and exposes all its tools to the
model. The selected skills are copied to `/skills/<name>` and offered through `list_skills` /
`load_skill`.

## Tuning via Harness env

The server reads these variables when the matching flag is not set. Set them in the Harness
`spec.env`; `mcp.json` deliberately leaves them unset so the Harness value applies.

| Variable | Effect |
|---|---|
| `KAGENT_TOOLS_READ_ONLY=true` | Do not register write tools (same as `--read-only`) |
| `KAGENT_TOOLS_TOOLS=k8s,helm` | Register only these providers (same as `--tools`) |
| `KUBECONFIG=<path>` | Use a different kubeconfig instead of the bundled egress kubeconfig |
| `PROMETHEUS_URL=https://prometheus.example.com` | Prometheus endpoint; also add its origin to an EgressPolicy |
| `OTEL_EXPORTER_OTLP_ENDPOINT=...` | Export traces; without it, traces go to stderr in stdio mode |

```yaml
spec:
  env:
    - name: KAGENT_TOOLS_READ_ONLY
      value: "true"
    - name: KAGENT_TOOLS_TOOLS
      value: k8s,helm
```

Kubeconfig precedence is `--kubeconfig` > `KUBECONFIG` > `KAGENT_TOOLS_DEFAULT_KUBECONFIG` (set
by `mcp.json` to the bundled kubeconfig) > in-cluster config. The resolved path is exported as
`KUBECONFIG` so every CLI (helm, istioctl, cilium, argo) targets the same cluster.

## Skills-only mode

Use this on kagent releases that still enforce the 100 MiB limit on mounted packages, or with
harnesses that do not start plugin MCP servers. Deploy the server with the Helm chart as usual,
then reference it as a `RemoteMCPServer` and select the skills-only root:

```yaml
apiVersion: api.kagent.dev/v1alpha3
kind: RemoteMCPServer
metadata:
  name: kagent-tools
  namespace: kagent
spec:
  description: kagent-tools MCP server
  url: http://kagent-tools.kagent:8084/mcp
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: k8s-operator
  namespace: kagent
spec:
  systemPrompt: Use the kagent-tools skills and tools to operate the cluster.
  tools:
    - mcp:
        server:
          kind: RemoteMCPServer
          name: kagent-tools
  plugins:
    - source:
        oci: ghcr.io/kagent-dev/kagent/tools@sha256:<digest>
        path: opt/kagent-tools-skills
      skills:
        - k8s-troubleshooting
```

> **Never select both roots in one agent.** They contain the same skill names, and kagent
> rejects duplicate skills.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Agent fails with `artifact exceeds 104857600 bytes` | The kagent release applies the size limit to mounted packages. Use skills-only mode. |
| Tools return `401 Unauthorized` / `403 Forbidden` | The EgressPolicy Secret is wrong or empty, or RBAC on `kagent-tools-agent` does not allow the request. |
| Tools return connection refused or a blocked-egress error | The Agent does not list the EgressPolicy in `egressPolicyRefs`, or the origin does not match `https://kubernetes.default.svc`. |
| `unable to read certificate-authority /run/kagent/egress/trust-bundle.pem` | The agent is not running behind the kagent egress gateway (no trust bundle mounted). Set `KUBECONFIG` on the Harness or use skills-only mode. |
| Skills appear but no `k8s_*` tools | The harness is not the Go ADK `kagent` harness. Use skills-only mode. |
| `exec`, `logs -f`, `port-forward` hang or fail | These need HTTP upgrade/streaming through the egress gateway and are best effort. Prefer non-streaming calls. |

## Verifying an image locally

```bash
make plugin-verify
```

This builds the image, exports its filesystem, checks both roots against kagent's package rules
(file types, symlinks, entry count, skills-only size), and runs an MCP `initialize` +
`tools/list` over stdio to confirm stdout carries only JSON-RPC.
