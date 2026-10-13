package registry_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elcuervo/emb/bench/hostcheck"
	"github.com/elcuervo/emb/internal/registry"
)

// writeCgroup builds a cgroup v2 layout: root/cpu.max, a mountinfo that mounts
// it, and the cgroup membership file that points at it. An empty quota writes
// no cpu.max at all, the shape of a host with no cgroup CPU file.
func writeCgroup(t *testing.T, membership, quota string) (mountinfo, membershipPath string) {
	t.Helper()
	root := t.TempDir()
	if quota != "" {
		if err := os.WriteFile(filepath.Join(root, "cpu.max"), []byte(quota+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := t.TempDir()
	mountinfo = filepath.Join(files, "mountinfo")
	if err := os.WriteFile(mountinfo, []byte("30 25 0:26 / "+root+" rw - cgroup2 cgroup2 rw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	membershipPath = filepath.Join(files, "cgroup")
	if err := os.WriteFile(membershipPath, []byte(membership), 0o644); err != nil {
		t.Fatal(err)
	}
	return mountinfo, membershipPath
}

// reportFor derives the host report's CPU fields from a synthetic cgroup
// layout through the server's own resolution, so the contract's verdict is
// taken on exactly what the server would plan from.
func reportFor(t *testing.T, mountinfo, membership string, presented, affinity int) hostcheck.Report {
	t.Helper()
	allowance, budget := registry.CgroupViewForTest(mountinfo, membership, presented, affinity)
	r := hostcheck.Report{
		OS:            "linux",
		Arch:          "arm64",
		PresentedCPUs: presented,
		Allowance: hostcheck.AllowanceView{
			CPUs:      allowance.CPUs,
			Source:    allowance.Source,
			Quota:     allowance.Source != "" && allowance.Source != "scheduler/affinity",
			Scheduler: allowance.Scheduler,
			Affinity:  allowance.Affinity,
		},
		Memory:       hostcheck.MemoryView{LimitBytes: 8 << 30, HostBytes: 8 << 30, Source: "cgroup"},
		ServerBudget: budget,
		Steal:        hostcheck.StealView{Measurable: true, Percent: 0.1, Verdict: hostcheck.VerdictOK},
		Throttle:     hostcheck.ThrottleView{Measurable: true, Verdict: hostcheck.VerdictOK},
		Thresholds:   hostcheck.DefaultThresholds(),
	}
	return r
}

// TestHostContractOverSyntheticCgroupLayouts drives the validation host
// contract from synthetic cgroup layouts, through registry.EffectiveCPUAllowance's
// own resolution. It is the check that the report and the server cannot
// disagree: both read the view this resolution produces.
func TestHostContractOverSyntheticCgroupLayouts(t *testing.T) {
	t.Run("sub-one-CPU quota is unqualified and names the quota", func(t *testing.T) {
		// The Fly shared-cpu-* shape: 5 ms of CPU every 80 ms, which the
		// integer budget floors to one CPU while the host presents four.
		mountinfo, membership := writeCgroup(t, "0::/\n", "5000 80000")
		r := reportFor(t, mountinfo, membership, 4, 4)
		r.Qualify()
		if r.Qualified() {
			t.Fatalf("verdict = %s, want unqualified", r.Verdict)
		}
		if !strings.Contains(r.FailedProperty, "cpu quota") {
			t.Fatalf("failed property = %q, want the cpu quota", r.FailedProperty)
		}
		if !strings.Contains(r.FailedProperty, "server budget 1") {
			t.Fatalf("failed property = %q, want the budget derived from the quota", r.FailedProperty)
		}
		if r.Allowance.CPUs != 0.0625 {
			t.Fatalf("allowance = %v CPUs, want the 0.0625 the quota states", r.Allowance.CPUs)
		}
	})

	t.Run("quota equal to the presented CPUs is qualified", func(t *testing.T) {
		mountinfo, membership := writeCgroup(t, "0::/\n", "400000 100000")
		r := reportFor(t, mountinfo, membership, 4, 4)
		r.Qualify()
		if !r.Qualified() {
			t.Fatalf("verdict = %s (%v), want qualified", r.Verdict, r.FailedProperties)
		}
		if r.ServerBudget != 4 {
			t.Fatalf("server budget = %d, want 4", r.ServerBudget)
		}
	})

	t.Run("missing cgroup file reports the scheduler/affinity source", func(t *testing.T) {
		mountinfo, membership := writeCgroup(t, "0::/\n", "")
		r := reportFor(t, mountinfo, membership, 4, 4)
		r.Qualify()
		if r.Allowance.Quota {
			t.Fatal("the contract reported a quota the layout does not have")
		}
		if r.Allowance.Source != "scheduler/affinity" {
			t.Fatalf("allowance source = %q, want scheduler/affinity", r.Allowance.Source)
		}
		if !r.Qualified() {
			t.Fatalf("verdict = %s (%v), want qualified", r.Verdict, r.FailedProperties)
		}
	})
}
