//go:build darwin

package registry

import (
	"golang.org/x/sys/unix"
)

func TotalSystemMemory() uint64 {
	val, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return val
}

// CurrentMemoryUsage falls back to the Go heap on darwin (fromRSS=false,
// per the resource-usage-reporting spec). A true RSS source such as
// mach_task_basic_info requires cgo and is a future option; the heap value
// is the spec'd fallback so INFO stays truthful on all platforms.
func CurrentMemoryUsage() (bytes uint64, fromRSS bool) {
	return HeapInUseBytes(), false
}
