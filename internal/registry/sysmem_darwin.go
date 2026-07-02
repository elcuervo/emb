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
