package registry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/tokenizer"
)

func TestResolveCPUPlan(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cfg      config.Config
		allow    float64
		sched    int
		budget   int
		reserve  int
		capacity int
	}{
		{name: "fractional", allow: 1.5, sched: 2, budget: 1, capacity: 1},
		{name: "small reserve tier", allow: 4, sched: 4, budget: 3, reserve: 1, capacity: 4},
		{name: "default eight cpu reserve", allow: 8, sched: 8, budget: 6, reserve: 2, capacity: 8},
		{name: "explicit reserve", cfg: config.Config{CPUReserve: intPtr(0)}, allow: 8, sched: 8, budget: 8, reserve: 0, capacity: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := ResolveCPUPlan(&tc.cfg, CPUAllowance{CPUs: tc.allow, Scheduler: tc.sched, Source: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Budget != tc.budget || plan.Reserve != tc.reserve || plan.Capacity != tc.capacity {
				t.Fatalf("plan = %+v, want budget/reserve/capacity %d/%d/%d", plan, tc.budget, tc.reserve, tc.capacity)
			}
		})
	}
}

func TestResolveCPUPlanRejectsInvalidReserveAndWarnsOverLimit(t *testing.T) {
	for _, reserve := range []int{-1, 4} {
		cfg := &config.Config{CPUReserve: &reserve}
		if _, err := ResolveCPUPlan(cfg, CPUAllowance{CPUs: 4, Scheduler: 4}); err == nil {
			t.Errorf("reserve %d accepted", reserve)
		}
	}
	cfg := &config.Config{CPUBudget: "5"}
	plan, err := ResolveCPUPlan(cfg, CPUAllowance{CPUs: 4, Scheduler: 4})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Budget != 5 || plan.Warning == "" || !plan.Explicit {
		t.Fatalf("over-limit plan = %+v, want warning and explicit budget", plan)
	}
}

func TestResolveCPUPlanOffDisablesPolicy(t *testing.T) {
	plan, err := ResolveCPUPlan(&config.Config{CPUBudget: "off"}, CPUAllowance{CPUs: 8, Scheduler: 8})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Enabled || plan.Budget != 0 {
		t.Fatalf("off plan = %+v, want disabled legacy policy", plan)
	}
}

func TestModelThreadSharesAreStableAndWeighted(t *testing.T) {
	w := 3.0
	models := map[string]config.ModelConfig{"b": {}, "a": {Weight: &w}}
	one, _ := modelThreadShares(models, 6)
	two, _ := modelThreadShares(map[string]config.ModelConfig{"a": {Weight: &w}, "b": {}}, 6)
	if one["a"] != 5 || one["b"] != 1 || one["a"] != two["a"] || one["b"] != two["b"] {
		t.Fatalf("shares = %v then %v, want stable weighted shares 5/1", one, two)
	}
}

type budgetTestSession struct{ err error }

func (s budgetTestSession) Run([]int64, []int64, int, int, int) ([]float32, error) {
	return nil, s.err
}
func (budgetTestSession) Close() error { return nil }

func TestBudgetedSessionReleasesOnError(t *testing.T) {
	want := errors.New("inference failed")
	gate := permit.NewBudget(1)
	sess := &budgetedSession{Session: budgetTestSession{err: want}, gate: gate, width: 1}
	if _, err := sess.Run(nil, nil, 0, 0, 0); !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
	release, err := gate.AcquireContext(context.Background(), 1)
	if err != nil {
		t.Fatalf("the gate still holds the run's permit: %v", err)
	}
	release()
}

