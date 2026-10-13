package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"
)

// ---- task 1.2: the metric set covers the spec, once each ----

func TestMetricSetCoversSpec(t *testing.T) {
	required := []string{
		"process.memory.usage", "process.cpu.time", "process.uptime",
		"emb.requests", "emb.tokens", "emb.errors", "emb.active_requests",
		"emb.connections", "emb.cache.hits", "emb.cache.misses",
		"emb.cache.evictions", "emb.models.loaded", "emb.truncated.texts",
		"emb.truncated.pairs", "emb.truncated.images",
		"emb.dispatch.wait", "emb.inference.duration", "emb.inference.runs",
		"emb.sessions.busy", "emb.sessions.total", "emb.padding.efficiency",
		"emb.script.dispatch.wait", "emb.script.inference.duration",
		"emb.script.inference.runs",
		"emb.admission.wait", "emb.cpu.admission.wait", "emb.cpu.admission.waiters",
		"emb.cpu.admission.timeouts",
	}
	counts := map[string]int{}
	for _, d := range metricDefs {
		counts[d.name]++
		if (d.global == nil) == (d.model == nil) {
			t.Errorf("metric %q must set exactly one of global/model", d.name)
		}
		if d.desc == "" || d.unit == "" {
			t.Errorf("metric %q must have a description and unit", d.name)
		}
	}
	for _, name := range required {
		if counts[name] != 1 {
			t.Errorf("metric %q appears %d times, want exactly 1", name, counts[name])
		}
	}
	for _, d := range histogramDefs {
		if d.desc == "" || d.unit == "" {
			t.Errorf("histogram %q must have a description and unit", d.name)
		}
		if counts[d.name] != 0 {
			t.Errorf("histogram %q collides with a metric definition", d.name)
		}
	}
}

// ---- task 1.1: the request-latency histogram accumulator ----

func TestLatencyHistogramBucketPlacement(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want int
	}{
		{10 * time.Microsecond, 0},                                  // below the first boundary
		{50 * time.Microsecond, 0},                                  // exactly on the first boundary
		{51 * time.Microsecond, 1},                                  // just past it
		{100 * time.Microsecond, 1},                                 // exactly on the second boundary
		{150 * time.Microsecond, 2},                                 // between two boundaries
		{1 * time.Second, 13},                                       // exactly on the 1s boundary
		{10 * time.Second, 16},                                      // exactly on the last boundary
		{10*time.Second + time.Microsecond, latencyBucketCount - 1}, // overflow
		{time.Hour, latencyBucketCount - 1},                         // far overflow
	}
	for _, tc := range cases {
		if got := bucketIndex(tc.d.Seconds()); got != tc.want {
			t.Errorf("bucketIndex(%v) = %d, want %d", tc.d, got, tc.want)
		}
	}
}

func TestLatencyHistogramObserveAndRead(t *testing.T) {
	h := NewLatencyHistogram(nil)
	h.Observe("m", 100*time.Microsecond) // bucket 1
	h.Observe("m", 100*time.Microsecond) // bucket 1
	h.Observe("m", 30*time.Second)       // overflow
	h.Observe("other", 1*time.Millisecond)

	got, ok := h.Read("m")
	if !ok {
		t.Fatal("Read returned no series for an observed model")
	}
	if len(got.Buckets) != latencyBucketCount {
		t.Fatalf("buckets = %d entries, want %d", len(got.Buckets), latencyBucketCount)
	}
	if got.Buckets[1] != 2 || got.Buckets[latencyBucketCount-1] != 1 {
		t.Errorf("buckets = %v, want 2 in bucket 1 and 1 in overflow", got.Buckets)
	}
	if got.Count != 3 {
		t.Errorf("count = %d, want 3", got.Count)
	}
	if want := int64(200 + 30_000_000); got.SumUs != want {
		t.Errorf("sum = %dus, want %dus", got.SumUs, want)
	}

	other, ok := h.Read("other")
	if !ok || other.Count != 1 || other.Buckets[4] != 1 {
		t.Errorf("second series = %+v (ok=%v), want one 1ms observation in bucket 4", other, ok)
	}
	if _, ok := h.Read("never-observed"); ok {
		t.Error("Read reported a series for a name never observed")
	}
}

