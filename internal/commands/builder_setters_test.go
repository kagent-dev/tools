package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuilderSettersValidation exercises both the accept and reject branches of
// the validating setters so invalid input is silently dropped, not applied.
func TestBuilderSettersValidation(t *testing.T) {
	t.Run("WithToken", func(t *testing.T) {
		cb := NewCommandBuilder("kubectl").WithToken("secret")
		assert.Equal(t, "secret", cb.token)
		// Empty token is a no-op and keeps the previous value.
		cb.WithToken("")
		assert.Equal(t, "secret", cb.token)
	})

	t.Run("WithContext", func(t *testing.T) {
		cb := NewCommandBuilder("kubectl").WithContext("prod-cluster")
		assert.Equal(t, "prod-cluster", cb.context)
		// Injection attempt is rejected, leaving the prior value intact.
		cb.WithContext("ctx; rm -rf /")
		assert.Equal(t, "prod-cluster", cb.context)
	})

	t.Run("WithKubeconfig", func(t *testing.T) {
		cb := NewCommandBuilder("kubectl").WithKubeconfig("/home/user/.kube/config")
		assert.Equal(t, "/home/user/.kube/config", cb.kubeconfig)
		// Path traversal is rejected.
		cb.WithKubeconfig("../../etc/passwd")
		assert.Equal(t, "/home/user/.kube/config", cb.kubeconfig)
	})

	t.Run("WithLabel", func(t *testing.T) {
		cb := NewCommandBuilder("kubectl").WithLabel("app", "nginx")
		assert.Equal(t, "nginx", cb.labels["app"])
		// Empty key is invalid and must not be stored.
		cb.WithLabel("", "x")
		assert.NotContains(t, cb.labels, "")
	})

	t.Run("WithAnnotation", func(t *testing.T) {
		cb := NewCommandBuilder("kubectl").WithAnnotation("team", "sre")
		assert.Equal(t, "sre", cb.annotations["team"])
		// Invalid key format is rejected.
		cb.WithAnnotation("bad key!", "v")
		assert.NotContains(t, cb.annotations, "bad key!")
	})
}
