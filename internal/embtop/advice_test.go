package embtop

import (
	"strings"
	"testing"
	"time"
)

// adviceWindow builds a node series of n polls a second apart.
func adviceWindow(n int, mutate func(i int, p *Point)) []Point {
	out := make([]Point, 0, n)
	for i := range n {
		p := Point{
			At:         time.Unix(int64(i), 0),
			GoMaxProcs: 2,
			CPUBudget:  2, CPUBudgetEnabled: true, CPUPlanKnown: true,
			ReqRate: 50, CPUPercent: 60,
			TotalRequests: int64(i * 50),
		}
		if mutate != nil {
			mutate(i, &p)
		}
		out = append(out, p)
	}
	return out
}

// adviceModels builds one model's series with the window's totals.
func adviceModels(name string, base, last ModelPoint) map[string][]ModelPoint {
	base.At = time.Unix(0, 0)
	last.At = time.Unix(5, 0)
	return map[string][]ModelPoint{name: {base, last}}
}

// busyPaths is an event window with enough evidence to act on.
func busyPaths(reran int) PathCounts {
	return PathCounts{Total: 40, Cached: 40 - reran, Reran: reran, Attributed: 40}
}

func TestAdviseQuietOnAnIdleNodeWithSteal(t *testing.T) {
	// The frame this change came from: nothing running, 0/0 periods throttled,
	// no microseconds throttled, and ten milliseconds stolen over the window.
	hist := adviceWindow(6, func(i int, p *Point) {
		p.ReqRate = 0
		p.CPUPercent = 0
		p.CPUActiveTokens = 0
		p.TotalRequests = 0
		p.CPUStealUsec = int64(i) * 2_000 // 10 ms across the window
	})

	if rec := RecommendNode(hist); !rec.Empty() {
		t.Errorf("an idle node with 10 ms of steal still reports %q: %s", rec.Kind, rec.Text)
	}
	if adv := Advise(AdviceInput{Window: hist, Split: PhaseSplit{}, Paths: PathCounts{}}); !adv.Empty() {
		t.Errorf("advice on an idle node: %s", AdviceText(adv))
	}
}

func TestAdviseThinTailStatesItsSampleCount(t *testing.T) {
	hist := adviceWindow(6, nil)
	in := AdviceInput{
		Window: hist,
		Split:  PhaseSplit{RunUs: 900_000, Runs: 1},
		Paths:  PathCounts{Total: 3, Cached: 2, Reran: 1, Attributed: 3},
	}
	adv := Advise(in)
	if !adv.Thin {
		t.Fatalf("a one-run window was not reported as thin: %+v", adv)
	}
	if !adv.Empty() {
		t.Errorf("a thin tail produced an action: %s", AdviceText(adv))
	}
	if !strings.Contains(adv.Text, "3 events") || !strings.Contains(adv.Text, "1 of them runs") {
		t.Errorf("thin tail does not state its sample count: %q", adv.Text)
	}

	// A window the cache answered entirely rests on its events but on no runs:
	// the line must not claim the p95 came from nothing.
	allCached := Advise(AdviceInput{
		Window: hist,
		Paths:  PathCounts{Total: 3, Cached: 3, Attributed: 3},
	})
	if !allCached.Thin {
		t.Fatalf("an all-cached small window was not reported as thin: %+v", allCached)
	}
	if !strings.Contains(allCached.Text, "3 events, 0 of them runs") {
		t.Errorf("all-cached thin tail misstates its evidence: %q", allCached.Text)
	}
}

