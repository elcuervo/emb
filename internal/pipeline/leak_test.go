package pipeline

import (
	"runtime"
	"runtime/metrics"
	"testing"
)

// heapInUseBytes reads the Go heap in use via runtime/metrics. The pipeline
// package cannot import registry (import cycle), so the probe is inlined.
func heapInUseBytes(t *testing.T) uint64 {
	t.Helper()
	s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(s)
	if s[0].Value.Kind() == metrics.KindUint64 {
		return s[0].Value.Uint64()
	}
	t.Fatal("heap metric unavailable")
	return 0
}

// TestBatcherNoLeakAcrossBatches is the automated memory/goroutine leak guard
// for the batcher: thousands of batches on fast, deterministic fakes must leave
// the goroutine count back at baseline and heap growth flat (one-batch
// retention only). No sleeps — the gated-fake determinism rule.
func TestBatcherNoLeakAcrossBatches(t *testing.T) {
	sess := &recordingSession{dim: 4}
	tok := &fakeTok{}
	b := NewBatcher(sess, tok, 4, 16, false, "mean", 100000, 32, 16384, 0)
	defer b.Close()

	// Warmup: initialize the run loop and any lazy allocations.
	if _, err := b.Embed([]string{"warmup"}); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	baseGoroutines := runtime.NumGoroutine()
	baseHeap := heapInUseBytes(t)

	const batches = 2000
	for i := 0; i < batches; i++ {
		if _, err := b.Embed([]string{"leak-probe"}); err != nil {
			t.Fatal(err)
		}
	}

	runtime.GC()
	gotGoroutines := runtime.NumGoroutine()
	gotHeap := heapInUseBytes(t)

	if gotGoroutines > baseGoroutines+2 {
		t.Errorf("goroutines grew across %d batches: %d -> %d", batches, baseGoroutines, gotGoroutines)
	}
	if growth := int64(gotHeap) - int64(baseHeap); growth > 4<<20 {
		t.Errorf("heap grew %d bytes across %d batches (leak?)", growth, batches)
	}
}
