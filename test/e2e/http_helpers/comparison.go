package http_helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// ComparisonResult represents the result of comparing two tool outputs
// T037 Implementation: Result comparison helpers
type ComparisonResult struct {
	Match           bool
	MatchPercentage float64
	Differences     []string
	HTTPOutput      interface{}
	StdioOutput     interface{}
	RawHTTPData     string
	RawStdioData    string
}

// OutputComparer provides utilities for comparing HTTP and stdio tool outputs
type OutputComparer struct {
	ignoreFields []string
	normalizers  []Normalizer
}

// Normalizer defines a function to normalize output before comparison
type Normalizer func(interface{}) interface{}

// NewOutputComparer creates a new output comparer
func NewOutputComparer() *OutputComparer {
	return &OutputComparer{
		ignoreFields: []string{},
		normalizers:  []Normalizer{},
	}
}

// IgnoreField adds a field to ignore during comparison
func (oc *OutputComparer) IgnoreField(fieldName string) *OutputComparer {
	oc.ignoreFields = append(oc.ignoreFields, fieldName)
	return oc
}

// AddNormalizer adds a normalizer function to be applied before comparison
func (oc *OutputComparer) AddNormalizer(norm Normalizer) *OutputComparer {
	oc.normalizers = append(oc.normalizers, norm)
	return oc
}

// Compare compares HTTP output with stdio output
// T037 Implementation: Deep comparison with detailed diff reporting
func (oc *OutputComparer) Compare(httpOutput, stdioOutput interface{}) *ComparisonResult {
	result := &ComparisonResult{
		HTTPOutput:      httpOutput,
		StdioOutput:     stdioOutput,
		Differences:     []string{},
		Match:           true,
		MatchPercentage: 100.0,
	}

	// Apply normalizers
	normalizedHTTP := httpOutput
	normalizedStdio := stdioOutput
	for _, norm := range oc.normalizers {
		normalizedHTTP = norm(normalizedHTTP)
		normalizedStdio = norm(normalizedStdio)
	}

	// Store raw JSON representations
	if data, err := json.MarshalIndent(normalizedHTTP, "", "  "); err == nil {
		result.RawHTTPData = string(data)
	}
	if data, err := json.MarshalIndent(normalizedStdio, "", "  "); err == nil {
		result.RawStdioData = string(data)
	}

	// Perform comparison
	oc.compare(normalizedHTTP, normalizedStdio, "", result)

	// Update match status
	result.Match = len(result.Differences) == 0
	if !result.Match {
		result.MatchPercentage = 100.0 - (float64(len(result.Differences)) * 10.0)
		if result.MatchPercentage < 0 {
			result.MatchPercentage = 0
		}
	}

	return result
}

// compare recursively compares two values
func (oc *OutputComparer) compare(http, stdio interface{}, path string, result *ComparisonResult) {
	// Handle nil cases
	if http == nil && stdio == nil {
		return
	}
	if http == nil || stdio == nil {
		result.Differences = append(result.Differences, fmt.Sprintf("at %s: HTTP=%v, stdio=%v", path, http, stdio))
		result.Match = false
		return
	}

	// Compare types
	httpType := reflect.TypeOf(http)
	stdioType := reflect.TypeOf(stdio)

	if httpType != stdioType {
		// Try to handle compatible types (e.g., float64 vs int)
		if !oc.areCompatibleTypes(http, stdio) {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: type mismatch HTTP=%T, stdio=%T", path, http, stdio))
			result.Match = false
			return
		}
	}

	// Compare based on type
	switch httpVal := http.(type) {
	case map[string]interface{}:
		oc.compareMaps(httpVal, stdio.(map[string]interface{}), path, result)
	case []interface{}:
		oc.compareArrays(httpVal, stdio.([]interface{}), path, result)
	case string:
		if httpVal != stdio.(string) {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: HTTP=\"%s\", stdio=\"%s\"", path, httpVal, stdio))
			result.Match = false
		}
	case float64:
		httpFloat := httpVal
		var stdioFloat float64
		switch stdioVal := stdio.(type) {
		case float64:
			stdioFloat = stdioVal
		case json.Number:
			f, _ := stdioVal.Float64()
			stdioFloat = f
		default:
			stdioFloat, _ = stdio.(float64)
		}
		if httpFloat != stdioFloat {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: HTTP=%v, stdio=%v", path, httpFloat, stdioFloat))
			result.Match = false
		}
	case bool:
		if httpVal != stdio.(bool) {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: HTTP=%v, stdio=%v", path, httpVal, stdio))
			result.Match = false
		}
	default:
		// Default string comparison
		if fmt.Sprint(http) != fmt.Sprint(stdio) {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: HTTP=%v, stdio=%v", path, http, stdio))
			result.Match = false
		}
	}
}

// compareMaps compares two map structures
func (oc *OutputComparer) compareMaps(httpMap, stdioMap map[string]interface{}, path string, result *ComparisonResult) {
	allKeys := make(map[string]bool)
	for k := range httpMap {
		allKeys[k] = true
	}
	for k := range stdioMap {
		allKeys[k] = true
	}

	for key := range allKeys {
		// Skip ignored fields
		if oc.shouldIgnore(key) {
			continue
		}

		httpVal, httpOK := httpMap[key]
		stdioVal, stdioOK := stdioMap[key]

		var newPath string
		if path != "" {
			newPath = path + "." + key
		} else {
			newPath = key
		}

		if !httpOK && !stdioOK {
			continue
		} else if !httpOK {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: missing in HTTP", newPath))
			result.Match = false
		} else if !stdioOK {
			result.Differences = append(result.Differences, fmt.Sprintf("at %s: missing in stdio", newPath))
			result.Match = false
		} else {
			oc.compare(httpVal, stdioVal, newPath, result)
		}
	}
}

