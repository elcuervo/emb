package registry

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
)

// layoutEntry adds a model to a registry with the given process budget and
// returns it, ready for planCPULayout.
func layoutEntry(t *testing.T, budget string, calibrate *bool, name string, models ...string) (*Registry, *ModelEntry) {
	t.Helper()
	cfg := &config.Config{CPUBudget: budget, Calibrate: calibrate, Models: map[string]config.ModelConfig{}}
	for _, m := range models {
		cfg.Models[m] = config.ModelConfig{}
	}
	reg, err := NewWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var entry *ModelEntry
	for _, m := range models {
		e := &ModelEntry{Name: m, cfg: config.ModelConfig{Dim: 3, MaxLength: 32}}
		reg.Add(m, e)
		if m == name {
			entry = e
		}
	}
	return reg, entry
}

func widthScales(threads int) calFakeSession {
	return calFakeSession{dur: time.Duration(16/threads) * time.Millisecond}
}

func plan(t *testing.T, e *ModelEntry, cfg config.ModelConfig, batched bool, sessions func(int) calFakeSession) (cpuLayout, error) {
	t.Helper()
	factory := func(threads int) (onnx.Session, error) { return sessions(threads), nil }
	return e.planCPULayout(&cfg, registryTok{}, factory, batched)
}

// An idle configured peer must not cap the active model: with a budget of 4 and
// two equal-weight models each model's share is 2, but the candidate ceiling is
// the process budget, so a width-scaling batched model reaches 4.
func TestCalibrationCeilingIgnoresIdlePeerShare(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	_, busy := layoutEntry(t, "4", nil, "busy", "busy", "idle")
	if busy.cpuThreads != 2 {
		t.Fatalf("share = %d, want the equal split 2", busy.cpuThreads)
	}
	layout, err := plan(t, busy, config.ModelConfig{Dim: 3, MaxLength: 32}, true, widthScales)
	if err != nil {
		t.Fatal(err)
	}
	defer layout.releaseMemory()
	if layout.Threads != 4 || layout.Ceiling != 4 || layout.Source != "calibrated" {
		t.Fatalf("layout = %+v, want the width-4 winner under the process ceiling 4, not the share 2", layout.calibratedLayout)
	}
	if got := layout.outcome(); got.Workers != layout.Workers || got.Ceiling != 4 || got.Source != "calibrated" {
		t.Fatalf("outcome = %+v, want the immutable record of %+v", got, layout.calibratedLayout)
	}
}

// calibrate: false is a documented process-capable layout, not the old
// share-sized one and not the governor off.
func TestCalibrateFalseSelectsProcessCapableLayout(t *testing.T) {
	off := false
	_, entry := layoutEntry(t, "4", &off, "a", "a", "b")
	layout, err := plan(t, entry, config.ModelConfig{Dim: 3, MaxLength: 32}, false, widthScales)
	if err != nil {
		t.Fatal(err)
	}
	defer layout.releaseMemory()
	if layout.Workers != 1 || layout.Threads != 4 || layout.Source != "default" || layout.Reason != "calibration_disabled" {
		t.Fatalf("layout = %+v, want one session spanning the budget 4 (share is 2)", layout.calibratedLayout)
	}
	workers, err := plan(t, entry, config.ModelConfig{Dim: 3, MaxLength: 32, Workers: 2}, false, widthScales)
	if err != nil {
		t.Fatal(err)
	}
	defer workers.releaseMemory()
	if workers.Workers != 2 || workers.Threads != 2 {
		t.Fatalf("explicit workers layout = %+v, want 2 sessions of budget/2 = 2 threads", workers.calibratedLayout)
	}

	// cpu_budget: off is a different thing: no policy, no outcome.
	reg, err := NewWithConfig(&config.Config{CPUBudget: "off", Calibrate: &off, Models: map[string]config.ModelConfig{"a": {}}})
	if err != nil {
		t.Fatal(err)
	}
	legacy := &ModelEntry{Name: "a"}
	reg.Add("a", legacy)
	if _, ok := legacy.Calibration(); ok || legacy.cpuPolicy {
		t.Fatal("cpu_budget off produced a CPU layout outcome")
	}
}

// limitFor returns a memory limit whose usable budget (limit less the safety
// margin) is between headroom and headroom+1 bytes.
func limitFor(headroom uint64) uint64 {
	limit := headroom * memorySafetyDivisor / (memorySafetyDivisor - 1)
	for limit-limit/memorySafetyDivisor < headroom {
		limit++
	}
	return limit
}

func withMemory(t *testing.T, limit, inUse uint64) {
	t.Helper()
	prevLimit, prevUse := memoryLimit, memoryInUse
	t.Cleanup(func() { memoryLimit, memoryInUse = prevLimit, prevUse })
	memoryLimit = func() uint64 { return limit }
	memoryInUse = func() uint64 { return inUse }
}

