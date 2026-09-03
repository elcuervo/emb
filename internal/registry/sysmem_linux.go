//go:build linux

package registry

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func TotalSystemMemory() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) * uint64(info.Unit)
}

// CurrentMemoryUsage returns the process resident set size in bytes and
// whether the value came from a real RSS source. On Linux it reads VmRSS from
// /proc/self/status, which includes native (CGo/ONNX Runtime) allocations that
// live outside the Go heap.
func CurrentMemoryUsage() (bytes uint64, fromRSS bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
