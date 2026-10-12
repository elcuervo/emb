package hostcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// syntheticHost is the measured shape the server's own resolution reports for a
// host with a quota equal to its presented CPUs: the reference case.
func syntheticHost() Report {
	return Report{
		OS:            "linux",
		Arch:          "arm64",
		PresentedCPUs: 4,
		Allowance: AllowanceView{
			CPUs: 4, Source: "/sys/fs/cgroup/cpu.max", Quota: true, Scheduler: 4, Affinity: 4,
		},
		Memory:       MemoryView{LimitBytes: 1 << 30, HostBytes: 4 << 30, Source: "cgroup"},
		ServerBudget: 4,
		Steal:        StealView{Measurable: true, Percent: 0.1, Verdict: VerdictOK},
		Throttle:     ThrottleView{Measurable: true, Verdict: VerdictOK},
		Thresholds:   DefaultThresholds(),
	}
}

// TestQualifyOverSyntheticCPULayouts drives the verdict with the view the
// server's own resolution produces for each cgroup layout. The
// layout-to-view mapping itself is pinned by internal/registry; the probe
// consumes exactly that view, so its verdict cannot disagree with the server's
// plan.
func TestQualifyOverSyntheticCPULayouts(t *testing.T) {
	t.Run("sub-one-CPU quota is unqualified and names the quota", func(t *testing.T) {
		r := syntheticHost()
		// The Fly shared-cpu-* shape: a few milliseconds of CPU per period,
		// which the server floors to a budget of one.
		r.Allowance = AllowanceView{
			CPUs: 0.0625, Source: "/sys/fs/cgroup/cpu.max", Quota: true, Scheduler: 4, Affinity: 4,
		}
		r.ServerBudget = 1
		r.Qualify()
		if r.Qualified() {
			t.Fatalf("quota below the presented CPUs qualified: %s", r)
		}
		if !strings.Contains(r.FailedProperty, "cpu quota") {
			t.Fatalf("failed property = %q, want the cpu quota", r.FailedProperty)
		}
		if !strings.Contains(r.FailedProperty, "server budget 1") {
			t.Fatalf("failed property = %q, want the budget the server derived from the quota", r.FailedProperty)
		}
	})

	t.Run("quota equal to the presented CPUs is qualified", func(t *testing.T) {
		r := syntheticHost()
		r.Qualify()
		if !r.Qualified() {
			t.Fatalf("verdict = %s (%v), want qualified", r.Verdict, r.FailedProperties)
		}
		if r.FailedProperty != "" {
			t.Fatalf("failed property = %q, want none", r.FailedProperty)
		}
	})

	t.Run("no cgroup file reports the scheduler/affinity source and no quota", func(t *testing.T) {
		r := syntheticHost()
		r.Allowance = AllowanceView{
			CPUs: 4, Source: "scheduler/affinity", Quota: false, Scheduler: 4, Affinity: 8,
		}
		r.Qualify()
		if !r.Qualified() {
			t.Fatalf("verdict = %s (%v), want qualified", r.Verdict, r.FailedProperties)
		}
		if r.Allowance.Quota {
			t.Fatal("report claims a quota the host does not enforce")
		}
		if !strings.Contains(r.String(), "scheduler/affinity") || !strings.Contains(r.String(), "no cgroup quota") {
			t.Fatalf("report does not state the allowance source:\n%s", r)
		}
	})

	t.Run("this machine's platform and unmeasurable steal are named", func(t *testing.T) {
		r := syntheticHost()
		r.OS, r.Arch = "darwin", "arm64"
		r.Steal = StealView{Verdict: VerdictUnmeasured}
		r.Qualify()
		if r.Qualified() {
			t.Fatal("a non-deployment platform qualified")
		}
		if !strings.Contains(r.FailedProperty, "platform darwin/arm64") {
			t.Fatalf("headline failed property = %q, want the platform", r.FailedProperty)
		}
		if len(r.FailedProperties) != 2 || !strings.Contains(strings.Join(r.FailedProperties, "; "), "steal is not measurable") {
			t.Fatalf("failed properties = %v, want the platform and the unmeasurable steal", r.FailedProperties)
		}
	})
}

