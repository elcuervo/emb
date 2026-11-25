package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

func TestHTTPReadOnlyAndBoundedViewers(t *testing.T) {
	spec, _ := embtop.ParseNodeSpec("127.0.0.1:6379")
	m := newMonitor([]embtop.NodeSpec{spec}, "password-DO-NOT-LEAK", false, "")
	server := httptest.NewServer(m.handler())
	defer server.Close()
	client := server.Client()
	for _, path := range []string{"/", "/api/snapshot", "/assets/app.js", "/health"} {
		resp, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, resp.StatusCode, body)
		}
		if strings.Contains(string(body), "password-DO-NOT-LEAK") || resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("unsafe GET %s", path)
		}
	}
	for _, path := range []string{"/", "/api/snapshot", "/api/stream", "/assets/app.js", "/health"} {
		req, _ := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader("target=http://169.254.169.254"))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s accepted: %d", path, resp.StatusCode)
		}
	}
	resp, err := client.Get(server.URL + "/api/snapshot?target=127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("browser target accepted: %d", resp.StatusCode)
	}
	m.mu.Lock()
	for range maxViewers {
		m.viewers[make(chan []byte, 1)] = struct{}{}
	}
	m.mu.Unlock()
	resp, err = client.Get(server.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unbounded SSE viewers: %d", resp.StatusCode)
	}
}

func readEvent(t *testing.T, r *bufio.Reader) snapshot {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("unexpected stream frame %q", line)
	}
	var s snapshot
	if err := json.Unmarshal(bytes.TrimSpace([]byte(strings.TrimPrefix(line, "data: "))), &s); err != nil {
		t.Fatal(err)
	}
	_, err = r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSSELateJoinFanoutAndSuperseding(t *testing.T) {
	m := newMonitor(nil, "", false, "")
	at := time.Unix(123, 0)
	m.now = func() time.Time { return at }
	m.record()
	server := httptest.NewServer(m.handler())
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	resp1, err := client.Get(server.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	resp2, err := client.Get(server.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	r1, r2 := bufio.NewReader(resp1.Body), bufio.NewReader(resp2.Body)
	if a, b := readEvent(t, r1), readEvent(t, r2); len(a.Fleet.History) != 1 || len(b.Fleet.History) != 1 {
		t.Fatal("late viewers did not receive retained full history")
	}
	m.mu.Lock()
	slow := make(chan []byte, 1)
	m.viewers[slow] = struct{}{}
	m.mu.Unlock()
	for range 3 {
		at = at.Add(time.Second)
		m.record()
	}
	if a, b := readEvent(t, r1), readEvent(t, r2); len(a.Fleet.History) < 2 || len(b.Fleet.History) < 2 {
		t.Fatal("viewers missed complete snapshots")
	}
	select {
	case data := <-slow:
		var s snapshot
		if err := json.Unmarshal(data, &s); err != nil || len(s.Fleet.History) != 4 {
			t.Fatalf("slow viewer did not receive latest complete snapshot: %v %+v", err, s.Fleet)
		}
	default:
		t.Fatal("slow viewer lost latest snapshot")
	}
	m.mu.Lock()
	delete(m.viewers, slow)
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/snapshot", nil)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var current snapshot
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil || len(current.Fleet.History) != 4 {
		t.Fatalf("snapshot/SSE shape diverged: %v", err)
	}
	_ = resp1.Body.Close()
	reconnected, err := client.Get(server.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Body.Close()
	if latest := readEvent(t, bufio.NewReader(reconnected.Body)); len(latest.Fleet.History) != 4 {
		t.Fatal("reconnected viewer did not receive current complete window")
	}
}

func TestSnapshotFrameBoundReportsOverflow(t *testing.T) {
	m := newMonitor(nil, "", false, "")
	m.publishSnapshot(snapshot{At: time.Now(), Nodes: []nodeSnapshot{{Name: strings.Repeat("x", maxFrameBytes)}}})
	if len(m.latest) > maxFrameBytes {
		t.Fatalf("unbounded frame: %d", len(m.latest))
	}
	var s snapshot
	if err := json.Unmarshal(m.latest, &s); err != nil || s.Error == "" {
		t.Fatalf("overflow silently returned stale snapshot: %v", err)
	}
}
