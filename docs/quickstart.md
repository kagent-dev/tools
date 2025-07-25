
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
curl -sL https://raw.githubusercontent.com/kagent-dev/tools/refs/heads/main/scripts/install.sh | bash
curl -sL https://raw.githubusercontent.com/kagent-dev/tools/refs/heads/main/scripts/agentgateway-config-tools.yaml
curl -sL https://raw.githubusercontent.com/agentgateway/agentgateway/refs/heads/main/common/scripts/get-agentproxy | bash

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


1. Download the agentgateway binary and install it.
```
curl -sL https://raw.githubusercontent.com/kagent-dev/tools/refs/heads/main/scripts/install.sh | bash
```

2. Create `.cursor/mcp.json` 

```json
{
    "mcpServers": {
        "kagent-tools": {
            "command": "kagent-tools",
            "args": ["--stdio", "--kubeconfig", "~/.kube/config"]
        }
    }
}
```



