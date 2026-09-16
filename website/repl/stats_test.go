package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- hub -------------------------------------------------------------------

func TestStatsHubLatestSupersedes(t *testing.T) {
	h := newStatsHub(4)
	ch, _, cancel, ok := h.subscribe()
	if !ok {
		t.Fatal("subscribe refused below the bound")
	}
	defer cancel()

	h.publish(statsEvent{Kind: "frame", HTML: "one"})
	h.publish(statsEvent{Kind: "frame", HTML: "two"})
	select {
	case payload := <-ch:
		var ev statsEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if ev.HTML != "two" {
			t.Fatalf("subscriber received a stale frame: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber received no frame")
	}

	_, latest, cancel2, ok := h.subscribe()
	if !ok {
		t.Fatal("second subscribe refused below the bound")
	}
	defer cancel2()
	var ev statsEvent
	if err := json.Unmarshal(latest, &ev); err != nil {
		t.Fatalf("late viewer got no decodable event: %v", err)
	}
	if ev.HTML != "two" {
		t.Fatalf("late viewer did not get the newest frame: %+v", ev)
	}
}

func TestStatsHubBoundsSubscribers(t *testing.T) {
	h := newStatsHub(1)
	_, _, cancel, ok := h.subscribe()
	if !ok {
		t.Fatal("first subscribe refused")
	}
	defer cancel()
	if _, _, _, ok := h.subscribe(); ok {
		t.Fatal("a second subscriber was admitted past the bound")
	}
}

// --- producer --------------------------------------------------------------

func TestReadFramesPublishesRenderedFrames(t *testing.T) {
	s := newStatsService()
	r := strings.NewReader(
		"{\"ansi\":\"\\u001b[38;2;255;90;31mhi\\u001b[0m\"}\n" +
			"this line is not JSON\n")
	if err := s.readFrames(r); err != io.EOF {
		t.Fatalf("readFrames = %v, want io.EOF at end of stream", err)
	}

	_, latest, cancel, ok := s.hub.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}
	defer cancel()
	var ev statsEvent
	if err := json.Unmarshal(latest, &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Kind != "frame" || !strings.Contains(ev.HTML, "#ff5a1f") || !strings.Contains(ev.HTML, "hi") {
		t.Fatalf("frame was not rendered into markup: %+v", ev)
	}
}

func TestStatsServiceRestartsProducer(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	t.Setenv("STATS_TEST_RUNS", runs)
	producer := writeProducer(t, `echo run >> "$STATS_TEST_RUNS"; printf '{"ansi":"hi"}\n'`)

	s := newStatsService()
	s.retryDelay = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.start(ctx, producer, "127.0.0.1:1", time.Millisecond, 1)

	deadline := time.Now().Add(5 * time.Second)
	for {
		body, _ := os.ReadFile(runs)
		if strings.Count(string(body), "\n") >= 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("producer ran %d time(s), want at least 2 (no restart)",
				strings.Count(string(body), "\n"))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func writeProducer(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "producer")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write producer: %v", err)
	}
	return p
}

// --- endpoints -------------------------------------------------------------

func TestStatsStreamSendsCurrentThenLiveFrames(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	b.stats.hub.publish(statsEvent{Kind: "frame", HTML: "<span>one</span>"})

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	r := bufio.NewReader(resp.Body)
	if ev := readSSE(t, r); ev.Kind != "frame" || ev.HTML != "<span>one</span>" {
		t.Fatalf("joining viewer did not get the current frame: %+v", ev)
	}
	b.stats.hub.publish(statsEvent{Kind: "frame", HTML: "<span>two</span>"})
	if ev := readSSE(t, r); ev.Kind != "frame" || ev.HTML != "<span>two</span>" {
		t.Fatalf("live frame not delivered: %+v", ev)
	}
}

func TestStatsStreamIsReadOnly(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/stats", "application/json", strings.NewReader(`{"args":["EMB"]}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/stats = %d, want 405", resp.StatusCode)
	}
}

func TestStatsStreamReportsStartingWithoutProducer(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	ev := readSSE(t, bufio.NewReader(resp.Body))
	if ev.Kind != "status" || !strings.Contains(ev.Text, "starting") {
		t.Fatalf("empty producer was not reported as starting: %+v", ev)
	}
}

func TestStatsStreamRefusesPastSubscriberBound(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	b.stats = &statsService{hub: newStatsHub(1)}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	_, _, cancel, ok := b.stats.hub.subscribe()
	if !ok {
		t.Fatal("could not occupy the one slot")
	}
	defer cancel()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	ev := readSSE(t, bufio.NewReader(resp.Body))
	if ev.Kind != "status" || !strings.Contains(ev.Text, "capacity") {
		t.Fatalf("over-bound viewer was not refused legibly: %+v", ev)
	}
}

func TestStatsStreamSendsNoUpstreamCommand(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	_ = readSSE(t, bufio.NewReader(resp.Body))
	if got := f.received(); len(got) != 0 {
		t.Fatalf("the live view issued commands upstream: %v", got)
	}
}

func TestStatsPageIsReadOnlyAndLinked(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stats")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /stats = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	lower := strings.ToLower(string(body))
	for _, banned := range []string{"<form", "<input", "<button", "<select", "<textarea"} {
		if strings.Contains(lower, banned) {
			t.Errorf("the live view carries a control: %s", banned)
		}
	}
	if !strings.Contains(string(body), `id="frame"`) {
		t.Errorf("the live view has no frame target")
	}

	// The standalone terminal is the door to it.
	tresp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	tbody, _ := io.ReadAll(tresp.Body)
	tresp.Body.Close()
	if !strings.Contains(string(tbody), `href="/stats"`) {
		t.Errorf("the standalone terminal does not link to /stats")
	}

	// /stats is exactly one path, like the terminal.
	nresp, err := http.Get(srv.URL + "/stats/extra")
	if err != nil {
		t.Fatalf("get /stats/extra: %v", err)
	}
	nresp.Body.Close()
	if nresp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /stats/extra = %d, want 404", nresp.StatusCode)
	}
}

// readSSE reads one server-sent event from the stream.
func readSSE(t *testing.T, r *bufio.Reader) statsEvent {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev statsEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("event is not JSON: %v (%q)", err, line)
		}
		return ev
	}
}
