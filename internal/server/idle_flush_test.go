package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// runCounterSession counts total ONNX runs so batching tests can assert
// coalescing behavior without timing flakiness.
type runCounterSession struct {
	mu    sync.Mutex
	total int
	max   int
	cur   int
}

func (s *runCounterSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	s.mu.Lock()
	s.cur++
	s.total++
	if s.cur > s.max {
		s.max = s.cur
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cur--
		s.mu.Unlock()
	}()

	time.Sleep(500 * time.Microsecond) // small work so the run loop drains
	data := make([]float32, batchSize*seqLen*dim)
	return data, nil
}

func (s *runCounterSession) Close() error { return nil }

func (s *runCounterSession) totals() (total, maxConcurrent int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total, s.max
}

// TestIdleFlushServesLoneRequestImmediately verifies a lone request through a
// batched pool is served immediately instead of waiting out the batching
// window (2s here): idle-flush latency must be far below the window.
func TestIdleFlushServesLoneRequestImmediately(t *testing.T) {
	timeoutMS := 2000
	sess := &runCounterSession{}
	addr, _ := serveWithPool(t, func() (onnx.Session, error) { return sess, nil }, 1, timeoutMS, 32)

	c := dial(t, addr)
	start := time.Now()
	c.Write(respCommand("EMB", "test", "hello world"))
	resp := readRESP(t, c)
	c.Close()
	elapsed := time.Since(start)

	if strings.HasPrefix(resp, "-") {
		t.Fatalf("EMB failed: %q", resp)
	}
	// A 2s window would make the pre-idle-flush path take ~2s. Allow generous
	// headroom for CI noise while still proving we did not wait the window.
	if elapsed > time.Duration(timeoutMS)*time.Millisecond/2 {
		t.Fatalf("lone request waited %v with a %dms window; idle-flush failed", elapsed, timeoutMS)
	}
	total, _ := sess.totals()
	if total != 1 {
		t.Fatalf("expected 1 run for one request, got %d", total)
	}
}

// TestIdleFlushBurstStillCoalesces verifies a burst of concurrent requests is
// still merged into shared ONNX runs (idle-flush must not serialize every
// request into its own run).
func TestIdleFlushBurstStillCoalesces(t *testing.T) {
	sess := &runCounterSession{}
	addr, _ := serveWithPool(t, func() (onnx.Session, error) { return sess, nil }, 1, 500, 64)

	const n = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				errs[i] = err
				return
			}
			defer conn.Close()
			conn.Write(respCommand("EMB", "test", "text-"+strconv.Itoa(i)))
			line, rerr := readLine(conn)
			if rerr != nil {
				errs[i] = rerr
				return
			}
			if len(line) == 0 || line[0] != '$' {
				errs[i] = fmt.Errorf("unexpected response %q", line)
				return
			}
			if _, perr := readBulkPayload(conn, line); perr != nil {
				errs[i] = perr
			}
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("client error: %v", err)
		}
	}

	total, maxConcurrent := sess.totals()
	if total >= n {
		t.Fatalf("burst not coalesced: %d runs for %d requests", total, n)
	}
	if maxConcurrent > 1 {
		t.Logf("max concurrent runs: %d", maxConcurrent) // informative only
	}
	t.Logf("coalesced %d requests into %d runs", n, total)
}

// readLine reads a header line ending in CRLF.
func readLine(conn net.Conn) ([]byte, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		if _, err := conn.Read(buf); err != nil {
			return nil, err
		}
		line = append(line, buf[0])
		if len(line) >= 2 && line[len(line)-2] == '\r' && line[len(line)-1] == '\n' {
			return line, nil
		}
	}
}

// readBulkPayload reads the payload of a $<n> bulk response whose header line
// was already read.
func readBulkPayload(conn net.Conn, header []byte) ([]byte, error) {
	n, err := strconv.Atoi(strings.TrimSpace(string(header[1:])))
	if err != nil || header[0] != '$' {
		return nil, fmt.Errorf("bad bulk header %q", header)
	}
	payload := make([]byte, n)
	if _, err := readFull(conn, payload); err != nil {
		return nil, err
	}
	trail := make([]byte, 2)
	if _, err := readFull(conn, trail); err != nil {
		return nil, err
	}
	return payload, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
