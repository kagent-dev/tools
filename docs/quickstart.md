
# Quickstart Guide for KAgnet Tools

## About this guide

This guide provides a quick overview of how to set up and run KAgent tools using AgentGateway.

For more detaled information on KAgent tools, please refer to the [KAgent Tools Documentation](https://kagent.dev/tools).

To learn more about agentgateway, see [AgentGateway](https://agentgateway.dev/docs/about/)

### Running KAgent Tools using AgentGateway

1. Download tools binary and install it.
2. Download tools configuration file for agentgateway.
3. Download the agentgateway binary and install it.
4. Run the agentgateway with the configuration file.
5. open http://localhost:15000/ui

```bash
# Install KAgent Tools
curl -sL https://raw.githubusercontent.com/kagent-dev/tools/refs/heads/main/scripts/install.sh | bash

# Download AgentGateway configuration
curl -sL https://raw.githubusercontent.com/kagent-dev/tools/refs/heads/main/scripts/agentgateway-config-tools.yaml -o agentgateway-config-tools.yaml

# Install AgentGateway
curl -sL https://raw.githubusercontent.com/agentgateway/agentgateway/refs/heads/main/common/scripts/get-agentproxy | bash

# Add to PATH and run
export PATH=$PATH:$HOME/.local/bin/
agentgateway -f agentgateway-config-tools.yaml
```

agentgateway-config-tools.yaml:
```yaml
binds:
  - port: 30805
    listeners:
      - routes:
          - policies:
              cors:
                allowOrigins:
                  - "*"
                allowHeaders:
                  - mcp-protocol-version
                  - content-type
            backends:
              - mcp:
                  name: default
                  targets:
                    - name: kagent-tools
                      stdio:
                        cmd: kagent-tools
                        args: ["--stdio", "--kubeconfig", "~/.kube/config"]
```
Afterwards, you can run it with make command 
```bash
make run-agentgateway
```

### Running KAgent Tools using Cursor MCP

1. Install KAgent Tools:
```bash
curl -sL https://raw.githubusercontent.com/kagent-dev/tools/refs/heads/main/scripts/install.sh | bash
```

2. Create `.cursor/mcp.json` in your project root:

```json
{
    "mcpServers": {
        "kagent-tools": {
            "command": "kagent-tools",
            "args": ["--stdio", "--kubeconfig", "~/.kube/config"],
            "env": {
                "LOG_LEVEL": "info"
            }
        }
    }
}
```

3. Restart Cursor and the KAgent Tools will be available through the MCP interface.

### Available Tools

Once connected, you'll have access to all KAgent tool categories:
- **Kubernetes**: `kubectl_get`, `kubectl_describe`, `kubectl_logs`, `kubectl_apply`, etc.
- **Helm**: `helm_list`, `helm_install`, `helm_upgrade`, etc.
- **Istio**: `istio_proxy_status`, `istio_analyze`, `istio_install`, etc.
- **Argo Rollouts**: `promote_rollout`, `pause_rollout`, `set_rollout_image`, etc.
- **Cilium**: `cilium_status_and_version`, `install_cilium`, `upgrade_cilium`, etc.
- **Prometheus**: `prometheus_query`, `prometheus_range_query`, `prometheus_labels`, etc.
- **Utils**: `shell`, `current_date_time`, etc.

## ArgoCD CLI with Local Kind Cluster

This guide demonstrates how to set up and use ArgoCD CLI with a local Kind cluster for GitOps workflows.

### Prerequisites

- Docker (for running Kind)
- kubectl (Kubernetes CLI)
- Kind (Kubernetes in Docker)
- ArgoCD CLI

### Step 1: Set Up Kind Cluster

First, create a local Kind cluster using the project's setup script:

```bash
# Navigate to the project directory
cd /Users/dimetron/p6s/cncf/kagent/kagent-tools

# Run the Kind setup script
bash scripts/kind/setup-kind.sh
```

This will:
1. Create a Docker registry container at `localhost:5001`
2. Create a Kind cluster named `kagent`
3. Configure the cluster with containerd registry support
4. Set up local registry hosting ConfigMap

Verify the cluster is running:

```bash
# List Kind clusters
kind get clusters

# Get cluster info
kubectl cluster-info --context kind-kagent

# Check nodes
kubectl get nodes
```

### Step 2: Install ArgoCD CLI

#### macOS

```bash
# Using Homebrew
brew install argocd

# Or download directly
curl -sSL -o /usr/local/bin/argocd https://github.com/argoproj/argo-cd/releases/latest/download/argocd-darwin-amd64
chmod +x /usr/local/bin/argocd
```

#### Linux

```bash
# Download latest ArgoCD CLI
curl -sSL -o argocd https://github.com/argoproj/argo-cd/releases/latest/download/argocd-linux-amd64
chmod +x argocd
sudo mv argocd /usr/local/bin/
```

#### Windows

```powershell
# Using Chocolatey
choco install argocd-cli

# Or download from GitHub releases
```

Verify installation:

```bash
argocd version
```

### Step 3: Install ArgoCD Server in Kind Cluster

Create a dedicated namespace and install ArgoCD:

```bash
# Create ArgoCD namespace
kubectl create namespace argocd

# Install ArgoCD using the official manifest
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for ArgoCD components to be ready
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s

# Verify installation
kubectl get pods -n argocd
```

Expected pods:
- `argocd-application-controller-*`
- `argocd-dex-server-*`
- `argocd-redis-*`
- `argocd-repo-server-*`
- `argocd-server-*`

### Step 4: Access ArgoCD Server Locally

#### Port Forward to ArgoCD Server

```bash
# Port forward the ArgoCD server (runs in background)
kubectl port-forward svc/argocd-server -n argocd 8080:443 &

# Or run in foreground (keep terminal open)
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

ArgoCD UI will be available at: `https://localhost:8080`

#### Get Initial Admin Password

```bash
# Extract the initial admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Example output: rA1b2cD3eF4gH5iJ6kL7mN8oP9qR0
```

### Step 5: Login with ArgoCD CLI

```bash
# Login to ArgoCD (use 'admin' as username)
argocd login localhost:18080 --insecure

# You'll be prompted for password - use the one from Step 4
```

For non-interactive login:

```bash
# Store the password in a variable
ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)

# Login without interaction
argocd login localhost:8080 --username admin --password $ARGOCD_PASSWORD --insecure
```

Verify login:

```bash
argocd cluster list
argocd account list
```

### Step 6: Add Local Cluster to ArgoCD

```bash
# Get the current cluster context
kubectl config current-context
# Output: kind-kagent

# Add the cluster to ArgoCD
argocd cluster add kind-kagent

# Verify the cluster was added
argocd cluster list
```

Output should show:
```
SERVER                          NAME              VERSION  STATUS     MESSAGE
https://127.0.0.1:6443         in-cluster        1.34.0   Successful
https://kubernetes.default.svc in-cluster        1.34.0   Successful
```

### Step 7: Create a Test Application

Create a sample Git repository application:

```bash
# Create a sample application manifest
cat > /tmp/guestbook-app.yaml << 'EOF'
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: guestbook
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: guestbook
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
EOF

# Apply the application
kubectl apply -f /tmp/guestbook-app.yaml
```

### Step 8: Manage Applications with ArgoCD CLI

#### List Applications

```bash
# List all applications
argocd app list

# Get detailed info about an application
argocd app info guestbook
```

#### Sync Application

```bash
# Sync application (deploy from Git)
argocd app sync guestbook

# Sync and wait for health
argocd app sync guestbook --wait

# Sync with specific revision
argocd app sync guestbook --revision main
```

#### Check Application Status

```bash
# Get application status
argocd app get guestbook

# Watch application status
argocd app get guestbook --watch
```

#### View Application Logs

```bash
# Get application logs
argocd app logs guestbook --tail 50
```

#### Delete Application

```bash
# Delete application
argocd app delete guestbook
```

### Step 9: Common ArgoCD CLI Commands

#### Repository Management

```bash
# Add a Git repository
argocd repo add https://github.com/myorg/my-repo.git \
  --username <username> \
  --password <password>

# List repositories
argocd repo list

# Remove repository
argocd repo remove https://github.com/myorg/my-repo.git
```

#### Project Management

```bash
# List projects
argocd proj list

# Get project details
argocd proj get default

# Create new project
argocd proj create myproject \
  --description "My Project" \
  --source "*" \
  --destination "https://kubernetes.default.svc,*"
```

#### Account/User Management

```bash
# List accounts
argocd account list

# Update password
argocd account update-password

# Generate API token
argocd account generate-token
```

#### Cluster Management

```bash
# List clusters
argocd cluster list

# Get cluster info
argocd cluster get in-cluster

# Remove cluster
argocd cluster rm in-cluster
```

### Step 10: Advanced Usage

#### Deploy from Local Git Repository

```bash
# Create a local application using local chart
cat > /tmp/local-app.yaml << 'EOF'
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: local-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/your-repo.git
    targetRevision: main
    path: k8s/
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
      allowEmpty: false
    syncOptions:
    - CreateNamespace=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
EOF

kubectl apply -f /tmp/local-app.yaml
```

#### Enable Auto-Sync

```bash
# Enable automatic synchronization
argocd app set guestbook \
  --sync-policy automated \
  --auto-prune \
  --self-heal
```

#### Use Kustomize with ArgoCD

```bash
# Create Kustomize-based application
cat > /tmp/kustomize-app.yaml << 'EOF'
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: kustomize-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/your-repo.git
    targetRevision: main
    path: kustomize/
    plugin:
      name: kustomize
  destination:
    server: https://kubernetes.default.svc
    namespace: default
EOF

kubectl apply -f /tmp/kustomize-app.yaml
```

#### Use Helm with ArgoCD

```bash
# Create Helm-based application
cat > /tmp/helm-app.yaml << 'EOF'
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: helm-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://charts.example.com
    chart: my-chart
    targetRevision: "1.0.0"
    helm:
      values: |
        key1: value1
        key2: value2
  destination:
    server: https://kubernetes.default.svc
    namespace: default
EOF

kubectl apply -f /tmp/helm-app.yaml
```

### Troubleshooting

#### CLI Connection Issues

```bash
# Check server connectivity
argocd cluster list

# Re-login if needed
argocd login localhost:8080 --insecure

# Update kubeconfig context
kubectl config use-context kind-kagent
```

#### Application Sync Issues

```bash
# Check application status
argocd app get <app-name> --refresh

# View sync logs
argocd app logs <app-name> --tail 100

# Manually trigger sync
argocd app sync <app-name> --force
```

#### Server Connectivity Problems

```bash
# Check if port-forward is running
lsof -i :8080

# Restart port-forward if needed
pkill kubectl
kubectl port-forward svc/argocd-server -n argocd 8080:443 &
```

#### Resource Issues

```bash
# Check ArgoCD pod status
kubectl get pods -n argocd

# View pod logs
kubectl logs -n argocd <pod-name> --tail 50

# Check events
kubectl get events -n argocd
```

### Complete Workflow Example

Here's a complete workflow from setup to deployment:

```bash
#!/bin/bash
set -e

echo "1. Creating Kind cluster..."
bash scripts/kind/setup-kind.sh

echo "2. Installing ArgoCD..."
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s

echo "3. Setting up port forward..."
kubectl port-forward svc/argocd-server -n argocd 8080:443 > /dev/null 2>&1 &
sleep 3

echo "4. Getting initial password..."
ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
echo "ArgoCD Password: $ARGOCD_PASSWORD"

echo "5. Logging in..."
argocd login localhost:8080 --username admin --password $ARGOCD_PASSWORD --insecure

echo "6. Adding cluster..."
argocd cluster add kind-kagent

echo "7. Creating test application..."
kubectl apply -f /tmp/guestbook-app.yaml

echo "8. Syncing application..."
argocd app sync guestbook --wait

echo "Done! ArgoCD is ready at https://localhost:8080"
echo "Username: admin"
echo "Password: $ARGOCD_PASSWORD"
```

### Resources

- [ArgoCD Official Documentation](https://argo-cd.readthedocs.io/)
- [ArgoCD GitHub Repository](https://github.com/argoproj/argo-cd)
- [ArgoCD CLI Reference](https://argo-cd.readthedocs.io/en/stable/user-guide/commands/argocd/)
- [Kind Documentation](https://kind.sigs.k8s.io/)
- [GitOps Best Practices](https://www.gitops.tech/)



