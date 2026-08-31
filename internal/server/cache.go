package server

import (
	"container/list"
	"fmt"
	"strconv"
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

func autoTuneCache() int64 {
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

	// Ceiling of half the machine: never binds at the current constants
	// (auto is ~13% of RAM) but keeps a future formula change from pushing
	// the cache past half of total memory.
	ceiling := mem / 2
	if budget > int64(ceiling) {
		budget = int64(ceiling)
	}
	return budget
}

// percentCacheBudget converts a percentage of total system memory into a byte
// budget. pct must already be validated to (0, 100]; mem == 0 means the
// platform could not report system memory.
func percentCacheBudget(pct float64, mem uint64) (int64, error) {
	if mem == 0 {
		return 0, fmt.Errorf("cannot determine system memory for a percentage cache size")
	}
	return int64(pct / 100 * float64(mem)), nil
}

func parseCacheConfig(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.EqualFold(s, "auto") {
		return autoTuneCache(), nil
	}
	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || pct <= 0 || pct > 100 {
			return 0, fmt.Errorf("invalid cache percentage %q: must be a number greater than 0 and at most 100", s)
		}
		return percentCacheBudget(pct, registry.TotalSystemMemory())
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
