package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/telemetry"
)

// recordingSender is an in-process telemetry.Sender that keeps every payload.
type recordingSender struct {
	mu     sync.Mutex
	bodies [][]byte
	ch     chan struct{}
}

func newRecordingSender() *recordingSender {
	return &recordingSender{ch: make(chan struct{}, 64)}
}

func (r *recordingSender) Send(_ context.Context, payload []byte, _ map[string]string) error {
	r.mu.Lock()
	r.bodies = append(r.bodies, append([]byte(nil), payload...))
	r.mu.Unlock()
	select {
	case r.ch <- struct{}{}:
	default:
	}
	return nil
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bodies)
}

func (r *recordingSender) waitFor(t *testing.T, n int, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for r.count() < n {
		select {
		case <-r.ch:
		case <-deadline:
			t.Fatalf("timed out waiting for %d exports, got %d", n, r.count())
		}
	}
}

// sendCmd writes one RESP command and returns the raw reply.
func sendCmd(t *testing.T, c net.Conn, argv ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(argv)))
	b.WriteString("\r\n")
	for _, a := range argv {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(a)))
		b.WriteString("\r\n")
		b.WriteString(a)
		b.WriteString("\r\n")
	}
	if _, err := c.Write([]byte(b.String())); err != nil {
		t.Fatal(err)
	}
	return readRESP(t, c)
}

// ---- task 1.3: the telemetry snapshot reads the same source as INFO ----

func TestTelemetrySnapshotMatchesInfo(t *testing.T) {
	addr, srv := serveTestWithOptions(t)
	c := dial(t, addr)
	reply := sendCmd(t, c, "EMB", "test", "hello world")
	if !strings.HasPrefix(reply, "$") {
		t.Fatalf("EMB reply = %q", reply)
	}
	c.Close()

	snap := srv.telemetrySnapshot()
	info := srv.infoSnapshot()
	if snap.TotalRequests != info.totalReq {
		t.Errorf("total_requests: telemetry=%d INFO=%d", snap.TotalRequests, info.totalReq)
	}
	if snap.TotalTokens != info.totalTok {
		t.Errorf("total_tokens: telemetry=%d INFO=%d", snap.TotalTokens, info.totalTok)
	}
	if snap.ModelsLoaded != int64(info.models) {
		t.Errorf("models_loaded: telemetry=%d INFO=%d", snap.ModelsLoaded, info.models)
	}
	if snap.TotalRequests < 1 {
		t.Errorf("expected the driven request to be counted, got %d", snap.TotalRequests)
	}
	if len(snap.Models) != 1 || snap.Models[0].Name != "test" {
		t.Fatalf("per-model breakdown = %+v", snap.Models)
	}
	if snap.Models[0].Requests != snap.TotalRequests {
		t.Errorf("per-model requests %d != total %d", snap.Models[0].Requests, snap.TotalRequests)
	}

	// The per-model timing and session values the exporter carries must equal
	// what EMB.INFO reports for the same model, read back to back so no traffic
	// can advance the counters between them.
	fields := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	ms := snap.Models[0]
	if !ms.HasPool {
		t.Error("a model with a loaded pool must be marked as having one")
	}
	for _, tc := range []struct {
		name string
		got  int64
		want int64
	}{
		{"embed_dispatch_wait_us", ms.DispatchWaitUs, int64(intOf(t, fields["embed_dispatch_wait_us"]))},
		{"embed_run_us", ms.RunUs, int64(intOf(t, fields["embed_run_us"]))},
		{"embed_runs", ms.Runs, int64(intOf(t, fields["embed_runs"]))},
		{"embed_sessions_busy", ms.SessionsBusy, int64(intOf(t, fields["embed_sessions_busy"]))},
		{"embed_sessions_total", ms.SessionsTotal, int64(intOf(t, fields["embed_sessions_total"]))},
		{"script_dispatch_wait_us", ms.ScriptDispatchWaitUs, int64(intOf(t, fields["script_dispatch_wait_us"]))},
		{"script_run_us", ms.ScriptRunUs, int64(intOf(t, fields["script_run_us"]))},
		{"script_runs", ms.ScriptRuns, int64(intOf(t, fields["script_runs"]))},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: telemetry=%d EMB.INFO=%d", tc.name, tc.got, tc.want)
		}
	}
	if got, want := fmt.Sprintf("%.4f", ms.PaddingEfficiency), bulkOf(t, fields["padding_efficiency"]); got != want {
		t.Errorf("padding_efficiency: telemetry=%s EMB.INFO=%s", got, want)
	}
	if ms.Runs < 1 {
		t.Errorf("embedding runs = %d, want the driven request to have run", ms.Runs)
	}
	if ms.Latency.Count != 1 {
		t.Errorf("histogram observations = %d, want 1 for the driven EMB", ms.Latency.Count)
	}
}

// ---- task 3.1: one recording helper feeds the ring and the histogram ----

