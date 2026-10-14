package telemetry

// Kind is the OTLP instrument kind the exporter emits for a metric.
type Kind int

const (
	// Gauge reports a value that can go up or down; exported as-is every tick.
	Gauge Kind = iota
	// Sum reports a value that only increases; exported with delta temporality.
	Sum
)

// NumberType selects the OTLP data-point number encoding.
type NumberType int

const (
	// IntNumber encodes the value as asInt (a decimal string, per proto3 JSON).
	IntNumber NumberType = iota
	// DoubleNumber encodes the value as asDouble.
	DoubleNumber
)

// metricDef is one exported metric. Exactly one of global or model is set:
// global reads a whole-snapshot value, model reads a per-model value and is
// emitted with a `model` datapoint attribute. Names and units follow the
// OpenTelemetry semantic conventions for process metrics and an `emb.` prefix
// for server-specific metrics.
type metricDef struct {
	name   string
	kind   Kind
	unit   string
	desc   string
	num    NumberType
	global func(Snapshot) float64
	model  func(ModelSnapshot) float64
	// needsPool marks a metric whose per-model source is only measured for
	// models with a loaded embedding pool; models without one are omitted
	// rather than exported as zero.
	needsPool bool
}

// histogramDef is one exported explicit-bucket histogram. bounds is the
// strictly ascending ladder the datapoints index, in the unit they report.
type histogramDef struct {
	name   string
	unit   string
	desc   string
	bounds []float64
}

// histogramDefs is the exported histogram set. It is separate from metricDefs
// so the bucket bounds live with the definition and the number-encoding path
// stays untouched.
var histogramDefs = []histogramDef{
	{
		name: "emb.request.duration", unit: "s",
		desc:   "Completed inference request latency, by model",
		bounds: latencyBoundsSeconds[:],
	},
}

