package kubescape

import (
	"context"
	"errors"
	"testing"
	"time"

	kubescapefake "github.com/kubescape/storage/pkg/generated/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// workingClients returns a client set backed by fakes, standing in for a
// successful construction against a reachable API server.
func workingClients() *kubescapeClients {
	return &kubescapeClients{
		k8s:  kubefake.NewClientset(),
		spdx: kubescapefake.NewClientset().SpdxV1beta1(),
	}
}

// toolWithBuilder builds a tool whose client construction and clock are both
// under the test's control, so recovery and rate limiting can be exercised
// without a cluster and without sleeping.
func toolWithBuilder(build func() (*kubescapeClients, error), clock *time.Time) *KubescapeTool {
	return &KubescapeTool{
		buildClients: build,
		now:          func() time.Time { return *clock },
	}
}

// A client construction that fails once -- because the API server was not yet
// reachable when the pod started -- must not disable the provider for the
// lifetime of the process. This is P5: previously initError was set once and
// every handler short-circuited on it forever.
func TestEnsureClients_RecoversAfterTransientFailure(t *testing.T) {
	attempts := 0
	clock := time.Now()
	tool := toolWithBuilder(func() (*kubescapeClients, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("dial tcp 10.96.0.1:443: connect: connection refused")
		}
		return workingClients(), nil
	}, &clock)

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError, "first call should report the construction failure")

	clock = clock.Add(clientRetryInterval + time.Second)

	result, err = tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.False(t, result.IsError, "provider should recover once the API server is reachable")
	assert.Equal(t, 2, attempts)
}

// Once clients are built they are reused; a healthy provider must not rebuild
// its clients on every call.
func TestEnsureClients_BuildsOnce(t *testing.T) {
	attempts := 0
	clock := time.Now()
	tool := toolWithBuilder(func() (*kubescapeClients, error) {
		attempts++
		return workingClients(), nil
	}, &clock)

	for i := 0; i < 3; i++ {
		_, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
		require.NoError(t, err)
	}

	assert.Equal(t, 1, attempts)
}

// A failing construction is cached for clientRetryInterval, so a hot loop of
// tool calls during an outage cannot hammer the API server -- each attempt can
// block for a full dial timeout.
func TestEnsureClients_DoesNotRetryWithinInterval(t *testing.T) {
	attempts := 0
	clock := time.Now()
	tool := toolWithBuilder(func() (*kubescapeClients, error) {
		attempts++
		return nil, errors.New("connection refused")
	}, &clock)

	_, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	clock = clock.Add(clientRetryInterval / 2)

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError, "the cached failure is still reported")
	assert.Equal(t, 1, attempts, "no second construction attempt within the interval")
}

// P7: a call missing a required argument must report the missing argument, not
// a kubeconfig error. Argument validation needs no cluster, so it runs first --
// asserted by the builder never being called.
func TestHandlers_ValidateArgumentsBeforeBuildingClients(t *testing.T) {
	tests := []struct {
		name    string
		handler func(*KubescapeTool, context.Context) (string, error)
		want    string
	}{
		{
			name: "list_vulnerabilities without manifest_name",
			handler: func(k *KubescapeTool, ctx context.Context) (string, error) {
				r, err := k.HandleListVulnerabilitiesInManifest(ctx, makeRequest(nil))
				return getResultText(r), err
			},
			want: "manifest_name",
		},
		{
			name: "get_vulnerability_details without cve_id",
			handler: func(k *KubescapeTool, ctx context.Context) (string, error) {
				r, err := k.HandleGetVulnerabilityDetails(ctx, makeRequest(map[string]interface{}{
					"manifest_name": "some-manifest",
				}))
				return getResultText(r), err
			},
			want: "cve_id",
		},
		{
			name: "get_configuration_scan without manifest_name",
			handler: func(k *KubescapeTool, ctx context.Context) (string, error) {
				r, err := k.HandleGetConfigurationScan(ctx, makeRequest(nil))
				return getResultText(r), err
			},
			want: "manifest_name",
		},
		{
			name: "get_application_profile without name",
			handler: func(k *KubescapeTool, ctx context.Context) (string, error) {
				r, err := k.HandleGetApplicationProfile(ctx, makeRequest(map[string]interface{}{
					"namespace": "default",
				}))
				return getResultText(r), err
			},
			want: "name",
		},
		{
			name: "get_network_neighborhood without namespace",
			handler: func(k *KubescapeTool, ctx context.Context) (string, error) {
				r, err := k.HandleGetNetworkNeighborhood(ctx, makeRequest(map[string]interface{}{
					"name": "some-nn",
				}))
				return getResultText(r), err
			},
			want: "namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			clock := time.Now()
			tool := toolWithBuilder(func() (*kubescapeClients, error) {
				attempts++
				return nil, errors.New("failed to create kubernetes config: no configuration has been provided")
			}, &clock)

			text, err := tt.handler(tool, context.Background())
			require.NoError(t, err)

			assert.Contains(t, text, tt.want)
			assert.NotContains(t, text, "no configuration has been provided")
			assert.Equal(t, 0, attempts, "argument validation must not require a cluster")
		})
	}
}

// The exported test helpers keep behaving as the existing suite expects:
// pre-built clients are used as-is, and a tool built with an error stays in
// that error state rather than falling back to an ambient kubeconfig.
func TestNewKubescapeToolWithError_StaysFailed(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.True(t, result.IsError)

	tool.lastAttempt = time.Time{} // any retry window has long expired

	result, err = tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	assert.True(t, result.IsError, "must not fall through to an ambient kubeconfig")
}
