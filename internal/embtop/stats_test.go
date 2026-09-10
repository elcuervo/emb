package embtop

import (
	"math"
	"testing"
	"time"
)

// fakeClock returns successive times so sampler rate math is deterministic.
func newFakeClock(start time.Time, step time.Duration) func() time.Time {
	cur := start
	return func() time.Time {
		t := cur
		cur = cur.Add(step)
		return t
	}
}

func baseRes() *PollResult {
	return &PollResult{
		UptimeSecs:     10,
		TotalRequests:  0,
		TotalTokens:    0,
		TotalErrors:    0,
		ActiveRequests: 0,
		Connections:    1,
		MemMB:          100,
		CPUUserUsec:    0,
		CPUSysUsec:     0,
		Goroutines:     8,
		CacheHits:      0,
		CacheMisses:    0,
		CacheEvictions: 0,
		ModelsLoaded:   1,
		Models:         []ModelListEntry{{Name: "m", Dim: 384, Status: "ready"}},
		PerModel: map[string]*ModelStats{
			"m": {Dim: 384, Requests: 0, Tokens: 0, Errors: 0, AvgLatencyUs: 0, Pooling: "mean", Quantization: "none"},
		},
	}
}

const stepDur = time.Second

func pushAt(s *Sampler, res *PollResult) Point { return s.Push(res) }

func TestSamplerRatesFromCounterDeltas(t *testing.T) {
	s := NewSampler(10)
	s.SetClock(newFakeClock(time.Unix(0, 0), stepDur))

	r1 := baseRes()
	r1.TotalRequests = 38
	r1.TotalTokens = 172
	p1 := pushAt(s, r1) // first poll seeds counters; rates are zero
	// ...but cumulative totals are snapshot values and must show immediately.
	if p1.TotalRequests != 38 || p1.TotalTokens != 172 {
		t.Errorf("first poll totals = %d/%d, want 38/172", p1.TotalRequests, p1.TotalTokens)
	}

	r2 := baseRes()
	r2.TotalRequests = 48
	r2.TotalTokens = 572
	r2.TotalErrors = 2
	r2.CPUUserUsec = 500_000 // 0.5s of one core in one second
	r2.CPUSysUsec = 100_000
	r2.PerModel["m"].Requests = 9
	r2.PerModel["m"].Tokens = 350
	r2.PerModel["m"].Errors = 1
	p := pushAt(s, r2)

	if p.ReqRate != 10 {
		t.Errorf("req rate = %v, want 10", p.ReqRate)
	}
	if p.TokRate != 400 {
		t.Errorf("tok rate = %v, want 400", p.TokRate)
	}
	if p.ErrRate != 2 {
		t.Errorf("err rate = %v, want 2", p.ErrRate)
	}
	if math.Abs(p.CPUPercent-60) > 1e-9 {
		t.Errorf("cpu percent = %v, want 60", p.CPUPercent)
	}
	mp := s.LatestModels["m"]
	if mp.ReqRate != 9 || mp.TokRate != 350 || mp.ErrRate != 1 {
		t.Errorf("per-model rates = %+v, want 9/350/1", mp)
	}

	// Window keeps both points.
	_, window, _ := s.Snapshot()
	if len(window) != 2 {
		t.Fatalf("window len = %d, want 2", len(window))
	}
}

func TestSamplerCacheHitRateAndRates(t *testing.T) {
	s := NewSampler(10)
	s.SetClock(newFakeClock(time.Unix(0, 0), stepDur))

	r1 := baseRes()
	r1.CacheHits = 900
	r1.CacheMisses = 100
	pushAt(s, r1)

	r2 := baseRes()
	r2.CacheHits = 910
	r2.CacheMisses = 105
	r2.CacheEvictions = 5
	p := pushAt(s, r2)

	if math.Abs(p.CacheHitRate-89.65) > 0.01 { // 910/1015
		t.Errorf("cache hit rate = %v, want ~89.65", p.CacheHitRate)
	}
	if p.CacheHitPS != 10 || p.CacheMissPS != 5 || p.CacheEvictPS != 5 {
		t.Errorf("cache rates = %v/%v/%v, want 10/5/5", p.CacheHitPS, p.CacheMissPS, p.CacheEvictPS)
	}
}

func TestSamplerWindowEviction(t *testing.T) {
	s := NewSampler(3)
	s.SetClock(newFakeClock(time.Unix(0, 0), stepDur))
	for i := 0; i < 6; i++ {
		pushAt(s, baseRes())
	}
	_, window, _ := s.Snapshot()
	if len(window) != 3 {
		t.Fatalf("window len = %d, want 3", len(window))
	}
}