func TestLatencyHistogramOnlyTracksGatedNames(t *testing.T) {
	h := NewLatencyHistogram(func(name string) bool { return name == "known" })
	h.Observe("known", time.Millisecond)
	h.Observe("hostile", time.Millisecond)

	if _, ok := h.Read("known"); !ok {
		t.Error("a tracked name must create a series")
	}
	if _, ok := h.Read("hostile"); ok {
		t.Error("an untracked name must not create a series")
	}
}

func TestLatencyHistogramConcurrentObserve(t *testing.T) {
	h := NewLatencyHistogram(nil)
	const goroutines, perGoroutine = 8, 500
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perGoroutine {
				h.Observe("m", time.Duration(i)*time.Microsecond)
			}
		}()
	}
	wg.Wait()

	got, ok := h.Read("m")
	if !ok {
		t.Fatal("no series after concurrent observations")
	}
	if got.Count != goroutines*perGoroutine {
		t.Errorf("count = %d, want %d", got.Count, goroutines*perGoroutine)
	}
	var sum int64
	for _, b := range got.Buckets {
		sum += b
	}
	if sum != got.Count {
		t.Errorf("bucket counts sum to %d, want the count %d", sum, got.Count)
	}
}

// ---- task 1.2: a snapshot does not alias the live accumulator ----

func TestLatencyHistogramSnapshotIsIndependent(t *testing.T) {
	h := NewLatencyHistogram(nil)
	h.Observe("m", time.Millisecond)

	lat, ok := h.Read("m")
	if !ok {
		t.Fatal("no series after the first observation")
	}
	snap := Snapshot{Models: []ModelSnapshot{{Name: "m", Latency: lat}}}
	before := append([]int64(nil), snap.Models[0].Latency.Buckets...)

	// Mutate the accumulator after the snapshot was built.
	h.Observe("m", 5*time.Second)
	h.Observe("m", 5*time.Second)

	got := snap.Models[0].Latency
	if got.Count != 1 || got.SumUs != 1000 {
		t.Errorf("snapshot count/sum changed after later observations: %+v", got)
	}
	for i := range before {
		if got.Buckets[i] != before[i] {
			t.Fatalf("snapshot bucket %d changed from %d to %d after later observations", i, before[i], got.Buckets[i])
		}
	}
}

// ---- task 2.1: OTLP JSON structure ----

func fixedSnapshot() Snapshot {
	return Snapshot{
		Uptime: 42, ActiveRequests: 3, Connections: 2, Goroutines: 9, ModelsLoaded: 1,
		RSSBytes: 100 << 20, HeapBytes: 20 << 20, TotalSystemMemory: 16 << 30,
		CPUTimeUserUsec: 1_500_000, CPUTimeSysUsec: 500_000,
		NetInBytes: 1000, NetOutBytes: 2000,
		CacheHits: 7, CacheMisses: 2, CacheEvictions: 1,
		TotalRequests: 10, TotalTokens: 55, TotalErrors: 1,
		TruncatedTexts: 4, TruncatedPairs: 5, TruncatedImages: 6,
		Models: []ModelSnapshot{{Name: "minilm", Requests: 6, Tokens: 30, Errors: 1}},
	}
}

func decode(t *testing.T, payload []byte) exportRequest {
	t.Helper()
	var req exportRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return req
}

func lookupMetric(req exportRequest, name string) (metric, bool) {
	for _, sm := range req.ResourceMetrics[0].ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metric{}, false
}

func findMetric(t *testing.T, req exportRequest, name string) metric {
	t.Helper()
	m, ok := lookupMetric(req, name)
	if !ok {
		t.Fatalf("metric %q not found", name)
	}
	return m
}

