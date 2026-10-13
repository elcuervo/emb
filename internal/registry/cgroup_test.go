package registry

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestParseCgroupLimit(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"1073741824\n", 1073741824, true},
		{"max\n", 0, false},
		{"", 0, false},
		{"not-a-number", 0, false},
		{"9223372036854771712\n", 0, false}, // v1 "unlimited" sentinel
	}
	for _, c := range cases {
		got, ok := parseCgroupLimit(c.in)
		if got != c.want || ok != c.ok {
			t.Fatalf("parseCgroupLimit(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestReadCgroupMemoryLimit(t *testing.T) {
	v2 := t.TempDir()
	os.WriteFile(filepath.Join(v2, "memory.max"), []byte("268435456\n"), 0o644)
	if got := readCgroupMemoryLimit(v2); got != 268435456 {
		t.Fatalf("v2 memory.max = %d, want 268435456", got)
	}

	v1 := t.TempDir()
	os.MkdirAll(filepath.Join(v1, "memory"), 0o755)
	os.WriteFile(filepath.Join(v1, "memory", "memory.limit_in_bytes"), []byte("536870912\n"), 0o644)
	if got := readCgroupMemoryLimit(v1); got != 536870912 {
		t.Fatalf("v1 memory limit = %d, want 536870912", got)
	}

	if got := readCgroupMemoryLimit(t.TempDir()); got != 0 {
		t.Fatalf("absent limit = %d, want 0", got)
	}
}

func TestParseCPUQuotaFloat(t *testing.T) {
	for _, tc := range []struct {
		fields []string
		want   float64
		ok     bool
	}{{[]string{"50000", "100000"}, .5, true}, {[]string{"150000", "100000"}, 1.5, true}, {[]string{"max", "100000"}, 0, false}} {
		got, ok := parseCPUQuotaFloat(tc.fields)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("parseCPUQuotaFloat(%v) = (%v,%v), want (%v,%v)", tc.fields, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolveCPUAllowanceSeparatesQuotaAndScheduler(t *testing.T) {
	quota := resolveCPUAllowance(1.5, "cgroup", "", 4, 8)
	if quota.CPUs != 1.5 || quota.Scheduler != 4 || quota.Affinity != 8 {
		t.Fatalf("quota allowance = %+v, want fractional quota and distinct limits", quota)
	}
	fallback := resolveCPUAllowance(0, "", "", 2, 8)
	if fallback.CPUs != 2 || fallback.Source != "scheduler/affinity" || fallback.Scheduler != 2 {
		t.Fatalf("scheduler-limited allowance = %+v, want 2 CPUs", fallback)
	}
}

// A quota the process cannot use is not an allowance: affinity binds first, so
// planning must not admit more work than the CPUs the process can run on.
func TestResolveCPUAllowanceCapsByAffinity(t *testing.T) {
	capped := resolveCPUAllowance(8, "cgroup", "", 8, 2)
	if capped.CPUs != 2 {
		t.Fatalf("quota above affinity = %v CPUs, want 2 (the affinity)", capped.CPUs)
	}
	keep := resolveCPUAllowance(4, "cgroup", "", 8, 8)
	if keep.CPUs != 4 {
		t.Fatalf("quota below affinity = %v CPUs, want the quota 4", keep.CPUs)
	}
	unknown := resolveCPUAllowance(4, "cgroup", "", 8, 0)
	if unknown.CPUs != 4 {
		t.Fatalf("unknown affinity = %v CPUs, want the quota 4", unknown.CPUs)
	}
}

// cgroupFixture is a synthetic /proc/self/mountinfo + /proc/self/cgroup pair
// over a temp tree, so resolution is exercised exactly as on a host.
type cgroupFixture struct {
	t          *testing.T
	dir        string
	mountinfo  string
	membership string
}

func newCgroupFixture(t *testing.T, mountinfo []string, membership string) cgroupFixture {
	t.Helper()
	f := cgroupFixture{t: t, dir: t.TempDir()}
	f.mountinfo = f.write("mountinfo", strings.ReplaceAll(strings.Join(mountinfo, "\n"), "$DIR", f.dir)+"\n")
	f.membership = f.write("membership", membership)
	return f
}

// write creates a file under the fixture directory, relative to it.
func (f cgroupFixture) write(rel, content string) string {
	f.t.Helper()
	path := filepath.Join(f.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func (f cgroupFixture) quota() (float64, string, string) {
	return readCPUQuota(f.mountinfo, f.membership)
}

const (
	v2Mount = "30 25 0:26 / $DIR rw,nosuid - cgroup2 cgroup2 rw"
)

func TestParseMountinfo(t *testing.T) {
	data := strings.Join([]string{
		"22 1 0:20 / /sys rw - sysfs sysfs rw",
		"31 22 0:27 / /sys/fs/cgroup/cpu,cpuacct rw shared:9 - cgroup cgroup rw,cpu,cpuacct",
		"32 22 0:28 / /sys/fs/cgroup/memory rw - cgroup cgroup rw,memory",
		"33 22 0:29 /docker/abc /sys/fs/cgroup/unified\\040dir rw - cgroup2 cgroup2 rw",
		"not a mountinfo line",
	}, "\n")
	got := parseMountinfo(data)
	want := []cgroupMount{
		{root: "/", point: "/sys/fs/cgroup/cpu,cpuacct"},
		{root: "/docker/abc", point: "/sys/fs/cgroup/unified dir", v2: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("mounts = %+v, want %+v (cpu-less v1 mounts skipped, escapes decoded)", got, want)
	}
}

func TestReadCPUQuotaV2WalksAncestorsAndKeepsFraction(t *testing.T) {
	f := newCgroupFixture(t, []string{v2Mount}, "0::/parent/leaf\n")
	f.write("cpu.max", "200000 100000\n")
	f.write("parent/cpu.max", "150000 100000\n")
	f.write("parent/leaf/cpu.max", "300000 100000\n")
	got, source, prov := f.quota()
	if got != 1.5 || source != filepath.Join(f.dir, "parent", "cpu.max") || prov != "" {
		t.Fatalf("quota = %v from %q (%q); want 1.5 from the parent", got, source, prov)
	}
}

func TestReadCPUQuotaUnlimitedChildLimitedParent(t *testing.T) {
	f := newCgroupFixture(t, []string{v2Mount}, "0::/parent/leaf\n")
	f.write("parent/cpu.max", "50000 100000\n")
	f.write("parent/leaf/cpu.max", "max 100000\n")
	if got, _, _ := f.quota(); got != 0.5 {
		t.Fatalf("quota = %v, want the limited ancestor's 0.5", got)
	}
}

// A combined cpu,cpuacct v1 mount lives at a path no root-only reader guesses.
func TestReadCPUQuotaCombinedV1Mount(t *testing.T) {
	f := newCgroupFixture(t, []string{
		"31 22 0:27 / $DIR/cpu,cpuacct rw - cgroup cgroup rw,cpu,cpuacct",
		"32 22 0:28 / $DIR/memory rw - cgroup cgroup rw,memory",
	}, "5:cpu,cpuacct:/parent/leaf\n4:memory:/parent/leaf\n")
	f.write("cpu,cpuacct/cpu.cfs_quota_us", "200000")
	f.write("cpu,cpuacct/cpu.cfs_period_us", "100000")
	f.write("cpu,cpuacct/parent/cpu.cfs_quota_us", "150000")
	f.write("cpu,cpuacct/parent/cpu.cfs_period_us", "100000")
	f.write("cpu,cpuacct/parent/leaf/cpu.cfs_quota_us", "-1")
	f.write("cpu,cpuacct/parent/leaf/cpu.cfs_period_us", "100000")
	got, source, _ := f.quota()
	if got != 1.5 || source != filepath.Join(f.dir, "cpu,cpuacct", "parent", "cpu.cfs_quota_us") {
		t.Fatalf("quota = %v from %q; want 1.5 from the v1 parent (unlimited leaf ignored)", got, source)
	}
}

// Hybrid hosts expose a controller-less v2 mount next to the v1 cpu hierarchy:
// the limit lives in v1.
func TestReadCPUQuotaHybrid(t *testing.T) {
	f := newCgroupFixture(t, []string{
		"30 25 0:26 / $DIR/unified rw - cgroup2 cgroup2 rw",
		"31 22 0:27 / $DIR/cpu rw - cgroup cgroup rw,cpu",
	}, "4:cpu:/svc\n0::/svc\n")
	f.write("cpu/svc/cpu.cfs_quota_us", "50000")
	f.write("cpu/svc/cpu.cfs_period_us", "100000")
	if got, _, prov := f.quota(); got != 0.5 || prov != "" {
		t.Fatalf("quota = %v (%q), want the v1 limit 0.5", got, prov)
	}
}

// Inside a container the mount root is the container's own group, so the
// process's path is below that root, not below "/".
func TestReadCPUQuotaRemappedRoot(t *testing.T) {
	f := newCgroupFixture(t, []string{"30 25 0:26 /docker/abc $DIR rw - cgroup2 cgroup2 rw"}, "0::/docker/abc/worker\n")
	f.write("cpu.max", "100000 100000\n")
	f.write("worker/cpu.max", "max 100000\n")
	got, source, _ := f.quota()
	if got != 1 || source != filepath.Join(f.dir, "cpu.max") {
		t.Fatalf("quota = %v from %q, want the mount-root limit 1", got, source)
	}
}

func TestReadCPUQuotaProvenanceWhenNothingResolves(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mountinfo  []string
		membership string
		files      map[string]string
		want       string
	}{
		{"unlimited", []string{v2Mount}, "0::/g\n", map[string]string{"g/cpu.max": "max 100000\n"}, "no applicable cgroup limit"},
		{"malformed cpu.max", []string{v2Mount}, "0::/g\n", map[string]string{"g/cpu.max": "garbage\n"}, "malformed"},
		{"no cpu controller mount", []string{"22 1 0:20 / /sys rw - sysfs sysfs rw"}, "0::/g\n", nil, "no cgroup cpu controller mount"},
		{"group outside the mount root", []string{"30 25 0:26 /docker/abc $DIR rw - cgroup2 cgroup2 rw"}, "0::/elsewhere\n", nil, "outside mount root"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCgroupFixture(t, tc.mountinfo, tc.membership)
			for rel, content := range tc.files {
				f.write(rel, content)
			}
			got, source, prov := f.quota()
			if got != 0 || source != "" || !strings.Contains(prov, tc.want) {
				t.Fatalf("quota = %v from %q (%q), want none with provenance containing %q", got, source, prov, tc.want)
			}
		})
	}
	if _, _, prov := readCPUQuota(filepath.Join(t.TempDir(), "missing"), "/dev/null"); !strings.Contains(prov, "cannot read") {
		t.Fatalf("missing mountinfo provenance = %q, want a read error", prov)
	}
	f := newCgroupFixture(t, []string{v2Mount}, "")
	if _, _, prov := readCPUQuota(f.mountinfo, filepath.Join(f.dir, "absent")); !strings.Contains(prov, "cannot read") {
		t.Fatalf("missing membership provenance = %q, want a read error", prov)
	}
}

// A fallback allowance carries the provenance; a quota tighter than affinity
// does not hide that affinity binds first.
func TestResolveCPUFromFixture(t *testing.T) {
	f := newCgroupFixture(t, []string{v2Mount}, "0::/\n")
	f.write("cpu.max", "800000 100000\n")
	if got := resolveCPU(f.mountinfo, f.membership, 8, 2); got.CPUs != 2 || got.Affinity != 2 {
		t.Fatalf("affinity-bound allowance = %+v, want 2 CPUs", got)
	}
	empty := newCgroupFixture(t, []string{v2Mount}, "0::/\n")
	got := resolveCPU(empty.mountinfo, empty.membership, 4, 4)
	if got.Source != "scheduler/affinity" || got.Provenance == "" || got.CPUs != 4 {
		t.Fatalf("fallback allowance = %+v, want scheduler/affinity with provenance", got)
	}
}

func TestCPUAllowanceCapacityFloorsAndNeverRoundsUp(t *testing.T) {
	for _, tc := range []struct {
		cpus      float64
		scheduler int
		want      int
	}{{0.0625, 4, 1}, {0.5, 4, 1}, {1.5, 4, 1}, {4.9, 8, 4}, {8, 2, 2}, {2, 0, 2}} {
		if got := (CPUAllowance{CPUs: tc.cpus, Scheduler: tc.scheduler}).Capacity(); got != tc.want {
			t.Fatalf("Capacity(%v CPUs, scheduler %d) = %d, want %d", tc.cpus, tc.scheduler, got, tc.want)
		}
	}
}

// TestAutoTuneWorkersUsesCgroupMemory proves pool sizing reads the container
// limit rather than host memory: the same model file yields one worker under a
// tight cgroup limit and the full CPU allowance without it.
func TestAutoTuneWorkersUsesCgroupMemory(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.onnx")
	const size = 1 << 20
	if err := os.WriteFile(model, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}

	origRoot, origInfo, origMember := cgroupRoot, cgroupMountinfo, cgroupMembership
	t.Cleanup(func() { cgroupRoot, cgroupMountinfo, cgroupMembership = origRoot, origInfo, origMember })

	// 8 CPUs, no memory limit → capped by cores, not memory.
	full := newCgroupFixture(t, []string{v2Mount}, "0::/\n")
	full.write("cpu.max", "800000 100000\n")
	cgroupRoot, cgroupMountinfo, cgroupMembership = full.dir, full.mountinfo, full.membership
	if got := autoTuneWorkers(model, 0); got != min(8, runtime.GOMAXPROCS(0)) {
		t.Fatalf("no memory limit: workers = %d, want 8 (bounded by the scheduler)", got)
	}

	// 8 CPUs but a limit just above the model → memory clamps to 1.
	tight := newCgroupFixture(t, []string{v2Mount}, "0::/\n")
	tight.write("cpu.max", "800000 100000\n")
	tight.write("memory.max", "2000000\n")
	cgroupRoot, cgroupMountinfo, cgroupMembership = tight.dir, tight.mountinfo, tight.membership
	if got := autoTuneWorkers(model, 0); got != 1 {
		t.Fatalf("tight memory limit: workers = %d, want 1", got)
	}
}
