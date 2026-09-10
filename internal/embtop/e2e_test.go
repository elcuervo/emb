package embtop_test

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/server"
)

// TestRunOnceAgainstRealServer starts a real in-process emb server (no
// models, which is fine: EMB.MODELS/EMB.STATS work with zero models) and
// runs the headless once mode against it, asserting line format and
// non-negativity.
func TestRunOnceAgainstRealServer(t *testing.T) {
	addr := serveEmbedded(t)

	c := embtop.NewClient(addr, "", false)
	var out bytes.Buffer
	if err := embtop.RunOnce(c, 20*time.Millisecond, 2, &out); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "t=") {
			t.Fatalf("line %d missing t= prefix: %q", i, line)
		}
		if !strings.Contains(line, "models_loaded=0") {
			t.Fatalf("line %d missing models_loaded=0: %q", i, line)
		}
		// All numeric fields must parse and be non-negative.
		for _, f := range strings.Fields(line) {
			if !strings.Contains(f, "=") || strings.Contains(f, "model:") {
				continue
			}
			val := f[strings.Index(f, "=")+1:]
			if f == "pooling" || strings.HasPrefix(f, "dim=") {
				continue
			}
			if _, err := strconv.ParseFloat(val, 64); err != nil {
				t.Fatalf("line %d field %q not numeric: %v", i, f, err)
			}
		}
	}
}

// TestPollRealServerEmpty exercises a single pipelined poll against the real
// server (no models) and the client-side parsing of EMB.MODELS + EMB.STATS.
func TestPollRealServerEmpty(t *testing.T) {
	addr := serveEmbedded(t)

	c := embtop.NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	res, err := c.Poll(nil, 0)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(res.Models) != 0 {
		t.Fatalf("expected no models, got %+v", res.Models)
	}
	if res.UptimeSecs < 0 || res.TotalRequests < 0 {
		t.Fatalf("unexpected stats: %+v", res)
	}
}

// freeAddr returns an available localhost TCP address.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// serveEmbedded starts an in-process emb server on a free address and waits
// until it accepts connections, failing fast if the listener reports an error.
func serveEmbedded(t *testing.T) string {
	t.Helper()
	reg := registry.New()
	addr := freeAddr(t)
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
