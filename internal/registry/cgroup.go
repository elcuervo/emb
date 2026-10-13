package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// cgroupRoot is the cgroup filesystem root used for memory limits. It is a
// package var so tests can point it at fixture files; production always uses
// the real mount.
var cgroupRoot = "/sys/fs/cgroup"

// cgroupMountinfo and cgroupMembership locate the controller mounts and this
// process's cgroup. They are package vars for the same reason as cgroupRoot.
var (
	cgroupMountinfo  = "/proc/self/mountinfo"
	cgroupMembership = "/proc/self/cgroup"
)

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

// EffectiveNumCPU returns the integer CPU capacity the server plans from: the
// floored, scheduler-bounded effective allowance (see CPUAllowance.Capacity).
// Every integer CPU consumer goes through the same resolution as the startup
// plan, so there is one detector.
func EffectiveNumCPU() int { return EffectiveCPUAllowance().Capacity() }

// CPUAllowance keeps fractional cgroup bandwidth separate from the integer
// scheduler and admission limits. Provenance is non-empty only when no quota
// could be read and says why (missing mount, unreadable or malformed file).
type CPUAllowance struct {
	CPUs       float64
	Source     string
	Provenance string
	Scheduler  int
	Affinity   int
}

// Capacity is the integer admission capacity: the floored allowance (at least
// one) bounded by the Go scheduler. It never rounds a fraction up.
func (a CPUAllowance) Capacity() int {
	capacity := max(1, int(math.Floor(a.CPUs)))
	if a.Scheduler > 0 && a.Scheduler < capacity {
		capacity = a.Scheduler
	}
	return capacity
}

// EffectiveCPUAllowance reads the tightest visible quota in the current
// cgroup hierarchy and reports Go's scheduler and process affinity limits.
func EffectiveCPUAllowance() CPUAllowance {
	return resolveCPU(cgroupMountinfo, cgroupMembership, runtime.GOMAXPROCS(0), runtime.NumCPU())
}

func resolveCPU(mountinfo, membership string, scheduler, affinity int) CPUAllowance {
	quota, source, provenance := readCPUQuota(mountinfo, membership)
	return resolveCPUAllowance(quota, source, provenance, scheduler, affinity)
}

func resolveCPUAllowance(quota float64, source, provenance string, scheduler, affinity int) CPUAllowance {
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
	return CPUAllowance{CPUs: cpus, Source: "scheduler/affinity", Provenance: provenance, Scheduler: scheduler, Affinity: affinity}
}

// cgroupMount is one CPU-capable cgroup mount: the directory it is mounted at,
// the cgroup path its root corresponds to (not "/" in a remapped container),
// and whether it is the v2 unified hierarchy.
type cgroupMount struct {
	root, point string
	v2          bool
}

// parseMountinfo returns the cgroup v2 mounts and the v1 mounts that carry the
// cpu controller (a combined "cpu,cpuacct" mount included), in file order.
func parseMountinfo(data string) []cgroupMount {
	var mounts []cgroupMount
	for _, line := range strings.Split(data, "\n") {
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		pre, post := strings.Fields(left), strings.Fields(right)
		if len(pre) < 5 || len(post) < 3 {
			continue
		}
		m := cgroupMount{root: unescapeMountinfo(pre[3]), point: unescapeMountinfo(pre[4])}
		switch post[0] {
		case "cgroup2":
			m.v2 = true
		case "cgroup":
			if !slices.Contains(strings.Split(post[2], ","), "cpu") {
				continue
			}
		default:
			continue
		}
		mounts = append(mounts, m)
	}
	return mounts
}

// unescapeMountinfo decodes the \NNN octal escapes mountinfo uses for spaces
// and other awkward bytes in paths.
func unescapeMountinfo(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseMembership returns this process's cgroup path in the v2 hierarchy and in
// the v1 hierarchy carrying the cpu controller; either may be empty.
func parseMembership(data string) (v2, v1 string) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		switch {
		case fields[1] == "":
			v2 = fields[2]
		case slices.Contains(strings.Split(fields[1], ","), "cpu"):
			v1 = fields[2]
		}
	}
	return v2, v1
}