// datapointModels lists the model attribute of every datapoint of a metric,
// whichever instrument shape carries them.
func datapointModels(m metric) []string {
	var points []numberDataPoint
	switch {
	case m.Sum != nil:
		points = m.Sum.DataPoints
	case m.Gauge != nil:
		points = m.Gauge.DataPoints
	}
	out := make([]string, 0, len(points))
	for _, p := range points {
		for _, a := range p.Attributes {
			if a.Key == "model" && a.Value.StringValue != nil {
				out = append(out, *a.Value.StringValue)
			}
		}
	}
	return out
}

// latencySeries returns a fresh bucket slice with the given per-bucket counts.
func latencySeries(counts map[int]int64) []int64 {
	b := make([]int64, latencyBucketCount)
	for i, c := range counts {
		b[i] = c
	}
	return b
}

// withLatency attaches a cumulative latency series to the named model.
func withLatency(s Snapshot, model string, buckets []int64, sumUs int64) Snapshot {
	var count int64
	for _, b := range buckets {
		count += b
	}
	for i := range s.Models {
		if s.Models[i].Name == model {
			s.Models[i].Latency = ModelLatency{Buckets: buckets, Count: count, SumUs: sumUs}
		}
	}
	return s
}

// ---- task 2.1: the histogram wire shape and its delta diffing ----

func TestEncodeHistogramStructureAndDelta(t *testing.T) {
	prev := withLatency(fixedSnapshot(), "minilm", latencySeries(map[int]int64{2: 1}), 1000)
	cur := withLatency(fixedSnapshot(), "minilm",
		latencySeries(map[int]int64{2: 1, 4: 2, latencyBucketCount - 1: 1}), 4000)

	m := findMetric(t, decode(t, mustEncode(t, cur, &prev)), "emb.request.duration")
	if m.Histogram == nil {
		t.Fatalf("emb.request.duration is not a histogram: %+v", m)
	}
	if m.Histogram.AggregationTemporality != aggregationTemporalityDelta {
		t.Errorf("temporality = %d, want delta", m.Histogram.AggregationTemporality)
	}
	if m.Unit != "s" || m.Description == "" {
		t.Errorf("histogram metadata missing: %+v", m)
	}
	if len(m.Histogram.DataPoints) != 1 {
		t.Fatalf("want 1 datapoint, got %d", len(m.Histogram.DataPoints))
	}
	dp := m.Histogram.DataPoints[0]
	if len(dp.Attributes) != 1 || dp.Attributes[0].Key != "model" || *dp.Attributes[0].Value.StringValue != "minilm" {
		t.Errorf("datapoint attributes = %+v", dp.Attributes)
	}
	if dp.StartTimeUnixNano != "1000" || dp.TimeUnixNano != "2000" {
		t.Errorf("datapoint timestamps = %q..%q", dp.StartTimeUnixNano, dp.TimeUnixNano)
	}
	if dp.Count != "3" {
		t.Errorf("count = %s, want 3 (2 in bucket 4, 1 in overflow)", dp.Count)
	}
	if dp.Sum == nil || math.Abs(*dp.Sum-0.003) > 1e-12 {
		t.Errorf("sum = %v, want 0.003s (3000us)", dp.Sum)
	}
	if len(dp.BucketCounts) != len(dp.ExplicitBounds)+1 {
		t.Fatalf("bucket counts = %d, bounds = %d; want exactly one more count than bounds",
			len(dp.BucketCounts), len(dp.ExplicitBounds))
	}
	for i := 1; i < len(dp.ExplicitBounds); i++ {
		if dp.ExplicitBounds[i] <= dp.ExplicitBounds[i-1] {
			t.Fatalf("bounds not strictly ascending at %d: %v", i, dp.ExplicitBounds)
		}
	}
	want := latencySeries(map[int]int64{4: 2, latencyBucketCount - 1: 1})
	for i, w := range want {
		if dp.BucketCounts[i] != strconv.FormatInt(w, 10) {
			t.Errorf("bucket %d = %s, want %d", i, dp.BucketCounts[i], w)
		}
	}
}