// TestCacheUndersizedCountsLookupsOnce covers the share the cache rule is
// graded on: a hit is a request too, so the requests served are the lookups.
func TestCacheUndersizedCountsLookupsOnce(t *testing.T) {
	models := map[string][]ModelPoint{"m": {
		{At: time.Unix(0, 0), Requests: 0, CachedReqs: 0, CacheEvictions: 0},
		{At: time.Unix(5, 0), Requests: 400, CachedReqs: 80, CacheEvictions: 25},
	}}
	pct, model := cacheUndersized(models, adviceWindow(2, nil))
	if model != "m" {
		t.Fatalf("model = %q, want m", model)
	}
	if pct != 6.25 {
		t.Errorf("evicted share = %v%%, want 25 of the 400 requests served (6.25%%)", pct)
	}
}

func TestAdviseNamesTheGateLayout(t *testing.T) {
	hist := adviceWindow(6, func(i int, p *Point) { p.CPUActiveTokens = 2 }) // holds its whole budget
	in := AdviceInput{
		Window: hist,
		Split:  PhaseSplit{AdmissionWaitUs: 1_800_000, RunUs: 200_000, Runs: 8},
		Paths:  busyPaths(8),
		Models: adviceModels("m", ModelPoint{Runs: 1}, ModelPoint{Runs: 9, RunUs: 200_000, AdmissionWaitUs: 1_800_000}),
	}
	adv := Advise(in)
	if adv.Action != ActionThreads {
		t.Fatalf("action = %q, want %q: %s", adv.Action, ActionThreads, AdviceText(adv))
	}
	if !strings.Contains(AdviceText(adv), "intra_op_threads") {
		t.Errorf("the action does not name its knob: %s", AdviceText(adv))
	}
	if adv.Rejected != ActionCPUBudget || adv.Why == "" {
		t.Errorf("the rejected action is not stated: rejected=%q why=%q", adv.Rejected, adv.Why)
	}
}

func TestAdviseNamesTheModelWhenRunsHoldTheTail(t *testing.T) {
	hist := adviceWindow(6, nil)
	in := AdviceInput{
		Window:        hist,
		Split:         PhaseSplit{RunUs: 900_000, DispatchWaitUs: 40_000, AdmissionWaitUs: 60_000, Runs: 8},
		Paths:         busyPaths(8),
		UncachedP95Us: 1_900_000,
		Models: map[string][]ModelPoint{
			"hyperclusters": {{At: time.Unix(0, 0), Runs: 1}, {At: time.Unix(5, 0), Runs: 5, RunUs: 700_000}},
			"siglip2":       {{At: time.Unix(0, 0), Runs: 1}, {At: time.Unix(5, 0), Runs: 4, RunUs: 120_000}},
		},
	}
	adv := Advise(in)
	if adv.Action != ActionModel {
		t.Fatalf("action = %q, want %q: %s", adv.Action, ActionModel, AdviceText(adv))
	}
	if !strings.Contains(adv.Text, "hyperclusters") {
		t.Errorf("the advice does not name the model holding the tail: %s", adv.Text)
	}
	if adv.Rejected != ActionCPUBudget {
		t.Errorf("a run-bound tail did not reject more cpu: %q", adv.Rejected)
	}
}

func TestAdviseNamesWorkersOrTheBatchWindow(t *testing.T) {
	hist := adviceWindow(6, nil)
	split := PhaseSplit{DispatchWaitUs: 800_000, RunUs: 150_000, AdmissionWaitUs: 50_000, Runs: 8}
	counts := busyPaths(8)
	models := adviceModels("m", ModelPoint{Runs: 1}, ModelPoint{Runs: 9, DispatchWaitUs: 800_000, RunUs: 150_000})

	workers := Advise(AdviceInput{Window: hist, Split: split, Paths: counts, Models: models})
	if workers.Action != ActionWorkers {
		t.Fatalf("action = %q, want %q: %s", workers.Action, ActionWorkers, AdviceText(workers))
	}
	if workers.Rejected != ActionCPUBudget {
		t.Errorf("dispatch-bound advice did not reject more cpu: %q", workers.Rejected)
	}

	// The same node with a batch timeout is served by the batcher, so the knob
	// is the window rather than the worker count.
	batched := Advise(AdviceInput{
		Window: hist, Split: split, Paths: counts, Models: models,
		Raw: map[string]*ModelStats{"m": {BatchingTimeout: 2, Workers: 1}},
	})
	if batched.Action != ActionBatch {
		t.Fatalf("batched action = %q, want %q: %s", batched.Action, ActionBatch, AdviceText(batched))
	}
}