// readCPUQuota resolves the tightest positive finite CPU bandwidth (CPUs) that
// applies to this process: for every CPU-capable mount whose root contains the
// process's cgroup, the group directory and each visible ancestor up to the
// mount point are read and the smallest quota/period wins. The returned
// source names the file that decided it. When no quota is found, provenance
// says why nothing was read, so an unlimited cgroup is distinguishable from a
// missing or unreadable one.
func readCPUQuota(mountinfoPath, membershipPath string) (quota float64, source, provenance string) {
	info, err := os.ReadFile(mountinfoPath)
	if err != nil {
		return 0, "", fmt.Sprintf("cannot read %s: %v", mountinfoPath, err)
	}
	member, err := os.ReadFile(membershipPath)
	if err != nil {
		return 0, "", fmt.Sprintf("cannot read %s: %v", membershipPath, err)
	}
	mounts := parseMountinfo(string(info))
	if len(mounts) == 0 {
		return 0, "", "no cgroup cpu controller mount in " + mountinfoPath
	}
	v2Group, v1Group := parseMembership(string(member))
	var notes []string
	for _, m := range mounts {
		group := v1Group
		if m.v2 {
			group = v2Group
		}
		if group == "" {
			continue
		}
		rel, ok := relativeTo(m.root, group)
		if !ok {
			notes = append(notes, fmt.Sprintf("cgroup %s is outside mount root %s", group, m.root))
			continue
		}
		for dir := filepath.Join(m.point, rel); ; dir = filepath.Dir(dir) {
			if q, file, note := readDirQuota(dir, m.v2); note != "" {
				notes = append(notes, note)
			} else if q > 0 && (quota == 0 || q < quota) {
				quota, source = q, file
			}
			if dir == filepath.Clean(m.point) || dir == filepath.Dir(dir) {
				break
			}
		}
	}
	if quota > 0 {
		return quota, source, ""
	}
	if len(notes) == 0 {
		notes = append(notes, "no applicable cgroup limit")
	}
	return 0, "", strings.Join(notes, "; ")
}

// relativeTo returns group relative to a mount root: "/" when they are equal,
// and false when the group lies outside the root and is therefore not visible
// through this mount.
func relativeTo(root, group string) (string, bool) {
	root, group = filepath.Clean("/"+root), filepath.Clean("/"+group)
	switch {
	case group == root:
		return "/", true
	case root == "/":
		return group, true
	case strings.HasPrefix(group, root+"/"):
		return group[len(root):], true
	}
	return "", false
}

// readDirQuota reads one cgroup directory's CPU bandwidth. A missing file or an
// unlimited value is not a note; an unreadable or malformed one is.
func readDirQuota(dir string, v2 bool) (cpus float64, file, note string) {
	if v2 {
		file = filepath.Join(dir, "cpu.max")
		data, err := readKernelFile(file)
		if err != nil {
			return 0, file, readNote(file, err)
		}
		fields := strings.Fields(string(data))
		if len(fields) == 2 && fields[0] == "max" {
			return 0, file, ""
		}
		q, ok := parseCPUQuotaFloat(fields)
		if !ok {
			return 0, file, "malformed " + file
		}
		return q, file, ""
	}
	file = filepath.Join(dir, "cpu.cfs_quota_us")
	quota, err := readKernelFile(file)
	if err != nil {
		return 0, file, readNote(file, err)
	}
	period, err := readKernelFile(filepath.Join(dir, "cpu.cfs_period_us"))
	if err != nil {
		return 0, file, readNote(filepath.Join(dir, "cpu.cfs_period_us"), err)
	}
	q, ok := parseCPUQuotaFloat([]string{strings.TrimSpace(string(quota)), strings.TrimSpace(string(period))})
	if !ok {
		if strings.TrimSpace(string(quota)) == "-1" {
			return 0, file, ""
		}
		return 0, file, "malformed " + file
	}
	return q, file, ""
}

// readKernelFile reads a cgroup control file. The path is derived from the
// kernel's own mountinfo and membership files, not from request input.
func readKernelFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // kernel-provided path
}

func readNote(file string, err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	return fmt.Sprintf("cannot read %s: %v", file, err)
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