// compareArrays compares two array structures
func (oc *OutputComparer) compareArrays(httpArr, stdioArr []interface{}, path string, result *ComparisonResult) {
	if len(httpArr) != len(stdioArr) {
		result.Differences = append(result.Differences, fmt.Sprintf("at %s: length mismatch HTTP=%d, stdio=%d", path, len(httpArr), len(stdioArr)))
		result.Match = false
		// Continue comparison with min length to find other differences
	}

	minLen := len(httpArr)
	if len(stdioArr) < minLen {
		minLen = len(stdioArr)
	}

	for i := 0; i < minLen; i++ {
		newPath := fmt.Sprintf("%s[%d]", path, i)
		oc.compare(httpArr[i], stdioArr[i], newPath, result)
	}
}

// shouldIgnore checks if a field should be ignored during comparison
func (oc *OutputComparer) shouldIgnore(field string) bool {
	for _, ignoreField := range oc.ignoreFields {
		if field == ignoreField {
			return true
		}
	}
	return false
}

// areCompatibleTypes checks if two types can be compared
func (oc *OutputComparer) areCompatibleTypes(a, b interface{}) bool {
	// Allow float64/int comparisons
	switch a.(type) {
	case float64:
		switch b.(type) {
		case float64, int, int64, json.Number:
			return true
		}
	case int:
		switch b.(type) {
		case float64, int, int64, json.Number:
			return true
		}
	case string:
		switch b.(type) {
		case string:
			return true
		}
	}
	return false
}

// String returns a formatted string representation of the comparison result
func (cr *ComparisonResult) String() string {
	var buf bytes.Buffer
	buf.WriteString("ComparisonResult {\n")
	buf.WriteString(fmt.Sprintf("  Match: %v\n", cr.Match))
	buf.WriteString(fmt.Sprintf("  MatchPercentage: %.1f%%\n", cr.MatchPercentage))
	buf.WriteString(fmt.Sprintf("  Differences: %d\n", len(cr.Differences)))

	if len(cr.Differences) > 0 {
		buf.WriteString("  Details:\n")
		for i, diff := range cr.Differences {
			buf.WriteString(fmt.Sprintf("    [%d] %s\n", i+1, diff))
		}
	}

	buf.WriteString("}\n")
	return buf.String()
}

// DetailedDiff returns a detailed diff report suitable for debugging
// T037 Implementation: Detailed diff reporting
func (cr *ComparisonResult) DetailedDiff() string {
	if cr.Match {
		return "Outputs match perfectly ✓\n"
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Comparison Failed (%.1f%% match)\n", cr.MatchPercentage))
	buf.WriteString(strings.Repeat("=", 60) + "\n")

	// Show differences
	if len(cr.Differences) > 0 {
		buf.WriteString("Differences Found:\n")
		buf.WriteString(strings.Repeat("-", 60) + "\n")
		for i, diff := range cr.Differences {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, diff))
		}
		buf.WriteString("\n")
	}

	// Show side-by-side if raw data is available
	if cr.RawHTTPData != "" && cr.RawStdioData != "" {
		buf.WriteString("HTTP Output:\n")
		buf.WriteString(strings.Repeat("-", 60) + "\n")
		buf.WriteString(cr.RawHTTPData + "\n\n")

		buf.WriteString("Stdio Output:\n")
		buf.WriteString(strings.Repeat("-", 60) + "\n")
		buf.WriteString(cr.RawStdioData + "\n")
	}

	return buf.String()
}

// CompareJSON compares two JSON byte arrays
func CompareJSON(httpJSON, stdioJSON []byte) *ComparisonResult {
	var httpData interface{}
	var stdioData interface{}

	if err := json.Unmarshal(httpJSON, &httpData); err != nil {
		return &ComparisonResult{
			Match:       false,
			Differences: []string{fmt.Sprintf("failed to parse HTTP JSON: %v", err)},
		}
	}

	if err := json.Unmarshal(stdioJSON, &stdioData); err != nil {
		return &ComparisonResult{
			Match:       false,
			Differences: []string{fmt.Sprintf("failed to parse stdio JSON: %v", err)},
		}
	}

	comparer := NewOutputComparer()
	return comparer.Compare(httpData, stdioData)
}

// NormalizerRemoveTimestamps creates a normalizer that removes timestamp fields
func NormalizerRemoveTimestamps() Normalizer {
	return func(data interface{}) interface{} {
		if mapData, ok := data.(map[string]interface{}); ok {
			result := make(map[string]interface{})
			for k, v := range mapData {
				if k != "timestamp" && k != "updatedAt" && k != "createdAt" {
					result[k] = v
				}
			}
			return result
		}
		return data
	}
}

// NormalizerSortArrays creates a normalizer that sorts arrays for consistent comparison
func NormalizerSortArrays() Normalizer {
	return func(data interface{}) interface{} {
		if arr, ok := data.([]interface{}); ok {
			// Sort by string representation
			sort.Slice(arr, func(i, j int) bool {
				return fmt.Sprint(arr[i]) < fmt.Sprint(arr[j])
			})
		}
		return data
	}
}

// NormalizerLowerCase creates a normalizer that converts string values to lowercase
func NormalizerLowerCase() Normalizer {
	return func(data interface{}) interface{} {
		if str, ok := data.(string); ok {
			return strings.ToLower(str)
		}
		return data
	}
}
