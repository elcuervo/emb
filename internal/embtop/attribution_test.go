package embtop

import (
	"testing"
	"time"
)

func TestParseEventsReadsTheAttribution(t *testing.T) {
	row := Reply{Type: '*', Elems: []Reply{
		{Type: ':', Int: 7}, {Type: ':', Int: 1234}, {Type: '$', Str: "minilm"},
		{Type: ':', Int: 3}, {Type: ':', Int: 900}, {Type: ':', Int: 0},
		{Type: ':', Int: 1}, {Type: ':', Int: 1},
		{Type: ':', Int: 40}, {Type: ':', Int: 120}, {Type: ':', Int: 700},
	}}
	evs := parseEvents(Reply{Type: '*', Elems: []Reply{row}})
	if len(evs) != 1 {
		t.Fatalf("parsed %d events, want 1", len(evs))
	}
	ev := evs[0]
	if ev.CachedTexts != 1 || !ev.Attributed {
		t.Errorf("path not decoded: %+v", ev)
	}
	if ev.DispatchUs != 40 || ev.AdmitUs != 120 || ev.RunUs != 700 {
		t.Errorf("phases not decoded: %+v", ev)
	}
	if !ev.Reran() {
		t.Error("a partly cached request did not report having run")
	}
	if ev.CacheServed() {
		t.Error("a partly cached request reported being served entirely from cache")
	}
}

func TestParseEventsWithoutAttribution(t *testing.T) {
	// A server predating the fields sends the original six: the event must read
	// as unattributed, not as a run that took no time.
	row := Reply{Type: '*', Elems: []Reply{
		{Type: ':', Int: 1}, {Type: ':', Int: 10}, {Type: '$', Str: "m"},
		{Type: ':', Int: 1}, {Type: ':', Int: 500}, {Type: ':', Int: 0},
	}}
	evs := parseEvents(Reply{Type: '*', Elems: []Reply{row}})
	if len(evs) != 1 {
		t.Fatalf("parsed %d events, want 1", len(evs))
	}
	if evs[0].Attributed {
		t.Error("an event without the fields claims an attribution")
	}
	if evs[0].DispatchUs != 0 || evs[0].AdmitUs != 0 || evs[0].RunUs != 0 {
		t.Errorf("absent phases are not zero: %+v", evs[0])
	}
}

func TestPathCountsAndUncachedPercentiles(t *testing.T) {
	s := NewSampler(10)
	s.PushEvents([]Event{
		// Cache answers, microseconds each.
		{Seq: 1, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 2},
		{Seq: 2, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 3},
		{Seq: 3, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 4},
		// Runs, milliseconds to seconds.
		{Seq: 4, Model: "m", Texts: 1, CachedTexts: 0, Attributed: true, LatencyUs: 900_000,
			DispatchUs: 100, AdmitUs: 800_000, RunUs: 90_000},
		{Seq: 5, Model: "m", Texts: 1, CachedTexts: 0, Attributed: true, LatencyUs: 1_900_000,
			DispatchUs: 200, AdmitUs: 1_700_000, RunUs: 190_000},
	})

	counts := s.PathCounts()
	if counts.Total != 5 || counts.Cached != 3 || counts.Reran != 2 || counts.Attributed != 5 {
		t.Fatalf("path counts = %+v, want 5 total / 3 cached / 2 reran / 5 attributed", counts)
	}
	if got := counts.Share(); got != 60 {
		t.Errorf("cached share = %v%%, want 60%%", got)
	}

	// The uncached tail is the runs', not the blend with the cache answers.
	p50, p95, p99, n, ok := s.UncachedLatency()
	if !ok || n != 2 {
		t.Fatalf("uncached percentiles: ok=%v n=%d, want ok with 2 events", ok, n)
	}
	if p50 != 900_000 || p95 != 1_900_000 || p99 != 1_900_000 {
		t.Errorf("uncached percentiles = %d/%d/%d, want 900000/1900000/1900000", p50, p95, p99)
	}

	// The all-events percentile still describes the service, cache included.
	all50, _, _, ok := s.Latency()
	if !ok || all50 != 4 {
		t.Errorf("service p50 = %d, want the cache answers' 4", all50)
	}
}

// TestPathCountsIgnoreUnattributedEvents keeps the population honest: an image
// run carries no attribution, so it must not dilute the cached share nor pad a
// thin text tail into one that looks big enough to act on.
func TestPathCountsIgnoreUnattributedEvents(t *testing.T) {
	s := NewSampler(10)
	events := []Event{
		{Seq: 1, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 3},
		{Seq: 2, Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: 4},
		{Seq: 3, Model: "m", Texts: 1, CachedTexts: 0, Attributed: true, LatencyUs: 900},
	}
	for i := range 20 {
		events = append(events, Event{Seq: uint64(10 + i), Model: "img", Texts: 1, LatencyUs: 5000})
	}
	s.PushEvents(events)

	counts := s.PathCounts()
	if counts.Total != 3 || counts.Cached != 2 || counts.Reran != 1 || counts.Attributed != 3 {
		t.Fatalf("path counts = %+v, want only the 3 attributed events", counts)
	}
	if got := counts.Share(); got != 66.66666666666666 {
		t.Errorf("cached share = %v%%, want 2 of the 3 attributed events", got)
	}

	// The thin floor still holds on the attributed population.
	adv := Advise(AdviceInput{Window: adviceWindow(2, nil), Paths: counts,
		Split: PhaseSplit{RunUs: 900, Runs: 1}})
	if !adv.Thin {
		t.Errorf("20 unattributed events padded a 3-event tail: %q", adv.Text)
	}
}

