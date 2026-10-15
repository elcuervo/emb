package embtop

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Point is the derived rate view of one poll: everything the UI charts as a
// time series.
type Point struct {
	At               time.Time
	ReqRate          float64 // requests/second (aggregate)
	TokRate          float64 // tokens/second (aggregate)
	ErrRate          float64 // errors/second (aggregate)
	Active           int64
	Conns            int64
	Goroutines       int64
	MemMB            int64
	CPUPercent       float64 // percent of one core (≥ 100 means multi-core busy)
	CacheHitRate     float64 // cumulative hit percentage 0..100
	CacheHitPS       float64
	CacheMissPS      float64
	CacheEvictPS     float64
	UptimeSecs       int64
	TotalRequests    int64
	TotalTokens      int64
	TotalErrors      int64
	TruncatedTexts   int64
	TruncatedPairs   int64
	TruncatedImages  int64
	RegisteredModels int
	GoMaxProcs       int // node's own parallelism; 0 when the node did not report it

	// Process CPU plan snapshot: admission tokens in use versus the budget, and
	// the detected (possibly fractional) CPU allowance behind it.
	CPUBudgetEnabled bool
	CPUBudget        int64
	CPUActiveTokens  int64
	CPUPeakTokens    int64
	CPUAllowance     float64
	CPUAllowanceSrc  string
	CPUPlanKnown     bool // the node reported the EMB.STATS CPU plan fields
	// Admission pressure: the wait callers spent queueing at the CPU gate, the
	// queue depth now, and the waits that ended without a grant.
	CPUAdmissionWaitUs   int64
	CPUAdmissionWaiters  int64
	CPUAdmissionTimeouts int64

	// The kernel's own view of CPU this process could not use, cumulative:
	// cgroup throttling and stolen time.
	CPUThrottlePeriods  int64
	CPUThrottledPeriods int64
	CPUThrottleUsec     int64
	CPUStealUsec        int64

	// Cache-restore headroom against the server's own limit, and the entries it
	// dropped for want of memory.
	CacheRestoreLimitBytes    int64
	CacheRestoreHeadroomBytes int64
	CacheRestoreSkippedMemory int64

	// CacheBytes is the memory the model caches hold: the occupancy the
	// growth check compares against.
	CacheBytes int64

	// CacheHits and CacheMisses are the cumulative cache counters, so a
	// reader can take the window's cached share of the traffic: how much of
	// the service's work the cache removed, which the hit rate alone does not
	// say.
	CacheHits   int64
	CacheMisses int64
}

// CPUPlanText renders one node's (or an aggregate's) process CPU plan, or ""
// when the node did not report one — an older server, or one started with a
// config that predates the field.
func CPUPlanText(p Point) string {
	if !p.CPUPlanKnown {
		return ""
	}
	if !p.CPUBudgetEnabled && p.CPUBudget == 0 {
		return "cpu budget off (legacy sizing)"
	}
	line := fmt.Sprintf("cpu budget %d \u00b7 tokens %d/%d", p.CPUBudget, p.CPUActiveTokens, p.CPUBudget)
	if p.CPUPeakTokens > 0 {
		line += fmt.Sprintf(" \u00b7 peak %d", p.CPUPeakTokens)
	}
	if p.CPUAllowance > 0 {
		line += fmt.Sprintf(" \u00b7 %.2f cores", p.CPUAllowance)
		if p.CPUAllowanceSrc != "" {
			line += " (" + p.CPUAllowanceSrc + ")"
		}
	}
	// A gate with waiters or expired waits says the budget is the limit, not the
	// model; an idle gate stays quiet.
	if p.CPUAdmissionWaiters > 0 || p.CPUAdmissionTimeouts > 0 {
		line += fmt.Sprintf(" \u00b7 queued %d \u00b7 timeouts %d", p.CPUAdmissionWaiters, p.CPUAdmissionTimeouts)
	}
	return line
}

