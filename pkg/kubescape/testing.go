package kubescape

import (
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
		initError:    nil,
	}
}

// NewKubescapeToolWithError creates a KubescapeTool with an initialization error for testing error paths
func NewKubescapeToolWithError(err error) *KubescapeTool {
	return &KubescapeTool{
		initError: err,
	}
}
