package registry

import (
	"runtime"
	"testing"
	"time"
)

// TestCurrentMemoryUsageGrowsAfterAllocation verifies the RSS sampler returns a
// positive value that grows when the process allocates (real RSS on Linux and
// macOS via gopsutil; heap fallback elsewhere).
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
