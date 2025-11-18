package argo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/security"
)

// ArgoCDClient handles HTTP API calls to ArgoCD server
type ArgoCDClient struct {
	baseURL  string
	apiToken string
	client   *http.Client
}

// NewArgoCDClient creates a new ArgoCD client with the given base URL and API token
func NewArgoCDClient(baseURL, apiToken string) (*ArgoCDClient, error) {
	if err := security.ValidateURL(baseURL); err != nil {
		return nil, fmt.Errorf("invalid ArgoCD base URL: %w", err)
	}

	// Remove trailing slash if present
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &ArgoCDClient{
		baseURL:  baseURL,
		apiToken: apiToken,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// GetArgoCDClientFromEnv creates an ArgoCD client from environment variables
func GetArgoCDClientFromEnv() (*ArgoCDClient, error) {
	baseURL := strings.TrimSpace(getEnvOrDefault("ARGOCD_BASE_URL", ""))
	apiToken := strings.TrimSpace(getEnvOrDefault("ARGOCD_API_TOKEN", ""))

	if baseURL == "" {
		return nil, fmt.Errorf("ARGOCD_BASE_URL environment variable is required")
	}
	if apiToken == "" {
		return nil, fmt.Errorf("ARGOCD_API_TOKEN environment variable is required")
	}

	return NewArgoCDClient(baseURL, apiToken)
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	return val
}

// makeRequest performs an HTTP request to the ArgoCD API
func (c *ArgoCDClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/api/v1/%s", c.baseURL, strings.TrimPrefix(endpoint, "/"))
	reqURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	}

	logger.Get().Info("Making ArgoCD API request", "method", method, "url", reqURL.String())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ArgoCD API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ListApplicationsOptions represents options for listing applications
type ListApplicationsOptions struct {
	Search string
	Limit  *int
	Offset *int
}

// ListApplications lists ArgoCD applications
func (c *ArgoCDClient) ListApplications(ctx context.Context, opts *ListApplicationsOptions) (interface{}, error) {
	endpoint := "applications"
	if opts != nil {
		params := url.Values{}
		if opts.Search != "" {
			params.Add("search", opts.Search)
		}
		if opts.Limit != nil {
			params.Add("limit", fmt.Sprintf("%d", *opts.Limit))
		}
		if opts.Offset != nil {
			params.Add("offset", fmt.Sprintf("%d", *opts.Offset))
		}
		if len(params) > 0 {
			endpoint += "?" + params.Encode()
		}
	}

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetApplication retrieves an ArgoCD application by name
func (c *ArgoCDClient) GetApplication(ctx context.Context, name string, namespace *string) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s", url.PathEscape(name))
	if namespace != nil && *namespace != "" {
		endpoint += "?appNamespace=" + url.QueryEscape(*namespace)
	}

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetApplicationResourceTree retrieves the resource tree for an application
func (c *ArgoCDClient) GetApplicationResourceTree(ctx context.Context, name string) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/resource-tree", url.PathEscape(name))

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ManagedResourcesFilters represents filters for managed resources
type ManagedResourcesFilters struct {
	Kind         *string
	Namespace    *string
	Name         *string
	Version      *string
	Group        *string
	AppNamespace *string
	Project      *string
}

// GetApplicationManagedResources retrieves managed resources for an application
func (c *ArgoCDClient) GetApplicationManagedResources(ctx context.Context, name string, filters *ManagedResourcesFilters) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/managed-resources", url.PathEscape(name))

	if filters != nil {
		params := url.Values{}
		if filters.Kind != nil {
			params.Add("kind", *filters.Kind)
		}
		if filters.Namespace != nil {
			params.Add("namespace", *filters.Namespace)
		}
		if filters.Name != nil {
			params.Add("name", *filters.Name)
		}
		if filters.Version != nil {
			params.Add("version", *filters.Version)
		}
		if filters.Group != nil {
			params.Add("group", *filters.Group)
		}
		if filters.AppNamespace != nil {
			params.Add("appNamespace", *filters.AppNamespace)
		}
		if filters.Project != nil {
			params.Add("project", *filters.Project)
		}
		if len(params) > 0 {
			endpoint += "?" + params.Encode()
		}
	}

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ResourceRef represents a resource reference
type ResourceRef struct {
	UID       string `json:"uid"`
	Version   string `json:"version"`
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// GetWorkloadLogs retrieves logs for a workload resource
func (c *ArgoCDClient) GetWorkloadLogs(ctx context.Context, appName string, appNamespace string, resourceRef ResourceRef, container string) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/logs", url.PathEscape(appName))

	params := url.Values{}
	params.Add("appNamespace", appNamespace)
	params.Add("namespace", resourceRef.Namespace)
	params.Add("resourceName", resourceRef.Name)
	params.Add("resourceKind", resourceRef.Kind)
	params.Add("container", container)
	if resourceRef.Group != "" {
		params.Add("group", resourceRef.Group)
	}
	if resourceRef.Version != "" {
		params.Add("version", resourceRef.Version)
	}
	if resourceRef.UID != "" {
		params.Add("uid", resourceRef.UID)
	}

	endpoint += "?" + params.Encode()

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetApplicationEvents retrieves events for an application
func (c *ArgoCDClient) GetApplicationEvents(ctx context.Context, name string) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/events", url.PathEscape(name))

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetResourceEvents retrieves events for a specific resource
func (c *ArgoCDClient) GetResourceEvents(ctx context.Context, appName string, appNamespace string, resourceUID string, resourceNamespace string, resourceName string) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/resource-events", url.PathEscape(appName))

	params := url.Values{}
	params.Add("appNamespace", appNamespace)
	params.Add("uid", resourceUID)
	params.Add("resourceNamespace", resourceNamespace)
	params.Add("resourceName", resourceName)

	endpoint += "?" + params.Encode()

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetResource retrieves a resource manifest
func (c *ArgoCDClient) GetResource(ctx context.Context, appName string, appNamespace string, resourceRef ResourceRef) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/resource", url.PathEscape(appName))

	params := url.Values{}
	params.Add("appNamespace", appNamespace)
	params.Add("namespace", resourceRef.Namespace)
	params.Add("resourceName", resourceRef.Name)
	params.Add("resourceKind", resourceRef.Kind)
	if resourceRef.Group != "" {
		params.Add("group", resourceRef.Group)
	}
	if resourceRef.Version != "" {
		params.Add("version", resourceRef.Version)
	}
	if resourceRef.UID != "" {
		params.Add("uid", resourceRef.UID)
	}

	endpoint += "?" + params.Encode()

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// GetResourceActions retrieves available actions for a resource
func (c *ArgoCDClient) GetResourceActions(ctx context.Context, appName string, appNamespace string, resourceRef ResourceRef) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/resource/actions", url.PathEscape(appName))

	params := url.Values{}
	params.Add("appNamespace", appNamespace)
	params.Add("namespace", resourceRef.Namespace)
	params.Add("resourceName", resourceRef.Name)
	params.Add("resourceKind", resourceRef.Kind)
	if resourceRef.Group != "" {
		params.Add("group", resourceRef.Group)
	}
	if resourceRef.Version != "" {
		params.Add("version", resourceRef.Version)
	}
	if resourceRef.UID != "" {
		params.Add("uid", resourceRef.UID)
	}

	endpoint += "?" + params.Encode()

	body, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// CreateApplication creates a new ArgoCD application
func (c *ArgoCDClient) CreateApplication(ctx context.Context, application interface{}) (interface{}, error) {
	endpoint := "applications"

	body, err := c.makeRequest(ctx, "POST", endpoint, application)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// UpdateApplication updates an existing ArgoCD application
func (c *ArgoCDClient) UpdateApplication(ctx context.Context, name string, application interface{}) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s", url.PathEscape(name))

	body, err := c.makeRequest(ctx, "PUT", endpoint, application)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// DeleteApplicationOptions represents options for deleting an application
type DeleteApplicationOptions struct {
	AppNamespace      *string
	Cascade           *bool
	PropagationPolicy *string
}

// DeleteApplication deletes an ArgoCD application
func (c *ArgoCDClient) DeleteApplication(ctx context.Context, name string, options *DeleteApplicationOptions) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s", url.PathEscape(name))

	if options != nil {
		params := url.Values{}
		if options.AppNamespace != nil {
			params.Add("appNamespace", *options.AppNamespace)
		}
		if options.Cascade != nil {
			params.Add("cascade", fmt.Sprintf("%t", *options.Cascade))
		}
		if options.PropagationPolicy != nil {
			params.Add("propagationPolicy", *options.PropagationPolicy)
		}
		if len(params) > 0 {
			endpoint += "?" + params.Encode()
		}
	}

	body, err := c.makeRequest(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Handle empty response body
	if len(body) == 0 || string(body) == "{}" {
		return map[string]interface{}{}, nil
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// SyncApplicationOptions represents options for syncing an application
type SyncApplicationOptions struct {
	AppNamespace *string
	DryRun       *bool
	Prune        *bool
	Revision     *string
	SyncOptions  []string
}

// SyncApplication syncs an ArgoCD application
func (c *ArgoCDClient) SyncApplication(ctx context.Context, name string, options *SyncApplicationOptions) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/sync", url.PathEscape(name))

	params := url.Values{}
	if options != nil {
		if options.AppNamespace != nil {
			params.Add("appNamespace", *options.AppNamespace)
		}
		if options.DryRun != nil {
			params.Add("dryRun", fmt.Sprintf("%t", *options.DryRun))
		}
		if options.Prune != nil {
			params.Add("prune", fmt.Sprintf("%t", *options.Prune))
		}
		if options.Revision != nil {
			params.Add("revision", *options.Revision)
		}
		if len(options.SyncOptions) > 0 {
			for _, opt := range options.SyncOptions {
				params.Add("syncOptions", opt)
			}
		}
	}

	var syncBody interface{}
	if len(params) > 0 {
		syncBody = map[string]interface{}{}
		for key, values := range params {
			if len(values) > 0 {
				if key == "syncOptions" {
					syncBody.(map[string]interface{})[key] = values
				} else {
					syncBody.(map[string]interface{})[key] = values[0]
				}
			}
		}
	}

	body, err := c.makeRequest(ctx, "POST", endpoint, syncBody)
	if err != nil {
		return nil, err
	}

	// Handle empty response body
	if len(body) == 0 || string(body) == "{}" {
		return map[string]interface{}{}, nil
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// RunResourceAction runs an action on a resource
func (c *ArgoCDClient) RunResourceAction(ctx context.Context, appName string, appNamespace string, resourceRef ResourceRef, action string) (interface{}, error) {
	endpoint := fmt.Sprintf("applications/%s/resource/actions", url.PathEscape(appName))

	params := url.Values{}
	params.Add("appNamespace", appNamespace)
	params.Add("namespace", resourceRef.Namespace)
	params.Add("resourceName", resourceRef.Name)
	params.Add("resourceKind", resourceRef.Kind)
	params.Add("action", action)
	if resourceRef.Group != "" {
		params.Add("group", resourceRef.Group)
	}
	if resourceRef.Version != "" {
		params.Add("version", resourceRef.Version)
	}
	if resourceRef.UID != "" {
		params.Add("uid", resourceRef.UID)
	}

	endpoint += "?" + params.Encode()

	body, err := c.makeRequest(ctx, "POST", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}