// ModelPoint is the per-model rate view of one poll. It carries the model's
// cumulative cache counters and embedding phases beside its rates, so the
// advice can read the window's path split and phase means without a second
// request.
type ModelPoint struct {
	At           time.Time
	ReqRate      float64
	TokRate      float64
	ErrRate      float64
	AvgLatencyUs int64
	Requests     int64
	Tokens       int64
	Errors       int64
	CacheHitRate float64 // cumulative %, 100 when no cache stats (-1 unknown)
	Stale        bool    // no EMB.INFO reply this poll; values are last-known

	// The embedding path, cumulative: the cache-served requests, the entries
	// held and dropped, the runs that were admitted, the phases those runs
	// paid, and the live session gauges.
	CachedReqs      int64
	CacheEvictions  int64
	CacheEntries    int64
	Runs            int64
	DispatchWaitUs  int64
	AdmissionWaitUs int64
	RunUs           int64
	SessionsBusy    int64
	SessionsTotal   int64
}

// Sampler diffs cumulative counters between polls into rates and keeps a
// bounded window of history. It is safe for concurrent use.
type Sampler struct {
	mu       sync.Mutex
	window   int
	havePrev bool
	prevAt   time.Time
	prev     PollResult
	prevMod  map[string]*ModelStats
	now      func() time.Time

	Latest       Point
	LatestModels map[string]ModelPoint
	LatestRaw    map[string]*ModelStats // latest EMB.INFO snapshot per model
	Window       []Point
	ModHist      map[string][]ModelPoint
	Events       []Event // completed-request events (bounded window)
}

// NewSampler returns a Sampler keeping the last window Points per subject.
func NewSampler(window int) *Sampler {
	if window < 2 {
		window = 2
	}
	return &Sampler{
		window:  window,
		prevMod: map[string]*ModelStats{},
		ModHist: map[string][]ModelPoint{},
		now:     time.Now,
	}
}

// SetClock replaces the time source (tests use a fake clock).
func (s *Sampler) SetClock(f func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = f
}

// EventWindowBounds caps how many recent events are kept for percentile
// computation. Larger windows give smoother percentiles at higher cost.
const EventWindowBounds = 4096

// PushEvents appends completed-request events to the sample window, evicting
// older ones past the bound. A gap in sequence numbers (ring overwritten) is
// tolerated: rates come from STATS/INFO deltas, not events.
func (s *Sampler) PushEvents(evs []Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(evs) == 0 {
		return
	}
	s.Events = append(s.Events, evs...)
	if len(s.Events) > EventWindowBounds {
		s.Events = s.Events[len(s.Events)-EventWindowBounds:]
	}
}

// Reset discards all state (used on reconnect: counters are rebased).
func (s *Sampler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.havePrev = false
	s.prev = PollResult{}
	s.prevMod = map[string]*ModelStats{}
	s.Window = nil
	s.ModHist = map[string][]ModelPoint{}
	s.Latest = Point{}
	s.LatestModels = map[string]ModelPoint{}
	s.LatestRaw = map[string]*ModelStats{}
	s.Events = nil
}

