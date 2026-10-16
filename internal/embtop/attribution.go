package embtop

import "time"

// Phase names one part of the time a request spent before it could answer.
type Phase string

const (
	PhaseNone      Phase = ""
	PhaseDispatch  Phase = "dispatch"
	PhaseAdmission Phase = "admission"
	PhaseRun       Phase = "run"
)

// PhaseSplit is the time a window's requests spent, split by the phase they
// spent it in, with the runs those phases covered. Every field is measured:
// the split is read from what the node published, never inferred from a total.
type PhaseSplit struct {
	DispatchWaitUs  int64
	AdmissionWaitUs int64
	RunUs           int64
	Runs            int64
}

// TotalUs is the time the measured phases account for between them.
func (p PhaseSplit) TotalUs() int64 {
	return p.DispatchWaitUs + p.AdmissionWaitUs + p.RunUs
}

// Empty reports whether the split holds no measurement at all.
func (p PhaseSplit) Empty() bool { return p.TotalUs() == 0 && p.Runs == 0 }

// Dominant returns the phase holding the largest share of the measured time
// and that share (0..1). PhaseNone comes back when nothing was measured.
func (p PhaseSplit) Dominant() (Phase, float64) {
	total := p.TotalUs()
	if total <= 0 {
		return PhaseNone, 0
	}
	phase, most := PhaseDispatch, p.DispatchWaitUs
	if p.AdmissionWaitUs > most {
		phase, most = PhaseAdmission, p.AdmissionWaitUs
	}
	if p.RunUs > most {
		phase, most = PhaseRun, p.RunUs
	}
	return phase, float64(most) / float64(total)
}

// MeanUs returns the phase's time per run, 0 when no run was measured.
func (p PhaseSplit) MeanUs(phase Phase) time.Duration {
	if p.Runs <= 0 {
		return 0
	}
	switch phase {
	case PhaseDispatch:
		return time.Duration(p.DispatchWaitUs/p.Runs) * time.Microsecond
	case PhaseAdmission:
		return time.Duration(p.AdmissionWaitUs/p.Runs) * time.Microsecond
	case PhaseRun:
		return time.Duration(p.RunUs/p.Runs) * time.Microsecond
	default:
		return 0
	}
}

// ModelPhaseSplit diffs one model's history into the phases its window paid.
// A counter that reset (the model reloaded) contributes nothing rather than a
// negative burst.
func ModelPhaseSplit(hist []ModelPoint) PhaseSplit {
	if len(hist) < 2 {
		return PhaseSplit{}
	}
	first, last := hist[0], hist[len(hist)-1]
	return PhaseSplit{
		DispatchWaitUs:  delta(first.DispatchWaitUs, last.DispatchWaitUs),
		AdmissionWaitUs: delta(first.AdmissionWaitUs, last.AdmissionWaitUs),
		RunUs:           delta(first.RunUs, last.RunUs),
		Runs:            delta(first.Runs, last.Runs),
	}
}

// NodePhaseSplit sums every model's window into the node's own split.
func NodePhaseSplit(hist map[string][]ModelPoint) PhaseSplit {
	var total PhaseSplit
	for _, h := range hist {
		one := ModelPhaseSplit(h)
		total.DispatchWaitUs += one.DispatchWaitUs
		total.AdmissionWaitUs += one.AdmissionWaitUs
		total.RunUs += one.RunUs
		total.Runs += one.Runs
	}
	return total
}

// ModelCachedShare is the share of one model's window that the reply cache
// answered, in percent, plus the counts it was taken from: the lookups the
// cache answered over the lookups it saw. Both counters are the cache's own,
// so the share is bounded by 100 and is a share of the lookups rather than of
// the requests that ran — a hit never reaches a session, so a run count is not
// a lookup count.
func ModelCachedShare(hist []ModelPoint) (pct float64, cached, lookups int64) {
	if len(hist) < 2 {
		return 0, 0, 0
	}
	first, last := hist[0], hist[len(hist)-1]
	cached = delta(first.CachedReqs, last.CachedReqs)
	lookups = cached + delta(first.CacheMisses, last.CacheMisses)
	if lookups <= 0 {
		return 0, cached, 0
	}
	return float64(cached) / float64(lookups) * 100, cached, lookups
}

// CachedShare is the window's share of its cache lookups that the cache
// answered, in percent, with the counts behind it: the hit rate this window
// saw, from the node's own cumulative counters. It is a share of the lookups,
// never of the requests that ran — a cache-answered request reaches no session,
// so `total_requests` counts only the work that ran and dividing by it would
// report more than 100% of the traffic.
func CachedShare(hist []Point) (pct float64, cached, lookups int64) {
	if len(hist) < 2 {
		return 0, 0, 0
	}
	first, last := hist[0], hist[len(hist)-1]
	cached = delta(first.CacheHits, last.CacheHits)
	lookups = cached + delta(first.CacheMisses, last.CacheMisses)
	if lookups <= 0 {
		return 0, cached, 0
	}
	return float64(cached) / float64(lookups) * 100, cached, lookups
}

// CPUUsPerRun is the CPU a node's window spent per uncached run: the cost of
// the work that actually ran, which is what a mostly cache-served window's CPU
// number should be read against.
func CPUUsPerRun(hist []Point, runs int64) time.Duration {
	if len(hist) < 2 || runs <= 0 {
		return 0
	}
	// CPUPercent is percent of one core, sampled per poll; the window's CPU
	// time is the sum of each poll's share of that poll's interval.
	var us int64
	for i := 1; i < len(hist); i++ {
		dt := hist[i].At.Sub(hist[i-1].At)
		if dt <= 0 {
			continue
		}
		us += int64(hist[i].CPUPercent / 100 * float64(dt.Microseconds()))
	}
	return time.Duration(us/runs) * time.Microsecond
}

// PathCounts is the event window's split by path: how many completed requests
// the cache answered entirely, how many ran, how many carried a measured
// attribution, and the total.
type PathCounts struct {
	Total      int
	Cached     int
	Reran      int
	Attributed int
}

// Share is the cached share of the window's requests, in percent.
func (c PathCounts) Share() float64 {
	if c.Total == 0 {
		return 0
	}
	return float64(c.Cached) / float64(c.Total) * 100
}

// PathCounts returns the sampler's event window by path.
func (s *Sampler) PathCounts() PathCounts {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c PathCounts
	for _, e := range s.Events {
		// Only the events the server attributed speak for a path. An image run
		// carries no attribution, so counting it would dilute the cached share
		// and, worse, pad a thin tail into one that looks big enough to act on.
		if e.Err || !e.Attributed {
			continue
		}
		c.Total++
		c.Attributed++
		switch {
		case e.CacheServed():
			c.Cached++
		case e.Reran():
			c.Reran++
		}
	}
	return c
}

// UncachedLatency returns p50/p95/p99 (µs) over the events in the window that
// ran — the tail inference and queueing caused, apart from the cache's. It
// also reports how many events that tail rests on. ok is false when no such
// event is in the window.
func (s *Sampler) UncachedLatency() (p50, p95, p99 int64, n int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lats := s.latenciesLockedPath("", true)
	if len(lats) == 0 {
		return 0, 0, 0, 0, false
	}
	sortInt64s(lats)
	return lats[percentileIdx(len(lats), 0.50)], lats[percentileIdx(len(lats), 0.95)],
		lats[percentileIdx(len(lats), 0.99)], len(lats), true
}
