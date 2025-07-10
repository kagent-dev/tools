package cache

import (
	"context"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CacheEntry represents a cached item with TTL
type CacheEntry struct {
	Value       interface{}
	CreatedAt   time.Time
	ExpiresAt   time.Time
	AccessedAt  time.Time
	AccessCount int64
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// Cache is a thread-safe cache with TTL support
type Cache struct {
	mu              sync.RWMutex
	data            map[string]*CacheEntry
	defaultTTL      time.Duration
	maxSize         int
	cleanupInterval time.Duration
	stopCleanup     chan struct{}

	// Metrics
	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	size      metric.Int64UpDownCounter
}

// NewCache creates a new cache with specified configuration
func NewCache(defaultTTL time.Duration, maxSize int, cleanupInterval time.Duration) *Cache {
	meter := otel.Meter("kagent-tools/cache")

	hits, _ := meter.Int64Counter(
		"cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
	)

	misses, _ := meter.Int64Counter(
		"cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
	)

	evictions, _ := meter.Int64Counter(
		"cache_evictions_total",
		metric.WithDescription("Total number of cache evictions"),
	)

	size, _ := meter.Int64UpDownCounter(
		"cache_size",
		metric.WithDescription("Current number of items in cache"),
	)

	c := &Cache{
		data:            make(map[string]*CacheEntry),
		defaultTTL:      defaultTTL,
		maxSize:         maxSize,
		cleanupInterval: cleanupInterval,
		stopCleanup:     make(chan struct{}),
		hits:            hits,
		misses:          misses,
		evictions:       evictions,
		size:            size,
	}

	// Start background cleanup goroutine
	go c.cleanupExpired()

	return c
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		c.recordMiss(key)
		return nil, false
	}

	if entry.IsExpired() {
		c.recordMiss(key)
		// Don't delete here to avoid potential race conditions
		// Let the cleanup goroutine handle it
		return nil, false
	}

	// Update access statistics
	entry.AccessedAt = time.Now()
	entry.AccessCount++

	c.recordHit(key)
	return entry.Value, true
}

// Set stores a value in the cache with default TTL
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value in the cache with specified TTL
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if we need to evict items to make room
	if len(c.data) >= c.maxSize {
		c.evictLRU()
	}

	entry := &CacheEntry{
		Value:       value,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
		AccessedAt:  now,
		AccessCount: 1,
	}

	// Check if key already exists
	if _, exists := c.data[key]; !exists {
		c.size.Add(context.Background(), 1)
	}

	c.data[key] = entry

	logger.Get().Debug("Cache set", "key", key, "ttl", ttl)
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.data[key]; exists {
		delete(c.data, key)
		c.size.Add(context.Background(), -1)
		logger.Get().Debug("Cache delete", "key", key)
	}
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := len(c.data)
	c.data = make(map[string]*CacheEntry)
	c.size.Add(context.Background(), -int64(count))

	logger.Get().Info("Cache cleared", "items_removed", count)
}

// Size returns the current number of items in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Size:    len(c.data),
		MaxSize: c.maxSize,
		Expired: 0,
		Oldest:  time.Now(),
		Newest:  time.Time{},
	}

	for _, entry := range c.data {
		if entry.IsExpired() {
			stats.Expired++
		}

		if entry.CreatedAt.Before(stats.Oldest) {
			stats.Oldest = entry.CreatedAt
		}

		if entry.CreatedAt.After(stats.Newest) {
			stats.Newest = entry.CreatedAt
		}
	}

	return stats
}

// CacheStats represents cache statistics
type CacheStats struct {
	Size    int       `json:"size"`
	MaxSize int       `json:"max_size"`
	Expired int       `json:"expired"`
	Oldest  time.Time `json:"oldest"`
	Newest  time.Time `json:"newest"`
}