// metricDefs is the exported metric set. Every metric named in the
// telemetry-export spec's conventions requirement appears exactly once.
var metricDefs = []metricDef{
	{
		name: "process.memory.usage", kind: Gauge, unit: "By", num: IntNumber,
		desc:   "Resident memory of the emb process, including native allocations",
		global: func(s Snapshot) float64 { return float64(s.RSSBytes) },
	},
	{
		name: "process.cpu.time", kind: Sum, unit: "s", num: DoubleNumber,
		desc:   "Cumulative CPU time used by the emb process, user and system",
		global: func(s Snapshot) float64 { return float64(s.CPUTimeUserUsec+s.CPUTimeSysUsec) / 1e6 },
	},
	{
		name: "process.uptime", kind: Gauge, unit: "s", num: DoubleNumber,
		desc:   "Seconds since the emb process started",
		global: func(s Snapshot) float64 { return float64(s.Uptime) },
	},
	{
		name: "system.memory.limit", kind: Gauge, unit: "By", num: IntNumber,
		desc:   "Total physical memory available to the host",
		global: func(s Snapshot) float64 { return float64(s.TotalSystemMemory) },
	},
	{
		name: "emb.active_requests", kind: Gauge, unit: "{request}", num: IntNumber,
		desc:   "Inference requests currently being processed",
		global: func(s Snapshot) float64 { return float64(s.ActiveRequests) },
	},
	{
		name: "emb.connections", kind: Gauge, unit: "{connection}", num: IntNumber,
		desc:   "Currently accepted, open client connections",
		global: func(s Snapshot) float64 { return float64(s.Connections) },
	},
	{
		name: "emb.goroutines", kind: Gauge, unit: "{goroutine}", num: IntNumber,
		desc:   "Live goroutines in the emb process",
		global: func(s Snapshot) float64 { return float64(s.Goroutines) },
	},
	{
		name: "emb.models.loaded", kind: Gauge, unit: "{model}", num: IntNumber,
		desc:   "Models currently loaded",
		global: func(s Snapshot) float64 { return float64(s.ModelsLoaded) },
	},
	{
		name: "emb.cache.hits", kind: Sum, unit: "{hit}", num: IntNumber,
		desc:   "Embedding reply-cache hits",
		global: func(s Snapshot) float64 { return float64(s.CacheHits) },
	},
	{
		name: "emb.cache.misses", kind: Sum, unit: "{miss}", num: IntNumber,
		desc:   "Embedding reply-cache misses",
		global: func(s Snapshot) float64 { return float64(s.CacheMisses) },
	},
	{
		name: "emb.cache.evictions", kind: Sum, unit: "{eviction}", num: IntNumber,
		desc:   "Embedding reply-cache evictions",
		global: func(s Snapshot) float64 { return float64(s.CacheEvictions) },
	},
	{
		name: "emb.truncated.texts", kind: Sum, unit: "{text}", num: IntNumber,
		desc:   "Texts dropped by the per-command max_texts cap",
		global: func(s Snapshot) float64 { return float64(s.TruncatedTexts) },
	},
	{
		name: "emb.truncated.pairs", kind: Sum, unit: "{pair}", num: IntNumber,
		desc:   "Pairs dropped by the per-command max_pairs cap",
		global: func(s Snapshot) float64 { return float64(s.TruncatedPairs) },
	},
	{
		name: "emb.truncated.images", kind: Sum, unit: "{image}", num: IntNumber,
		desc:   "Images dropped by the per-command max_images cap",
		global: func(s Snapshot) float64 { return float64(s.TruncatedImages) },
	},
	{
		name: "emb.net.input.bytes", kind: Sum, unit: "By", num: IntNumber,
		desc:   "RESP bytes received from all connections since start",
		global: func(s Snapshot) float64 { return float64(s.NetInBytes) },
	},
	{
		name: "emb.net.output.bytes", kind: Sum, unit: "By", num: IntNumber,
		desc:   "RESP bytes sent to all connections since start",
		global: func(s Snapshot) float64 { return float64(s.NetOutBytes) },
	},
	{
		name: "emb.requests", kind: Sum, unit: "{request}", num: IntNumber,
		desc:  "Inference requests served, by model",
		model: func(m ModelSnapshot) float64 { return float64(m.Requests) },
	},
	{
		name: "emb.tokens", kind: Sum, unit: "{token}", num: IntNumber,
		desc:  "Tokens embedded, by model",
		model: func(m ModelSnapshot) float64 { return float64(m.Tokens) },
	},
	{
		name: "emb.errors", kind: Sum, unit: "{error}", num: IntNumber,
		desc:  "Failed inference requests, by model",
		model: func(m ModelSnapshot) float64 { return float64(m.Errors) },
	},
	{
		name: "emb.dispatch.wait", kind: Sum, unit: "s", num: DoubleNumber,
		desc:      "Seconds requests spent waiting for an inference slot, by model",
		model:     func(m ModelSnapshot) float64 { return float64(m.DispatchWaitUs) / 1e6 },
		needsPool: true,
	},
	{
		name: "emb.inference.duration", kind: Sum, unit: "s", num: DoubleNumber,
		desc:      "Seconds of embedding inference work, by model",
		model:     func(m ModelSnapshot) float64 { return float64(m.RunUs) / 1e6 },
		needsPool: true,
	},
	{
		name: "emb.inference.runs", kind: Sum, unit: "{run}", num: IntNumber,
		desc:      "Embedding inference runs, by model",
		model:     func(m ModelSnapshot) float64 { return float64(m.Runs) },
		needsPool: true,
	},
	{
		name: "emb.sessions.busy", kind: Gauge, unit: "{session}", num: IntNumber,
		desc:      "Embedding sessions currently running work, by model",
		model:     func(m ModelSnapshot) float64 { return float64(m.SessionsBusy) },
		needsPool: true,
	},
	{
		name: "emb.sessions.total", kind: Gauge, unit: "{session}", num: IntNumber,
		desc:      "Embedding sessions available to a model",
		model:     func(m ModelSnapshot) float64 { return float64(m.SessionsTotal) },
		needsPool: true,
	},
	{
		name: "emb.padding.efficiency", kind: Gauge, unit: "1", num: DoubleNumber,
		desc:      "Embedding tokens consumed divided by token slots processed, by model",
		model:     func(m ModelSnapshot) float64 { return m.PaddingEfficiency },
		needsPool: true,
	},
	{
		name: "emb.script.dispatch.wait", kind: Sum, unit: "s", num: DoubleNumber,
		desc:  "Seconds scripted evaluations spent waiting for an inference slot, by model",
		model: func(m ModelSnapshot) float64 { return float64(m.ScriptDispatchWaitUs) / 1e6 },
	},
	{
		name: "emb.script.inference.duration", kind: Sum, unit: "s", num: DoubleNumber,
		desc:  "Seconds of scripted-evaluation inference work, by model",
		model: func(m ModelSnapshot) float64 { return float64(m.ScriptRunUs) / 1e6 },
	},
	{
		name: "emb.script.inference.runs", kind: Sum, unit: "{run}", num: IntNumber,
		desc:  "Scripted-evaluation inference runs, by model",
		model: func(m ModelSnapshot) float64 { return float64(m.ScriptRuns) },
	},
	{
		name: "emb.admission.wait", kind: Sum, unit: "s", num: DoubleNumber,
		desc:      "Seconds embedding requests spent waiting for CPU admission, by model",
		model:     func(m ModelSnapshot) float64 { return float64(m.AdmissionWaitUs) / 1e6 },
		needsPool: true,
	},
	{
		name: "emb.cpu.admission.wait", kind: Sum, unit: "s", num: DoubleNumber,
		desc:   "Seconds all inference paths spent queueing at the process CPU admission gate",
		global: func(s Snapshot) float64 { return float64(s.CPUAdmissionWaitUs) / 1e6 },
	},
	{
		name: "emb.cpu.admission.waiters", kind: Gauge, unit: "{waiter}", num: IntNumber,
		desc:   "Callers queueing for process CPU admission now",
		global: func(s Snapshot) float64 { return float64(s.CPUAdmissionWaiters) },
	},
	{
		name: "emb.cpu.admission.timeouts", kind: Sum, unit: "{wait}", num: IntNumber,
		desc:   "Admission waits that ended without a grant (refused or abandoned)",
		global: func(s Snapshot) float64 { return float64(s.CPUAdmissionTimeouts) },
	},
}
