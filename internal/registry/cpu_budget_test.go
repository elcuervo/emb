package registry

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
	"github.com/elcuervo/emb/internal/pipeline"
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
	one := modelThreadShares(models, 6)
	two := modelThreadShares(map[string]config.ModelConfig{"a": {Weight: &w}, "b": {}}, 6)
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
	sess := budgetedSession{Session: budgetTestSession{err: want}, gate: gate, width: 1}
	if _, err := sess.Run(nil, nil, 0, 0, 0); !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
	release := gate.Acquire(1)
	release()
}

// Two models share one process budget, so a busy model with a narrow run can
// lend idle capacity to another model rather than each holding a reservation.
func TestSharedBudgetLendsIdleCapacityAcrossModels(t *testing.T) {
	gate := permit.NewBudget(3)
	modelA := budgetedSession{Session: budgetTestSession{}, gate: gate, width: 1}
	modelB := budgetedSession{Session: budgetTestSession{}, gate: gate, width: 2}
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
// models keep legacy sizing.
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
			shares := modelThreadShares(models, budget)
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