func TestEncodeHistogramSkipsFirstTickAndNewModels(t *testing.T) {
	cur := withLatency(fixedSnapshot(), "minilm", latencySeries(map[int]int64{0: 1}), 500)
	if m, ok := lookupMetric(decode(t, mustEncode(t, cur, nil)), "emb.request.duration"); ok {
		t.Errorf("first tick emitted a histogram with no previous measurement: %+v", m)
	}

	prev := fixedSnapshot() // no latency series yet
	cur2 := fixedSnapshot()
	cur2.Models = append(cur2.Models, ModelSnapshot{
		Name: "bge", HasPool: true,
		Latency: ModelLatency{Buckets: latencySeries(map[int]int64{0: 1}), Count: 1, SumUs: 10},
	})
	if m, ok := lookupMetric(decode(t, mustEncode(t, cur2, &prev)), "emb.request.duration"); ok {
		t.Errorf("a model absent from the previous snapshot must emit no histogram: %+v", m)
	}
}

func TestEncodeHistogramSkipsIntervalWithoutObservations(t *testing.T) {
	prev := withLatency(fixedSnapshot(), "minilm", latencySeries(map[int]int64{0: 1}), 500)
	cur := withLatency(fixedSnapshot(), "minilm", latencySeries(map[int]int64{0: 1}), 500)
	if m, ok := lookupMetric(decode(t, mustEncode(t, cur, &prev)), "emb.request.duration"); ok {
		t.Errorf("an interval that observed no request must emit no histogram: %+v", m)
	}
}

// ---- task 2.2: per-model timing sums, session gauges, and pool gating ----

func TestEncodeModelTimingAndSessionSeries(t *testing.T) {
	prev := fixedSnapshot()
	prev.Models[0].HasPool = true
	prev.Models[0].DispatchWaitUs = 1_000_000
	prev.Models[0].RunUs = 3_000_000
	prev.Models[0].Runs = 1
	prev.Models[0].SessionsTotal = 2
	prev.Models[0].PaddingEfficiency = 0.5

	cur := fixedSnapshot()
	cur.Models[0].HasPool = true
	cur.Models[0].DispatchWaitUs = 2_000_000
	cur.Models[0].RunUs = 5_000_000
	cur.Models[0].Runs = 4
	cur.Models[0].SessionsBusy = 2
	cur.Models[0].SessionsTotal = 2
	cur.Models[0].PaddingEfficiency = 0.75
	cur.Models[0].ScriptDispatchWaitUs = 500
	cur.Models[0].ScriptRunUs = 4_000
	cur.Models[0].ScriptRuns = 2

	req := decode(t, mustEncode(t, cur, &prev))

	if got := *findMetric(t, req, "emb.dispatch.wait").Sum.DataPoints[0].AsDouble; math.Abs(got-1.0) > 1e-12 {
		t.Errorf("dispatch wait delta = %v, want 1s", got)
	}
	if got := *findMetric(t, req, "emb.inference.duration").Sum.DataPoints[0].AsDouble; math.Abs(got-2.0) > 1e-12 {
		t.Errorf("inference duration delta = %v, want 2s", got)
	}
	if got := *findMetric(t, req, "emb.inference.runs").Sum.DataPoints[0].AsInt; got != "3" {
		t.Errorf("inference runs delta = %s, want 3", got)
	}
	// The admission wait is its own series, exported apart from the run time.
	cur.Models[0].AdmissionWaitUs = 1_500_000
	cur.CPUAdmissionWaitUs = 1_500_000
	cur.CPUAdmissionWaiters = 2
	cur.CPUAdmissionTimeouts = 1
	req = decode(t, mustEncode(t, cur, &prev))
	if got := *findMetric(t, req, "emb.admission.wait").Sum.DataPoints[0].AsDouble; math.Abs(got-1.5) > 1e-12 {
		t.Errorf("admission wait delta = %v, want 1.5s", got)
	}
	if got := *findMetric(t, req, "emb.cpu.admission.wait").Sum.DataPoints[0].AsDouble; math.Abs(got-1.5) > 1e-12 {
		t.Errorf("gate admission wait delta = %v, want 1.5s", got)
	}
	if got := *findMetric(t, req, "emb.cpu.admission.waiters").Gauge.DataPoints[0].AsInt; got != "2" {
		t.Errorf("gate waiters = %s, want 2", got)
	}
	if got := *findMetric(t, req, "emb.cpu.admission.timeouts").Sum.DataPoints[0].AsInt; got != "1" {
		t.Errorf("gate admission timeouts = %s, want 1", got)
	}
	if got := *findMetric(t, req, "emb.script.inference.runs").Sum.DataPoints[0].AsInt; got != "2" {
		t.Errorf("script runs delta = %s, want 2", got)
	}

	busy := findMetric(t, req, "emb.sessions.busy")
	if busy.Gauge == nil || len(busy.Gauge.DataPoints) != 1 {
		t.Fatalf("emb.sessions.busy is not a per-model gauge: %+v", busy)
	}
	if got := *busy.Gauge.DataPoints[0].AsInt; got != "2" {
		t.Errorf("sessions busy = %s, want 2", got)
	}
	if got := *findMetric(t, req, "emb.sessions.total").Gauge.DataPoints[0].AsInt; got != "2" {
		t.Errorf("sessions total = %s, want 2", got)
	}
	if got := *findMetric(t, req, "emb.padding.efficiency").Gauge.DataPoints[0].AsDouble; got != 0.75 {
		t.Errorf("padding efficiency = %v, want 0.75", got)
	}
}

