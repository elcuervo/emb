package registry

import (
	"os"
	"runtime"
	"runtime/metrics"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

// Process resource samplers backed by gopsutil (cross-platform process/CPU
// accounting, no cgo) plus the Go runtime's own non-blocking metrics. Safe to
// call from INFO/EMB.STATS handlers, which run concurrently per connection.

// CurrentMemoryUsage returns the process resident set size in bytes and whether
// the value is a real RSS measurement. gopsutil reads the kernel's own process
// accounting (Linux /proc/self/statm, macOS proc_pidinfo, …) and includes
// native (CGo/ONNX Runtime) allocations that live outside the Go heap. If the
// platform is unsupported the Go heap is reported instead (fromRSS = false).
func CurrentMemoryUsage() (bytes uint64, fromRSS bool) {
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return HeapInUseBytes(), false
	}
	mi, err := p.MemoryInfo()
	if err != nil || mi == nil {
		return HeapInUseBytes(), false
	}
	return mi.RSS, true
}

// TotalSystemMemory returns the host's total physical memory in bytes.
func TotalSystemMemory() uint64 {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return vm.Total
}

// CPUUserUsec returns the cumulative user-mode processor time of the process
// in microseconds since start, including work performed inside cgo calls.
func CPUUserUsec() uint64 {
	ti, err := processTimes()
	if err != nil {
		return 0
	}
	return uint64(ti.User * 1e6)
}

// CPUSysUsec returns the cumulative system-mode processor time in microseconds.
func CPUSysUsec() uint64 {
	ti, err := processTimes()
	if err != nil {
		return 0
	}
	return uint64(ti.System * 1e6)
}

func processTimes() (*cpu.TimesStat, error) {
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	return p.Times()
}

// HeapInUseBytes returns the Go heap currently in use in bytes.
// (/memory/classes/heap/objects:bytes is the in-use heap; the older
// heap/in-use name is gone from recent Go metric sets.)
func HeapInUseBytes() uint64 {
	return sampleUint64("/memory/classes/heap/objects:bytes")
}

func sampleUint64(name string) uint64 {
	s := []metrics.Sample{{Name: name}}
	metrics.Read(s)
	if s[0].Value.Kind() == metrics.KindUint64 {
		return s[0].Value.Uint64()
	}
	return 0
}

// NumGoroutines returns the current number of live goroutines.
func NumGoroutines() int64 {
	return int64(runtime.NumGoroutine())
}
