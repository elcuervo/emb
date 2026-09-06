package server

import (
	"container/list"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/go-units"

	"github.com/elcuervo/emb/internal/registry"
)

type cacheEntry struct {
	key   string
	value []byte
}

// cacheModelStats tracks per-model cache activity for INFO's Keyspace section.
// Guarded by Cache.mu like the entry map.
type cacheModelStats struct {
	hits, misses, evictions, entries int64
}

type CacheModelStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Entries   int64
}

type Cache struct {
	mu                sync.Mutex
	maxBytes          int64
	curBytes          int64
	ll                *list.List
	entries           map[string]*list.Element
	byModel           map[string]*cacheModelStats
	hits              atomic.Int64
	misses            atomic.Int64
	evictions         atomic.Int64
	generation        uint64
	flushes           uint64
	flushedEntries    uint64
	lastFlushDuration time.Duration
}

type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Entries   int
	MaxBytes  int64
	CurBytes  int64
	// ByModel reports per-model hits/misses/evictions/entries. Callers must not
	// mutate the returned map.
	ByModel           map[string]CacheModelStats
	Generation        uint64
	Flushes           uint64
	FlushedEntries    uint64
	LastFlushDuration time.Duration
}

// CacheSnapshotEntry is an immutable shallow view of one cache entry. Values
// published through Set are never mutated in place, so snapshot encoding can
// safely retain the slice after the cache lock is released.
type CacheSnapshotEntry struct {
	Key   string
	Value []byte
}

// CacheSnapshot is ordered most-recently-used to least-recently-used.
type CacheSnapshot struct {
	Entries         []CacheSnapshotEntry
	Generation      uint64
	CurBytes        int64
	CaptureDuration time.Duration
}

func NewCache(maxBytes int64) *Cache {
	return &Cache{
		maxBytes: maxBytes,
		ll:       list.New(),
		entries:  make(map[string]*list.Element),
		byModel:  make(map[string]*cacheModelStats),
	}
}

// modelOf extracts the model prefix from a cache key ("model:text"). Keys
// without a colon are attributed to the whole key.
func modelOf(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i]
	}
	return key
}

// perModel returns (creating on demand) the stats bucket for a model. Caller
// holds c.mu.
func (c *Cache) perModel(model string) *cacheModelStats {
	ms, ok := c.byModel[model]
	if !ok {
		ms = &cacheModelStats{}
		c.byModel[model] = ms
	}
	return ms
}

func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	model := modelOf(key)
	ms := c.perModel(model)

	elem, ok := c.entries[key]
	if !ok {
		ms.misses++
		c.misses.Add(1)
		return nil, false
	}
	c.ll.MoveToFront(elem)
	val := elem.Value.(*cacheEntry).value
	ms.hits++
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
		c.generation++
		return
	}

	entryBytes := int64(len(key) + len(value) + 48)
	for c.maxBytes > 0 && c.curBytes+entryBytes > c.maxBytes {
		if c.evictTailLocked() == nil {
			break
		}
	}

	elem := c.ll.PushFront(&cacheEntry{key: key, value: value})
	c.entries[key] = elem
	c.curBytes += entryBytes
	c.perModel(modelOf(key)).entries++
	c.generation++
}

// evictTailLocked removes the least-recently-used entry and updates byte and
// per-model accounting. Returns the removed entry (nil when empty). Caller
// holds c.mu.
func (c *Cache) evictTailLocked() *cacheEntry {
	back := c.ll.Back()
	if back == nil {
		return nil
	}
	removed := back.Value.(*cacheEntry)
	c.curBytes -= int64(len(removed.key) + len(removed.value) + 48)
	delete(c.entries, removed.key)
	c.ll.Remove(back)
	c.evictions.Add(1)
	ms := c.perModel(modelOf(removed.key))
	ms.evictions++
	ms.entries--
	c.generation++
	return removed
}

// SetMaxBytes resizes the cache budget immediately, evicting the LRU tail as
// needed so the cache never exceeds the new budget.
func (c *Cache) SetMaxBytes(maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxBytes = maxBytes
	for c.maxBytes > 0 && c.curBytes > c.maxBytes {
		if c.evictTailLocked() == nil {
			break
		}
	}
}

