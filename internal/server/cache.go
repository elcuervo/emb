package server

import (
	"container/list"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/docker/go-units"

	"github.com/elcuervo/emb/internal/registry"
)

type cacheEntry struct {
	key   string
	value []byte
}

type Cache struct {
	mu        sync.Mutex
	maxBytes  int64
	curBytes  int64
	ll        *list.List
	entries   map[string]*list.Element
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Entries   int
	MaxBytes  int64
	CurBytes  int64
}

func NewCache(maxBytes int64) *Cache {
	return &Cache{
		maxBytes: maxBytes,
		ll:       list.New(),
		entries:  make(map[string]*list.Element),
	}
}

func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	elem, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		c.misses.Add(1)
		return nil, false
	}
	c.ll.MoveToFront(elem)
	val := elem.Value.(*cacheEntry).value
	c.mu.Unlock()
	c.hits.Add(1)
	return val, true
}

func (c *Cache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.entries[key]; ok {
		c.ll.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		c.curBytes -= int64(len(entry.value))
		c.curBytes += int64(len(value))
		entry.value = value
		return
	}

	entryBytes := int64(len(key) + len(value) + 48)
	for c.maxBytes > 0 && c.curBytes+entryBytes > c.maxBytes {
		back := c.ll.Back()
		if back == nil {
			break
		}
		removed := back.Value.(*cacheEntry)
		c.curBytes -= int64(len(removed.key) + len(removed.value) + 48)
		delete(c.entries, removed.key)
		c.ll.Remove(back)
		c.evictions.Add(1)
	}

	elem := c.ll.PushFront(&cacheEntry{key: key, value: value})
	c.entries[key] = elem
	c.curBytes += entryBytes
}

func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	entries := c.ll.Len()
	curBytes := c.curBytes
	c.mu.Unlock()
	return CacheStats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
		Entries:   entries,
		MaxBytes:  c.maxBytes,
		CurBytes:  curBytes,
	}
}

func autoTuneCache(defaultDim int) int64 {
	mem := registry.TotalSystemMemory()
	if mem == 0 {
		return 100 * 1024 * 1024
	}

	safetyMargin := mem / 10
	modelEstimate := mem / 4
	remaining := mem - safetyMargin - modelEstimate
	budget := int64(float64(remaining) * 0.2)
	if budget < 64*1024*1024 {
		budget = 64 * 1024 * 1024
	}

	maxBytes := int64(math.Min(float64(budget), float64(500*1024*1024)))
	return maxBytes
}

func parseCacheConfig(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.EqualFold(s, "auto") {
		return autoTuneCache(384), nil
	}
	bytes, err := units.FromHumanSize(s)
	if err != nil {
		return 0, fmt.Errorf("invalid cache size %q: %w", s, err)
	}
	if bytes <= 0 {
		return 0, fmt.Errorf("cache size must be positive, got %q", s)
	}
	return bytes, nil
}