func TestSamplerCounterResetClampsNegative(t *testing.T) {
	s := NewSampler(10)
	s.SetClock(newFakeClock(time.Unix(0, 0), stepDur))

	r1 := baseRes()
	r1.TotalRequests = 100
	pushAt(s, r1)

	r2 := baseRes()
	r2.TotalRequests = 10 // server restarted, counters went down
	p := pushAt(s, r2)
	if p.ReqRate != 0 {
		t.Errorf("req rate = %v after reset, want 0", p.ReqRate)
	}
	// TotalRequests still tracks the live counter for the status line.
	if p.TotalRequests != 10 {
		t.Errorf("total requests = %v, want 10", p.TotalRequests)
	}
}

func TestSamplerResetRebases(t *testing.T) {
	s := NewSampler(10)
	s.SetClock(newFakeClock(time.Unix(0, 0), stepDur))

	r1 := baseRes()
	r1.TotalRequests = 100
	pushAt(s, r1)

	s.Reset()

	r2 := baseRes()
	r2.TotalRequests = 105
	p := pushAt(s, r2)
	if p.ReqRate != 0 {
		t.Errorf("req rate after reset = %v, want 0 (rebased)", p.ReqRate)
	}
}

func TestSamplerModelGonePrunesHistory(t *testing.T) {
	s := NewSampler(10)
	s.SetClock(newFakeClock(time.Unix(0, 0), stepDur))
	pushAt(s, baseRes())
	r2 := baseRes()
	r2.PerModel = map[string]*ModelStats{}
	pushAt(s, r2)
	_, _, hist := s.Snapshot()
	if _, ok := hist["m"]; ok {
		t.Error("history for vanished model should be pruned")
	}
}

func TestPercentilesFromEvents(t *testing.T) {
	s := NewSampler(10)
	evs := []Event{
		{Seq: 1, Model: "a", LatencyUs: 100},
		{Seq: 2, Model: "a", LatencyUs: 200},
		{Seq: 3, Model: "a", LatencyUs: 300},
		{Seq: 4, Model: "a", LatencyUs: 400},
		{Seq: 5, Model: "b", LatencyUs: 9999, Err: true}, // excluded
		{Seq: 6, Model: "b", LatencyUs: 1000},            // other model
	}
	s.PushEvents(evs)

	p50, p95, p99, ok := s.Latency()
	if !ok {
		t.Fatal("expected percentiles")
	}
	// Non-error samples, sorted: 100,200,300,400,1000. Nearest rank:
	// p50 = ceil(5*0.5)-1 = 2 -> 300; p95 = ceil(4.75)-1 = 4 -> 1000;
	// p99 = ceil(4.95)-1 = 4 -> 1000.
	if p50 != 300 {
		t.Errorf("p50 = %d, want 300", p50)
	}
	if p95 != 1000 {
		t.Errorf("p95 = %d, want 1000", p95)
	}
	if p99 != 1000 {
		t.Errorf("p99 = %d, want 1000", p99)
	}

	// Model a only: 100,200,300,400 -> p50 200, p95 400, p99 400.
	ma50, ma95, ma99, ok := s.ModelLatency("a")
	if !ok || ma50 != 200 || ma95 != 400 || ma99 != 400 {
		t.Errorf("model a percentiles = %d/%d/%d, want 200/400/400", ma50, ma95, ma99)
	}
}

func TestEventWindowEviction(t *testing.T) {
	s := NewSampler(10)
	evs := make([]Event, 0, EventWindowBounds+100)
	for i := 0; i < EventWindowBounds+100; i++ {
		evs = append(evs, Event{Seq: uint64(i + 1), Model: "m", LatencyUs: int64(i)})
	}
	s.PushEvents(evs)
	s.mu.Lock()
	n := len(s.Events)
	s.mu.Unlock()
	if n != EventWindowBounds {
		t.Fatalf("event window = %d, want %d", n, EventWindowBounds)
	}
}

func TestEventGapTolerated(t *testing.T) {
	// Seq jumps (ring overwritten between polls) must not break anything.
	s := NewSampler(10)
	s.PushEvents([]Event{{Seq: 5, Model: "m", LatencyUs: 10}, {Seq: 8000, Model: "m", LatencyUs: 20}})
	if _, _, _, ok := s.Latency(); !ok {
		t.Fatal("expected percentiles after gap")
	}
}

func TestSamplerResetClearsEventWindow(t *testing.T) {
	s := NewSampler(10)
	s.PushEvents([]Event{{Seq: 1, Model: "m", LatencyUs: 500}})
	if _, _, _, ok := s.Latency(); !ok {
		t.Fatal("expected events before reset")
	}
	s.Reset()
	if _, _, _, ok := s.Latency(); ok {
		t.Fatal("Reset left stale events visible to latency percentiles")
	}
	if _, ok := s.LastEvent(); ok {
		t.Fatal("Reset left a stale last event")
	}
}
