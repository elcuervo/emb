package telemetry

import (
	"sync"
	"sync/atomic"
	"time"
)

// latencyBoundsSeconds are the declared bucket boundaries of the
// request-latency histogram, in seconds: ~2-2.5x steps from 50us to 10s.
// Bucket i counts durations in (bounds[i-1], bounds[i]], so a duration exactly
// equal to a boundary is counted in that boundary's bucket, not the next one.
// The bucket after the last boundary is the implicit +Inf overflow, which
// keeps requests slower than the largest boundary countable.
var latencyBoundsSeconds = [17]float64{
	0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025,
	0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// latencyBucketCount is one bucket per declared boundary plus the overflow
// bucket, so a datapoint's bucket counts hold exactly one more entry than its
// bounds.
const latencyBucketCount = len(latencyBoundsSeconds) + 1

// ModelLatency is one model's cumulative request-latency histogram: bucket
// counts, observation count, and total microseconds. Values are copies, so a
// reader never observes a later observation through one.
type ModelLatency struct {
	// Buckets holds one count per declared boundary plus the overflow bucket.
	Buckets []int64
	Count   int64
	SumUs   int64
}

// modelLatency is the live per-model accumulator behind ModelLatency. Every
// field is atomic, so the request path observes without a lock and without
// allocating.
type modelLatency struct {
	buckets [latencyBucketCount]atomic.Int64
	count   atomic.Int64
	sumUs   atomic.Int64
}

func (m *modelLatency) observe(d time.Duration) {
	m.buckets[bucketIndex(d.Seconds())].Add(1)
	m.count.Add(1)
	m.sumUs.Add(d.Microseconds())
}

// bucketIndex returns the bucket a duration in seconds belongs to: the first
// boundary it does not exceed, or the overflow bucket after the last boundary.
func bucketIndex(secs float64) int {
	for i, b := range latencyBoundsSeconds {
		if secs <= b {
			return i
		}
	}
	return len(latencyBoundsSeconds)
}

// LatencyHistogram accumulates a per-model distribution of completed-request
// latency. Only names tracked accepts create a series, so a client-controlled
// model name can neither allocate a series nor evict a legitimate one: the
// server tracks its configured model set. The series map is bounded by that
// set, independent of what clients send.
type LatencyHistogram struct {
	tracked func(string) bool
	series  sync.Map // model name -> *modelLatency
}

// NewLatencyHistogram returns a histogram whose series are limited to the
// names tracked accepts. A nil tracked accepts every name.
func NewLatencyHistogram(tracked func(string) bool) *LatencyHistogram {
	return &LatencyHistogram{tracked: tracked}
}

// Observe records one completed request's latency for model. It is safe for
// concurrent use; a name the histogram does not track is ignored and creates
// no series.
func (h *LatencyHistogram) Observe(model string, d time.Duration) {
	if h == nil || (h.tracked != nil && !h.tracked(model)) {
		return
	}
	v, ok := h.series.Load(model)
	if !ok {
		v, _ = h.series.LoadOrStore(model, &modelLatency{})
	}
	v.(*modelLatency).observe(d)
}

// Read returns a copy of model's cumulative counts; ok is false when the
// histogram holds no series for that name. The bucket slice is freshly
// allocated, so later observations cannot change an already-read value.
func (h *LatencyHistogram) Read(model string) (ModelLatency, bool) {
	if h == nil {
		return ModelLatency{}, false
	}
	v, ok := h.series.Load(model)
	if !ok {
		return ModelLatency{}, false
	}
	s := v.(*modelLatency)
	out := ModelLatency{
		Buckets: make([]int64, latencyBucketCount),
		Count:   s.count.Load(),
		SumUs:   s.sumUs.Load(),
	}
	for i := range out.Buckets {
		out.Buckets[i] = s.buckets[i].Load()
	}
	return out, true
}