// Push diffs res against the previous poll and appends the resulting Point.
// The first poll after construction (or Reset) seeds the counter snapshot;
// its rates read zero.
func (s *Sampler) Push(res *PollResult) Point {
	s.mu.Lock()
	defer s.mu.Unlock()

	at := s.now()
	p := Point{
		At:               at,
		Active:           res.ActiveRequests,
		Conns:            res.Connections,
		Goroutines:       res.Goroutines,
		MemMB:            res.MemMB,
		UptimeSecs:       res.UptimeSecs,
		TruncatedTexts:   res.TruncatedTexts,
		TruncatedPairs:   res.TruncatedPairs,
		TruncatedImages:  res.TruncatedImages,
		RegisteredModels: res.ModelsLoaded,
		GoMaxProcs:       res.GoMaxProcs,

		CPUBudgetEnabled: res.CPUBudgetEnabled,
		CPUBudget:        res.CPUBudget,
		CPUActiveTokens:  res.CPUActiveTokens,
		CPUPeakTokens:    res.CPUPeakTokens,
		CPUAllowance:     res.CPUAllowance,
		CPUAllowanceSrc:  res.CPUAllowanceSrc,
		CPUPlanKnown:     res.CPUPlanKnown,

		CPUAdmissionWaitUs:   res.CPUAdmissionWaitUs,
		CPUAdmissionWaiters:  res.CPUAdmissionWaiters,
		CPUAdmissionTimeouts: res.CPUAdmissionTimeouts,

		CPUThrottlePeriods:  res.CPUThrottlePeriods,
		CPUThrottledPeriods: res.CPUThrottledPeriods,
		CPUThrottleUsec:     res.CPUThrottleUsec,
		CPUStealUsec:        res.CPUStealUsec,

		CacheRestoreLimitBytes:    res.CacheRestoreLimitBytes,
		CacheRestoreHeadroomBytes: res.CacheRestoreHeadroomBytes,
		CacheRestoreSkippedMemory: res.CacheRestoreSkippedMemory,
	}
	for _, ms := range res.PerModel {
		p.CacheBytes += ms.CacheCurBytes
	}

	totalHits := res.CacheHits
	totalMisses := res.CacheMisses
	if h := totalHits + totalMisses; h > 0 {
		p.CacheHitRate = float64(totalHits) / float64(h) * 100
	}

	// Cumulative totals for the status line — set on every poll, including
	// the first (they are snapshot values, not rates).
	p.TotalRequests = res.TotalRequests
	p.TotalTokens = res.TotalTokens
	p.TotalErrors = res.TotalErrors
	p.CacheHits = res.CacheHits
	p.CacheMisses = res.CacheMisses

	if s.havePrev {
		dt := at.Sub(s.prevAt).Seconds()
		if dt > 0 {
			prev := &s.prev
			p.ReqRate = rate(prev.TotalRequests, res.TotalRequests, dt)
			p.TokRate = rate(prev.TotalTokens, res.TotalTokens, dt)
			p.ErrRate = rate(prev.TotalErrors, res.TotalErrors, dt)
			p.CacheHitPS = rate(prev.CacheHits, res.CacheHits, dt)
			p.CacheMissPS = rate(prev.CacheMisses, res.CacheMisses, dt)
			p.CacheEvictPS = rate(prev.CacheEvictions, res.CacheEvictions, dt)
			cpuDeltaUs := (res.CPUUserUsec - prev.CPUUserUsec) + (res.CPUSysUsec - prev.CPUSysUsec)
			if cpuDeltaUs >= 0 {
				p.CPUPercent = float64(cpuDeltaUs) / 1e6 / dt * 100
			}
		}
	}

	// Per-model rates relative to the previous snapshot.
	mp := map[string]ModelPoint{}
	for name, cur := range res.PerModel {
		mpnt := ModelPoint{
			At:           at,
			Requests:     cur.Requests,
			Tokens:       cur.Tokens,
			Errors:       cur.Errors,
			AvgLatencyUs: cur.AvgLatencyUs,
		}
		if prev, ok := s.prevMod[name]; ok && s.havePrev {
			dt := at.Sub(s.prevAt).Seconds()
			if dt > 0 {
				mpnt.ReqRate = rate(prev.Requests, cur.Requests, dt)
				mpnt.TokRate = rate(prev.Tokens, cur.Tokens, dt)
				mpnt.ErrRate = rate(prev.Errors, cur.Errors, dt)
			}
		}
		hits := cur.CacheHits
		misses := cur.CacheMisses
		if h := hits + misses; h > 0 {
			mpnt.CacheHitRate = float64(hits) / float64(h) * 100
		} else {
			mpnt.CacheHitRate = -1
		}
		mpnt.CachedReqs = hits
		mpnt.CacheEvictions = cur.CacheEvictions
		mpnt.CacheEntries = cur.CacheEntries
		mpnt.Runs = cur.EmbedRuns
		mpnt.DispatchWaitUs = cur.EmbedDispatchWaitUs
		mpnt.AdmissionWaitUs = cur.EmbedAdmissionWaitUs
		mpnt.RunUs = cur.EmbedRunUs
		mpnt.SessionsBusy = cur.EmbedSessionsBusy
		mpnt.SessionsTotal = cur.EmbedSessionsTotal
		mp[name] = mpnt
	}

	// A model with no EMB.INFO reply this poll keeps its last-known sample,
	// flagged stale, so its row neither blanks nor shrinks. Raw stats carry
	// forward too, so identity metadata survives and the next poll diffs
	// against the last known cumulative counters.
	for name, prev := range s.LatestModels {
		if _, ok := mp[name]; ok {
			continue
		}
		prev.At = at
		prev.Stale = true
		mp[name] = prev
	}
	raw := make(map[string]*ModelStats, len(res.PerModel))
	for name, ms := range res.PerModel {
		raw[name] = ms
	}
	for name, ms := range s.LatestRaw {
		if _, ok := raw[name]; !ok {
			raw[name] = ms
		}
	}

	// Advance history. A missing model repeats its last point so its strip
	// stays column-aligned with its neighbours; histories are kept for
	// models that leave so a returning model keeps its row slot and strip.
	s.Window = append(s.Window, p)
	if len(s.Window) > s.window {
		s.Window = s.Window[len(s.Window)-s.window:]
	}
	for name, mpnt := range mp {
		s.ModHist[name] = append(s.ModHist[name], mpnt)
		if len(s.ModHist[name]) > s.window {
			s.ModHist[name] = s.ModHist[name][len(s.ModHist[name])-s.window:]
		}
	}

	s.Latest = p
	s.LatestModels = mp
	s.LatestRaw = raw
	s.prev = *res
	s.prevMod = raw
	s.prevAt = at
	s.havePrev = true
	return p
}

