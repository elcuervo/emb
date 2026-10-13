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

// CPUAllowance keeps fractional cgroup bandwidth separate from the integer
// scheduler and admission limits.
type CPUAllowance struct {
	CPUs      float64
	Source    string
	Scheduler int
	Affinity  int
}

// EffectiveCPUAllowance reads the tightest visible quota in the current
// cgroup hierarchy and reports Go's scheduler and process affinity limits.
func EffectiveCPUAllowance() CPUAllowance {
	scheduler, affinity := runtime.GOMAXPROCS(0), runtime.NumCPU()
	quota, source := readCurrentCPUQuota(cgroupRoot, "/proc/self/cgroup")
	return resolveCPUAllowance(quota, source, scheduler, affinity)
}

func resolveCPUAllowance(quota float64, source string, scheduler, affinity int) CPUAllowance {
	if quota > 0 {
		// A quota above the process's affinity cannot be used: the process can
		// only run on the CPUs it is pinned to, so plan from the smaller number.
		if affinity > 0 && float64(affinity) < quota {
			quota = float64(affinity)
		}
		return CPUAllowance{CPUs: quota, Source: source, Scheduler: scheduler, Affinity: affinity}
	}
	cpus := float64(affinity)
	if scheduler < affinity {
		cpus = float64(scheduler)
	}
	return CPUAllowance{CPUs: cpus, Source: "scheduler/affinity", Scheduler: scheduler, Affinity: affinity}
}

func readCurrentCPUQuota(root, membershipFile string) (float64, string) {
	b, err := os.ReadFile(membershipFile)
	if err != nil {
		return 0, ""
	}
	var group, mount string
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		if fields[0] == "0" && fields[1] == "" {
			group, mount = fields[2], root
			break
		}
		for _, controller := range strings.Split(fields[1], ",") {
			if controller == "cpu" {
				group, mount = fields[2], filepath.Join(root, "cpu")
				break
			}
		}
	}
	if group == "" || mount == "" {
		return 0, ""
	}
	quota, source := 0.0, ""
	for dir := filepath.Clean(filepath.Join(mount, group)); ; dir = filepath.Dir(dir) {
		if data, err := os.ReadFile(filepath.Join(dir, "cpu.max")); err == nil {
			if q, ok := parseCPUQuotaFloat(strings.Fields(string(data))); ok && (quota == 0 || q < quota) {
				quota, source = q, filepath.Join(dir, "cpu.max")
			}
		}
		if data, err := os.ReadFile(filepath.Join(dir, "cpu.cfs_quota_us")); err == nil {
			period, err := os.ReadFile(filepath.Join(dir, "cpu.cfs_period_us"))
			if err == nil {
				if q, ok := parseCPUQuotaFloat([]string{strings.TrimSpace(string(data)), strings.TrimSpace(string(period))}); ok && (quota == 0 || q < quota) {
					quota, source = q, filepath.Join(dir, "cpu.cfs_quota_us")
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir || (parent != filepath.Clean(mount) && !strings.HasPrefix(parent, filepath.Clean(mount)+string(filepath.Separator))) {
			break
		}
	}
	return quota, source
}

func parseCPUQuotaFloat(fields []string) (float64, bool) {
	if len(fields) != 2 || fields[0] == "max" {
		return 0, false
	}
	quota, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || quota <= 0 {
		return 0, false
	}
	period, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || period <= 0 {
		return 0, false
	}
	return quota / period, true
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

// CPUThrottle is the kernel's own accounting of CPU the process could not use:
// cgroup throttling (periods the group was held off the CPU and the time it was
// kept off) plus the host's stolen-CPU time. Every counter is cumulative, so a
// reader diffs two samples; Known is false when neither source is readable (a
// platform without cgroups), and zeros must then not be read as "measured zero".
type CPUThrottle struct {
	Periods    uint64
	Throttled  uint64
	ThrottleUs uint64
	StealUs    uint64
	Known      bool
}

// ReadCPUThrottle reads the process's cgroup cpu.stat (v2, else the v1
// cpu/cpu.stat) and the host's steal time from /proc/stat. It never fails: an
// unreadable source leaves its fields zero so a stats read cannot break a server
// on a platform without cgroups.
func ReadCPUThrottle() CPUThrottle {
	var out CPUThrottle
	for _, p := range []string{
		"cpu.stat",     // v2
		"cpu/cpu.stat", // v1
	} {
		data, err := os.ReadFile(filepath.Join(cgroupRoot, p))
		if err != nil {
			continue
		}
		out.Periods, out.Throttled, out.ThrottleUs = parseCPUStat(string(data))
		out.Known = true
		break
	}
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		if steal, ok := parseProcStatSteal(string(data)); ok {
			out.StealUs = steal
			out.Known = true
		}
	}
	return out
}

// parseCPUStat parses cgroup cpu.stat for nr_periods, nr_throttled and the
// throttled time. v2 reports throttled_usec; the older v1 format reports
// throttled_time in nanoseconds, normalized here to microseconds.
func parseCPUStat(s string) (periods, throttled, usec uint64) {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "nr_periods":
			periods = v
		case "nr_throttled":
			throttled = v
		case "throttled_usec":
			usec = v
		case "throttled_time":
			usec = v / 1000
		}
	}
	return periods, throttled, usec
}

// parseProcStatSteal reads the aggregate "cpu" line of /proc/stat and returns
// its steal column in microseconds. The counters are in USER_HZ ticks; 100Hz is
// what Linux reports here (and the kernel's own steal accounting is in ticks).
func parseProcStatSteal(s string) (uint64, bool) {
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// cpu user nice system idle iowait irq softirq steal
		if len(fields) < 9 {
			return 0, false
		}
		ticks, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return 0, false
		}
		return ticks * 10_000, true // ticks at 100Hz -> microseconds
	}
	return 0, false
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
