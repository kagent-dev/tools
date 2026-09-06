package kubescape

import (
	"time"

	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
)

// NewKubescapeToolWithClients creates a KubescapeTool with pre-configured clients for testing
func NewKubescapeToolWithClients(
	k8sClient kubernetes.Interface,
	apiExtClient apiextensionsclientset.Interface,
	spdxClient spdxv1beta1.SpdxV1beta1Interface,
) *KubescapeTool {
	return &KubescapeTool{
		k8sClient:    k8sClient,
		apiExtClient: apiExtClient,
		spdxClient:   spdxClient,
		now:          time.Now,
	}
}

// NewKubescapeToolWithError creates a KubescapeTool with an initialization error for testing error paths.
//
// The failure is permanent: the injected builder always returns err, so the
// tool never falls back to whatever kubeconfig the test machine happens to
// have once the retry interval elapses.
func NewKubescapeToolWithError(err error) *KubescapeTool {
	return &KubescapeTool{
		buildClients: func() (*kubescapeClients, error) { return nil, err },
		now:          time.Now,
	}
}
