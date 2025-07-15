package security

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s", e.Field, e.Message)
}

// Common validation patterns
var (
	// K8s resource name pattern (RFC 1123)
	k8sNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// Namespace pattern
	namespacePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// Container image pattern
	imagePattern = regexp.MustCompile(`^[a-z0-9]+(([._-][a-z0-9]+)*(/[a-z0-9]+(([._-][a-z0-9]+)*)?)*)?(:([a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?))$`)

	// Path pattern (no directory traversal)
	pathPattern = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)

	// Command injection patterns to reject
	commandInjectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`[;&|` + "`" + `$(){}[\]\\<>*?~!#\n\r\t]`),
		regexp.MustCompile(`\.\./`),
		regexp.MustCompile(`\$\{`),
		regexp.MustCompile(`\$\(`),
		regexp.MustCompile(`\|\|`),
		regexp.MustCompile(`&&`),
	}
)

// ValidateK8sResourceName validates a Kubernetes resource name
func ValidateK8sResourceName(name string) error {
	if name == "" {
		return ValidationError{Field: "name", Message: "cannot be empty"}
	}

	if len(name) > 63 {
		return ValidationError{Field: "name", Message: "cannot exceed 63 characters"}
	}

	if !k8sNamePattern.MatchString(name) {
		return ValidationError{Field: "name", Message: "must follow RFC 1123 naming convention"}
	}

	return nil
}

// ValidateNamespace validates a Kubernetes namespace
func ValidateNamespace(namespace string) error {
	if namespace == "" {
		return nil // Empty namespace is allowed (defaults to 'default')
	}

	if len(namespace) > 63 {
		return ValidationError{Field: "namespace", Message: "cannot exceed 63 characters"}
	}

	if !namespacePattern.MatchString(namespace) {
		return ValidationError{Field: "namespace", Message: "must follow RFC 1123 naming convention"}
	}

	// Reserved namespaces
	reserved := []string{"kube-system", "kube-public", "kube-node-lease"}
	for _, res := range reserved {
		if namespace == res {
			return ValidationError{Field: "namespace", Message: fmt.Sprintf("'%s' is a reserved namespace", namespace)}
		}
	}

	return nil
}

// ValidateContainerImage validates a container image reference
func ValidateContainerImage(image string) error {
	if image == "" {
		return ValidationError{Field: "image", Message: "cannot be empty"}
	}

	if len(image) > 255 {
		return ValidationError{Field: "image", Message: "cannot exceed 255 characters"}
	}

	if !imagePattern.MatchString(image) {
		return ValidationError{Field: "image", Message: "invalid image format"}
	}

	return nil
}

// ValidateFilePath validates a file path for security
func ValidateFilePath(path string) error {
	if path == "" {
		return ValidationError{Field: "path", Message: "cannot be empty"}
	}

	if len(path) > 4096 {
		return ValidationError{Field: "path", Message: "path too long"}
	}

	if strings.Contains(path, "..") {
		return ValidationError{Field: "path", Message: "path traversal not allowed"}
	}

	if !pathPattern.MatchString(path) {
		return ValidationError{Field: "path", Message: "contains invalid characters"}
	}

	return nil
}

// ValidateCommandInput validates command inputs for injection attacks
func ValidateCommandInput(input string) error {
	if input == "" {
		return ValidationError{Field: "input", Message: "cannot be empty"}
	}

	if len(input) > 1024 {
		return ValidationError{Field: "input", Message: "input too long"}
	}

	for _, pattern := range commandInjectionPatterns {
		if pattern.MatchString(input) {
			return ValidationError{Field: "input", Message: "potentially dangerous characters detected"}
		}
	}

	return nil
}

