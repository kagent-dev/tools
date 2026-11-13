package redact

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const secret = "s3cr3t-value"

func TestArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"token", []string{"get", "pods", "--token", secret}, []string{"get", "pods", "--token", Placeholder}},
		{"token inline", []string{"get", "--token=" + secret}, []string{"get", "--token=" + Placeholder}},
		{"kube-token", []string{"list", "--kube-token", secret}, []string{"list", "--kube-token", Placeholder}},
		{"kube-token inline", []string{"list", "--kube-token=" + secret}, []string{"list", "--kube-token=" + Placeholder}},
		{"password", []string{"login", "--password", secret, "-n", "x"}, []string{"login", "--password", Placeholder, "-n", "x"}},
		{"password inline", []string{"login", "--password=" + secret}, []string{"login", "--password=" + Placeholder}},
		{"client-key", []string{"get", "--client-key", secret}, []string{"get", "--client-key", Placeholder}},
		{"client-key inline", []string{"get", "--client-key=" + secret}, []string{"get", "--client-key=" + Placeholder}},
		{"token assignment", []string{"--set", "auth.token=" + secret}, []string{"--set", "auth.token=" + Placeholder}},
		{"password assignment upper case", []string{"--set", "DB_PASSWORD=" + secret}, []string{"--set", "DB_PASSWORD=" + Placeholder}},
		{"secret assignment in list", []string{"--set", "a=1,aws.secret=" + secret + ",b=2"}, []string{"--set", "a=1,aws.secret=" + Placeholder}},
		{"flag at end without value", []string{"get", "--token"}, []string{"get", "--token"}},
		{"unrelated args unchanged", []string{"get", "pods", "-n", "default", "app=web"}, []string{"get", "pods", "-n", "default", "app=web"}},
		{"value after flag is not inspected again", []string{"--token", "--password", "x"}, []string{"--token", Placeholder, "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]string(nil), tt.in...)
			got := Args(tt.in)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, in, tt.in, "input must not be modified")
			for _, a := range got {
				assert.NotContains(t, a, secret)
			}
		})
	}
	assert.Nil(t, Args(nil))
	assert.Nil(t, Args([]string{}))
}

func TestString(t *testing.T) {
	inputs := []string{
		"get pods --token " + secret + " -n default",
		"get pods --token=" + secret,
		"list --kube-token " + secret,
		"list --kube-token=" + secret,
		"login --password " + secret,
		"login --password=" + secret,
		"get --client-key " + secret,
		"get --client-key=" + secret,
		"install x --set auth.Token=" + secret,
		"PASSWORD=" + secret,
		"aws_secret=" + secret,
		"Authorization: Bearer " + secret,
		"[Kubernetes] get pods --token\t" + secret + " failed: exit status 1",
	}
	for _, in := range inputs {
		t.Run(in[:min(len(in), 30)], func(t *testing.T) {
			got := String(in)
			assert.NotContains(t, got, secret)
			assert.Contains(t, got, Placeholder)
		})
	}

	t.Run("text without credentials unchanged", func(t *testing.T) {
		in := "[Kubernetes] get pods -n default failed: exit status 1"
		assert.Equal(t, in, String(in))
	})
	t.Run("idempotent", func(t *testing.T) {
		in := "get --token=" + secret
		assert.Equal(t, String(in), String(String(in)))
	})
}

func TestError(t *testing.T) {
	assert.NoError(t, Error(nil))
	base := errors.New("kubectl get --token " + secret + " failed")
	err := Error(base)
	assert.False(t, strings.Contains(err.Error(), secret))
	assert.ErrorIs(t, err, base)
}
