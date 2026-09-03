package registry

import (
	"math"
	"runtime"
	"runtime/metrics"
)

// init forces one GC at process start so the /cpu/classes/* metrics are
// initialized: they stay NaN until the first GC, which would otherwise make
// INFO CPU report 0 usec for the first minutes of a fresh server.
func init() {
	runtime.GC()
}

// Process resource samplers. All of them are non-blocking: runtime/metrics
// reads are lock-free and never stop the world, and NumGoroutine is a cheap
// atomic read. They are safe to call from INFO/EMB.STATS handlers, which run
// concurrently per connection.
//
// These cover Go-managed memory and processor time. Native (CGo/ONNX Runtime)
// allocations live outside the Go heap and are only visible through the
// RSS-based CurrentMemoryUsage (see sysmem_linux.go).

// HeapInUseBytes returns the Go heap currently in use in bytes.
// (/memory/classes/heap/objects:bytes is the in-use heap; the older
// heap/in-use name is gone from recent Go metric sets.)
func HeapInUseBytes() uint64 {
	return sampleUint64("/memory/classes/heap/objects:bytes")
}

// CPUUserUsec returns the cumulative user-mode processor time of the process
// in microseconds since start, including work performed inside cgo calls.
func CPUUserUsec() uint64 {
	return cpuUsec("/cpu/classes/user:cpu-seconds")
}

// CPUSysUsec returns the cumulative system-mode processor time in
// microseconds. The metric set exposes only total and user classes, so system
// time is derived as total minus user.
func CPUSysUsec() uint64 {
	total := cpuUsec("/cpu/classes/total:cpu-seconds")
	user := cpuUsec("/cpu/classes/user:cpu-seconds")
	if total >= user {
		return total - user
	}
	return 0
}

func sampleUint64(name string) uint64 {
	s := []metrics.Sample{{Name: name}}
	metrics.Read(s)
	if s[0].Value.Kind() == metrics.KindUint64 {
		return s[0].Value.Uint64()
	}
	return 0
}

func cpuUsec(name string) uint64 {
	s := []metrics.Sample{{Name: name}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	v := s[0].Value.Float64()
	// The CPU classes are NaN until the first GC and must never be negative.
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	return uint64(v * 1e6)
}

// NumGoroutines returns the current number of live goroutines.
func NumGoroutines() int64 {
	return int64(runtime.NumGoroutine())
}
