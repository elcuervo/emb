package embstress

import (
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/elcuervo/emb/internal/embtop"
)

// scripted is a canned RESP server for the real client's tests: it answers
// EMB.MODELS and EMB with fixed replies, and can spoil a chosen reply so the
// client's recovery is exercised.
type scripted struct {
	ln net.Listener

	mu       sync.Mutex
	conns    int
	spoiled  int // the connection number whose first reply is garbage
	spoiledN int // how many replies have been spoilt so far
}

// startScripted listens on a free port and answers every command.
func startScripted(t *testing.T, spoilConns ...int) (*scripted, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &scripted{ln: ln}
	for _, n := range spoilConns {
		if n == 1 {
			s.spoiled = 1
		}
	}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s, ln.Addr().String()
}

func (s *scripted) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns++
		n := s.conns
		s.mu.Unlock()
		go s.handle(conn, n)
	}
}

func (s *scripted) handle(conn net.Conn, n int) {
	defer func() { _ = conn.Close() }()
	buf := make([]byte, 4096)
	first := true
	for {
		if _, err := conn.Read(buf); err != nil {
			return
		}
		cmd := string(buf)
		switch {
		case strings.Contains(cmd, "EMB.MODELS"):
			_, _ = conn.Write([]byte(modelsWire))
		case strings.Contains(cmd, "EMB"):
			if first && n == s.spoiled {
				// A reply that is not a reply: the stream is no longer at a
				// boundary, which is exactly what a stalled or restarted peer
				// looks like.
				_, _ = conn.Write([]byte("not-a-resp-reply\r\n"))
				return
			}
			_, _ = conn.Write([]byte(embedWire))
		default:
			_, _ = conn.Write([]byte("-ERR unknown\r\n"))
		}
		first = false
	}
}

func (s *scripted) connCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

// modelsWire is EMB.MODELS: one row of [name, dim, status].
const modelsWire = "*1\r\n*3\r\n$1\r\nm\r\n:384\r\n$5\r\nready\r\n"

// embedWire is an EMB reply: one embedding as a binary bulk string.
const embedWire = "*1\r\n$8\r\n12345678\r\n"

// TestClientModelsReadsTheFleet covers the real endpoint's first call.
func TestClientModelsReadsTheFleet(t *testing.T) {
	_, addr := startScripted(t)
	c := NewClient(addr, addr, "", false)
	defer func() { _ = c.Close() }()

	models, err := c.Models()
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 || models[0].Name != "m" || models[0].Dim != 384 || models[0].Status != "ready" {
		t.Fatalf("Models = %+v, want one ready model named m at 384", models)
	}
	if c.Label() != addr {
		t.Fatalf("Label = %q, want the address", c.Label())
	}
}

// TestClientConnIsOnePerWorker covers the fan-out's concurrency model: every
// Conn is its own connection, because two workers interleaving on one socket
// corrupt the stream.
func TestClientConnIsOnePerWorker(t *testing.T) {
	s, addr := startScripted(t)
	c := NewClient(addr, addr, "", false)
	defer func() { _ = c.Close() }()

	for i := range 3 {
		w, err := c.Conn()
		if err != nil {
			t.Fatalf("Conn %d: %v", i, err)
		}
		if err := w.Embed("m", "hello"); err != nil {
			t.Fatalf("Embed on worker %d: %v", i, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("closing worker %d: %v", i, err)
		}
	}
	if got := s.connCount(); got != 3 {
		t.Fatalf("the server saw %d connections, want one per worker", got)
	}
}

// TestClientEmbedSurvivesABrokenReply is the regression test for the flood: a
// reply that arrives broken must close the connection so the next request
// re-dials, instead of every later request misreading the leftover bytes.
func TestClientEmbedSurvivesABrokenReply(t *testing.T) {
	s, addr := startScripted(t)
	s.spoiled = 1 // the first connection's first EMB reply is garbage
	c := NewClient(addr, addr, "", false)
	defer func() { _ = c.Close() }()

	w, err := c.Conn()
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer func() { _ = w.Close() }()

	if err := w.Embed("m", "hello"); err == nil {
		t.Fatal("a broken reply was reported as a success")
	}
	// The same worker must recover on its next request, not repeat the failure
	// forever.
	for i := range 3 {
		if err := w.Embed("m", "hello"); err != nil {
			t.Fatalf("Embed %d after the broken reply: %v", i+1, err)
		}
	}
	if got := s.connCount(); got < 2 {
		t.Fatalf("the server saw %d connections, want a fresh one after the break", got)
	}
}

// TestClientConnRefusedNamesTheEndpoint covers the failure a fleet that is not
// listening produces.
func TestClientConnRefusedNamesTheEndpoint(t *testing.T) {
	// A port nothing listens on: bind and release it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	c := NewClient(addr, addr, "", false)
	defer func() { _ = c.Close() }()
	if _, err := c.Conn(); err == nil {
		t.Fatal("Conn to a closed port succeeded")
	}
	_, err = c.Models()
	if err == nil {
		t.Fatal("Models against a closed port succeeded")
	}
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("the failure does not name what could not be reached: %v", err)
	}
}

// TestReconcileAgainstTheRealClient ties the two halves together: what the real
// client reads is what reconciliation decides on.
func TestReconcileAgainstTheRealClient(t *testing.T) {
	_, addr := startScripted(t)
	c := NewClient(addr, "node-1", "", false)
	defer func() { _ = c.Close() }()

	models, err := c.Models()
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	pairs, drift := Reconcile(
		[]ConfiguredModel{{Name: "m", Dim: 384, MaxLength: 64}},
		map[string][]embtop.ModelListEntry{c.Label(): models},
	)
	if len(drift) != 0 {
		t.Fatalf("drift = %v, want none", drift)
	}
	if len(pairs) != 1 || pairs[0].Endpoint != "node-1" || pairs[0].Model != "m" {
		t.Fatalf("pairs = %+v, want node-1 serving m", pairs)
	}
}