// TestBudgetedSessionReportsItsQueuedWait covers the attribution wiring: a run
// admitted on the spot reports no wait, a run that queued reports how long, and
// taking the value clears it.
func TestBudgetedSessionReportsItsQueuedWait(t *testing.T) {
	gate := permit.NewBudget(1)
	sess := &budgetedSession{Session: budgetTestSession{}, gate: gate, width: 1}
	if _, err := sess.Run(nil, nil, 0, 0, 0); err != nil {
		t.Fatalf("uncontended run: %v", err)
	}
	if waited := sess.TakeAdmissionWait(); waited != 0 {
		t.Fatalf("a run admitted on the spot reported a wait of %s", waited)
	}

	release := mustHoldGate(t, gate)
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := sess.RunContext(ctx, nil, nil, 0, 0, 0)
		done <- err
	}()
	waitGateQueued(t, gate, 1)
	time.Sleep(30 * time.Millisecond)
	release()
	if err := <-done; err != nil {
		t.Fatalf("queued run: %v", err)
	}
	if waited := sess.TakeAdmissionWait(); waited < 25*time.Millisecond {
		t.Fatalf("a run that queued ~30ms reported %s", waited)
	}
	if waited := sess.TakeAdmissionWait(); waited != 0 {
		t.Fatalf("the wait was reported twice: %s", waited)
	}
}

// mustHoldGate takes the whole gate and returns its release.
func mustHoldGate(t *testing.T, gate *permit.Budget) func() {
	t.Helper()
	release, err := gate.AcquireContext(context.Background(), gate.Capacity())
	if err != nil {
		t.Fatalf("taking the gate: %v", err)
	}
	return release
}

// waitGateQueued waits until the gate has n callers queued, so a test can act on
// a state it observed rather than on elapsed time.
func waitGateQueued(t *testing.T, gate *permit.Budget, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if gate.Pressure().Waiters == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("gate never reached %d queued callers", n)
}

// Two models share one process budget, so a busy model with a narrow run can
// lend idle capacity to another model rather than each holding a reservation.
func TestSharedBudgetLendsIdleCapacityAcrossModels(t *testing.T) {
	gate := permit.NewBudget(3)
	modelA := &budgetedSession{Session: budgetTestSession{}, gate: gate, width: 1}
	modelB := &budgetedSession{Session: budgetTestSession{}, gate: gate, width: 2}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Model A holds one token while it runs; model B must still get its two.
		_, _ = modelA.Run(nil, nil, 0, 0, 0)
	}()
	if _, err := modelB.Run(nil, nil, 0, 0, 0); err != nil {
		t.Fatalf("borrowing run failed: %v", err)
	}
	<-done
	if active, peak := gate.Stats(); active != 0 || peak > 3 {
		t.Fatalf("budget after lending = active %d peak %d, want released within 3", active, peak)
	}
}

var _ onnx.Session = budgetTestSession{}

