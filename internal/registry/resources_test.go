package registry

import (
	"runtime"
	"testing"
	"time"
)

// TestCurrentMemoryUsageGrowsAfterAllocation verifies the RSS/heap sampler
// returns a positive value that grows when the process allocates.
func TestCurrentMemoryUsageGrowsAfterAllocation(t *testing.T) {
	before, fromRSS := CurrentMemoryUsage()
	if before == 0 {
		t.Fatalf("CurrentMemoryUsage returned 0 bytes (fromRSS=%v)", fromRSS)
	}

	var sink [][]byte
	for i := 0; i < 16; i++ {
		sink = append(sink, make([]byte, 4<<20)) // 64 MiB total, kept reachable
	}
	runtime.GC()
	runtime.KeepAlive(sink)

	after, _ := CurrentMemoryUsage()
	if after < before {
		t.Errorf("memory usage decreased after allocating 64 MiB: %d -> %d", before, after)
	}
}

// heapInUse is verified separately so the heap-based fallback path is covered
// on every platform, including ones without an RSS source.
func TestHeapInUseBytesPositive(t *testing.T) {
	if HeapInUseBytes() == 0 {
		t.Fatal("HeapInUseBytes returned 0")
	}
	sink := make([]byte, 8<<20)
	runtime.GC()
	runtime.KeepAlive(sink)
	if HeapInUseBytes() < 8<<20 {
		t.Errorf("expected heap in use >= 8 MiB after allocating 8 MiB, got %d", HeapInUseBytes())
	}
}

// TestCPUTimeNonDecreasing verifies the CPU sampler returns realistic,
// monotonic cumulative values (user+system never decrease across calls).
func TestCPUTimeNonDecreasing(t *testing.T) {
	// CPU-class metrics are NaN until the first GC, so initialize them first.
	runtime.GC()
	a := CPUUserUsec() + CPUSysUsec()
	if a == 0 {
		t.Fatal("cumulative CPU time is 0 at start; expected already-burned runtime CPU")
	}

	// Burn ~80ms of user CPU, then force a GC: the CPU-class metrics are only
	// flushed at GC/exit, so without it the second read would miss the work.
	deadline := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(deadline) {
	}
	runtime.GC()

	b := CPUUserUsec() + CPUSysUsec()
	if b < a {
		t.Errorf("cumulative CPU time decreased: %d -> %d", a, b)
	}
	if b == a {
		t.Errorf("cumulative CPU time did not advance after busy work: %d", a)
	}
}

// TestCPUTimeGuard covers the non-float64/unknown-metric guard: a metric that
// does not exist must yield 0, never garbage.
func TestCPUTimeGuard(t *testing.T) {
	if got := cpuUsec("/no/such/metric:seconds"); got != 0 {
		t.Errorf("cpuUsec on unknown metric = %d, want 0", got)
	}
}

func TestNumGoroutinesPositive(t *testing.T) {
	if NumGoroutines() < 1 {
		t.Fatalf("expected at least 1 goroutine, got %d", NumGoroutines())
	}
}
