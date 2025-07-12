package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/telemetry"
)

// CacheType represents the type of cache using enum pattern
type CacheType int

const (
	CacheTypeKubernetes CacheType = iota
	CacheTypeCommand
	CacheTypeHelm
	CacheTypeIstio
)

// String returns the string representation of CacheType
func (ct CacheType) String() string {
	switch ct {
	case CacheTypeKubernetes:
		return "kubernetes"
	case CacheTypeCommand:
		return "command"
	case CacheTypeHelm:
		return "helm"
	case CacheTypeIstio:
		return "istio"
	default:
		return "unknown"
	}
}

// Command to cache type mapping
var commandToCacheType = map[string]CacheType{
	"kubectl":  CacheTypeKubernetes,
	"helm":     CacheTypeHelm,
	"istioctl": CacheTypeIstio,
	"cilium":   CacheTypeCommand, // Use command cache for cilium
	"argo":     CacheTypeCommand, // Use command cache for argo
}

// CacheEntry represents a cached item with TTL
type CacheEntry[T any] struct {
	Value       T
	CreatedAt   time.Time
	ExpiresAt   time.Time
	AccessedAt  time.Time
	AccessCount int64
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry[T]) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// Cache is a thread-safe cache with TTL support
type Cache[T any] struct {
	mu              sync.RWMutex
	data            map[string]*CacheEntry[T]
	name            string
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

// NewCache creates a new cache with specified configuration and name
func NewCache[T any](name string, defaultTTL time.Duration, maxSize int, cleanupInterval time.Duration) *Cache[T] {
	meter := otel.Meter(fmt.Sprintf("kagent-tools/cache/%s", name))

	// Create metrics with cache name as a label
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

	cache := &Cache[T]{
		data:            make(map[string]*CacheEntry[T]),
		name:            name,
		defaultTTL:      defaultTTL,
		maxSize:         maxSize,
		cleanupInterval: cleanupInterval,
		stopCleanup:     make(chan struct{}),
		hits:            hits,
		misses:          misses,
		evictions:       evictions,
		size:            size,
	}

	// Start background cleanup
	go cache.cleanupExpired()

	return cache
}

// Get retrieves a value from the cache
func (c *Cache[T]) Get(key string) (T, bool) {
	ctx := context.Background()
	_, span := telemetry.StartSpan(ctx, "cache.get",
		attribute.String("cache.name", c.name),
		attribute.String("cache.key", key),
	)
	defer span.End()

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		var zero T
		c.recordMiss(key)
		telemetry.AddEvent(span, "cache.miss",
			attribute.String("cache.result", "miss"),
		)
		span.SetAttributes(attribute.String("cache.result", "miss"))
		return zero, false
	}

	if entry.IsExpired() {
		var zero T
		c.recordMiss(key)
		telemetry.AddEvent(span, "cache.miss",
			attribute.String("cache.result", "miss"),
			attribute.String("cache.miss_reason", "expired"),
		)
		span.SetAttributes(
			attribute.String("cache.result", "miss"),
			attribute.String("cache.miss_reason", "expired"),
		)
		return zero, false
	}

	// Update access time and count
	entry.AccessedAt = time.Now()
	entry.AccessCount++

	c.recordHit(key)
	telemetry.AddEvent(span, "cache.hit",
		attribute.String("cache.result", "hit"),
		attribute.Int64("cache.access_count", entry.AccessCount),
	)
	span.SetAttributes(
		attribute.String("cache.result", "hit"),
		attribute.Int64("cache.access_count", entry.AccessCount),
	)

	logger.Get().Debug("Cache hit", "key", key, "access_count", entry.AccessCount)
	return entry.Value, true
}

