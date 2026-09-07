package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"testing"
)

var cacheBenchSink []byte

func BenchmarkCacheGet(b *testing.B) {
	c := NewCache(64 << 20)
	value := make([]byte, 384*4)
	c.Set("minilm:benchmark text", value)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cacheBenchSink, _ = c.Get("minilm:benchmark text")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := NewCache(64 << 20)
	value := make([]byte, 384*4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		c.Set(fmt.Sprintf("minilm:text-%d", i&1023), value)
	}
}

func BenchmarkServerEMBCached(b *testing.B) {
	benchmarkServerEMB(b, true)
}

func BenchmarkServerEMBUncached(b *testing.B) {
	benchmarkServerEMB(b, false)
}

func benchmarkServerEMB(b *testing.B, cached bool) {
	addr := serveTestWithCache(b, "64MB")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = conn.Close() })
	reader := bufio.NewReader(conn)
	if cached {
		if _, err := conn.Write(respCommand("EMB", "test", "benchmark")); err != nil {
			b.Fatal(err)
		}
		benchmarkReadRESP(b, reader)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		text := "benchmark"
		if !cached {
			text = fmt.Sprintf("benchmark-%d", i)
		}
		if _, err := conn.Write(respCommand("EMB", "test", text)); err != nil {
			b.Fatal(err)
		}
		benchmarkReadRESP(b, reader)
	}
}

// benchmarkReadRESP consumes one complete bulk-string RESP reply from the
// benchmark connection so every iteration measures a full request/response
// cycle. A single net.Conn.Read may return only a prefix of a reply (and a
// fixed buffer may be smaller than large embedding payloads), which would let
// the next iteration read stale reply bytes and skew the cached/uncached
// timing split.
func benchmarkReadRESP(b *testing.B, r *bufio.Reader) {
	b.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		b.Fatal(err)
	}
	if len(line) < 4 || line[0] != '$' {
		b.Fatalf("unexpected RESP reply line %q", line)
	}
	var n int
	if _, err := fmt.Sscanf(line[1:len(line)-2], "%d", &n); err != nil || n < 0 {
		b.Fatalf("invalid RESP bulk length %q", line)
	}
	if _, err := io.ReadFull(r, make([]byte, n+2)); err != nil {
		b.Fatal(err)
	}
}