// serveTelemetryRecordTest starts a server whose model "test" can serve EMB
// (embedding pool), EMB.IMG (image session) and EMB.EVAL (a constant script
// needs no script resources), so the request-record path can be driven for
// every inference command without a downloaded model.
func serveTelemetryRecordTest(t *testing.T, opts ...Option) (string, *Server) {
	t.Helper()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{
		Name: "test",
		Dim:  4,
		Pool: pool,
		ImageRes: &registry.ImageResources{
			Sessions:     []onnx.NamedSession{&fakeImageSession{dim: testImageDim}},
			Plan:         testImagePlan(4),
			OutputTensor: "image_embeds",
			Pooling:      "none",
			Dim:          testImageDim,
		},
	})

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil, opts...)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

// settle waits until every command already answered on c has been fully
// handled: redcon serves one connection's commands in order, so the reply to
// this PING cannot arrive before the earlier handlers' deferred records ran.
func settle(t *testing.T, c net.Conn) {
	t.Helper()
	if got := doCmd(t, c, "PING"); got != "+PONG\r\n" {
		t.Fatalf("PING reply = %q", got)
	}
}

func TestRecordRequestObservesEveryInferenceCommand(t *testing.T) {
	addr, srv := serveTelemetryRecordTest(t)
	c := dial(t, addr)
	defer c.Close()

	if got := doCmd(t, c, "EMB", "test", "hello"); !strings.HasPrefix(got, "$") {
		t.Fatalf("EMB reply = %q", got)
	}
	if got := doCmd(t, c, "EMB.IMG", "test", solidImagePNG(t, color.White)); !strings.HasPrefix(got, "$") {
		t.Fatalf("EMB.IMG reply = %q", got)
	}
	if got := doCmd(t, c, "EMB.EVAL", "test", "return 1", "1", "x"); got != ":1\r\n" {
		t.Fatalf("EMB.EVAL reply = %q", got)
	}
	settle(t, c)

	lat, ok := srv.latency.Read("test")
	if !ok {
		t.Fatal("no latency series after three recorded requests")
	}
	if lat.Count != 3 {
		t.Errorf("observations = %d, want one per EMB, EMB.IMG and EMB.EVAL", lat.Count)
	}
	var sum int64
	for _, b := range lat.Buckets {
		sum += b
	}
	if sum != lat.Count {
		t.Errorf("bucket counts sum to %d, want the observation count %d", sum, lat.Count)
	}
	if lat.SumUs <= 0 {
		t.Errorf("observed total = %dus, want > 0", lat.SumUs)
	}
	if n := srv.monitor.LastSeq(); n != 3 {
		t.Errorf("MONITOR recorded %d events, want 3", n)
	}
}

func TestRecordRequestIncludesFailuresAndIgnoresUnconfiguredModels(t *testing.T) {
	addr, srv := serveTelemetryRecordTest(t)
	c := dial(t, addr)
	defer c.Close()

	// A configured model whose request fails (URL arguments are rejected before
	// any inference) and a name the registry has no entry for.
	if got := doCmd(t, c, "EMB.IMG", "test", "https://example.com/cat.jpg"); !strings.HasPrefix(got, "-") {
		t.Fatalf("URL rejection = %q, want an error", got)
	}
	if got := doCmd(t, c, "EMB", "ghost", "hello"); !strings.HasPrefix(got, "-") {
		t.Fatalf("unknown model = %q, want an error", got)
	}
	settle(t, c)

	lat, ok := srv.latency.Read("test")
	if !ok {
		t.Fatal("a failed request for a configured model must still be observed")
	}
	if lat.Count != 1 {
		t.Errorf("observations for the failed request = %d, want 1", lat.Count)
	}
	if _, ok := srv.latency.Read("ghost"); ok {
		t.Error("an unconfigured model name must not create a latency series")
	}

	events := srv.monitor.Since(0, 10)
	if len(events) != 2 {
		t.Fatalf("MONITOR events = %d, want one per request: %+v", len(events), events)
	}
	if events[0].Model != "test" || !events[0].Err {
		t.Errorf("failed request event = %+v, want model test with Err set", events[0])
	}
	if events[1].Model != "ghost" || !events[1].Err {
		t.Errorf("unconfigured model event = %+v, want model ghost with Err set", events[1])
	}
}

// ---- task 4.2: server lifecycle wiring ----

func TestServerWithoutTelemetryStartsNoExporter(t *testing.T) {
	_, srv := serveTestWithOptions(t)
	if srv.telemetryExport != nil {
		t.Fatal("a server built without telemetry must not create an exporter")
	}
}

func TestServerTelemetryDisabledEndpointStartsNothing(t *testing.T) {
	sender := newRecordingSender()
	_, srv := serveTestWithOptions(t, WithTelemetry(telemetry.Config{Disabled: true, Endpoint: "http://x"}, telemetry.WithSender(sender)))
	if srv.telemetryExport != nil {
		t.Fatal("a disabled telemetry config must not create an exporter")
	}
}

