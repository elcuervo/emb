package registry

import (
	"errors"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
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

var _ onnx.Session = budgetTestSession{}
