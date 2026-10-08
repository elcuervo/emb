package server

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestCacheFlush(t *testing.T) {
	c := NewCache(1 << 20)
	c.Set("a:one", []byte{1})
	c.Set("b:two", []byte{2})
	before := c.Stats()
	if got := c.Flush(); got != 2 {
		t.Fatalf("Flush removed %d entries, want 2", got)
	}
	after := c.Stats()
	if after.Entries != 0 || after.CurBytes != 0 || after.MaxBytes != before.MaxBytes {
		t.Fatalf("bad post-flush stats: %+v", after)
	}
	if after.Evictions != before.Evictions || after.Flushes != 1 || after.FlushedEntries != 2 {
		t.Fatalf("flush accounting mixed with eviction: before=%+v after=%+v", before, after)
	}
	if got := c.Flush(); got != 0 || c.Stats().Flushes != 2 {
		t.Fatalf("empty flush = %d, stats=%+v", got, c.Stats())
	}
}

func TestCacheFlushModelPreservesOtherEntriesAndOrder(t *testing.T) {
	c := NewCache(220)
	c.Set("a:old", []byte{1})
	c.Set("b:old", []byte{2})
	c.Set("a:new", []byte{3})
	c.Set("b:new", []byte{4})
	if got := c.FlushModel("a"); got != 2 {
		t.Fatalf("FlushModel removed %d entries, want 2", got)
	}
	if _, ok := c.Get("a:new"); ok {
		t.Fatal("flushed model entry survived")
	}
	if got, ok := c.Get("b:new"); !ok || got[0] != 4 {
		t.Fatal("other model entry missing")
	}
	st := c.Stats()
	if st.ByModel["a"].Entries != 0 || st.ByModel["b"].Entries != 2 {
		t.Fatalf("bad per-model counts: %+v", st.ByModel)
	}
}

func TestCacheFlushScenarios(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		want   int
		remain int
	}{
		{name: "whole", want: 3, remain: 0},
		{name: "scoped", model: "a", want: 2, remain: 1},
		{name: "empty scope", model: "missing", want: 0, remain: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCache(1 << 20)
			c.Set("a:one", []byte{1})
			c.Set("b:two", []byte{2})
			c.Set("a:three", []byte{3})
			before := c.Stats()
			var removed int
			if tc.model == "" {
				removed = c.Flush()
			} else {
				removed = c.FlushModel(tc.model)
			}
			after := c.Stats()
			if removed != tc.want || after.Entries != tc.remain {
				t.Fatalf("removed/remaining = %d/%d, want %d/%d", removed, after.Entries, tc.want, tc.remain)
			}
			if after.CurBytes < 0 || after.Evictions != before.Evictions || after.MaxBytes != before.MaxBytes {
				t.Fatalf("accounting changed incorrectly: before=%+v after=%+v", before, after)
			}
			c.Set("b:after", []byte{9})
			if got, ok := c.Get("b:after"); !ok || got[0] != 9 {
				t.Fatal("post-flush insertion failed")
			}
		})
	}
}

func TestCacheSnapshotValueIsImmutableView(t *testing.T) {
	c := NewCache(1 << 20)
	original := []byte{1, 2, 3}
	c.Set("m:key", original)
	snap := c.Snapshot()
	c.Set("m:key", []byte{9, 9, 9})
	c.Flush()
	if !bytes.Equal(snap.Entries[0].Value, original) {
		t.Fatalf("snapshot value changed: %v", snap.Entries[0].Value)
	}
}

func TestCacheLifecycleConcurrent(t *testing.T) {
	c := NewCache(32 << 10)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := range 500 {
				key := fmt.Sprintf("m%d:k%d", worker&1, i&31)
				c.Set(key, bytes.Repeat([]byte{byte(i)}, 1+i%3000))
				if st := c.Stats(); st.CurBytes > st.MaxBytes {
					t.Errorf("budget exceeded: %+v", st)
				}
				c.Get(key)
				if i%73 == 0 {
					_ = c.Snapshot()
				}
			}
		}(worker)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := range 50 {
			switch i % 3 {
			case 0:
				c.FlushModel("m0")
			case 1:
				c.SetMaxBytes(int64(1+i%2) << 10)
			case 2:
				c.Flush()
			}
		}
	}()
	close(start)
	wg.Wait()
	st := c.Stats()
	if st.CurBytes < 0 || st.CurBytes > st.MaxBytes {
		t.Fatalf("cache accounting invalid: %+v", st)
	}
}

func TestCacheAdmissionBudget(t *testing.T) {
	// Initial entries cost 52 bytes: three key bytes, one value, 48 overhead.
	for _, tc := range []struct {
		name, key  string
		size       int
		rejected   bool
		keys       []string
		evictions  int64
		generation uint64
	}{
		{"oversized insert", "d:3", 158, true, []string{"c:2", "b:1", "a:0"}, 0, 3},
		{"oversized replacement", "a:0", 158, true, []string{"c:2", "b:1", "a:0"}, 0, 3},
		{"growing replacement", "a:0", 106, false, []string{"a:0"}, 2, 6},
		{"exact total", "a:0", 53, false, []string{"a:0", "c:2", "b:1"}, 0, 4},
		{"exact entry", "a:0", 157, false, []string{"a:0"}, 2, 6},
		{"exact insert", "d:3", 1, false, []string{"d:3", "c:2", "b:1", "a:0"}, 0, 4},
		{"shrinking replacement", "a:0", 0, false, []string{"a:0", "c:2", "b:1"}, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCache(208)
			for _, key := range []string{"a:0", "b:1", "c:2"} {
				c.Set(key, []byte{1})
			}
			before := c.Stats()
			old := c.Snapshot()
			value := bytes.Repeat([]byte{2}, tc.size)
			c.Set(tc.key, value)
			snap, st := c.Snapshot(), c.Stats()
			if tc.rejected && !reflect.DeepEqual(before, st) {
				t.Fatalf("rejected write mutated stats: %+v -> %+v", before, st)
			}
			if len(snap.Entries) != len(tc.keys) {
				t.Fatalf("entries = %v", snap.Entries)
			}
			var total int64
			counts := map[string]int64{}
			for i, e := range snap.Entries {
				want := []byte{1}
				if e.Key == tc.key && !tc.rejected {
					want = value
				}
				if e.Key != tc.keys[i] || !bytes.Equal(e.Value, want) {
					t.Fatalf("entry %d = %v, want %s/%v", i, e, tc.keys[i], want)
				}
				total += int64(len(e.Key) + len(e.Value) + 48)
				counts[modelOf(e.Key)]++
			}
			if st.CurBytes != total || total > st.MaxBytes || st.Entries != len(tc.keys) || st.Evictions != tc.evictions || st.Generation != tc.generation || st.Hits != 0 || st.Misses != 0 {
				t.Fatalf("bad accounting: %+v", st)
			}
			for model, ms := range st.ByModel {
				evicted := max(before.ByModel[model].Entries-counts[model], 0)
				if ms != (CacheModelStats{Entries: counts[model], Evictions: evicted}) {
					t.Fatalf("model %s: %+v", model, ms)
				}
			}
			for _, e := range old.Entries {
				if !bytes.Equal(e.Value, []byte{1}) {
					t.Fatal("snapshot value mutated")
				}
			}
		})
	}
	for _, budget := range []int64{0, -1} {
		c := NewCache(budget)
		c.Set("m:k", make([]byte, 1000))
		c.Set("m:k", make([]byte, 2000))
		if c.Stats().CurBytes != 2051 {
			t.Fatal("non-positive budget semantics changed")
		}
	}
}