func TestServerTelemetryFlushesOnShutdown(t *testing.T) {
	sender := newRecordingSender()
	_, srv := serveTestWithOptions(t, WithTelemetry(
		telemetry.Config{Endpoint: "http://x", Interval: time.Hour},
		telemetry.WithSender(sender),
	))
	if srv.telemetryExport == nil {
		t.Fatal("an enabled telemetry config must create an exporter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx, config.ShutdownFlags{}); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("shutdown: %v", err)
	}
	if sender.count() < 1 {
		t.Fatal("shutdown must flush a final export")
	}
}

// ---- task 5.1: CONFIG GET exposes telemetry read-only ----

func TestConfigGetTelemetryReadOnly(t *testing.T) {
	sender := newRecordingSender()
	addr, _ := serveTestWithOptions(t, WithTelemetry(
		telemetry.Config{Endpoint: "http://otel.example:4318", Interval: 90 * time.Second},
		telemetry.WithSender(sender),
	))
	c := dial(t, addr)

	reply := sendCmd(t, c, "CONFIG", "GET", "otel*")
	for _, want := range []string{"otel_enabled", "otel_endpoint", "otel_interval", "http://otel.example:4318", "1m30s", "true"} {
		if !strings.Contains(reply, want) {
			t.Errorf("CONFIG GET otel* reply missing %q:\n%s", want, reply)
		}
	}

	setReply := sendCmd(t, c, "CONFIG", "SET", "otel_endpoint", "http://elsewhere:4318")
	if !strings.HasPrefix(setReply, "-ERR") {
		t.Errorf("CONFIG SET of a telemetry parameter must fail, got %q", setReply)
	}
	c.Close()
}

// ---- task 6.1: end-to-end export ----

func TestTelemetryEndToEnd(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies [][]byte
		hit    = make(chan struct{}, 64)
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		select {
		case hit <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	addr, srv := serveTestWithOptions(t, WithTelemetry(telemetry.Config{
		Endpoint: ts.URL,
		Interval: 40 * time.Millisecond,
	}))
	c := dial(t, addr)
	for range 3 {
		if reply := sendCmd(t, c, "EMB", "test", "hello"); !strings.HasPrefix(reply, "$") {
			t.Fatalf("EMB reply = %q", reply)
		}
	}
	c.Close()

	// Wait for at least two exports so the second can carry deltas.
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		n := len(bodies)
		mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-hit:
		case <-deadline:
			t.Fatal("timed out waiting for telemetry exports")
		}
	}

	mu.Lock()
	last := bodies[len(bodies)-1]
	mu.Unlock()

	req := decodeOTLP(t, last)
	attrs := resourceAttrs(t, req)
	if attrs["service.name"] != "emb" {
		t.Errorf("resource service.name = %q", attrs["service.name"])
	}

	gauge := metricByName(t, req, "emb.active_requests")
	if gauge["gauge"] == nil {
		t.Fatalf("emb.active_requests is not a gauge: %+v", gauge)
	}

	reqs := metricByName(t, req, "emb.requests")
	sum, ok := reqs["sum"].(map[string]any)
	if !ok {
		t.Fatalf("emb.requests is not a sum: %+v", reqs)
	}
	if sum["aggregationTemporality"] != float64(1) || sum["isMonotonic"] != true {
		t.Errorf("emb.requests sum metadata = %+v", sum)
	}
	points, _ := sum["dataPoints"].([]any)
	if len(points) != 1 {
		t.Fatalf("emb.requests dataPoints = %+v", sum["dataPoints"])
	}
	point := points[0].(map[string]any)
	gotModel := attributeValue(point["attributes"])
	if gotModel != "test" {
		t.Errorf("emb.requests model attribute = %q", gotModel)
	}
	if n := point["asInt"]; n == "0" || n == nil {
		t.Errorf("emb.requests delta = %v, want > 0", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx, config.ShutdownFlags{})
}

// ---- minimal OTLP JSON navigation ----

func decodeOTLP(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode OTLP payload: %v", err)
	}
	return m
}

func resourceAttrs(t *testing.T, req map[string]any) map[string]string {
	t.Helper()
	rms, _ := req["resourceMetrics"].([]any)
	if len(rms) == 0 {
		t.Fatal("no resourceMetrics")
	}
	rm := rms[0].(map[string]any)
	res := rm["resource"].(map[string]any)
	return attributesMap(res["attributes"])
}

func metricByName(t *testing.T, req map[string]any, name string) map[string]any {
	t.Helper()
	rms := req["resourceMetrics"].([]any)
	rm := rms[0].(map[string]any)
	sms := rm["scopeMetrics"].([]any)
	for _, sm := range sms {
		metrics, _ := sm.(map[string]any)["metrics"].([]any)
		for _, m := range metrics {
			mm := m.(map[string]any)
			if mm["name"] == name {
				return mm
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return nil
}

func attributesMap(v any) map[string]string {
	out := map[string]string{}
	attrs, _ := v.([]any)
	for _, a := range attrs {
		am := a.(map[string]any)
		val := am["value"].(map[string]any)
		if s, ok := val["stringValue"].(string); ok {
			out[am["key"].(string)] = s
		}
	}
	return out
}

func attributeValue(v any) string {
	attrs, _ := v.([]any)
	for _, a := range attrs {
		am := a.(map[string]any)
		if am["key"] == "model" {
			return am["value"].(map[string]any)["stringValue"].(string)
		}
	}
	return ""
}