func TestMemoryPlannerSerializesConcurrentLoads(t *testing.T) {
	const per = 1000
	withMemory(t, limitFor(5*per+per/2), 0)
	m := newMemoryPlanner()
	var wg sync.WaitGroup
	fits := make([]int, 2)
	for i, name := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fit, _ := m.reserve(name, per, 4)
			fits[i] = fit
		}()
	}
	wg.Wait()
	if fits[0]+fits[1] > 5 {
		t.Fatalf("two loads reserved %v sessions together, want at most the 5 that fit", fits)
	}
	if fits[0]+fits[1] != 5 {
		t.Fatalf("two loads reserved %v, want the headroom fully but not doubly used (5)", fits)
	}
}

func TestMemoryPlannerCountsRSSCacheAndUnloadedPeers(t *testing.T) {
	const per = 1000
	withMemory(t, limitFor(10*per+per/2), 2*per)
	m := newMemoryPlanner()
	m.cache.Store(3 * per)
	m.register("peer", per) // configured but not resident: one session reserved
	if fit, release := m.reserve("a", per, 10); fit != 4 {
		t.Fatalf("fit = %d, want 10 - rss 2 - cache 3 - peer 1 = 4", fit)
	} else {
		release()
	}
	m.settle("peer") // resident peers are already in RSS, not reserved twice
	if fit, _ := m.reserve("a", per, 10); fit != 5 {
		t.Fatalf("fit after the peer settled = %d, want 5", fit)
	}
}

func TestMemoryPlannerReleaseReturnsHeadroom(t *testing.T) {
	const per = 1000
	withMemory(t, limitFor(3*per+per/2), 0)
	m := newMemoryPlanner()
	first, release := m.reserve("a", per, 3)
	second, _ := m.reserve("b", per, 3)
	if first != 3 || second != 0 {
		t.Fatalf("fits = %d,%d, want 3 then none left", first, second)
	}
	release()
	if again, _ := m.reserve("b", per, 3); again != 3 {
		t.Fatalf("fit after release = %d, want 3", again)
	}
}

func modelFile(t *testing.T, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// When the larger candidate cannot fit, the smaller valid layout is chosen and
// its memory-limited reason is reported.
func TestLargerCandidateThatCannotFitFallsBackToSmallerLayout(t *testing.T) {
	withCalibrationSeams(t, liveCPU(), nil)
	path := modelFile(t, 1000)
	per := sessionEstimate(path)
	cfg := config.ModelConfig{Dim: 3, MaxLength: 32, ONNX: path}
	steady := func(int) calFakeSession { return calFakeSession{dur: 5 * time.Millisecond} }

	withMemory(t, limitFor(100*per), 0)
	_, roomy := layoutEntry(t, "4", nil, "m", "m")
	full, err := plan(t, roomy, cfg, false, steady)
	if err != nil {
		t.Fatal(err)
	}
	defer full.releaseMemory()
	if full.Workers != 4 || full.Limit != "" {
		t.Fatalf("unconstrained layout = %+v, want 4 sessions with no limit", full.calibratedLayout)
	}

	withMemory(t, limitFor(per+per/2), 0)
	_, tight := layoutEntry(t, "4", nil, "m", "m")
	small, err := plan(t, tight, cfg, false, steady)
	if err != nil {
		t.Fatal(err)
	}
	defer small.releaseMemory()
	if small.Workers != 1 || small.Limit != "memory" || small.Ceiling != 4 {
		t.Fatalf("memory-limited layout = %+v, want 1 session limited by memory under ceiling 4", small.calibratedLayout)
	}
	if got := small.outcome(); got.Limit != "memory" {
		t.Fatalf("outcome limit = %q, want memory", got.Limit)
	}

	// An explicit worker count is authoritative: it fails before allocating.
	explicit := cfg
	explicit.Workers = 3
	if _, err := plan(t, tight, explicit, false, steady); err == nil || !strings.Contains(err.Error(), "workers=3") {
		t.Fatalf("explicit workers that cannot fit: err = %v, want a clear memory error", err)
	}
}

// Session construction dominating a candidate must be counted: the second
// candidate is not started when the first one's whole cost leaves no room.
func TestCalibrationPredictsFromTotalCostWhenConstructionDominates(t *testing.T) {
	clock := time.Unix(0, 0)
	withCalibrationSeams(t, liveCPU(), func() time.Time { return clock })
	built := 0
	factory := func(int) (onnx.Session, error) {
		built++
		clock = clock.Add(2500 * time.Millisecond) // construction advances the fake clock
		return calFakeSession{dur: time.Millisecond}, nil
	}
	layout := calibrateLayout(factory, registryTok{}, 3, 32, 4, 4, 0, 0, false, 1)
	if built != 1 {
		t.Fatalf("sessions built = %d, want only the baseline (2.5s of the 4s deadline, a second 2.5s candidate cannot fit)", built)
	}
	if layout.Workers != 1 || layout.Threads != 1 || layout.Elapsed < 2500*time.Millisecond {
		t.Fatalf("layout = %+v, want the baseline with the whole construction cost in its duration", layout)
	}
}