// RawModels returns the latest raw per-model EMB.INFO stats.
func (s *Sampler) RawModels() map[string]*ModelStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LatestRaw
}

// Snapshot returns the latest Point and copies of the histories under lock.
func (s *Sampler) Snapshot() (Point, []Point, map[string][]ModelPoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	window := make([]Point, len(s.Window))
	copy(window, s.Window)
	hist := make(map[string][]ModelPoint, len(s.ModHist))
	for k, v := range s.ModHist {
		hist[k] = append([]ModelPoint(nil), v...)
	}
	return s.Latest, window, hist
}

// Latency returns aggregate p50/p95/p99 latency (µs) over the event window,
// computed from successful events only. ok is false when there are no events.
func (s *Sampler) Latency() (p50, p95, p99 int64, ok bool) {
	return s.percentiles("")
}

// ModelLatency returns per-model p50/p95/p99 (µs) over the event window.
func (s *Sampler) ModelLatency(model string) (p50, p95, p99 int64, ok bool) {
	return s.percentiles(model)
}

// LastEvent returns the most recent event, if any.
func (s *Sampler) LastEvent() (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Events) == 0 {
		return Event{}, false
	}
	return s.Events[len(s.Events)-1], true
}

// percentiles computes p50/p95/p99 over successful events, optionally for one
// model ("" = all). A bounded sort per poll is cheap (≤4096 samples).
func (s *Sampler) percentiles(model string) (p50, p95, p99 int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lats := s.latenciesLocked(model)
	if len(lats) == 0 {
		return 0, 0, 0, false
	}
	sortInt64s(lats)
	n := len(lats)
	return lats[percentileIdx(n, 0.50)], lats[percentileIdx(n, 0.95)], lats[percentileIdx(n, 0.99)], true
}

func (s *Sampler) latenciesLocked(model string) []int64 {
	return s.latenciesLockedPath(model, false)
}

// latenciesLockedPath collects the window's successful latencies, optionally
// restricted to the requests that ran (at least one text not cache-answered).
// Oldest first. Caller holds s.mu.
func (s *Sampler) latenciesLockedPath(model string, reranOnly bool) []int64 {
	var lats []int64
	for _, e := range s.Events {
		if e.Err {
			continue
		}
		if model != "" && e.Model != model {
			continue
		}
		if reranOnly && !e.Reran() {
			continue
		}
		lats = append(lats, e.LatencyUs)
	}
	return lats
}

// sortInt64s sorts in place; the percentile computation is the only user.
func sortInt64s(v []int64) {
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
}

// percentileIdx maps a fraction to a sorted slice index using the
// nearest-rank definition: idx = ceil(n*f) - 1 (so p95 of 5 samples is the
// largest sample, not the second largest).
func percentileIdx(n int, f float64) int {
	idx := int(math.Ceil(float64(n)*f)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return idx
}

// rate returns the per-second rate of a cumulative counter delta, clamped to
// ≥ 0 (counters reset on server restart; rebasing handles the spike).
func rate(prev, cur int64, dt float64) float64 {
	if cur < prev {
		return 0
	}
	return float64(cur-prev) / dt
}