// SanitizeInput sanitizes input strings by replacing potentially dangerous characters
func SanitizeInput(input string) string {
	// Replace dangerous characters with safe alternatives
	sanitized := strings.ReplaceAll(input, "\n", " ")
	sanitized = strings.ReplaceAll(sanitized, "\r", " ")
	sanitized = strings.ReplaceAll(sanitized, "\t", " ")

	// Replace multiple spaces with single space
	spacePattern := regexp.MustCompile(`\s+`)
	sanitized = spacePattern.ReplaceAllString(sanitized, " ")

	sanitized = strings.TrimSpace(sanitized)

	return sanitized
}

// ValidateK8sLabel validates a Kubernetes label key and value
func ValidateK8sLabel(key, value string) error {
	if key == "" {
		return ValidationError{Field: "label_key", Message: "cannot be empty"}
	}

	if len(key) > 63 {
		return ValidationError{Field: "label_key", Message: "cannot exceed 63 characters"}
	}

	if len(value) > 63 {
		return ValidationError{Field: "label_value", Message: "cannot exceed 63 characters"}
	}

	// Label key validation
	labelKeyPattern := regexp.MustCompile(`^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$`)
	if !labelKeyPattern.MatchString(key) {
		return ValidationError{Field: "label_key", Message: "invalid label key format"}
	}

	// Label value validation (can be empty)
	if value != "" {
		labelValuePattern := regexp.MustCompile(`^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$`)
		if !labelValuePattern.MatchString(value) {
			return ValidationError{Field: "label_value", Message: "invalid label value format"}
		}
	}

	return nil
}

// ValidatePromQLQuery validates a PromQL query for basic security
func ValidatePromQLQuery(query string) error {
	if query == "" {
		return ValidationError{Field: "query", Message: "cannot be empty"}
	}

	if len(query) > 8192 {
		return ValidationError{Field: "query", Message: "query too long"}
	}

	// Basic PromQL validation - no shell commands
	dangerousPatterns := []string{
		"`", "$", "$(", "${", "&&", "||", ";", "|", ">", "<", "&",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(query, pattern) {
			return ValidationError{Field: "query", Message: "potentially dangerous characters in query"}
		}
	}

	return nil
}

// ValidateYAMLContent validates YAML content for basic security
func ValidateYAMLContent(content string) error {
	if content == "" {
		return ValidationError{Field: "content", Message: "cannot be empty"}
	}

	if len(content) > 1024*1024 { // 1MB limit
		return ValidationError{Field: "content", Message: "content too large"}
	}

	// Check for potentially dangerous YAML content
	dangerousPatterns := []string{
		"!!python/object/apply",
		"!!python/object/new",
		"!!python/object",
		"__import__",
		"eval(",
		"exec(",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(content, pattern) {
			return ValidationError{Field: "content", Message: "potentially dangerous YAML content detected"}
		}
	}

	return nil
}

// ValidateHelmReleaseName validates a Helm release name
func ValidateHelmReleaseName(name string) error {
	if name == "" {
		return ValidationError{Field: "release_name", Message: "cannot be empty"}
	}

	if len(name) > 53 {
		return ValidationError{Field: "release_name", Message: "cannot exceed 53 characters"}
	}

	// Helm release name pattern
	helmNamePattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !helmNamePattern.MatchString(name) {
		return ValidationError{Field: "release_name", Message: "must follow DNS naming convention"}
	}

	return nil
}

// ValidateURL validates a URL for basic security
func ValidateURL(url string) error {
	if url == "" {
		return ValidationError{Field: "url", Message: "cannot be empty"}
	}

	if len(url) > 2048 {
		return ValidationError{Field: "url", Message: "URL too long"}
	}

	// Basic URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ValidationError{Field: "url", Message: "must start with http:// or https://"}
	}

	// Check for dangerous URL patterns
	dangerousPatterns := []string{
		"javascript:", "data:", "file:", "ftp:",
		"<script", "</script", "javascript",
	}

	lowerURL := strings.ToLower(url)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerURL, pattern) {
			return ValidationError{Field: "url", Message: "potentially dangerous URL detected"}
		}
	}

	return nil
}
