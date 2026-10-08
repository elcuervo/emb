package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
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
	_ = srv.Shutdown(ctx)
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
