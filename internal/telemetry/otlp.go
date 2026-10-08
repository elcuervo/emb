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
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Gauge       *gauge `json:"gauge,omitempty"`
	Sum         *sum   `json:"sum,omitempty"`
}

type gauge struct {
	DataPoints []numberDataPoint `json:"dataPoints"`
}

type sum struct {
	AggregationTemporality int               `json:"aggregationTemporality"`
	IsMonotonic            bool              `json:"isMonotonic"`
	DataPoints             []numberDataPoint `json:"dataPoints"`
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
	metrics := make([]metric, 0, len(metricDefs))
	for _, def := range metricDefs {
		m := encodeMetric(def, cur, prev, startNano, nowNano)
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

// encodeModelMetric renders a per-model sum with one datapoint per model that
// was present in the previous snapshot too (a model seen for the first time
// has no measured delta yet).
func encodeModelMetric(def metricDef, cur Snapshot, prev *Snapshot, startNano, nowNano uint64) *metric {
	if prev == nil {
		return nil
	}
	prevByName := make(map[string]ModelSnapshot, len(prev.Models))
	for _, m := range prev.Models {
		prevByName[m.Name] = m
	}
	points := make([]numberDataPoint, 0, len(cur.Models))
	for _, m := range cur.Models {
		pm, ok := prevByName[m.Name]
		if !ok {
			continue
		}
		delta := def.model(m) - def.model(pm)
		if delta < 0 {
			delta = 0
		}
		points = append(points, numberDataPointFor(def, delta, startNano, nowNano,
			[]attribute{strAttr("model", m.Name)}))
	}
	if len(points) == 0 {
		return nil
	}
	return &metric{
		Name: def.name, Description: def.desc, Unit: def.unit,
		Sum: &sum{
			AggregationTemporality: aggregationTemporalityDelta,
			IsMonotonic:            true,
			DataPoints:             points,
		},
	}
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