func TestEncodeModelSeriesOmitModelsWithoutPool(t *testing.T) {
	prev := fixedSnapshot()
	prev.Models = append(prev.Models, ModelSnapshot{Name: "gliner"})
	cur := fixedSnapshot()
	cur.Models[0].HasPool = true
	cur.Models = append(cur.Models, ModelSnapshot{Name: "gliner"})

	req := decode(t, mustEncode(t, cur, &prev))
	for _, name := range []string{
		"emb.dispatch.wait", "emb.inference.duration", "emb.inference.runs",
		"emb.sessions.busy", "emb.sessions.total", "emb.padding.efficiency",
		"emb.admission.wait",
	} {
		m := findMetric(t, req, name)
		for _, got := range datapointModels(m) {
			if got == "gliner" {
				t.Errorf("%s exported a datapoint for a model with no embedding pool", name)
			}
		}
	}
	// The scripted path is independent of the pool, so it still reports the
	// model's (zero) scripted counters.
	if got := datapointModels(findMetric(t, req, "emb.script.inference.runs")); len(got) != 2 {
		t.Errorf("script series models = %v, want both models", got)
	}
}

func TestEncodeModelGaugesOnFirstTick(t *testing.T) {
	cur := fixedSnapshot()
	cur.Models[0].HasPool = true
	cur.Models[0].SessionsBusy = 1
	cur.Models[0].SessionsTotal = 3

	req := decode(t, mustEncode(t, cur, nil))
	if got := *findMetric(t, req, "emb.sessions.busy").Gauge.DataPoints[0].AsInt; got != "1" {
		t.Errorf("first tick sessions busy = %s, want the current gauge value 1", got)
	}
	if _, ok := lookupMetric(req, "emb.inference.runs"); ok {
		t.Error("first tick must not emit a sum with no previous measurement")
	}
}

