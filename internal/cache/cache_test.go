package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCache(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)

	if cache.defaultTTL != 1*time.Minute {
		t.Errorf("Expected default TTL of 1 minute, got %v", cache.defaultTTL)
	}

	if cache.maxSize != 100 {
		t.Errorf("Expected max size of 100, got %d", cache.maxSize)
	}

	if cache.cleanupInterval != 10*time.Second {
		t.Errorf("Expected cleanup interval of 10 seconds, got %v", cache.cleanupInterval)
	}

	if cache.name != "test-cache" {
		t.Errorf("Expected cache name 'test-cache', got %s", cache.name)
	}

	cache.Close()
}

func TestCacheName(t *testing.T) {
	cache := NewCache[string]("my-test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	if cache.Name() != "my-test-cache" {
		t.Errorf("Expected cache name 'my-test-cache', got %s", cache.Name())
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	// Test set and get
	cache.Set("key1", "value1")
	value, found := cache.Get("key1")

	if !found {
		t.Error("Expected to find key1")
	}

	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}
}

func TestCacheSetWithTTL(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	// Test set with custom TTL
	cache.SetWithTTL("key1", "value1", 100*time.Millisecond)

	// Should be found immediately
	value, found := cache.Get("key1")
	if !found {
		t.Error("Expected to find key1")
	}
	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not be found after expiration
	_, found = cache.Get("key1")
	if found {
		t.Error("Expected key1 to be expired")
	}
}

func TestCacheDelete(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	cache.Set("key1", "value1")
	cache.Delete("key1")

	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be deleted")
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	if cache.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}
}

func TestCacheEviction(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 2, 10*time.Second) // Small cache
	defer cache.Close()

	// Fill cache to capacity
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Add one more item - should evict LRU
	cache.Set("key3", "value3")

	// key1 should be evicted (oldest)
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be evicted")
	}

	// key2 and key3 should still be there
	_, found = cache.Get("key2")
	if !found {
		t.Error("Expected key2 to be present")
	}

	_, found = cache.Get("key3")
	if !found {
		t.Error("Expected key3 to be present")
	}
}

func TestCacheExpiration(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 50*time.Millisecond) // Fast cleanup
	defer cache.Close()

	// Set item with short TTL
	cache.SetWithTTL("key1", "value1", 100*time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Item should be cleaned up
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be cleaned up")
	}
}

