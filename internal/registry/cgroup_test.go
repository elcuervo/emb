package registry

import (
	"os"
	"path/filepath"
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

func TestParseCPUQuota(t *testing.T) {
	if got := parseCPUQuota([]string{"200000", "100000"}); got != 2 {
		t.Fatalf("quota 200000/100000 = %d, want 2", got)
	}
	if got := parseCPUQuota([]string{"150000", "100000"}); got != 1 {
		t.Fatalf("quota 150000/100000 = %d, want 1 (floor)", got)
	}
	if got := parseCPUQuota([]string{"max", "100000"}); got != 0 {
		t.Fatalf("max quota = %d, want 0", got)
	}
	if got := parseCPUQuota([]string{"100000"}); got != 0 {
		t.Fatalf("short fields = %d, want 0", got)
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
	quota := resolveCPUAllowance(1.5, "cgroup", 4, 8)
	if quota.CPUs != 1.5 || quota.Scheduler != 4 || quota.Affinity != 8 {
		t.Fatalf("quota allowance = %+v, want fractional quota and distinct limits", quota)
	}
	fallback := resolveCPUAllowance(0, "", 2, 8)
	if fallback.CPUs != 2 || fallback.Source != "scheduler/affinity" || fallback.Scheduler != 2 {
		t.Fatalf("scheduler-limited allowance = %+v, want 2 CPUs", fallback)
	}
}

func TestReadCgroupCPUQuota(t *testing.T) {
	v2 := t.TempDir()
	os.WriteFile(filepath.Join(v2, "cpu.max"), []byte("400000 100000\n"), 0o644)
	if got := readCgroupCPUQuota(v2); got != 4 {
		t.Fatalf("v2 cpu.max = %d, want 4", got)
	}

	v1 := t.TempDir()
	os.MkdirAll(filepath.Join(v1, "cpu"), 0o755)
	os.WriteFile(filepath.Join(v1, "cpu", "cpu.cfs_quota_us"), []byte("600000\n"), 0o644)
	os.WriteFile(filepath.Join(v1, "cpu", "cpu.cfs_period_us"), []byte("100000\n"), 0o644)
	if got := readCgroupCPUQuota(v1); got != 6 {
		t.Fatalf("v1 quota = %d, want 6", got)
	}

	if got := readCgroupCPUQuota(t.TempDir()); got != 0 {
		t.Fatalf("absent quota = %d, want 0", got)
	}
}

func TestReadCurrentCPUQuotaKeepsFractionAndParentLimit(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "parent", "leaf")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cpu.max"), []byte("200000 100000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "parent", "cpu.max"), []byte("150000 100000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leaf, "cpu.max"), []byte("300000 100000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	membership := filepath.Join(t.TempDir(), "cgroup")
	if err := os.WriteFile(membership, []byte("0::/parent/leaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, source := readCurrentCPUQuota(root, membership)
	if got != 1.5 || source != filepath.Join(root, "parent", "cpu.max") {
		t.Fatalf("quota = %v, source = %q; want 1.5 from parent", got, source)
	}
}

func TestReadCurrentCPUQuotaV1(t *testing.T) {
	root := t.TempDir()
	cpuRoot := filepath.Join(root, "cpu")
	leaf := filepath.Join(cpuRoot, "parent", "leaf")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, quota := range map[string]string{cpuRoot: "200000", filepath.Join(cpuRoot, "parent"): "150000", leaf: "300000"} {
		if err := os.WriteFile(filepath.Join(dir, "cpu.cfs_quota_us"), []byte(quota), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cpu.cfs_period_us"), []byte("100000"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	membership := filepath.Join(t.TempDir(), "cgroup")
	if err := os.WriteFile(membership, []byte("5:cpu,cpuacct:/parent/leaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, source := readCurrentCPUQuota(root, membership)
	if got != 1.5 || source != filepath.Join(cpuRoot, "parent", "cpu.cfs_quota_us") {
		t.Fatalf("quota = %v, source = %q; want 1.5 from v1 parent", got, source)
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

	orig := cgroupRoot
	t.Cleanup(func() { cgroupRoot = orig })

	// 8 CPUs, no memory limit → capped by cores, not memory.
	full := t.TempDir()
	os.WriteFile(filepath.Join(full, "cpu.max"), []byte("800000 100000\n"), 0o644)
	cgroupRoot = full
	if got := autoTuneWorkers(model, 0); got != 8 {
		t.Fatalf("no memory limit: workers = %d, want 8", got)
	}

	// 8 CPUs but a limit just above the model → memory clamps to 1.
	tight := t.TempDir()
	os.WriteFile(filepath.Join(tight, "cpu.max"), []byte("800000 100000\n"), 0o644)
	os.WriteFile(filepath.Join(tight, "memory.max"), []byte("2000000\n"), 0o644)
	cgroupRoot = tight
	if got := autoTuneWorkers(model, 0); got != 1 {
		t.Fatalf("tight memory limit: workers = %d, want 1", got)
	}
}