func TestEncodeRequestStructure(t *testing.T) {
	prev := fixedSnapshot()
	cur := fixedSnapshot()
	cur.TotalRequests += 5
	cur.ActiveRequests = 4
	res := Resource{
		ServiceName: "emb", ServiceVersion: "1.2.3", Host: "h1", PID: 4242,
		Executable: "emb", Extra: map[string]string{"deployment.environment": "prod"},
	}
	payload, err := encodeRequest(cur, &prev, 1000, 2000, res)
	if err != nil {
		t.Fatal(err)
	}
	req := decode(t, payload)

	if len(req.ResourceMetrics) != 1 {
		t.Fatalf("want one resourceMetrics, got %d", len(req.ResourceMetrics))
	}
	rm := req.ResourceMetrics[0]
	attrs := map[string]string{}
	for _, a := range rm.Resource.Attributes {
		if a.Value.StringValue != nil {
			attrs[a.Key] = *a.Value.StringValue
		}
	}
	for k, want := range map[string]string{
		"service.name": "emb", "service.version": "1.2.3", "host.name": "h1",
		"process.pid": "4242", "process.executable.name": "emb",
		"deployment.environment": "prod",
	} {
		if attrs[k] != want {
			t.Errorf("resource attribute %q = %q, want %q", k, attrs[k], want)
		}
	}
	if rm.ScopeMetrics[0].Scope.Name != "emb" {
		t.Errorf("scope name = %q", rm.ScopeMetrics[0].Scope.Name)
	}

	mem := findMetric(t, req, "process.memory.usage")
	if mem.Gauge == nil || len(mem.Gauge.DataPoints) != 1 {
		t.Fatalf("process.memory.usage is not a single-point gauge: %+v", mem)
	}
	if got := *mem.Gauge.DataPoints[0].AsInt; got != "104857600" {
		t.Errorf("process.memory.usage = %s", got)
	}
	if mem.Unit != "By" || mem.Description == "" {
		t.Errorf("process.memory.usage metadata missing: %+v", mem)
	}

	reqs := findMetric(t, req, "emb.requests")
	if reqs.Sum == nil {
		t.Fatalf("emb.requests is not a sum: %+v", reqs)
	}
	if reqs.Sum.AggregationTemporality != aggregationTemporalityDelta || !reqs.Sum.IsMonotonic {
		t.Errorf("emb.requests sum metadata wrong: %+v", reqs.Sum)
	}
	if len(reqs.Sum.DataPoints) != 1 {
		t.Fatalf("emb.requests want 1 datapoint, got %d", len(reqs.Sum.DataPoints))
	}
	dp := reqs.Sum.DataPoints[0]
	if dp.Attributes[0].Key != "model" || *dp.Attributes[0].Value.StringValue != "minilm" {
		t.Errorf("emb.requests model attribute wrong: %+v", dp.Attributes)
	}
	if dp.StartTimeUnixNano != "1000" || dp.TimeUnixNano != "2000" {
		t.Errorf("emb.requests timestamps = %q..%q", dp.StartTimeUnixNano, dp.TimeUnixNano)
	}
}

// ---- task 2.2: delta temporality ----

func TestEncodeFirstTickOmitsSums(t *testing.T) {
	cur := fixedSnapshot()
	payload, err := encodeRequest(cur, nil, 0, 2000, Resource{ServiceName: "emb"})
	if err != nil {
		t.Fatal(err)
	}
	req := decode(t, payload)
	for _, m := range req.ResourceMetrics[0].ScopeMetrics[0].Metrics {
		if m.Sum != nil {
			t.Errorf("first tick emitted sum %q with no previous measurement", m.Name)
		}
	}
	if findMetric(t, req, "emb.active_requests").Gauge == nil {
		t.Error("first tick must still emit gauges")
	}
}

func TestEncodeDeltaReflectsInterval(t *testing.T) {
	prev := fixedSnapshot()
	cur := fixedSnapshot()
	cur.TotalRequests += 5
	cur.CacheHits += 3
	cur.Models[0].Tokens += 11

	req := decode(t, mustEncode(t, cur, &prev))
	if got := *findMetric(t, req, "emb.cache.hits").Sum.DataPoints[0].AsInt; got != "3" {
		t.Errorf("cache.hits delta = %s, want 3", got)
	}
	tokens := findMetric(t, req, "emb.tokens")
	if got := *tokens.Sum.DataPoints[0].AsInt; got != "11" {
		t.Errorf("tokens delta = %s, want 11", got)
	}
}