func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	entries := c.ll.Len()
	curBytes := c.curBytes
	generation := c.generation
	flushes := c.flushes
	flushedEntries := c.flushedEntries
	lastFlushDuration := c.lastFlushDuration
	byModel := make(map[string]CacheModelStats, len(c.byModel))
	for m, ms := range c.byModel {
		byModel[m] = CacheModelStats{Hits: ms.hits, Misses: ms.misses, Evictions: ms.evictions, Entries: ms.entries}
	}
	c.mu.Unlock()
	return CacheStats{
		Hits:              c.hits.Load(),
		Misses:            c.misses.Load(),
		Evictions:         c.evictions.Load(),
		Entries:           entries,
		MaxBytes:          c.maxBytes,
		CurBytes:          curBytes,
		ByModel:           byModel,
		Generation:        generation,
		Flushes:           flushes,
		FlushedEntries:    flushedEntries,
		LastFlushDuration: lastFlushDuration,
	}
}

// Flush removes all entries in O(1), preserving the budget and cumulative
// hit/miss/eviction counters. It returns the number of removed entries.
func (c *Cache) Flush() int {
	start := time.Now()
	c.mu.Lock()
	n := c.ll.Len()
	c.ll = list.New()
	c.entries = make(map[string]*list.Element)
	c.curBytes = 0
	for _, ms := range c.byModel {
		ms.entries = 0
	}
	if n > 0 {
		c.generation++
	}
	c.flushes++
	c.flushedEntries += uint64(n)
	c.lastFlushDuration = time.Since(start)
	c.mu.Unlock()
	return n
}

// FlushModel removes entries for model while preserving the relative LRU
// order of all remaining entries. It returns the number removed.
func (c *Cache) FlushModel(model string) int {
	start := time.Now()
	c.mu.Lock()
	n := 0
	for elem := c.ll.Front(); elem != nil; {
		next := elem.Next()
		entry := elem.Value.(*cacheEntry)
		if modelOf(entry.key) == model {
			c.curBytes -= int64(len(entry.key) + len(entry.value) + 48)
			delete(c.entries, entry.key)
			c.ll.Remove(elem)
			n++
		}
		elem = next
	}
	if ms := c.byModel[model]; ms != nil {
		ms.entries -= int64(n)
	}
	if n > 0 {
		c.generation++
	}
	c.flushes++
	c.flushedEntries += uint64(n)
	c.lastFlushDuration = time.Since(start)
	c.mu.Unlock()
	return n
}

// Snapshot captures only immutable descriptors while holding the cache lock;
// callers perform payload encoding and I/O after the lock is released.
func (c *Cache) Snapshot() CacheSnapshot {
	start := time.Now()
	c.mu.Lock()
	entries := make([]CacheSnapshotEntry, 0, c.ll.Len())
	for elem := c.ll.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*cacheEntry)
		entries = append(entries, CacheSnapshotEntry{Key: entry.key, Value: entry.value})
	}
	snapshot := CacheSnapshot{
		Entries:    entries,
		Generation: c.generation,
		CurBytes:   c.curBytes,
	}
	c.mu.Unlock()
	snapshot.CaptureDuration = time.Since(start)
	return snapshot
}

// restoreAppendMRU appends an entry read in MRU-to-LRU order without eviction.
// It is used only by an unpublished staging cache during startup restore.
func (c *Cache) restoreAppendMRU(key string, value []byte) bool {
	entryBytes := int64(len(key) + len(value) + 48)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxBytes <= 0 || entryBytes > c.maxBytes-c.curBytes {
		return false
	}
	if _, exists := c.entries[key]; exists {
		return false
	}
	entry := &cacheEntry{key: key, value: value}
	c.entries[key] = c.ll.PushBack(entry)
	c.curBytes += entryBytes
	c.perModel(modelOf(key)).entries++
	return true
}

// replaceStorageFrom atomically publishes a validated staging cache while
// preserving this process's cumulative counters and configured byte budget.
func (c *Cache) replaceStorageFrom(staging *Cache) {
	staging.mu.Lock()
	c.mu.Lock()
	c.ll = staging.ll
	c.entries = staging.entries
	c.byModel = staging.byModel
	c.curBytes = staging.curBytes
	c.generation++
	c.mu.Unlock()
	staging.mu.Unlock()
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