func TestCacheStats(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	stats := cache.Stats()

	if stats.Size != 2 {
		t.Errorf("Expected stats size 2, got %d", stats.Size)
	}

	if stats.MaxSize != 100 {
		t.Errorf("Expected stats max size 100, got %d", stats.MaxSize)
	}

	if stats.Expired != 0 {
		t.Errorf("Expected 0 expired items, got %d", stats.Expired)
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{"single component", []string{"key1"}, "key1"},
		{"multiple components", []string{"key1", "key2", "key3"}, "key1:key2:key3"},
		{"empty components", []string{}, ""},
		{"empty string component", []string{"key1", "", "key3"}, "key1::key3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CacheKey(tt.components...)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCacheResult(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	callCount := 0
	testFunction := func() (string, error) {
		callCount++
		return "result", nil
	}

	// First call should execute function
	result, err := CacheResult(cache, "test-key", 1*time.Minute, testFunction)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "result" {
		t.Errorf("Expected 'result', got %q", result)
	}
	if callCount != 1 {
		t.Errorf("Expected function to be called once, got %d", callCount)
	}

	// Second call should use cache
	result, err = CacheResult(cache, "test-key", 1*time.Minute, testFunction)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "result" {
		t.Errorf("Expected 'result', got %q", result)
	}
	if callCount != 1 {
		t.Errorf("Expected function to be called once (cached), got %d", callCount)
	}
}

func TestCacheResultWithError(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	testFunction := func() (string, error) {
		return "", &testError{message: "test error"}
	}

	result, err := CacheResult(cache, "test-key", 1*time.Minute, testFunction)
	if err == nil {
		t.Error("Expected error")
	}
	if result != "" {
		t.Errorf("Expected empty result, got %q", result)
	}

	// Check that error result is not cached
	_, found := cache.Get("test-key")
	if found {
		t.Error("Expected error result not to be cached")
	}
}

func TestCacheInitialization(t *testing.T) {
	// Test that all cache types are properly initialized
	types := []CacheType{
		CacheTypeKubernetes,
		CacheTypeCommand,
		CacheTypeHelm,
		CacheTypeIstio,
	}

	for _, cacheType := range types {
		t.Run(cacheType.String(), func(t *testing.T) {
			cache := GetCacheByType(cacheType)
			if cache == nil {
				t.Errorf("Expected cache for type %s to be initialized", cacheType.String())
			}
			if cache.Name() != cacheType.String() {
				t.Errorf("Expected cache name %s, got %s", cacheType.String(), cache.Name())
			}
		})
	}
}

func TestCacheEntry(t *testing.T) {
	now := time.Now()
	entry := &CacheEntry[string]{
		Value:       "test",
		CreatedAt:   now,
		ExpiresAt:   now.Add(1 * time.Minute),
		AccessedAt:  now,
		AccessCount: 1,
	}

	// Should not be expired
	if entry.IsExpired() {
		t.Error("Expected entry not to be expired")
	}

	// Make it expired
	entry.ExpiresAt = now.Add(-1 * time.Minute)

	// Should be expired
	if !entry.IsExpired() {
		t.Error("Expected entry to be expired")
	}
}

func TestCachePerformCleanup(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 100, 10*time.Second)
	defer cache.Close()

	// Add expired item
	cache.SetWithTTL("expired", "value", -1*time.Minute)

	// Add valid item
	cache.Set("valid", "value")

	// Perform cleanup
	cache.performCleanup()

	// Expired item should be removed
	_, found := cache.Get("expired")
	if found {
		t.Error("Expected expired item to be removed")
	}

	// Valid item should remain
	_, found = cache.Get("valid")
	if !found {
		t.Error("Expected valid item to remain")
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewCache[string]("test-cache", 1*time.Minute, 1000, 10*time.Second)
	defer cache.Close()

	// Test concurrent operations
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get(fmt.Sprintf("key%d", i))
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Cache should have items
	if cache.Size() == 0 {
		t.Error("Expected cache to have items")
	}
}

// Helper types for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}

func TestCacheTypeString(t *testing.T) {
	tests := []struct {
		cacheType CacheType
		expected  string
	}{
		{CacheTypeKubernetes, "kubernetes"},
		{CacheTypeCommand, "command"},
		{CacheTypeHelm, "helm"},
		{CacheTypeIstio, "istio"},
		{CacheType(999), "unknown"}, // Test unknown type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.cacheType.String()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetCacheByType(t *testing.T) {
	// Test all valid cache types
	types := []CacheType{
		CacheTypeKubernetes,
		CacheTypeCommand,
		CacheTypeHelm,
		CacheTypeIstio,
	}

	for _, cacheType := range types {
		t.Run(cacheType.String(), func(t *testing.T) {
			cache := GetCacheByType(cacheType)
			if cache == nil {
				t.Errorf("Expected cache for type %s, got nil", cacheType.String())
			}
			if cache.Name() != cacheType.String() {
				t.Errorf("Expected cache name %s, got %s", cacheType.String(), cache.Name())
			}
		})
	}
}

func TestGetCacheByCommand(t *testing.T) {
	tests := []struct {
		command      string
		expectedType CacheType
	}{
		{"kubectl", CacheTypeKubernetes},
		{"helm", CacheTypeHelm},
		{"istioctl", CacheTypeIstio},
		{"cilium", CacheTypeCommand},
		{"argo", CacheTypeCommand},
		{"unknown-command", CacheTypeCommand}, // Should default to command cache
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			cache := GetCacheByCommand(tt.command)
			if cache == nil {
				t.Errorf("Expected cache for command %s, got nil", tt.command)
			}
			if cache.Name() != tt.expectedType.String() {
				t.Errorf("Expected cache name %s for command %s, got %s",
					tt.expectedType.String(), tt.command, cache.Name())
			}
		})
	}
}

func TestCacheOTelTracing(t *testing.T) {
	// This test verifies that OTEL tracing calls don't panic
	// The actual tracing verification would require setting up an OTEL test environment
	cache := NewCache[string]("test-tracing", 1*time.Minute, 10, 5*time.Minute)
	defer cache.Close()

	// Test cache miss with tracing
	_, found := cache.Get("missing-key")
	assert.False(t, found)

	// Test cache hit with tracing
	cache.Set("test-key", "test-value")
	value, found := cache.Get("test-key")
	assert.True(t, found)
	assert.Equal(t, "test-value", value)

	// Test CacheResult with tracing
	callCount := 0
	result, err := CacheResult(cache, "result-key", 1*time.Minute, func() (string, error) {
		callCount++
		return "computed-value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "computed-value", result)
	assert.Equal(t, 1, callCount)

	// Test cache hit on second call
	result2, err := CacheResult(cache, "result-key", 1*time.Minute, func() (string, error) {
		callCount++
		return "computed-value", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "computed-value", result2)
	assert.Equal(t, 1, callCount) // Should not increment due to cache hit

	// Test cache invalidation with tracing
	oldSize := cache.Size()
	InvalidateByType(CacheTypeCommand)
	assert.True(t, oldSize > 0) // Verify we had items to clear
}