func TestEncodeDeltaNeverNegative(t *testing.T) {
	prev := fixedSnapshot()
	cur := fixedSnapshot()
	cur.CacheHits = prev.CacheHits - 5 // counter reset (restart)
	cur.Models[0].Requests = prev.Models[0].Requests - 2

	req := decode(t, mustEncode(t, cur, &prev))
	if got := *findMetric(t, req, "emb.cache.hits").Sum.DataPoints[0].AsInt; got != "0" {
		t.Errorf("reset cache.hits delta = %s, want 0", got)
	}
	if got := *findMetric(t, req, "emb.requests").Sum.DataPoints[0].AsInt; got != "0" {
		t.Errorf("reset per-model delta = %s, want 0", got)
	}
}

func TestEncodeSkipsUnseenModel(t *testing.T) {
	prev := fixedSnapshot()
	cur := fixedSnapshot()
	cur.Models = append(cur.Models, ModelSnapshot{Name: "bge", Requests: 9})

	req := decode(t, mustEncode(t, cur, &prev))
	reqs := findMetric(t, req, "emb.requests")
	if len(reqs.Sum.DataPoints) != 1 {
		t.Fatalf("a model seen for the first time must not report a delta, got %d points", len(reqs.Sum.DataPoints))
	}
	if got := *reqs.Sum.DataPoints[0].Attributes[0].Value.StringValue; got != "minilm" {
		t.Errorf("unexpected model %q", got)
	}
}

func mustEncode(t *testing.T, cur Snapshot, prev *Snapshot) []byte {
	t.Helper()
	payload, err := encodeRequest(cur, prev, 1000, 2000, Resource{ServiceName: "emb"})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// ---- task 3.1: configuration precedence ----

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func strPtr(s string) *string { return &s }

func durPtr(d time.Duration) *time.Duration { return &d }

func TestConfigPrecedence(t *testing.T) {
	env := envFrom(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":        "http://env:4318",
		"OTEL_METRIC_EXPORT_INTERVAL":        "15000",
		"OTEL_SERVICE_NAME":                  "env-svc",
		"OTEL_RESOURCE_ATTRIBUTES":           "team=core",
		"OTEL_EXPORTER_OTLP_METRICS_HEADERS": "dd-api-key=env-key",
	})
	base := FromEnv(env)
	if base.Endpoint != "http://env:4318" || base.Interval != 15*time.Second || base.ServiceName != "env-svc" {
		t.Fatalf("FromEnv = %+v", base)
	}

	yaml := Overlay{Endpoint: strPtr("http://yaml:4318"), Interval: durPtr(45 * time.Second)}
	flags := Overlay{Endpoint: strPtr("http://flag:4318"), Headers: []string{"dd-api-key=flag-key"}}

	cfg := base.With(yaml).With(flags)
	if cfg.Endpoint != "http://flag:4318" {
		t.Errorf("flag must beat YAML and env, got %q", cfg.Endpoint)
	}
	if cfg.Interval != 45*time.Second {
		t.Errorf("YAML interval must beat env, got %v", cfg.Interval)
	}
	if cfg.Headers["dd-api-key"] != "flag-key" {
		t.Errorf("flag headers must win, got %v", cfg.Headers)
	}
	if cfg.ServiceName != "env-svc" || cfg.Extra["team"] != "core" {
		t.Errorf("unset overlay fields must keep env values: %+v", cfg)
	}
}

func TestConfigSDKDisabledWins(t *testing.T) {
	base := FromEnv(envFrom(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://env:4318",
		"OTEL_SDK_DISABLED":           "true",
	}))
	enabled := false
	cfg := base.With(Overlay{Disabled: &enabled}) // flags try to re-enable
	if cfg.Enabled() {
		t.Fatalf("OTEL_SDK_DISABLED must win over a later overlay: %+v", cfg)
	}
}