// cleanupExpired removes expired entries from the cache
func (c *Cache) cleanupExpired() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performCleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// performCleanup removes expired entries
func (c *Cache) performCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	keysToDelete := make([]string, 0)

	for key, entry := range c.data {
		if entry.IsExpired() {
			keysToDelete = append(keysToDelete, key)
		}
	}

	if len(keysToDelete) > 0 {
		for _, key := range keysToDelete {
			delete(c.data, key)
			c.evictions.Add(context.Background(), 1)
		}

		c.size.Add(context.Background(), -int64(len(keysToDelete)))
		logger.Get().Debug("Cache cleanup", "expired_items", len(keysToDelete))
	}
}

// evictLRU removes the least recently used item
func (c *Cache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time = time.Now()

	for key, entry := range c.data {
		if entry.AccessedAt.Before(oldestTime) {
			oldestTime = entry.AccessedAt
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(c.data, oldestKey)
		c.evictions.Add(context.Background(), 1)
		c.size.Add(context.Background(), -1)
		logger.Get().Debug("Cache LRU eviction", "key", oldestKey)
	}
}

// recordHit records a cache hit
func (c *Cache) recordHit(key string) {
	c.hits.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.result", "hit"),
	))
}

// recordMiss records a cache miss
func (c *Cache) recordMiss(key string) {
	c.misses.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.result", "miss"),
	))
}

// Close stops the cache cleanup goroutine
func (c *Cache) Close() {
	close(c.stopCleanup)
}

// Global cache instances for different use cases
var (
	// KubernetesCache for caching Kubernetes API responses
	KubernetesCache *Cache

	// PrometheusCache for caching Prometheus query results
	PrometheusCache *Cache

	// CommandCache for caching command execution results
	CommandCache *Cache

	// HelmCache for caching Helm repository and release information
	HelmCache *Cache

	// IstioCache for caching Istio configuration and status
	IstioCache *Cache

	// MetadataCache for caching metadata like namespaces, labels, etc.
	MetadataCache *Cache

	once sync.Once
)

// InitCaches initializes all global cache instances
func InitCaches() {
	once.Do(func() {
		// Initialize caches with different TTL and size based on use case
		KubernetesCache = NewCache(5*time.Minute, 1000, 1*time.Minute)
		PrometheusCache = NewCache(2*time.Minute, 500, 30*time.Second)
		CommandCache = NewCache(10*time.Minute, 200, 1*time.Minute)
		HelmCache = NewCache(15*time.Minute, 300, 2*time.Minute)
		IstioCache = NewCache(5*time.Minute, 500, 1*time.Minute)
		MetadataCache = NewCache(30*time.Minute, 100, 5*time.Minute)

		logger.Get().Info("Caches initialized")
	})
}

// GetKubernetesCache returns the Kubernetes cache instance
func GetKubernetesCache() *Cache {
	InitCaches()
	return KubernetesCache
}

// GetPrometheusCache returns the Prometheus cache instance
func GetPrometheusCache() *Cache {
	InitCaches()
	return PrometheusCache
}

// GetCommandCache returns the command cache instance
func GetCommandCache() *Cache {
	InitCaches()
	return CommandCache
}

// GetHelmCache returns the Helm cache instance
func GetHelmCache() *Cache {
	InitCaches()
	return HelmCache
}

// GetIstioCache returns the Istio cache instance
func GetIstioCache() *Cache {
	InitCaches()
	return IstioCache
}

// GetMetadataCache returns the metadata cache instance
func GetMetadataCache() *Cache {
	InitCaches()
	return MetadataCache
}

// CacheKey generates a consistent cache key from components
func CacheKey(components ...string) string {
	result := ""
	for i, component := range components {
		if i > 0 {
			result += ":"
		}
		result += component
	}
	return result
}

// CacheResult is a helper function to cache the result of a function
func CacheResult[T any](cache *Cache, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	var zero T

	// Try to get from cache first
	if cachedResult, found := cache.Get(key); found {
		if result, ok := cachedResult.(T); ok {
			return result, nil
		}
	}

	// Not in cache, execute function
	result, err := fn()
	if err != nil {
		return zero, err
	}

	// Store in cache
	cache.SetWithTTL(key, result, ttl)

	return result, nil
}
