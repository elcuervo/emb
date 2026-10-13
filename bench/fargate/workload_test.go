package main

// Tests for the redis-benchmark / mixed-workload fixes: wall-clock throughput,
// literal p90, and explicit __rand_int__ randomization.

import (
	"net"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRedisWorkloadRandomizesUniqueTextExplicitly(t *testing.T) {
	args, err := redisWorkload("unique-text", "minilm")
	if err != nil {
		t.Fatal(err)
	}
	// redis-benchmark only expands __rand_int__ under -r; without it the token
	// is sent verbatim and every "unique" request carries the same text.
	i := slices.Index(args, "-r")
	if i < 0 || args[i+1] != uniqueKeyspace {
		t.Fatalf("unique-text must pass -r %s, got %v", uniqueKeyspace, args)
	}
	if !slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "__rand_int__") }) {
		t.Fatalf("unique-text must carry __rand_int__: %v", args)
	}
	for _, w := range []string{"fixed-length", "cache-hit"} {
		got, err := redisWorkload(w, "minilm")
		if err != nil || slices.Contains(got, "-r") || strings.Contains(strings.Join(got, " "), "__rand_int__") {
			t.Fatalf("%s must not randomize: %v (%v)", w, got, err)
		}
	}
	if _, err := redisWorkload("nope", "minilm"); err == nil {
		t.Fatal("an unknown workload must be an error")
	}
}

func TestParseWallMs(t *testing.T) {
	if ms, err := parseWallMs("mixed clients=8 count=100 wall_ms=2500.5\n"); err != nil || ms != 2500.5 {
		t.Fatalf("wall = %v, %v", ms, err)
	}
	for _, bad := range []string{"", "mixed clients=8", "wall_ms=0"} {
		if _, err := parseWallMs(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

func TestPctIsNearestRank(t *testing.T) {
	ten := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := pct(ten, 90); got != 9 {
		t.Fatalf("p90 of 1..10 = %v, want 9", got)
	}
	// The floor-indexed percentile returned 2 here, under-reporting the tail.
	if got := pct([]float64{1, 2, 3}, 90); got != 3 {
		t.Fatalf("p90 of three samples = %v, want the largest", got)
	}
	if got := pct(ten, 50); got != 5 {
		t.Fatalf("p50 of 1..10 = %v, want 5", got)
	}
	if pct(nil, 90) != 0 {
		t.Fatal("empty sample must be 0")
	}
}

func TestParseBenchDoesNotInventP90(t *testing.T) {
	out := `  500 requests completed in 0.50 seconds
1000.00 requests per second

latency summary (msec):
          avg       min       p50       p95       p99       max
        3.311     1.808     2.287     6.079    29.791    31.487
`
	b, err := parseBench(out)
	if err != nil {
		t.Fatal(err)
	}
	if b.p90 != 0 {
		t.Fatalf("redis-benchmark prints no p90; got %v (an approximation from p95 is not a p90)", b.p90)
	}
	if b.p50 != 2.287 || b.p95 != 6.079 || b.p99 != 29.791 {
		t.Fatalf("summary quantiles = %+v", b)
	}
}

// TestMixedCellThroughputUsesWallElapsedTime drives load.rb against a server
// that takes 20 ms per reply: four clients complete ~200 req/s. Dividing by the
// sum of request latencies (the old computation) reports ~50.
func TestMixedCellThroughputUsesWallElapsedTime(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby is part of the nix develop profile; run the harness tests there")
	}
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				buf := make([]byte, 1<<16)
				for {
					if _, err := conn.Read(buf); err != nil {
						return
					}
					time.Sleep(20 * time.Millisecond)
					if _, err := conn.Write([]byte("$4\r\nabcd\r\n")); err != nil {
						return
					}
				}
			}()
		}
	}()

	o := &options{
		count: 40, mixedCount: 40, outDir: t.TempDir(), clients: []int{4}, model: "m",
		shortRatio: 0.8, longLen: 500, modelDir: t.TempDir(), maxLen: 512, maxBatch: 32,
	}
	res := &result{}
	if err := collectMixedCell(o, res, 1, ln.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatal(err)
	}
	c := res.Cells[0]
	if c.Reqs < 100 || c.Reqs > 300 {
		t.Fatalf("req/s = %.1f, want ~200 (wall elapsed time over 4 clients), not ~50", c.Reqs)
	}
	if c.P90 < 20 || c.P90 < c.P50 || c.P99 < c.P90 {
		t.Fatalf("quantiles must be literal and ordered: p50=%v p90=%v p99=%v", c.P50, c.P90, c.P99)
	}
}