func TestConfigIntervalDefault(t *testing.T) {
	cfg := Config{Endpoint: "http://x:4318"}
	if cfg.IntervalOrDefault() != DefaultInterval {
		t.Errorf("IntervalOrDefault = %v, want %v", cfg.IntervalOrDefault(), DefaultInterval)
	}
}

// ---- task 3.2 / 3.3: the periodic reader ----

type fakeSender struct {
	mu       sync.Mutex
	calls    int
	payloads [][]byte
	block    chan struct{}
	blockCh  chan struct{}
	err      error
}

func (f *fakeSender) Send(ctx context.Context, payload []byte, _ map[string]string) error {
	f.mu.Lock()
	f.calls++
	f.blockCh = f.block
	err := f.err
	f.mu.Unlock()
	if f.blockCh != nil {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.payloads = append(f.payloads, append([]byte(nil), payload...))
	f.mu.Unlock()
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func testConfig(endpoint string) Config {
	return Config{Endpoint: endpoint, Interval: time.Hour}
}

func TestExporterSkipsTickWhileSendInFlight(t *testing.T) {
	gate := make(chan struct{})
	sender := &fakeSender{block: gate}
	snaps := 0
	e := New(testConfig("http://x"), func() Snapshot { snaps++; return fixedSnapshot() }, WithSender(sender))

	done := make(chan struct{})
	go func() { e.tick(context.Background()); close(done) }()

	// Wait until the first send is in flight.
	deadline := time.Now().Add(2 * time.Second)
	for sender.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.tick(context.Background()) // must be skipped, not queued
	if got := sender.count(); got != 1 {
		t.Fatalf("a tick during an in-flight send must be skipped, sends=%d", got)
	}
	if snaps != 1 {
		t.Fatalf("a skipped tick must not snapshot, snapshots=%d", snaps)
	}
	close(gate)
	<-done
}

func TestExporterThrottlesFailureLogs(t *testing.T) {
	sender := &fakeSender{err: errors.New("boom")}
	var logs int
	e := New(testConfig("http://x"), fixedSnapshot,
		WithSender(sender), WithLogger(func(string, ...any) { logs++ }))

	for range 5 {
		e.tick(context.Background())
	}
	if logs != 1 {
		t.Fatalf("five consecutive failures must log once, got %d", logs)
	}

	sender.mu.Lock()
	sender.err = nil
	sender.mu.Unlock()
	e.tick(context.Background())
	if logs != 2 {
		t.Fatalf("recovery must log once, got %d", logs)
	}
}

func fixedSnapshotFunc() Snapshot { return fixedSnapshot() }

func TestExporterShutdownFlushesWithinDeadline(t *testing.T) {
	sender := &fakeSender{}
	e := New(testConfig("http://x"), fixedSnapshotFunc, WithSender(sender))
	e.Start(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	e.Shutdown(ctx)
	if time.Since(start) > time.Second {
		t.Fatal("shutdown took too long")
	}
	if sender.count() < 1 {
		t.Fatal("shutdown must perform a final export")
	}
}

func TestExporterShutdownUnreachableIsNonFatal(t *testing.T) {
	sender := &fakeSender{err: errors.New("connection refused")}
	e := New(testConfig("http://x"), fixedSnapshotFunc,
		WithSender(sender), WithLogger(func(string, ...any) {}))
	e.Start(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	e.Shutdown(ctx) // must return without error or delay
}

func TestExporterDisabledDoesNotSend(t *testing.T) {
	sender := &fakeSender{}
	e := New(Config{Endpoint: "http://x", Disabled: true}, fixedSnapshotFunc, WithSender(sender))
	e.Start(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e.Shutdown(ctx)
	if sender.count() != 0 {
		t.Fatalf("disabled exporter sent %d payloads", sender.count())
	}
}
