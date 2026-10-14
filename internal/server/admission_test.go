package server

import (
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// TestAdmissionTimeoutRefusesWhileHealthAnswers covers the bounded-admission
// contract at the server boundary: a request that cannot be served inside its
// admission deadline is refused with the busy shape and counted, and it does
// not hold the client for the duration of the work it was waiting on.
func TestAdmissionTimeoutRefusesWhileHealthAnswers(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	addr, _ := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate, entered: entered}, nil },
		1, 0, 32,
		WithAdmissionTimeout(100*time.Millisecond),
	)
	c1 := dial(t, addr)
	c2 := dial(t, addr)

	// c1 occupies the only worker: wait for the run itself to start, since
	// active_requests counts admitted commands, not picked-up work.
	c1.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first request never entered the session")
	}

	start := time.Now()
	c2.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nworld\r\n"))
	resp := readRESP(t, c2)
	if !strings.HasPrefix(resp, "-ERR busy") {
		t.Fatalf("expected a busy refusal, got %q", resp)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("the refusal waited %s", waited)
	}
	if got := statsIntField(t, c2, "admission_refusals"); got != 1 {
		t.Fatalf("the refusal was not counted: admission_refusals=%d", got)
	}
	// The new admission signals are part of the reply, and an idle gate with no
	// run that waited for it reports zero rather than a missing field.
	for _, f := range []string{"cpu_admission_wait_us", "cpu_admission_waiters", "cpu_admission_timeouts"} {
		if got := statsIntField(t, c2, f); got != 0 {
			t.Fatalf("%s = %d with an idle gate, want 0", f, got)
		}
	}

	// Control commands keep answering while admission refuses work.
	c2.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if resp := readRESP(t, c2); resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}
	c2.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
	if resp := readRESP(t, c2); !strings.HasPrefix(resp, "*") {
		t.Fatalf("expected a STATS array, got %q", resp)
	}

	// The refused command never took the worker; the in-flight one completes.
	close(gate)
	if done := readRESP(t, c1); len(done) < 3 || done[0] != '$' {
		t.Fatalf("the in-flight request did not complete: %q", done)
	}
}

// TestMultiAdmissionTimeoutDoesNotHang covers the EMB.MULTI path against the
// same bound as EMB: a pair that cannot be dispatched inside the admission
// deadline fails its slot instead of waiting on context.Background until the
// worker frees. Before this test the pair used Pool.Embed (a background
// context), so with the single worker parked it never returned.
func TestMultiAdmissionTimeoutDoesNotHang(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	addr, _ := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate, entered: entered}, nil },
		1, 0, 32,
		WithAdmissionTimeout(100*time.Millisecond),
	)
	c1 := dial(t, addr)
	c2 := dial(t, addr)

	// c1 occupies the only worker.
	c1.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first request never entered the session")
	}

	// The read deadline turns the old unbounded wait into a failure, not a hang.
	if err := c2.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	c2.Write([]byte("*3\r\n$9\r\nEMB.MULTI\r\n$4\r\ntest\r\n$5\r\nworld\r\n"))
	resp := readRESP(t, c2)
	if waited := time.Since(start); waited > 3*time.Second {
		t.Fatalf("EMB.MULTI waited %s for admission; want it bounded by the admission deadline", waited)
	}
	if !strings.HasPrefix(resp, "*") {
		t.Fatalf("expected an EMB.MULTI array reply, got %q", resp)
	}
	// The pair could not be dispatched, so its slot is null rather than a
	// fabricated embedding (MGET-style partial failure).
	if !strings.Contains(resp, "$-1") {
		t.Fatalf("expected the un-admitted pair's slot to be null, got %q", resp)
	}

	// The in-flight EMB still completes once the worker frees.
	close(gate)
	if done := readRESP(t, c1); len(done) < 3 || done[0] != '$' {
		t.Fatalf("the in-flight request did not complete: %q", done)
	}
}