// Set stores a value in the cache with default TTL
func (c *Cache[T]) Set(key string, value T) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value in the cache with specified TTL
func (c *Cache[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if we need to evict items to make room
	if len(c.data) >= c.maxSize {
		c.evictLRU()
	}

	entry := &CacheEntry[T]{
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
func (c *Cache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.data[key]; exists {
		delete(c.data, key)
		c.size.Add(context.Background(), -1)
		logger.Get().Debug("Cache delete", "key", key)
	}
}

// Clear removes all items from the cache
func (c *Cache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := len(c.data)
	c.data = make(map[string]*CacheEntry[T])
	c.size.Add(context.Background(), -int64(count))

	logger.Get().Info("Cache cleared", "items_removed", count)
}

// Size returns the current number of items in the cache
func (c *Cache[T]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Name returns the name of the cache
func (c *Cache[T]) Name() string {
	return c.name
}

// Stats returns cache statistics
func (c *Cache[T]) Stats() CacheStats {
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
func (c *Cache[T]) cleanupExpired() {
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
func (c *Cache[T]) performCleanup() {
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
func (c *Cache[T]) evictLRU() {
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
func (c *Cache[T]) recordHit(key string) {
	c.hits.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.result", "hit"),
		attribute.String("cache.name", c.name),
	))
}

// recordMiss records a cache miss
func (c *Cache[T]) recordMiss(key string) {
	c.misses.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.result", "miss"),
		attribute.String("cache.name", c.name),
	))
}

// Close stops the cache cleanup goroutine
func (c *Cache[T]) Close() {
	close(c.stopCleanup)
}

// InvalidateByType clears the entire cache for a specific cache type
func InvalidateByType(cacheType CacheType) {
	ctx := context.Background()
	_, span := telemetry.StartSpan(ctx, "cache.invalidate",
		attribute.String("cache.type", cacheType.String()),
		attribute.String("cache.operation", "invalidate"),
	)
	defer span.End()

	InitCaches()
	if cache, exists := cacheRegistry[cacheType]; exists {
		oldSize := cache.Size()
		cache.Clear()

		telemetry.AddEvent(span, "cache.invalidated",
			attribute.String("cache.name", cache.name),
			attribute.Int("cache.items_cleared", oldSize),
		)
		span.SetAttributes(
			attribute.String("cache.name", cache.name),
			attribute.Int("cache.items_cleared", oldSize),
		)
		telemetry.RecordSuccess(span, "Cache invalidated successfully")

		logger.Get().Info("Cache invalidated", "cache_type", cacheType.String(), "reason", "modification_command", "items_cleared", oldSize)
	} else {
		telemetry.RecordError(span, fmt.Errorf("cache type not found: %s", cacheType.String()), "Cache type not found")
	}
}

// InvalidateKubernetesCache clears the Kubernetes cache
func InvalidateKubernetesCache() {
	InvalidateByType(CacheTypeKubernetes)
}

// InvalidateHelmCache clears the Helm cache
func InvalidateHelmCache() {
	InvalidateByType(CacheTypeHelm)
}

// InvalidateIstioCache clears the Istio cache
func InvalidateIstioCache() {
	InvalidateByType(CacheTypeIstio)
}

// InvalidateCommandCache clears the Command cache
func InvalidateCommandCache() {
	InvalidateByType(CacheTypeCommand)
}

// InvalidateCacheForCommand invalidates the appropriate cache based on command type
func InvalidateCacheForCommand(command string) {
	if cacheType, exists := commandToCacheType[command]; exists {
		InvalidateByType(cacheType)
	} else {
		// Default to command cache for unknown commands
		InvalidateCommandCache()
	}
}

// Global cache instances for different use cases
var (
	// cacheRegistry holds all cache instances by type
	cacheRegistry = make(map[CacheType]*Cache[string])
	once          sync.Once
)

// InitCaches initializes all global cache instances
func InitCaches() {
	once.Do(func() {
		// Initialize caches with optimized TTL values based on use case
		// Kubernetes: 45s - K8s resources change frequently, users expect fresh data
		cacheRegistry[CacheTypeKubernetes] = NewCache[string](CacheTypeKubernetes.String(), 45*time.Second, 1000, 1*time.Minute)

		// Istio: 1m - Service mesh config more stable than pods, but proxy status can change
		cacheRegistry[CacheTypeIstio] = NewCache[string](CacheTypeIstio.String(), 1*time.Minute, 500, 1*time.Minute)

		// Helm: 2m - Releases change less frequently, chart info is stable
		cacheRegistry[CacheTypeHelm] = NewCache[string](CacheTypeHelm.String(), 2*time.Minute, 300, 2*time.Minute)

		// Command: 3m - General CLI commands have stable output, status commands don't change rapidly
		cacheRegistry[CacheTypeCommand] = NewCache[string](CacheTypeCommand.String(), 3*time.Minute, 200, 1*time.Minute)

		logger.Get().Info("Caches initialized")
	})
}

// GetCacheByType returns a cache instance by cache type
func GetCacheByType(cacheType CacheType) *Cache[string] {
	InitCaches()
	if cache, exists := cacheRegistry[cacheType]; exists {
		return cache
	}
	// Fallback to command cache if type not found
	return cacheRegistry[CacheTypeCommand]
}

// GetCacheByCommand returns a cache instance based on the command name
func GetCacheByCommand(command string) *Cache[string] {
	InitCaches()
	if cacheType, exists := commandToCacheType[command]; exists {
		return GetCacheByType(cacheType)
	}
	// Default to command cache for unknown commands
	return GetCacheByType(CacheTypeCommand)
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
func CacheResult[T any](cache *Cache[T], key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	ctx := context.Background()
	_, span := telemetry.StartSpan(ctx, "cache.result",
		attribute.String("cache.name", cache.name),
		attribute.String("cache.key", key),
		attribute.String("cache.ttl", ttl.String()),
	)
	defer span.End()

	var zero T

	// Try to get from cache first
	if cachedResult, found := cache.Get(key); found {
		telemetry.AddEvent(span, "cache.result.hit",
			attribute.String("cache.operation", "get"),
			attribute.String("cache.result", "hit"),
		)
		span.SetAttributes(
			attribute.String("cache.operation", "get"),
			attribute.String("cache.result", "hit"),
		)
		telemetry.RecordSuccess(span, "Cache hit - returning cached result")
		return cachedResult, nil
	}

	// Not in cache, execute function
	telemetry.AddEvent(span, "cache.result.miss",
		attribute.String("cache.operation", "compute"),
		attribute.String("cache.result", "miss"),
	)
	span.SetAttributes(
		attribute.String("cache.operation", "compute"),
		attribute.String("cache.result", "miss"),
	)

	result, err := fn()
	if err != nil {
		telemetry.RecordError(span, err, "Function execution failed")
		return zero, err
	}

	// Store in cache
	cache.SetWithTTL(key, result, ttl)

	telemetry.AddEvent(span, "cache.result.stored",
		attribute.String("cache.operation", "set"),
	)
	span.SetAttributes(attribute.String("cache.operation", "set"))
	telemetry.RecordSuccess(span, "Function executed and result cached")

	return result, nil
}
