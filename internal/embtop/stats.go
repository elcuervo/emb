package embtop

import (
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
	RegisteredModels int
}

// ModelPoint is the per-model rate view of one poll.
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
		RegisteredModels: res.ModelsLoaded,
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
		mp[name] = mpnt
	}

	// Advance history.
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
	// Prune histories for models that vanished.
	for name := range s.ModHist {
		if _, ok := mp[name]; !ok {
			delete(s.ModHist, name)
		}
	}

	s.Latest = p
	s.LatestModels = mp
	s.LatestRaw = res.PerModel
	s.prev = *res
	s.prevMod = res.PerModel
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
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	n := len(lats)
	return lats[percentileIdx(n, 0.50)], lats[percentileIdx(n, 0.95)], lats[percentileIdx(n, 0.99)], true
}

func (s *Sampler) latenciesLocked(model string) []int64 {
	var lats []int64
	for _, e := range s.Events {
		if e.Err {
			continue
		}
		if model != "" && e.Model != model {
			continue
		}
		lats = append(lats, e.LatencyUs)
	}
	return lats
}

// percentileIdx maps a fraction to a sorted slice index (nearest-rank, ≥1).
func percentileIdx(n int, f float64) int {
	idx := int(float64(n)*f) - 1
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
