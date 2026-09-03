package server

import (
	"net"
	"runtime"
	"testing"

	"github.com/elcuervo/emb/internal/registry"
)

// TestServerNoLeakAcrossRequests is the server-level leak guard: thousands of
// EMB requests across persistent connections must leave the goroutine count at
// baseline and heap growth flat after GC — an automated "no memory leak" gate
// so growth regressions fail CI instead of needing manual RSS watching.
func TestServerNoLeakAcrossRequests(t *testing.T) {
	addr, _ := serveTestWithServer(t)
	conns := make([]net.Conn, 3)
	for i := range conns {
		conns[i] = dial(t, addr)
	}
	emb := func(c net.Conn, text string) {
		t.Helper()
		c.Write(respCommand("EMB", "test", text))
		readRESP(t, c)
	}

	// Warmup so connections, workers, and lazy allocations exist before the
	// baseline is taken.
	emb(conns[0], "warmup")
	runtime.GC()
	baseGoroutines := runtime.NumGoroutine()
	baseHeap := registry.HeapInUseBytes()

	const reqs = 2000
	for i := 0; i < reqs; i++ {
		emb(conns[i%len(conns)], "leak-probe")
	}

	runtime.GC()
	gotGoroutines := runtime.NumGoroutine()
	gotHeap := registry.HeapInUseBytes()

	if gotGoroutines > baseGoroutines+3 {
		t.Errorf("goroutines grew across %d requests: %d -> %d", reqs, baseGoroutines, gotGoroutines)
	}
	if growth := int64(gotHeap) - int64(baseHeap); growth > 4<<20 {
		t.Errorf("heap grew %d bytes across %d requests (leak?)", growth, reqs)
	}
}