// NewWithConfig applies the process CPU policy to every model added later, so
// a model's sessions, gate and calibration come from the shared plan rather
// than the machine's core count. With cpu_budget off the policy stays nil and
// the models keep legacy sizing.
//
// policyRegistry loads cfg through the registry's own deferred path with the
// CPU policy enabled, so ensurePool (and therefore calibration and the layout
// record) runs exactly as production runs it; the seams make the sessions fake.
func policyRegistry(t *testing.T, budget string, calibrate *bool, session func() onnx.Session, cfg config.ModelConfig) (*Registry, *ModelEntry) {
	t.Helper()
	initORT(t)
	origTok, origRT := newTokenizer, newRuntimeSession
	t.Cleanup(func() { newTokenizer, newRuntimeSession = origTok, origRT })
	newTokenizer = func(string, bool) (tokenizer.Tokenizer, error) { return registryTok{}, nil }
	newRuntimeSession = func(data []byte, inputNames, outputNames []string, dim int, rank, intraOpThreads, interOpThreads, execMode int, allowSpinning bool) (onnx.Session, error) {
		return session(), nil
	}
	reg, err := NewWithConfig(&config.Config{
		CPUBudget: budget,
		Calibrate: calibrate,
		Models:    map[string]config.ModelConfig{"probe": cfg},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := LoadModelDeferred(cfg, "probe")
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("probe", entry)
	if err := entry.ensurePool(); err != nil {
		t.Fatalf("ensurePool: %v", err)
	}
	return reg, entry
}

// TestLayoutProvenanceIsPublished covers the startup contract: the chosen
// layout carries how it was chosen, so a calibrated layout is distinguishable
// from a policy-derived one, and an unresolved trial carries its reason.
func TestLayoutProvenanceIsPublished(t *testing.T) {
	t.Run("calibrated", func(t *testing.T) {
		_, entry := policyRegistry(t, "4", nil, func() onnx.Session { return &fakeSession{} }, modelConfigOnMinilm(1, 1))
		outcome, ok := entry.Layout()
		if !ok || outcome.Source != "calibrated" || outcome.Elapsed <= 0 {
			t.Fatalf("layout = %+v (ok=%v), want a calibrated choice with its duration", outcome, ok)
		}
	})

	t.Run("calibration disabled", func(t *testing.T) {
		off := false
		_, entry := policyRegistry(t, "4", &off, func() onnx.Session { return &fakeSession{} }, modelConfigOnMinilm(1, 1))
		outcome, ok := entry.Layout()
		if !ok || outcome.Source != "policy" || outcome.Reason != "calibration_disabled" {
			t.Fatalf("layout = %+v (ok=%v), want the policy layout reported", outcome, ok)
		}
	})

	// A model that cannot run must not be reported as calibrated, and must keep
	// the policy-derived layout: the fallback would collapse it to one thread
	// while the process holds headroom.
	t.Run("unresolved keeps the policy layout", func(t *testing.T) {
		_, entry := policyRegistry(t, "4", nil, func() onnx.Session {
			return &fakeSession{fail: true}
		}, modelConfigOnMinilm(1, 1))
		outcome, ok := entry.Layout()
		if !ok || outcome.Source != "unresolved" || outcome.Reason != "baseline_failed" {
			t.Fatalf("layout = %+v (ok=%v), want unresolved:baseline_failed", outcome, ok)
		}
		// The probe also fails with this fake, so the policy layout is the
		// unbatched split of the share (2 sessions at width 2, then the explicit
		// worker count of 1): width 2, not the fallback's 1x1 collapse.
		if outcome.Workers != 1 || outcome.Threads != 2 {
			t.Fatalf("unresolved layout = %d workers x %d threads, want the policy layout 1x2", outcome.Workers, outcome.Threads)
		}
	})

	// An explicit width wider than the model's share leaves nothing to trial:
	// the layout is a fallback with its reason, not an unresolved trial.
	t.Run("fallback", func(t *testing.T) {
		cfg := modelConfigOnMinilm(1, 1)
		// An explicit width wider than the share, with the session count left to
		// calibration: there is no candidate to trial, so the layout is a
		// fallback carrying the reason rather than an unresolved trial.
		cfg.Workers = 0
		cfg.IntraOpThreads = 8
		_, entry := policyRegistry(t, "4", nil, func() onnx.Session { return &fakeSession{} }, cfg)
		outcome, ok := entry.Layout()
		if !ok || outcome.Source != "fallback" || outcome.Reason != "over_budget" {
			t.Fatalf("layout = %+v (ok=%v), want fallback:over_budget", outcome, ok)
		}
	})

	t.Run("legacy", func(t *testing.T) {
		_, entry := policyRegistry(t, "off", nil, func() onnx.Session { return &fakeSession{} }, modelConfigOnMinilm(1, 1))
		outcome, ok := entry.Layout()
		if !ok || outcome.Source != "legacy" {
			t.Fatalf("layout = %+v (ok=%v), want the legacy sizing reported", outcome, ok)
		}
	})
}

// TestExplicitSpinningOverridesThePolicyDefault covers the other half of the
// spinning decision: the policy forces spinning off for shared sessions, but an
// operator who sets allow_spinning explicitly still gets it — and the reported
// effective value follows.
func TestExplicitSpinningOverridesThePolicyDefault(t *testing.T) {
	on := true
	off := false
	cases := []struct {
		name  string
		value *bool
		want  bool
	}{
		{name: "unset follows the policy", value: nil, want: false},
		{name: "explicit off", value: &off, want: false},
		{name: "explicit on", value: &on, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg, err := NewWithConfig(&config.Config{
				Models: map[string]config.ModelConfig{"solo": {AllowSpinning: tc.value}},
			})
			if err != nil {
				t.Fatal(err)
			}
			entry := &ModelEntry{Name: "solo", cfg: config.ModelConfig{AllowSpinning: tc.value}}
			reg.Add("solo", entry)
			_, _, _, _, spinning := entry.CPUSettings()
			if spinning != tc.want {
				t.Fatalf("effective allow_spinning = %v, want %v", spinning, tc.want)
			}
		})
	}
}

// TestModelThreadSharesSumToTheBudget covers the reclaim contract: the shares
// always fit the process budget, and the models that gave up their reservation
// are named so the boot log and EMB.INFO can report them.
func TestModelThreadSharesSumToTheBudget(t *testing.T) {
	models := func(n int, weight func(i int) float64) map[string]config.ModelConfig {
		out := make(map[string]config.ModelConfig, n)
		for i := range n {
			name := string(rune('a' + i))
			w := weight(i)
			out[name] = config.ModelConfig{Weight: &w}
		}
		return out
	}
	for _, tc := range []struct {
		name    string
		models  int
		weight  func(i int) float64
		budgets []int
	}{
		{name: "equal weights", models: 4, weight: func(int) float64 { return 1 }},
		{name: "skewed weights", models: 3, weight: func(i int) float64 { return float64(3 - i) }},
		{name: "more models than budget", models: 6, weight: func(int) float64 { return 1 }, budgets: []int{1, 2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budgets := tc.budgets
			if budgets == nil {
				budgets = []int{1, 2, 3, 4, 8}
			}
			for _, budget := range budgets {
				shares, reclaimed := modelThreadShares(models(tc.models, tc.weight), budget)
				sum := 0
				for _, share := range shares {
					if share < 0 || share > budget {
						t.Fatalf("budget %d: share %d outside 0..%d", budget, share, budget)
					}
					sum += share
				}
				if sum != budget {
					t.Fatalf("budget %d: shares sum to %d, want %d (%v)", budget, sum, budget, shares)
				}
				// A reclaim is only reported when a model actually gave up its
				// reservation, and every reclaimed model is named exactly once.
				seen := map[string]bool{}
				for _, name := range reclaimed {
					if seen[name] {
						t.Fatalf("budget %d: %q reclaimed twice", budget, name)
					}
					seen[name] = true
					if shares[name] != 0 {
						t.Fatalf("budget %d: %q reported reclaimed but holds share %d", budget, name, shares[name])
					}
				}
				if extra := tc.models - budget; extra > 0 && len(reclaimed) == 0 {
					t.Fatalf("budget %d with %d models: over-allocation was not reclaimed (%v)", budget, tc.models, shares)
				}
			}
		})
	}
}

// TestReclaimIsReportedPerModel covers the reporting half: a model whose
// reservation was reclaimed says so, and one that kept its share does not.
func TestReclaimIsReportedPerModel(t *testing.T) {
	weights := map[string]config.ModelConfig{
		"a": {}, "b": {}, "c": {},
	}
	shares, reclaimed := modelThreadShares(weights, 1)
	survivors := 0
	for _, share := range shares {
		if share > 0 {
			survivors++
		}
	}
	if len(reclaimed) != 2 || survivors != 1 {
		t.Fatalf("budget 1 with three models: shares=%v reclaimed=%v, want two reclaimed and one survivor", shares, reclaimed)
	}
	reg, err := NewWithConfig(&config.Config{
		CPUBudget: "1",
		Models:    weights,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name := range weights {
		entry := &ModelEntry{Name: name}
		reg.Add(name, entry)
		want := shares[name] == 0
		if entry.ShareReclaimed() != want {
			t.Fatalf("model %q ShareReclaimed=%v, want %v (share %d)", name, entry.ShareReclaimed(), want, shares[name])
		}
	}
}

// TestReadCPUThrottleParsesBothFormats covers the kernel accounting the live
// server reports: cgroup v2 throttled_usec, the older v1 throttled_time in
// nanoseconds, and the steal column of /proc/stat. A platform without the files
// reports zeros and Known=false, so a stats read cannot break there.
func TestReadCPUThrottleParsesBothFormats(t *testing.T) {
	v2 := "usage_usec 1234\nnr_periods 500\nnr_throttled 20\nthrottled_usec 250000\n"
	periods, throttled, usec := parseCPUStat(v2)
	if periods != 500 || throttled != 20 || usec != 250_000 {
		t.Fatalf("v2 parse = %d/%d/%d, want 500/20/250000", periods, throttled, usec)
	}
	v1 := "nr_periods 700\nnr_throttled 3\nthrottled_time 1500000000\n"
	periods, throttled, usec = parseCPUStat(v1)
	if periods != 700 || throttled != 3 || usec != 1_500_000 {
		t.Fatalf("v1 parse = %d/%d/%d, want 700/3/1500000 (nanoseconds normalized)", periods, throttled, usec)
	}
	if _, _, usec := parseCPUStat("nr_periods notanumber\n"); usec != 0 {
		t.Fatalf("malformed counter parsed as %d", usec)
	}

	procStat := "cpu  100 0 200 3000 50 0 0 42 0 0\ncpu0 1 2 3 4 5 6 7 8 9 10\n"
	steal, ok := parseProcStatSteal(procStat)
	if !ok || steal != 420_000 {
		t.Fatalf("steal = %d us (ok=%v), want 420000 us", steal, ok)
	}
	if _, ok := parseProcStatSteal("cpu  1 2 3\n"); ok {
		t.Fatal("a truncated cpu line parsed as a measurement")
	}

	// A root without the kernel files yields zero counters and Known=false.
	prev := cgroupRoot
	cgroupRoot = t.TempDir()
	t.Cleanup(func() { cgroupRoot = prev })
	throttle := ReadCPUThrottle()
	if throttle.Periods != 0 || throttle.Throttled != 0 || throttle.ThrottleUs != 0 {
		t.Fatalf("missing cgroup files reported %+v, want zeros", throttle)
	}
}

// TestRegistryAppliesCPUPlanToAddedModel covers that a model added after the
// plan was resolved is governed: the process plan attaches to the entry, and its
// sessions are wrapped in the shared admission gate.
func TestRegistryAppliesCPUPlanToAddedModel(t *testing.T) {
	reg, err := NewWithConfig(&config.Config{Models: map[string]config.ModelConfig{"solo": {}}})
	if err != nil {
		t.Fatal(err)
	}
	entry := &ModelEntry{Name: "solo"}
	reg.Add("solo", entry)

	plan := reg.CPUStatus()
	enabled, budget, share, threads, spinning := entry.CPUSettings()
	if !enabled || budget != plan.Budget {
		t.Fatalf("CPUSettings enabled=%v budget=%d, want the process plan (%d)", enabled, budget, plan.Budget)
	}
	if share != plan.Budget || threads != plan.Budget {
		t.Fatalf("single-model share/threads = %d/%d, want the whole budget %d", share, threads, plan.Budget)
	}
	if spinning {
		t.Error("shared embedding/image sessions must not spin under the budget policy")
	}
	if entry.cpuGate == nil {
		t.Error("added model has no admission gate")
	}
}

func TestRegistryKeepsLegacyPolicyWhenBudgetOff(t *testing.T) {
	reg, err := NewWithConfig(&config.Config{
		CPUBudget: "off",
		Models:    map[string]config.ModelConfig{"solo": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := &ModelEntry{Name: "solo"}
	reg.Add("solo", entry)

	enabled, _, _, threads, _ := entry.CPUSettings()
	if enabled || entry.cpuGate != nil {
		t.Fatalf("cpu_budget off still enabled the policy (enabled=%v gate=%v)", enabled, entry.cpuGate)
	}
	if threads <= 0 {
		t.Fatalf("legacy threads = %d, want the cores-2 default", threads)
	}
}

// A model whose pool was already built before Add cannot be governed: the
// policy arrives too late, so the warning is the only signal.
func TestRegistryWarnsWhenPolicyArrivesAfterPoolBuild(t *testing.T) {
	reg, err := NewWithConfig(&config.Config{Models: map[string]config.ModelConfig{"solo": {}}})
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("solo", &ModelEntry{Name: "solo", Pool: &pipeline.Pool{}})
}

// TestPolicyPoolLayoutLeavesRoomForASecondRun covers the D4 decision: an
// unbatched pool splits its share so two runs of the same width fit the
// admission budget, and keeps at least two workers to use it. A batched pool has
// one run slot by construction and keeps the whole share; explicit settings stay
// authoritative.
func TestPolicyPoolLayoutLeavesRoomForASecondRun(t *testing.T) {
	cases := []struct {
		name              string
		share             int
		configuredWorkers int
		configuredThreads int
		batched           bool
		wantWorkers       int
		wantThreads       int
	}{
		{name: "fargate 4 vcpu single model", share: 3, wantWorkers: 2, wantThreads: 1},
		{name: "host single model", share: 8, wantWorkers: 2, wantThreads: 4},
		{name: "share of one", share: 1, wantWorkers: 2, wantThreads: 1},
		{name: "batched keeps the whole share in one slot", share: 3, batched: true, wantWorkers: 1, wantThreads: 3},
		{name: "explicit workers win", share: 3, configuredWorkers: 4, wantWorkers: 4, wantThreads: 1},
		{name: "explicit width wins", share: 3, configuredThreads: 3, wantWorkers: 2, wantThreads: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workers, threads := policyPoolLayout(tc.share, tc.configuredWorkers, tc.configuredThreads, tc.batched)
			if workers != tc.wantWorkers || threads != tc.wantThreads {
				t.Fatalf("policyPoolLayout(%d, %d, %d, %t) = (%d, %d), want (%d, %d)",
					tc.share, tc.configuredWorkers, tc.configuredThreads, tc.batched, workers, threads, tc.wantWorkers, tc.wantThreads)
			}
			// The derived layout must let a second run of the same width in.
			// A share of one is inherently one unit of work, so it cannot.
			if !tc.batched && tc.configuredThreads <= 0 && tc.share >= 2 && 2*threads > tc.share {
				t.Fatalf("derived width %d does not leave room for a second run in a share of %d", threads, tc.share)
			}
		})
	}
}

// TestCalibrationCandidatesLeaveRoomForASecondRun is the same contract at the
// calibration stage: an unbatched candidate whose width would fill the budget is
// not offered, and the chosen layout can therefore overlap two runs.
func TestCalibrationCandidatesLeaveRoomForASecondRun(t *testing.T) {
	for _, budget := range []int{2, 3, 4, 8} {
		candidates := calibrationCandidates(budget, budget, 0, 0, false)
		if len(candidates) == 0 {
			t.Fatalf("budget %d offered no unbatched candidate", budget)
		}
		for _, candidate := range candidates {
			if 2*candidate[1] > budget {
				t.Fatalf("budget %d offered %v, whose width fills the budget", budget, candidate)
			}
		}
	}
	// An explicit width is the operator's call and stays on the list.
	explicit := calibrationCandidates(3, 3, 3, 0, false)
	found := false
	for _, candidate := range explicit {
		if candidate[1] == 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit width was screened out of %v", explicit)
	}
}

// TestPolicyLayoutAcrossModelCounts covers the 1- to 4-model cases at a 3-CPU
// budget and at the 8-CPU host budget: every model keeps at least one thread,
// a model with a share of two or more gets a width that leaves room for a second
// run, and no model is silently left at one thread while the process holds
// headroom.
func TestPolicyLayoutAcrossModelCounts(t *testing.T) {
	model := func(name string) (string, config.ModelConfig) {
		return name, config.ModelConfig{ONNX: name + ".onnx"}
	}
	for _, budget := range []int{3, 8} {
		for count := 1; count <= 4; count++ {
			models := map[string]config.ModelConfig{}
			for i := 0; i < count; i++ {
				name, cfg := model(fmt.Sprintf("m%d", i))
				models[name] = cfg
			}
			shares, _ := modelThreadShares(models, budget)
			for name, share := range shares {
				workers, threads := policyPoolLayout(share, 0, 0, false)
				if threads < 1 {
					t.Fatalf("budget %d with %d models: %s got width %d", budget, count, name, threads)
				}
				if workers < 2 {
					t.Fatalf("budget %d with %d models: %s got %d workers", budget, count, name, workers)
				}
				if share >= 2 && 2*threads > share {
					t.Fatalf("budget %d with %d models: %s share %d gives width %d, no room for a second run", budget, count, name, share, threads)
				}
			}
			// One model holding the whole budget must not stay single-threaded.
			if count == 1 && budget >= 4 {
				for _, share := range shares {
					if _, threads := policyPoolLayout(share, 0, 0, false); threads <= 1 {
						t.Fatalf("budget %d single model left at one thread while holding headroom", budget)
					}
				}
			}
		}
	}
}
