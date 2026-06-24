package helm

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	RegisterTools(s, false) // false = enable all tools including write operations
}

// Test Helm List Releases
func TestHandleHelmListReleases(t *testing.T) {
	tests := []struct {
		name           string
		input          helmListReleasesInput
		expectedArgs   []string
		expectedOutput string
		expectError    bool
	}{
		{
			name:         "basic_list_releases",
			input:        helmListReleasesInput{},
			expectedArgs: []string{"list"},
			expectedOutput: `NAME    NAMESPACE       REVISION        STATUS          CHART
app1    default         1               deployed        my-chart-1.0.0
app2    default         2               deployed        my-chart-2.0.0`,
			expectError: false,
		},
		{
			name: "list_releases_with_namespace",
			input: helmListReleasesInput{
				Namespace: "production",
			},
			expectedArgs: []string{"list", "-n", "production"},
			expectedOutput: `NAME    NAMESPACE       REVISION        STATUS          CHART
prod-app    production      1               deployed        my-chart-1.0.0`,
			expectError: false,
		},
		{
			name: "list_releases_with_all_namespaces",
			input: helmListReleasesInput{
				AllNamespaces: true,
			},
			expectedArgs: []string{"list", "-A"},
			expectedOutput: `NAME    NAMESPACE       REVISION        STATUS          CHART
app1    default         1               deployed        my-chart-1.0.0
prod-app    production      1               deployed        my-chart-1.0.0`,
			expectError: false,
		},
		{
			name: "list_releases_with_multiple_flags",
			input: helmListReleasesInput{
				AllNamespaces: true,
				All:           true,
				Failed:        true,
				Output:        "json",
			},
			expectedArgs: []string{"list", "-A", "-a", "--failed", "-o", "json"},
			expectedOutput: `[
    {
        "name": "app1",
        "namespace": "default",
        "revision": "1",
        "status": "deployed"
    }
]`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := cmd.NewMockShellExecutor()
			mock.AddCommandString("helm", tt.expectedArgs, tt.expectedOutput, nil)
			ctx := cmd.WithShellExecutor(context.Background(), mock)

			result, _, err := handleHelmListReleases(ctx, &mcp.CallToolRequest{}, tt.input)

			assert.NoError(t, err)
			assert.False(t, result.IsError)

			// Verify the expected output
			content := getResultText(result)
			if tt.name == "basic_list_releases" {
				assert.Contains(t, content, "app1")
				assert.Contains(t, content, "app2")
			} else if tt.name == "list_releases_with_namespace" {
				assert.Contains(t, content, "prod-app")
				assert.Contains(t, content, "production")
			} else if tt.name == "list_releases_with_all_namespaces" {
				assert.Contains(t, content, "app1")
				assert.Contains(t, content, "prod-app")
			} else if tt.name == "list_releases_with_multiple_flags" {
				assert.Contains(t, content, "app1")
				assert.Contains(t, content, "default")
			}

			// Verify the correct command was called
			callLog := mock.GetCallLog()
			require.Len(t, callLog, 1)
			assert.Equal(t, "helm", callLog[0].Command)
			assert.Equal(t, tt.expectedArgs, callLog[0].Args)
		})
	}

	t.Run("helm command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("helm", []string{"list"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmListReleases(ctx, &mcp.CallToolRequest{}, helmListReleasesInput{})

		assert.NoError(t, err) // MCP handlers should not return Go errors
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "**Helm Error**")
	})
}

