package registry

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// cgroupRoot is the cgroup filesystem root. It is a package var so tests can
// point it at fixture files; production always uses the real mount.
var cgroupRoot = "/sys/fs/cgroup"

// EffectiveMemoryLimit returns the process's container memory limit in bytes,
// falling back to the host's total physical memory when no cgroup limit is
// set. Containers report host memory through the ordinary OS interfaces, so
// sizing worker pools from the host value overshoots in Fargate/EKS and risks
// the OOM killer.
func EffectiveMemoryLimit() uint64 {
	if v := readCgroupMemoryLimit(cgroupRoot); v > 0 {
		return v
	}
	return TotalSystemMemory()
}

// EffectiveNumCPU returns the process's effective CPU allowance, using the
// cgroup CPU quota when present and GOMAXPROCS otherwise (Go already applies
// the quota to GOMAXPROCS on recent releases; reading the quota keeps the
// budget explicit and testable).
func EffectiveNumCPU() int {
	if q := readCgroupCPUQuota(cgroupRoot); q > 0 {
		return q
	}
	return runtime.GOMAXPROCS(0)
}

// readCgroupMemoryLimit reads a cgroup v2 memory.max or v1
// memory/memory.limit_in_bytes. A missing or unlimited value returns 0.
func readCgroupMemoryLimit(root string) uint64 {
	for _, p := range []string{
		"memory.max",                   // v2
		"memory/memory.limit_in_bytes", // v1
	} {
		data, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			continue
		}
		if v, ok := parseCgroupLimit(string(data)); ok {
			return v
		}
	}
	return 0
}

// parseCgroupLimit parses a cgroup limit value. "max" and the v1
// effectively-unlimited sentinel (a value near 2^63) mean unlimited.
func parseCgroupLimit(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "max" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	if v > 1<<60 { // v1 PAGE_COUNTER_MAX / "no limit"
		return 0, false
	}
	return v, true
}

// readCgroupCPUQuota reads a cgroup v2 cpu.max ("$QUOTA $PERIOD", quota may be
// "max") or v1 cpu/cpu.cfs_quota_us and cpu.cfs_period_us, and returns floor
// (quota/period) CPUs. A missing or unlimited quota returns 0.
func readCgroupCPUQuota(root string) int {
	if data, err := os.ReadFile(filepath.Join(root, "cpu.max")); err == nil {
		if q := parseCPUQuota(strings.Fields(string(data))); q > 0 {
			return q
		}
	}
	quota, qerr := os.ReadFile(filepath.Join(root, "cpu", "cpu.cfs_quota_us"))
	period, perr := os.ReadFile(filepath.Join(root, "cpu", "cpu.cfs_period_us"))
	if qerr == nil && perr == nil {
		if q := parseCPUQuota([]string{
			strings.TrimSpace(string(quota)),
			strings.TrimSpace(string(period)),
		}); q > 0 {
			return q
		}
	}
	return 0
}

// parseCPUQuota expects [quota, period] and returns max(1, quota/period) when
// both are positive integers, else 0. The floor deliberately never rounds up:
// oversubscribing the cgroup quota is worse than leaving a fraction unused.
func parseCPUQuota(fields []string) int {
	if len(fields) != 2 {
		return 0
	}
	q, err := strconv.Atoi(fields[0])
	if err != nil || q <= 0 {
		return 0
	}
	p, err := strconv.Atoi(fields[1])
	if err != nil || p <= 0 {
		return 0
	}
	return max(1, q/p)
}
