//go:build darwin

package registry

import (
	"runtime"

	"golang.org/x/sys/unix"
)

func totalSystemMemory() uint64 {
	val, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return val
}

func currentMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}