func TestUncachedPercentilesWhenEverythingIsCached(t *testing.T) {
	s := NewSampler(10)
	s.PushEvents([]Event{{Seq: 1, Model: "m", Texts: 2, CachedTexts: 2, Attributed: true, LatencyUs: 3}})
	if _, _, _, _, ok := s.UncachedLatency(); ok {
		t.Error("a fully cached window reported an uncached tail")
	}
}

func TestModelPhaseSplitAndDominance(t *testing.T) {
	base := time.Unix(0, 0)
	hist := []ModelPoint{
		{At: base, Requests: 10, Runs: 4, CachedReqs: 6, AdmissionWaitUs: 100, RunUs: 400},
		{At: base.Add(time.Second), Requests: 30, Runs: 12, CachedReqs: 18,
			DispatchWaitUs: 20_000, AdmissionWaitUs: 1_800_000, RunUs: 380_000},
	}
	split := ModelPhaseSplit(hist)
	if split.Runs != 8 {
		t.Errorf("runs = %d, want 8", split.Runs)
	}
	if split.AdmissionWaitUs != 1_799_900 || split.DispatchWaitUs != 20_000 || split.RunUs != 379_600 {
		t.Errorf("split = %+v", split)
	}
	phase, share := split.Dominant()
	if phase != PhaseAdmission {
		t.Errorf("dominant phase = %q, want admission", phase)
	}
	if share < 0.8 {
		t.Errorf("dominance share = %v, want the gate wait to dominate", share)
	}
	if got := split.MeanUs(PhaseAdmission); got != time.Duration(1_799_900/8)*time.Microsecond {
		t.Errorf("mean admission wait = %s", got)
	}

	// The cached share of the same window: 12 of the 20 requests it added.
	pct, cached, requests := ModelCachedShare(hist)
	if cached != 12 || requests != 20 || pct != 60 {
		t.Errorf("cached share = %v%% of %d/%d, want 60%%", pct, cached, requests)
	}
}

func TestNodePhaseSplitSumsTheModels(t *testing.T) {
	base := time.Unix(0, 0)
	hist := map[string][]ModelPoint{
		"a": {{At: base, Runs: 1, RunUs: 100}, {At: base.Add(time.Second), Runs: 3, RunUs: 900}},
		"b": {{At: base, Runs: 0, AdmissionWaitUs: 0}, {At: base.Add(time.Second), Runs: 1, AdmissionWaitUs: 500}},
	}
	got := NodePhaseSplit(hist)
	if got.Runs != 3 || got.RunUs != 800 || got.AdmissionWaitUs != 500 {
		t.Errorf("node split = %+v", got)
	}
}

func TestPhaseSplitHandlesCounterReset(t *testing.T) {
	// A model that reloaded mid-window reports smaller counters: the window
	// contributes nothing rather than a negative burst.
	base := time.Unix(0, 0)
	hist := []ModelPoint{
		{At: base, Runs: 9, RunUs: 9000},
		{At: base.Add(time.Second), Runs: 1, RunUs: 100},
	}
	got := ModelPhaseSplit(hist)
	if got.Runs != 0 || got.RunUs != 0 {
		t.Errorf("reset contributed %+v, want nothing", got)
	}
}

func TestEmptyPhaseSplitDominatesNothing(t *testing.T) {
	phase, share := (PhaseSplit{}).Dominant()
	if phase != PhaseNone || share != 0 {
		t.Errorf("empty split = %q/%v, want no phase", phase, share)
	}
}

func TestCachedShareOfTheNode(t *testing.T) {
	base := time.Unix(0, 0)
	hist := []Point{
		{At: base, TotalRequests: 100, CacheHits: 70},
		{At: base.Add(time.Second), TotalRequests: 200, CacheHits: 170},
	}
	pct, cached, requests := CachedShare(hist)
	if cached != 100 || requests != 100 || pct != 100 {
		t.Fatalf("cached share = %v%% of %d/%d, want 100%% of 100/100", pct, cached, requests)
	}
	// A window that answered nothing from the cache reads as 0, not as unknown.
	hist[0] = Point{At: base, TotalRequests: 0, CacheHits: 0}
	hist[1] = Point{At: base.Add(time.Second), TotalRequests: 50, CacheHits: 0}
	if pct, _, requests := CachedShare(hist); pct != 0 || requests != 50 {
		t.Errorf("uncached window = %v%% of %d, want 0%% of 50", pct, requests)
	}
}

func TestCPUPerRunUsesTheWindow(t *testing.T) {
	base := time.Unix(0, 0)
	// Two polls a second apart at 100% of one core each: 2s of CPU over 2 runs.
	hist := []Point{
		{At: base, CPUPercent: 100},
		{At: base.Add(time.Second), CPUPercent: 100},
		{At: base.Add(2 * time.Second), CPUPercent: 100},
	}
	got := CPUUsPerRun(hist, 2)
	if want := time.Second; got != want {
		t.Errorf("cpu per run = %s, want %s", got, want)
	}
	if got := CPUUsPerRun(hist, 0); got != 0 {
		t.Errorf("cpu per run with no runs = %s, want 0", got)
	}
}