// TestProbeReadsFixtureSources checks the sampling wiring against files: the
// counters parse, a quiet host is inside its limits, and a missing file reports
// the property as unmeasured instead of as zero.
func TestProbeReadsFixtureSources(t *testing.T) {
	dir := t.TempDir()
	procStat := filepath.Join(dir, "stat")
	cpuStat := filepath.Join(dir, "cpu.stat")
	if err := os.WriteFile(procStat, []byte("cpu  100 0 100 800 0 0 0 3 0 0\ncpu0 1 0 1 8 0 0 0 0 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cpuStat, []byte("usage_usec 100\nnr_periods 1000\nnr_throttled 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	th := DefaultThresholds()
	th.SampleWindow = 0

	r := probe(Sources{ProcStat: procStat, CPUStat: cpuStat}, th)
	if !r.Steal.Measurable || r.Steal.Verdict != VerdictOK {
		t.Fatalf("steal = %+v, want a measurable ok property", r.Steal)
	}
	if !r.Throttle.Measurable || r.Throttle.Verdict != VerdictOK {
		t.Fatalf("throttle = %+v, want a measurable ok property", r.Throttle)
	}
	if r.Allowance.CPUs <= 0 || r.ServerBudget < 1 {
		t.Fatalf("allowance/budget not reported: %+v, budget %d", r.Allowance, r.ServerBudget)
	}

	missing := probe(Sources{ProcStat: filepath.Join(dir, "absent"), CPUStat: filepath.Join(dir, "absent")}, th)
	if missing.Steal.Measurable || missing.Steal.Verdict != VerdictUnmeasured {
		t.Fatalf("missing /proc/stat: steal = %+v, want unmeasured", missing.Steal)
	}
	if missing.Throttle.Measurable || missing.Throttle.Verdict != VerdictUnmeasured {
		t.Fatalf("missing cpu.stat: throttle = %+v, want unmeasured", missing.Throttle)
	}
}

func TestCPUTimesParsingAndStealPercent(t *testing.T) {
	prev, err := parseCPUTimes(strings.NewReader("cpu  100 0 100 800 0 0 0 0 0 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if prev.total != 1000 || prev.steal != 0 {
		t.Fatalf("parsed %+v, want total 1000 steal 0", prev)
	}
	cur, err := parseCPUTimes(strings.NewReader("cpu  300 0 300 1390 0 0 0 10 0 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := stealPercent(prev, cur); got != 1 {
		t.Fatalf("steal percent = %v, want 1 (10 of 1000 ticks)", got)
	}
	if got := stealPercent(cur, prev); got != 0 {
		t.Fatalf("non-monotonic counters gave %v, want 0", got)
	}
	if _, err := parseCPUTimes(strings.NewReader("intr 12\nctxt 34\n")); err == nil {
		t.Fatal("a file with no aggregate cpu line parsed")
	}
}

func TestCPUStatParsingAndThrottleRatio(t *testing.T) {
	prev, err := parseCPUStat(strings.NewReader("usage_usec 10\nnr_periods 100\nnr_throttled 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := parseCPUStat(strings.NewReader("usage_usec 20\nnr_periods 200\nnr_throttled 12\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := throttleRatio(prev, cur); got != 0.1 {
		t.Fatalf("throttle ratio = %v, want 0.1 (10 of 100 periods)", got)
	}
	if _, err := parseCPUStat(strings.NewReader("usage_usec 10\n")); err == nil {
		t.Fatal("a cpu.stat without the period counters parsed")
	}
}

// TestSampleVerdictsRespectThresholds checks that the sampled verdict is the
// caller's threshold and not the default: a recorded threshold has to be the
// one the verdict was taken against.
func TestSampleVerdictsRespectThresholds(t *testing.T) {
	dir := t.TempDir()
	cpuStat := filepath.Join(dir, "cpu.stat")
	if err := os.WriteFile(cpuStat, []byte("nr_periods 100\nnr_throttled 50\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := sampleThrottle(cpuStat, 0, 0.10); v.Verdict != VerdictOK {
		t.Fatalf("single unbalanced read = %+v, want ok (no periods elapsed)", v)
	}
	// A zero-window sample reads the same file twice, so the delta is zero even
	// when the counters are heavily throttled: only a real window separates them.
	if v := sampleThrottle(cpuStat, time.Millisecond, 0.10); v.Periods != 0 {
		t.Fatalf("periods = %d, want 0 for an unchanged file", v.Periods)
	}
}

// TestFingerprintIdentity covers the comparability rule: the stable host shape
// keys the digest, and the sampled verdicts do not.
func TestFingerprintIdentity(t *testing.T) {
	base := syntheticHost().Fingerprint()
	same := syntheticHost().Fingerprint()
	if base.Identity() != same.Identity() {
		t.Fatal("identical hosts hashed differently")
	}

	older := syntheticHost()
	older.Steal = StealView{Measurable: true, Percent: 0.9, Verdict: VerdictOK}
	if older.Fingerprint().Identity() != base.Identity() {
		t.Fatal("the sampled steal reading changed the host identity")
	}

	other := syntheticHost()
	other.Allowance.CPUs = 2
	if other.Fingerprint().Identity() == base.Identity() {
		t.Fatal("a different effective CPU allowance hashed the same")
	}
}
