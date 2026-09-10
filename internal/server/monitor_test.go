package server

import (
	"regexp"
	"strings"
	"testing"
)

func TestMonitorBasic(t *testing.T) {
	m := NewMonitor(4)
	if m.LastSeq() != 0 {
		t.Fatalf("empty monitor LastSeq = %d, want 0", m.LastSeq())
	}
	m.Add(MonitorEvent{Model: "a", Texts: 1, LatencyUs: 10})
	m.Add(MonitorEvent{Model: "b", Texts: 2, LatencyUs: 20, Err: true})
	if m.LastSeq() != 2 {
		t.Fatalf("LastSeq = %d, want 2", m.LastSeq())
	}
	evs := m.Since(0, 100)
	if len(evs) != 2 || evs[0].Seq != 1 || evs[1].Seq != 2 || evs[1].Err != true {
		t.Fatalf("Since(0) = %+v", evs)
	}
	if after := m.Since(1, 100); len(after) != 1 || after[0].Seq != 2 {
		t.Fatalf("Since(1) = %+v", after)
	}
	if after := m.Since(2, 100); len(after) != 0 {
		t.Fatalf("Since(2) = %+v, want empty", after)
	}
}

func TestMonitorRingWrapEvictsOldest(t *testing.T) {
	m := NewMonitor(3)
	for i := 0; i < 6; i++ {
		m.Add(MonitorEvent{Model: "m", LatencyUs: int64(i)})
	}
	if m.LastSeq() != 6 {
		t.Fatalf("LastSeq = %d, want 6", m.LastSeq())
	}
	evs := m.Since(0, 100)
	// Only the last 3 survive, in seq order (4,5,6), oldest evicted.
	if len(evs) != 3 || evs[0].Seq != 4 || evs[1].Seq != 5 || evs[2].Seq != 6 {
		t.Fatalf("ring wrap eviction wrong: %+v", evs)
	}
}

func TestMonitorLimitClamping(t *testing.T) {
	m := NewMonitor(16)
	for i := 0; i < 10; i++ {
		m.Add(MonitorEvent{Model: "m", LatencyUs: int64(i)})
	}
	evs := m.Since(0, 3)
	if len(evs) != 3 || evs[0].Seq != 1 || evs[2].Seq != 3 {
		t.Fatalf("limit not honored: %+v", evs)
	}
}

func TestMonitorHandlerFlow(t *testing.T) {
	addr := serveTest(t)

	// Idle: empty monitor.
	c := dial(t, addr)
	c.Write([]byte("*1\r\n$7\r\nMONITOR\r\n"))
	if resp := readRESP(t, c); resp != "*0\r\n" {
		t.Fatalf("idle monitor = %q, want empty array", resp)
	}

	// One successful EMB with two texts.
	c.Write([]byte("*4\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$1\r\na\r\n$1\r\nb\r\n"))
	if resp := readRESP(t, c); !strings.HasPrefix(resp, "*2\r\n") {
		t.Fatalf("EMB reply = %q, want 2-element array", resp)
	}

	// Monitor: exactly one event, model test, texts 2, err 0, latency > 0.
	c.Write([]byte("*1\r\n$7\r\nMONITOR\r\n"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "*1\r\n") {
		t.Fatalf("monitor = %q, want 1 event", resp)
	}
	if !strings.Contains(resp, "$4\r\ntest\r\n") || !strings.Contains(resp, ":2\r\n") {
		t.Fatalf("event fields wrong: %q", resp)
	}
	if strings.Contains(resp, ":1\r\n") && len(resp) < 40 {
		t.Fatalf("unexpected error flag: %q", resp)
	}

	// Error request: unknown model records an error event.
	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$11\r\nnonexistent\r\n$1\r\nx\r\n"))
	if resp := readRESP(t, c); !strings.HasPrefix(resp, "-ERR") {
		t.Fatalf("unknown model reply = %q", resp)
	}
	c.Write([]byte("*1\r\n$7\r\nMONITOR\r\n"))
	resp = readRESP(t, c)
	// Both buffered events come back; the nonexistent one carries err=1.
	if !strings.HasPrefix(resp, "*2\r\n") || !strings.Contains(resp, "$11\r\nnonexistent\r\n") {
		t.Fatalf("monitor after error = %q", resp)
	}
	// seq, at_us, model, texts=1, latency, err=1 (latency varies per run).
	ok := regexp.MustCompile(`\$11\r\nnonexistent\r\n:1\r\n:\d+\r\n:1\r\n`).MatchString(resp)
	if !ok {
		t.Fatalf("nonexistent error event wrong: %q", resp)
	}
}

func TestMonitorIncrementalFetch(t *testing.T) {
	addr := serveTest(t)
	writeEmb := func(texts ...string) {
		c := dial(t, addr)
		args := "*" + itoa(2+len(texts)) + "\r\n$3\r\nEMB\r\n$4\r\ntest\r\n"
		for _, t := range texts {
			args += "$" + itoa(len(t)) + "\r\n" + t + "\r\n"
		}
		c.Write([]byte(args))
		readRESP(t, c)
	}
	writeEmb("a")
	writeEmb("b")

	c := dial(t, addr)
	c.Write([]byte("*1\r\n$7\r\nMONITOR\r\n"))
	first := readRESP(t, c) // seq 1 and 2
	if !strings.HasPrefix(first, "*2\r\n") {
		t.Fatalf("want 2 events, got %q", first)
	}
	// Fetch after seq 2 → empty.
	c.Write([]byte("*3\r\n$7\r\nMONITOR\r\n$1\r\n2\r\n$1\r\n5\r\n"))
	rest := readRESP(t, c)
	if rest != "*0\r\n" {
		t.Fatalf("after seq 2 want empty, got %q", rest)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestMonitorRESP3MapEvents(t *testing.T) {
	addr := serveTest(t)

	// Upgrade to RESP3 and issue one request.
	c := dial(t, addr)
	c.Write([]byte("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"))
	if resp := readRESP(t, c); !strings.HasPrefix(resp, "%") {
		t.Fatalf("HELLO 3 failed: %q", resp)
	}
	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$1\r\nx\r\n"))
	readRESP(t, c)

	// Under RESP3 each event is a map with the RESP2 field names.
	c.Write([]byte("*1\r\n$7\r\nMONITOR\r\n"))
	resp := readRESP(t, c)
	if !strings.HasPrefix(resp, "*1\r\n%6\r\n") {
		t.Fatalf("RESP3 MONITOR should be an array of 6-field maps, got %q", resp)
	}
	for _, key := range []string{"seq", "at_us", "model", "texts", "latency_us", "err"} {
		want := "$" + itoa(len(key)) + "\r\n" + key + "\r\n"
		if !strings.Contains(resp, want) {
			t.Fatalf("missing key %q in %q", key, resp)
		}
	}

	// RESP2 connections keep the flat per-event array.
	c2 := dial(t, addr)
	c2.Write([]byte("*1\r\n$7\r\nMONITOR\r\n"))
	if resp2 := readRESP(t, c2); !strings.HasPrefix(resp2, "*1\r\n*6\r\n") {
		t.Fatalf("RESP2 MONITOR should be flat 6-element arrays, got %q", resp2)
	}
}
