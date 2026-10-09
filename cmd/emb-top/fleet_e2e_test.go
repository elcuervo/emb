package main

import (
	"net"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/server"
)

// serveEmbeddedTop starts a real in-process emb server on a free address and
// waits until it accepts connections. It mirrors the helper the embtop e2e
// tests use, so the fleet is verified against the real RESP surface.
func serveEmbeddedTop(t *testing.T) string {
	t.Helper()
	reg := registry.New()
	addr := freeAddrTop(t)
	srv := server.New(addr, reg, "", "", nil)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	t.Cleanup(func() { srv.Close() })

	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-errCh:
			t.Fatalf("server exited before becoming ready: %v", err)
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return addr
		}
		if time.Now().After(deadline) {
			t.Fatalf("server at %s not ready within 5s: %v", addr, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func freeAddrTop(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// TestFleetPollsTwoEmbeddedNodesOneTick verifies each node is polled on its
// own connection against a real server, and both report in one round.
func TestFleetPollsTwoEmbeddedNodesOneTick(t *testing.T) {
	a := serveEmbeddedTop(t)
	b := serveEmbeddedTop(t)
	f := newFleet([]resolvedNode{rn(a), rn(b)}, &options{interval: time.Second, window: 10})
	defer f.close()

	if ok := f.pollAllSync(); ok != 2 {
		t.Fatalf("pollAllSync sampled %d nodes, want 2", ok)
	}
	for _, n := range f.nodes {
		if !n.reachable {
			t.Errorf("node %s not reachable", n.addr)
		}
		if n.tui.polls != 1 {
			t.Errorf("node %s polls = %d, want 1", n.addr, n.tui.polls)
		}
		// INFO cpu gives each node's own parallelism for the CPU denominator.
		if n.tui.sampler.Latest.GoMaxProcs <= 0 {
			t.Errorf("node %s did not report gomaxprocs via INFO cpu", n.addr)
		}
	}
}
