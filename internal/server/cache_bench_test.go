package server

import (
	"fmt"
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
	if cached {
		if _, err := conn.Write(respCommand("EMB", "test", "benchmark")); err != nil {
			b.Fatal(err)
		}
		benchmarkReadRESP(b, conn)
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
		benchmarkReadRESP(b, conn)
	}
}

func benchmarkReadRESP(b *testing.B, conn net.Conn) {
	b.Helper()
	buf := make([]byte, 4096)
	if _, err := conn.Read(buf); err != nil {
		b.Fatal(err)
	}
}
