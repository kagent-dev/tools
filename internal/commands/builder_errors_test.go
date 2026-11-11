package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/kagent-dev/tools/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteKeepsCommandOutputOnFailure checks that a failed command's own
// message reaches the caller, not only its exit status.
func TestExecuteKeepsCommandOutputOnFailure(t *testing.T) {
	exitStatus := fmt.Errorf("exit status 1")

	testCases := []struct {
		name          string
		output        string
		wantInError   string
		wantErrorCode string
	}{
		{
			name:        "admission webhook denial",
			output:      "Error from server (Forbidden): admission webhook \"policy.example.com\" denied the request: scaling prod needs approval\n",
			wantInError: `admission webhook "policy.example.com" denied the request: scaling prod needs approval`,
		},
		{
			name:          "RBAC denial gets the permission suggestions",
			output:        "Error from server (Forbidden): deployments.apps \"checkout\" is forbidden: User \"system:serviceaccount:kagent:kagent-tools\" cannot patch resource \"deployments\" in API group \"apps\" in the namespace \"prod\"\n",
			wantInError:   `cannot patch resource "deployments"`,
			wantErrorCode: "K8S_PERMISSION_ERROR",
		},
		{
			name:        "no output keeps the error as it was",
			output:      "",
			wantInError: "failed: exit status 1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockExecutor := cmd.NewMockShellExecutor()
			mockExecutor.AddPartialMatcherString("kubectl", []string{"patch"}, testCase.output, exitStatus)
			ctx := cmd.WithShellExecutor(context.Background(), mockExecutor)

			_, err := NewCommandBuilder("kubectl").
				WithArgs("patch", "deployment", "checkout", "--type=merge", "-p", `{"spec":{"replicas":6}}`).
				WithNamespace("prod").
				Execute(ctx)

			var toolError *errors.ToolError
			require.ErrorAs(t, err, &toolError)
			assert.Contains(t, err.Error(), testCase.wantInError)
			assert.ErrorIs(t, toolError.Cause, exitStatus, "the original error stays wrapped")
			if testCase.wantErrorCode != "" {
				assert.Equal(t, testCase.wantErrorCode, toolError.ErrorCode)
			}
		})
	}
}
