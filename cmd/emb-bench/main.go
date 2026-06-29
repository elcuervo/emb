package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"time"
)

func main() {
	addr := "127.0.0.1:6379"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	n := 50
	times := make([]float64, n)

	for i := range n {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connecting: %v\n", err)
			os.Exit(1)
		}

		start := time.Now()

		cmd := "*3\r\n$3\r\nEMB\r\n$6\r\nminilm\r\n$11\r\nhello world\r\n"
		_, _ = conn.Write([]byte(cmd))

		resp := make([]byte, 4096)
		_, _ = conn.Read(resp)

		elapsed := float64(time.Since(start).Microseconds()) / 1000
		times[i] = elapsed
		conn.Close()

		if i == 0 {
			fmt.Printf("  First request: %.1f ms\n", elapsed)
		}
	}

	sort.Float64s(times)

	p50 := times[int(n*50/100)]
	p95 := times[int(n*95/100)]
	p99 := times[int(n*99/100)]

	var sum float64
	for _, t := range times {
		sum += t
	}
	avg := sum / float64(n)

	fmt.Printf("  P50:  %.1f ms\n", p50)
	fmt.Printf("  P95:  %.1f ms\n", p95)
	fmt.Printf("  P99:  %.1f ms\n", p99)
	fmt.Printf("  Avg:  %.1f ms\n", avg)
	fmt.Printf("  Min:  %.1f ms  Max:  %.1f ms\n", times[0], times[n-1])

	output := fmt.Sprintf(`{
  "p50_ms": %.1f,
  "p95_ms": %.1f,
  "p99_ms": %.1f,
  "avg_ms": %.1f,
  "min_ms": %.1f,
  "max_ms": %.1f,
  "n": %d
}`, p50, p95, p99, avg, times[0], times[n-1], n)
	os.WriteFile("benchmark-responsetime.txt", []byte(output), 0644)
	fmt.Println("  Results -> benchmark-responsetime.txt")
}
