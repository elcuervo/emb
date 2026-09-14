// Package bounded provides a per-bucket, bounded map with deterministic
// eviction. The server's script-source cache and the script compiler's
// prototype cache need the same policy (independent buckets per model, a cap
// per bucket, evict the lexicographically smallest key), so it lives here once.
package bounded

import "sync"

// Map is a concurrency-safe cache of string-keyed values grouped into
// independent buckets (a bucket per model, in both call sites). Each bucket
// holds at most maxPerBucket entries; inserting into a full bucket evicts the
// lexicographically smallest key, which is arbitrary but deterministic (Go
// map iteration order is not).
type Map[V any] struct {
	mu           sync.Mutex
	by           map[string]map[string]V
	maxPerBucket int
}

// New returns a Map whose buckets hold at most maxPerBucket entries (floored
// at 1).
func New[V any](maxPerBucket int) *Map[V] {
	if maxPerBucket < 1 {
		maxPerBucket = 1
	}
	return &Map[V]{by: make(map[string]map[string]V), maxPerBucket: maxPerBucket}
}

// Get returns the value stored at bucket/key.
func (m *Map[V]) Get(bucket, key string) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var zero V
	per := m.by[bucket]
	if per == nil {
		return zero, false
	}
	v, ok := per[key]
	return v, ok
}

// GetOrCreate returns the value at bucket/key, calling create to produce it on
// a miss. create runs under the map lock, so concurrent callers for the same
// key build the value once. A create error stores nothing.
func (m *Map[V]) GetOrCreate(bucket, key string, create func() (V, error)) (V, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	per := m.by[bucket]
	if per != nil {
		if v, ok := per[key]; ok {
			return v, true, nil
		}
	} else {
		per = make(map[string]V)
		m.by[bucket] = per
	}
	v, err := create()
	if err != nil {
		var zero V
		return zero, false, err
	}
	if len(per) >= m.maxPerBucket {
		oldest := ""
		for k := range per {
			if oldest == "" || k < oldest {
				oldest = k
			}
		}
		if oldest != "" {
			delete(per, oldest)
		}
	}
	per[key] = v
	return v, false, nil
}

// Clear drops one bucket, or every bucket when bucket is "".
func (m *Map[V]) Clear(bucket string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if bucket == "" {
		m.by = make(map[string]map[string]V)
		return
	}
	delete(m.by, bucket)
}

// Len returns the number of entries in one bucket.
func (m *Map[V]) Len(bucket string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.by[bucket])
}
