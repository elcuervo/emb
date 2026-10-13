package registry

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

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
	gate := permit.NewBudget(1, nil)
	sess := newBudgetedSession(budgetTestSession{err: want}, gate, "m", 1, nil)
	if _, err := sess.Run(nil, nil, 0, 0, 0); !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
	release, err := gate.Acquire(context.Background(), "m", 1)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

type namedTestSession struct{ err error }

func (s namedTestSession) RunNamed([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	return nil, s.err
}
func (namedTestSession) Close() error { return nil }

// A named-tensor run that fails still gives back both its session charge and
// any caller charge.
func TestBudgetedNamedSessionReleasesOnError(t *testing.T) {
	want := errors.New("run failed")
	gate := permit.NewBudget(2, nil)
	sess := budgetedNamedSession{NamedSession: namedTestSession{err: want}, shared: gate.Shared("m", 2)}
	if _, err := sess.RunNamed(nil); !errors.Is(err, want) {
		t.Fatalf("RunNamed error = %v, want %v", err, want)
	}
	if active, _ := gate.Stats(); active != 0 {
		t.Fatalf("active = %d after a failed run, want 0", active)
	}
}

// Two models share one process budget, so a busy model with a narrow run can
// lend idle capacity to another model rather than each holding a reservation.
func TestSharedBudgetLendsIdleCapacityAcrossModels(t *testing.T) {
	gate := permit.NewBudget(3, nil)
	modelA := newBudgetedSession(budgetTestSession{}, gate, "a", 1, nil)
	modelB := newBudgetedSession(budgetTestSession{}, gate, "b", 2, nil)
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

// waitGateWaiters spins until n runs queue at the gate.
func waitGateWaiters(t *testing.T, gate *permit.Budget, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, waiters, _ := gate.Pressure(); waiters == n {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("gate waiters did not reach %d", n)
}

// The wait at the gate is reported once, as the path's admission wait, and
// handed to the pipeline to keep out of run time.
func TestBudgetedSessionReportsGateWait(t *testing.T) {
	gate := permit.NewBudget(1, nil)
	hold, err := gate.Acquire(context.Background(), "m", 1)
	if err != nil {
		t.Fatal(err)
	}
	var total atomic.Int64
	sess := newBudgetedSession(budgetTestSession{}, gate, "m", 1, &total)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = sess.Run(nil, nil, 0, 0, 0)
	}()
	waitGateWaiters(t, gate, 1)
	hold()
	<-done
	taken := sess.TakeAdmissionWait()
	if taken.Microseconds() != total.Load() {
		t.Fatalf("taken %v != path total %dus", taken, total.Load())
	}
	if again := sess.TakeAdmissionWait(); again != 0 {
		t.Fatalf("second take = %v, want 0", again)
	}
}

// A refused or expired admission surfaces from the run without touching the
// native session.
func TestBudgetedSessionRefusedAdmissionNeverRuns(t *testing.T) {
	gate := permit.NewBudget(1, nil)
	gate.SetTimeout(time.Millisecond)
	hold, err := gate.Acquire(context.Background(), "m", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	ran := errors.New("native session ran")
	sess := newBudgetedSession(budgetTestSession{err: ran}, gate, "m", 1, nil)
	if _, err := sess.Run(nil, nil, 0, 0, 0); !errors.Is(err, permit.ErrAdmissionTimeout) {
		t.Fatalf("Run error = %v, want the admission timeout (not %v)", err, ran)
	}
}

// gateWaitNamed is a named session that reports its whole 2ms call as gate wait.
type gateWaitNamed struct{ namedTestSession }

func (gateWaitNamed) runAdmitted([]onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error) {
	start := time.Now()
	for time.Since(start) < 2*time.Millisecond {
	}
	return nil, time.Since(start), nil
}

func TestScriptRunExcludesGateWait(t *testing.T) {
	res := idleScriptResources(gateWaitNamed{})
	if _, err := res.RunNamed(nil); err != nil {
		t.Fatal(err)
	}
	_, runUs, runs, _ := res.DispatchCounters()
	if runs != 1 || runUs >= 1000 {
		t.Fatalf("runs=%d runUs=%d, want the 2ms gate wait kept out of run time", runs, runUs)
	}
}

type refusedNamed struct{ namedTestSession }

func (refusedNamed) runAdmitted([]onnx.NamedTensor) (map[string]onnx.NamedTensor, time.Duration, error) {
	return nil, 0, permit.ErrAdmissionTimeout
}

func TestScriptRefusedAdmissionIsNotARun(t *testing.T) {
	res := idleScriptResources(refusedNamed{})
	if _, err := res.RunNamed(nil); !errors.Is(err, permit.ErrAdmissionTimeout) {
		t.Fatalf("RunNamed error = %v", err)
	}
	if _, _, runs, _ := res.DispatchCounters(); runs != 0 {
		t.Fatalf("runs = %d, want 0 for a refused admission", runs)
	}
}

// Shutdown with one queued and one running call: the queued call fails and
// leaves the queue, the running call keeps its token and finishes normally.
func TestRegistryCloseCancelsQueuedAdmissionNotRunning(t *testing.T) {
	reg, err := NewWithConfig(&config.Config{Models: map[string]config.ModelConfig{"m": {}}})
	if err != nil {
		t.Fatal(err)
	}
	gate := reg.cpuGate
	running, err := gate.Acquire(context.Background(), "m", gate.Capacity())
	if err != nil {
		t.Fatal(err)
	}
	queued := make(chan error, 1)
	go func() {
		release, err := gate.Acquire(context.Background(), "m", 1)
		if err == nil {
			release()
		}
		queued <- err
	}()
	waitGateWaiters(t, gate, 1)
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-queued; !errors.Is(err, permit.ErrAdmissionClosed) {
		t.Fatalf("queued error = %v, want ErrAdmissionClosed", err)
	}
	if plan := reg.CPUStatus(); plan.Active != gate.Capacity() || plan.AdmissionWaiters != 0 {
		t.Fatalf("after Close: active %d waiters %d, want the running call still holding %d", plan.Active, plan.AdmissionWaiters, gate.Capacity())
	}
	running()
	if plan := reg.CPUStatus(); plan.Active != 0 {
		t.Fatalf("tokens leaked: %d", plan.Active)
	}
}

func TestPathStatesDoNotInitializeLazyPaths(t *testing.T) {
	e := &ModelEntry{Name: "m", cfg: config.ModelConfig{Dim: 4, Pooling: "mean"}}
	embed, script, image := e.PathStates()
	if embed != PathUninitialized || script != PathUninitialized || image != PathUnavailable {
		t.Fatalf("lazy states = %s/%s/%s", embed, script, image)
	}
	e.scriptResPtr.Store(idleScriptResources(namedTestSession{}))
	e.poolReady.Store(true)
	embed, script, _ = e.PathStates()
	if embed != PathInitialized || script != PathInitialized {
		t.Fatalf("loaded states = %s/%s", embed, script)
	}
	if e.loaded.Load() || e.scriptRes != nil {
		t.Fatal("observing path state initialized a path")
	}
}