func TestAdviseNamesTheCacheWhenEntriesAreDropped(t *testing.T) {
	hist := adviceWindow(6, func(i int, p *Point) {
		p.TotalRequests = int64(i * 100)
		p.CacheHits = int64(i * 20) // 20% of the traffic never reaches a session
	})
	in := AdviceInput{
		Window: hist,
		Split:  PhaseSplit{RunUs: 900_000, Runs: 8},
		Paths:  PathCounts{Total: 40, Cached: 8, Reran: 32, Attributed: 40},
		Models: map[string][]ModelPoint{"m": {
			{At: time.Unix(0, 0), Runs: 1, Requests: 0, CachedReqs: 0},
			{At: time.Unix(5, 0), Runs: 9, Requests: 400, CachedReqs: 80, CacheEvictions: 90},
		}},
	}
	adv := Advise(in)
	if adv.Action != ActionCache {
		t.Fatalf("action = %q, want %q: %s", adv.Action, ActionCache, AdviceText(adv))
	}
	if adv.CachedPct <= 0 || adv.CachedPct > 25 {
		t.Errorf("cached share = %.1f%%, want the window's own 20%%", adv.CachedPct)
	}
	if adv.Rejected != ActionCPUBudget {
		t.Errorf("a cache-bound window did not reject more cpu: %q", adv.Rejected)
	}
}

func TestAdviseWithoutAttributionStaysSilent(t *testing.T) {
	// An older server sends no attribution: no phase claim is made, and the
	// absent phases are not read as a measured run.
	hist := adviceWindow(6, nil)
	adv := Advise(AdviceInput{Window: hist})
	if !adv.Empty() {
		t.Errorf("advice without attribution: %s", AdviceText(adv))
	}
	if adv.Thin {
		t.Error("an unattributed window was reported as a thin one rather than as unattributed")
	}
}

func TestAdviseNamesTheHostAndRejectsWorkers(t *testing.T) {
	hist := adviceWindow(6, func(i int, p *Point) {
		p.CPUActiveTokens = 0
		p.CPUThrottledPeriods = int64(i * 40)
		p.CPUThrottlePeriods = int64(i * 100)
		p.CPUThrottleUsec = int64(i) * 1_000_000
	})
	adv := Advise(AdviceInput{Window: hist, Split: PhaseSplit{RunUs: 500_000, Runs: 4}, Paths: busyPaths(4)})
	if adv.Action != ActionHostCPU {
		t.Fatalf("action = %q, want %q: %s", adv.Action, ActionHostCPU, AdviceText(adv))
	}
	if adv.Rejected != "workers" {
		t.Errorf("a starved host did not rule out more workers: %q", adv.Rejected)
	}
	if !strings.Contains(adv.Text, "periods throttled") {
		t.Errorf("the host advice does not state its evidence: %s", adv.Text)
	}
}

func TestAdviceTextNamesTheActionAndTheRejectedOne(t *testing.T) {
	adv := Advice{Action: ActionCPUBudget, Text: "cpu budget is the limit: tokens 2/2 held", Rejected: ActionWorkers, Why: "callers are refused at the gate"}
	line := AdviceText(adv)
	if !strings.HasPrefix(line, "raise cpu_budget — ") {
		t.Errorf("the line does not open with the action: %q", line)
	}
	if !strings.Contains(line, "not more workers: callers are refused at the gate") {
		t.Errorf("the line does not state the rejected action and why: %q", line)
	}
	if strings.Contains(AdviceText(Advice{}), "raise") {
		t.Error("an empty advice renders an action")
	}
}
