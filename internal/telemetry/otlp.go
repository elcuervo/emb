package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// aggregationTemporalityDelta is OTLP's AGGREGATION_TEMPORALITY_DELTA (1).
const aggregationTemporalityDelta = 1

// Resource identifies the exporting service and host as OTLP resource
// attributes.
type Resource struct {
	ServiceName    string
	ServiceVersion string
	Host           string
	PID            int64
	Executable     string
	Extra          map[string]string
}

// defaultResource builds the resource from configuration plus live process
// facts. Callers that need determinism (tests) construct a Resource directly.
func defaultResource(cfg Config) Resource {
	host := cfg.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	exe, _ := os.Executable()
	return Resource{
		ServiceName:    firstNonEmpty(cfg.ServiceName, "emb"),
		ServiceVersion: cfg.ServiceVersion,
		Host:           host,
		PID:            int64(os.Getpid()),
		Executable:     filepath.Base(exe),
		Extra:          cfg.Extra,
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---- OTLP/JSON wire types (the subset the exporter needs) ----

type exportRequest struct {
	ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
}

type resourceMetrics struct {
	Resource     resource       `json:"resource"`
	ScopeMetrics []scopeMetrics `json:"scopeMetrics"`
}

type resource struct {
	Attributes []attribute `json:"attributes"`
}

type scopeMetrics struct {
	Scope   scope    `json:"scope"`
	Metrics []metric `json:"metrics"`
}

type scope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type metric struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	Gauge       *gauge     `json:"gauge,omitempty"`
	Sum         *sum       `json:"sum,omitempty"`
	Histogram   *histogram `json:"histogram,omitempty"`
}

type gauge struct {
	DataPoints []numberDataPoint `json:"dataPoints"`
}

type sum struct {
	AggregationTemporality int               `json:"aggregationTemporality"`
	IsMonotonic            bool              `json:"isMonotonic"`
	DataPoints             []numberDataPoint `json:"dataPoints"`
}

// histogram is an OTLP explicit-bucket histogram with delta temporality, the
// wire shape of the request-latency distribution.
type histogram struct {
	AggregationTemporality int                  `json:"aggregationTemporality"`
	DataPoints             []histogramDataPoint `json:"dataPoints"`
}

// histogramDataPoint is one interval's distribution: the interval's count and
// sum, one bucket count per declared boundary plus the overflow bucket, and the
// strictly ascending bounds those counts index. count and bucketCounts are
// uint64 on the wire, which proto3 JSON encodes as decimal strings.
type histogramDataPoint struct {
	Attributes        []attribute `json:"attributes,omitempty"`
	StartTimeUnixNano string      `json:"startTimeUnixNano,omitempty"`
	TimeUnixNano      string      `json:"timeUnixNano"`
	Count             string      `json:"count"`
	Sum               *float64    `json:"sum,omitempty"`
	BucketCounts      []string    `json:"bucketCounts"`
	ExplicitBounds    []float64   `json:"explicitBounds"`
}

type numberDataPoint struct {
	Attributes        []attribute `json:"attributes,omitempty"`
	StartTimeUnixNano string      `json:"startTimeUnixNano,omitempty"`
	TimeUnixNano      string      `json:"timeUnixNano"`
	AsInt             *string     `json:"asInt,omitempty"`
	AsDouble          *float64    `json:"asDouble,omitempty"`
}

type attribute struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type anyValue struct {
	StringValue *string `json:"stringValue,omitempty"`
}

func strAttr(key, value string) attribute {
	v := value
	return attribute{Key: key, Value: anyValue{StringValue: &v}}
}

func resourceAttributes(r Resource) []attribute {
	attrs := []attribute{strAttr("service.name", r.ServiceName)}
	if r.ServiceVersion != "" {
		attrs = append(attrs, strAttr("service.version", r.ServiceVersion))
	}
	if r.Host != "" {
		attrs = append(attrs, strAttr("host.name", r.Host))
	}
	if r.PID != 0 {
		attrs = append(attrs, strAttr("process.pid", strconv.FormatInt(r.PID, 10)))
	}
	if r.Executable != "" {
		attrs = append(attrs, strAttr("process.executable.name", r.Executable))
	}
	for _, k := range sortedKeys(r.Extra) {
		attrs = append(attrs, strAttr(k, r.Extra[k]))
	}
	return attrs
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Small map; insertion sort keeps this allocation-free of extra imports.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// encodeRequest builds the OTLP/HTTP JSON body. prev is the previous tick's
// snapshot (nil on the first tick, when no sums are emitted). startNano is the
// previous tick's time; nowNano is this tick's time.
func encodeRequest(cur Snapshot, prev *Snapshot, startNano, nowNano uint64, res Resource) ([]byte, error) {
	metrics := make([]metric, 0, len(metricDefs)+len(histogramDefs))
	for _, def := range metricDefs {
		m := encodeMetric(def, cur, prev, startNano, nowNano)
		if m != nil {
			metrics = append(metrics, *m)
		}
	}
	for _, def := range histogramDefs {
		m := encodeHistogram(def, cur, prev, startNano, nowNano)
		if m != nil {
			metrics = append(metrics, *m)
		}
	}
	req := exportRequest{
		ResourceMetrics: []resourceMetrics{{
			Resource: resource{Attributes: resourceAttributes(res)},
			ScopeMetrics: []scopeMetrics{{
				Scope:   scope{Name: "emb", Version: res.ServiceVersion},
				Metrics: metrics,
			}},
		}},
	}
	return json.Marshal(req)
}

// encodeMetric renders one metric. Sums are skipped when there is no previous
// tick to diff against; gauges always render.
func encodeMetric(def metricDef, cur Snapshot, prev *Snapshot, startNano, nowNano uint64) *metric {
	if def.model != nil {
		return encodeModelMetric(def, cur, prev, startNano, nowNano)
	}
	m := metric{Name: def.name, Description: def.desc, Unit: def.unit}
	switch def.kind {
	case Gauge:
		m.Gauge = &gauge{DataPoints: []numberDataPoint{
			numberDataPointFor(def, def.global(cur), 0, nowNano, nil),
		}}
	case Sum:
		if prev == nil {
			return nil
		}
		delta := def.global(cur) - def.global(*prev)
		if delta < 0 {
			delta = 0
		}
		m.Sum = &sum{
			AggregationTemporality: aggregationTemporalityDelta,
			IsMonotonic:            true,
			DataPoints: []numberDataPoint{
				numberDataPointFor(def, delta, startNano, nowNano, nil),
			},
		}
	}
	return &m
}

// encodeModelMetric renders one datapoint per model: a sum is diffed against
// the previous snapshot (a model seen for the first time has no measured delta
// yet), a gauge reports its current value. Metrics whose source only exists for
// models with a loaded embedding pool skip models without one entirely.
func encodeModelMetric(def metricDef, cur Snapshot, prev *Snapshot, startNano, nowNano uint64) *metric {
	if def.kind == Sum && prev == nil {
		return nil
	}
	var prevByName map[string]ModelSnapshot
	if prev != nil {
		prevByName = make(map[string]ModelSnapshot, len(prev.Models))
		for _, m := range prev.Models {
			prevByName[m.Name] = m
		}
	}
	points := make([]numberDataPoint, 0, len(cur.Models))
	for _, m := range cur.Models {
		if def.needsPool && !m.HasPool {
			continue
		}
		attrs := []attribute{strAttr("model", m.Name)}
		if def.kind == Gauge {
			points = append(points, numberDataPointFor(def, def.model(m), 0, nowNano, attrs))
			continue
		}
		pm, ok := prevByName[m.Name]
		if !ok {
			continue
		}
		delta := def.model(m) - def.model(pm)
		if delta < 0 {
			delta = 0
		}
		points = append(points, numberDataPointFor(def, delta, startNano, nowNano, attrs))
	}
	if len(points) == 0 {
		return nil
	}
	out := metric{Name: def.name, Description: def.desc, Unit: def.unit}
	if def.kind == Gauge {
		out.Gauge = &gauge{DataPoints: points}
		return &out
	}
	out.Sum = &sum{
		AggregationTemporality: aggregationTemporalityDelta,
		IsMonotonic:            true,
		DataPoints:             points,
	}
	return &out
}

// encodeHistogram renders one explicit-bucket histogram with delta
// temporality: one datapoint per model present in the previous snapshot that
// observed at least one request during the interval. The counts are diffed
// exactly as sums are, so the first tick after startup and a model new to the
// snapshot emit nothing.
func encodeHistogram(def histogramDef, cur Snapshot, prev *Snapshot, startNano, nowNano uint64) *metric {
	if prev == nil {
		return nil
	}
	prevByName := make(map[string]ModelSnapshot, len(prev.Models))
	for _, m := range prev.Models {
		prevByName[m.Name] = m
	}
	want := len(def.bounds) + 1
	points := make([]histogramDataPoint, 0, len(cur.Models))
	for _, m := range cur.Models {
		pm, ok := prevByName[m.Name]
		if !ok || len(m.Latency.Buckets) != want || len(pm.Latency.Buckets) != want {
			continue
		}
		dp, ok := histogramDeltaPoint(def.bounds, m, pm, startNano, nowNano)
		if !ok {
			continue
		}
		points = append(points, dp)
	}
	if len(points) == 0 {
		return nil
	}
	return &metric{
		Name: def.name, Description: def.desc, Unit: def.unit,
		Histogram: &histogram{
			AggregationTemporality: aggregationTemporalityDelta,
			DataPoints:             points,
		},
	}
}

// histogramDeltaPoint builds one interval datapoint from a model's cumulative
// counts and the previous tick's. It reports ok=false when the interval holds
// no observation. The count is the sum of the per-bucket deltas, so the bucket
// counts always add up to the count even when a counter reset clamps one.
func histogramDeltaPoint(bounds []float64, cur, prev ModelSnapshot, startNano, nowNano uint64) (histogramDataPoint, bool) {
	counts := make([]string, len(cur.Latency.Buckets))
	var count int64
	for i, c := range cur.Latency.Buckets {
		d := c - prev.Latency.Buckets[i]
		if d < 0 {
			d = 0
		}
		count += d
		counts[i] = strconv.FormatInt(d, 10)
	}
	if count == 0 {
		return histogramDataPoint{}, false
	}
	sumUs := cur.Latency.SumUs - prev.Latency.SumUs
	if sumUs < 0 {
		sumUs = 0
	}
	sum := float64(sumUs) / 1e6
	return histogramDataPoint{
		Attributes:        []attribute{strAttr("model", cur.Name)},
		StartTimeUnixNano: strconv.FormatUint(startNano, 10),
		TimeUnixNano:      strconv.FormatUint(nowNano, 10),
		Count:             strconv.FormatInt(count, 10),
		Sum:               &sum,
		BucketCounts:      counts,
		ExplicitBounds:    bounds,
	}, true
}

func numberDataPointFor(def metricDef, v float64, startNano, nowNano uint64, attrs []attribute) numberDataPoint {
	p := numberDataPoint{
		TimeUnixNano: strconv.FormatUint(nowNano, 10),
		Attributes:   attrs,
	}
	if def.kind == Sum {
		p.StartTimeUnixNano = strconv.FormatUint(startNano, 10)
	}
	if def.num == DoubleNumber {
		p.AsDouble = &v
	} else {
		s := strconv.FormatInt(int64(v), 10)
		p.AsInt = &s
	}
	return p
}
