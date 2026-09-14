package registry

import (
	"runtime"
	"testing"
	"time"
)

// TestCurrentMemoryUsagePositive verifies the sampler contract without assuming
// aggregate process RSS is monotonic across a GC cycle. The OS may reclaim
// unrelated pages while a test allocation remains live, so RSS growth is not a
// deterministic unit-test assertion.
func TestCurrentMemoryUsagePositive(t *testing.T) {
	got, fromRSS := CurrentMemoryUsage()
	if got == 0 {
		t.Fatalf("CurrentMemoryUsage returned 0 bytes (fromRSS=%v)", fromRSS)
	}
}

// heapInUse is verified separately so the heap-based code path is covered.
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
// gopsutil reads the kernel's process accounting, so no GC is required to
// prime the sampler.
func TestCPUTimeNonDecreasing(t *testing.T) {
	a := CPUUserUsec() + CPUSysUsec()

	// Burn ~80ms of user CPU.
	deadline := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(deadline) {
	}

	b := CPUUserUsec() + CPUSysUsec()
	if b < a {
		t.Errorf("cumulative CPU time decreased: %d -> %d", a, b)
	}
	if b == a {
		t.Errorf("cumulative CPU time did not advance after busy work: %d", a)
	}
}

func TestTotalSystemMemoryPositive(t *testing.T) {
	if TotalSystemMemory() == 0 {
		t.Fatal("TotalSystemMemory returned 0")
	}
}

func TestNumGoroutinesPositive(t *testing.T) {
	if NumGoroutines() < 1 {
		t.Fatalf("expected at least 1 goroutine, got %d", NumGoroutines())
	}
}
