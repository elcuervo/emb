package telemetry

import (
	"context"
	"encoding/json"
	"errors"
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

func findMetric(t *testing.T, req exportRequest, name string) metric {
	t.Helper()
	for _, sm := range req.ResourceMetrics[0].ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metric{}
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