// Test Helm Get Release
func TestHandleHelmGetRelease(t *testing.T) {
	t.Run("get release all resources", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `REVISION: 1
RELEASED: Mon Jan 01 12:00:00 UTC 2023
CHART: myapp-1.0.0
VALUES:
replicaCount: 3`

		mock.AddCommandString("helm", []string{"get", "all", "myapp", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmGetRelease(ctx, &mcp.CallToolRequest{}, helmGetReleaseInput{
			Name:      "myapp",
			Namespace: "default",
		})

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "REVISION: 1")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"get", "all", "myapp", "-n", "default"}, callLog[0].Args)
	})

	t.Run("get release values only", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("helm", []string{"get", "values", "myapp", "-n", "default"}, "replicaCount: 3", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmGetRelease(ctx, &mcp.CallToolRequest{}, helmGetReleaseInput{
			Name:      "myapp",
			Namespace: "default",
			Resource:  "values",
		})

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with values resource
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"get", "values", "myapp", "-n", "default"}, callLog[0].Args)
	})

	t.Run("missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		// Test missing name
		result, _, err := handleHelmGetRelease(ctx, &mcp.CallToolRequest{}, helmGetReleaseInput{
			Namespace: "default",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name parameter is required")

		// Test missing namespace
		result, _, err = handleHelmGetRelease(ctx, &mcp.CallToolRequest{}, helmGetReleaseInput{
			Name: "myapp",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "namespace parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

// Test Helm Upgrade Release
func TestHandleHelmUpgradeRelease(t *testing.T) {
	t.Run("basic upgrade", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Release "myapp" has been upgraded. Happy Helming!
NAME: myapp
LAST DEPLOYED: Mon Jan 01 12:00:00 UTC 2023
NAMESPACE: default
STATUS: deployed
REVISION: 2`

		mock.AddCommandString("helm", []string{"upgrade", "myapp", "stable/myapp", "--timeout", "30s"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmUpgradeRelease(ctx, &mcp.CallToolRequest{}, helmUpgradeReleaseInput{
			Name:  "myapp",
			Chart: "stable/myapp",
		})

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "has been upgraded")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"upgrade", "myapp", "stable/myapp", "--timeout", "30s"}, callLog[0].Args)
	})

	t.Run("upgrade with all options", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedArgs := []string{
			"upgrade", "myapp", "stable/myapp",
			"-n", "production",
			"--version", "1.2.0",
			"-f", "values.yaml",
			"--set", "replicas=5",
			"--set", "image.tag=v1.2.0",
			"--install",
			"--dry-run",
			"--wait",
			"--timeout", "30s",
		}
		mock.AddCommandString("helm", expectedArgs, "Upgraded with options", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmUpgradeRelease(ctx, &mcp.CallToolRequest{}, helmUpgradeReleaseInput{
			Name:      "myapp",
			Chart:     "stable/myapp",
			Namespace: "production",
			Version:   "1.2.0",
			Values:    "values.yaml",
			Set:       "replicas=5,image.tag=v1.2.0",
			Install:   true,
			DryRun:    true,
			Wait:      true,
		})

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with all options
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, expectedArgs, callLog[0].Args)
	})

	t.Run("missing required parameters for upgrade", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		// Test missing chart
		result, _, err := handleHelmUpgradeRelease(ctx, &mcp.CallToolRequest{}, helmUpgradeReleaseInput{
			Name: "myapp",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name and chart parameters are required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

// Test Helm Uninstall
func TestHandleHelmUninstall(t *testing.T) {
	t.Run("basic uninstall", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `release "myapp" uninstalled`

		mock.AddCommandString("helm", []string{"uninstall", "myapp", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmUninstall(ctx, &mcp.CallToolRequest{}, helmUninstallInput{
			Name:      "myapp",
			Namespace: "default",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "uninstalled")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"uninstall", "myapp", "-n", "default"}, callLog[0].Args)
	})

	t.Run("uninstall with options", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `release "myapp" uninstalled`

		mock.AddCommandString("helm", []string{"uninstall", "myapp", "-n", "production", "--dry-run", "--wait"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmUninstall(ctx, &mcp.CallToolRequest{}, helmUninstallInput{
			Name:      "myapp",
			Namespace: "production",
			DryRun:    true,
			Wait:      true,
		})

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with options
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"uninstall", "myapp", "-n", "production", "--dry-run", "--wait"}, callLog[0].Args)
	})

	t.Run("missing required parameters for uninstall", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		// Test missing name
		result, _, err := handleHelmUninstall(ctx, &mcp.CallToolRequest{}, helmUninstallInput{
			Namespace: "default",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name and namespace parameters are required")

		// Test missing namespace
		result, _, err = handleHelmUninstall(ctx, &mcp.CallToolRequest{}, helmUninstallInput{
			Name: "myapp",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name and namespace parameters are required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

// Test Helm Repo Add
func TestHandleHelmRepoAdd(t *testing.T) {
	t.Run("basic repo add", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `"my-repo" has been added to your repositories`

		mock.AddCommandString("helm", []string{"repo", "add", "my-repo", "https://charts.example.com/"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmRepoAdd(ctx, &mcp.CallToolRequest{}, helmRepoAddInput{
			Name: "my-repo",
			URL:  "https://charts.example.com/",
		})

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "has been added")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"repo", "add", "my-repo", "https://charts.example.com/"}, callLog[0].Args)
	})

	t.Run("missing required parameters for repo add", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		// Test missing name
		result, _, err := handleHelmRepoAdd(ctx, &mcp.CallToolRequest{}, helmRepoAddInput{
			URL: "https://charts.example.com/",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name and url parameters are required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

// Test Helm Repo Update
func TestHandleHelmRepoUpdate(t *testing.T) {
	t.Run("basic repo update", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Hang tight while we grab the latest from your chart repositories...
...Successfully got an update from the "stable" chart repository
Update Complete. ⎈Happy Helming!⎈`

		mock.AddCommandString("helm", []string{"repo", "update"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleHelmRepoUpdate(ctx, &mcp.CallToolRequest{}, helmRepoUpdateInput{})

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "Successfully got an update")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"repo", "update"}, callLog[0].Args)
	})
}

// Helper function to extract text content from MCP result
func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}
