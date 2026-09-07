package server

import (
	"bytes"
	"fmt"
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
				c.Set(key, []byte{byte(i)})
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
