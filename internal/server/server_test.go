package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/registry"
)

type mockTokenizer struct{}

func (mockTokenizer) Encode(text string, maxLength int) ([]int64, []int64, error) {
	ids := []int64{101}
	mask := []int64{1}
	for _, r := range text {
		ids = append(ids, int64(r))
		mask = append(mask, 1)
	}
	ids = append(ids, 102)
	mask = append(mask, 1)
	if len(ids) > maxLength {
		ids = ids[:maxLength]
		mask = mask[:maxLength]
	}
	return ids, mask, nil
}

func (mockTokenizer) Close() error { return nil }

type mockSession struct {
	mu sync.Mutex
}

func (m *mockSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := make([]float32, batchSize*seqLen*dim)
	for i := range data {
		data[i] = float32(i % dim)
	}
	return data, nil
}

func (m *mockSession) Close() error { return nil }

func TestServerPING(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	resp := readRESP(t, c)
	if resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}
	c.Close()
}

func TestServerEMBSingle(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 3 || resp[0] != '$' {
		t.Fatalf("expected bulk string, got %q", resp)
	}
	c.Close()
}

func TestServerEMBBatch(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*4\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$1\r\na\r\n$1\r\nb\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 3 || resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerEMBUnknownModel(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$10\r\nnonexistent\r\n$4\r\ntest\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 5 || resp[:2] != "-E" {
		t.Fatalf("expected error, got %q", resp)
	}
	c.Close()
}

func TestServerEMBNoArgs(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$3\r\nEMB\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 5 || resp[:2] != "-E" {
		t.Fatalf("expected error, got %q", resp)
	}
	c.Close()
}

func TestServerMODELS(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$10\r\nEMB.MODELS\r\n"))
	resp := readRESP(t, c)
	if resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerINFO(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*2\r\n$8\r\nEMB.INFO\r\n$4\r\ntest\r\n"))
	resp := readRESP(t, c)
	if resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerINFONotFound(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*2\r\n$8\r\nEMB.INFO\r\n$10\r\nnonexistent\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 5 || resp[:2] != "-E" {
		t.Fatalf("expected error, got %q", resp)
	}
	c.Close()
}

func TestServerSTATS(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
	resp := readRESP(t, c)
	if resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerHELP(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$8\r\nEMB.HELP\r\n"))
	resp := readRESP(t, c)
	if resp[0] != '$' {
		t.Fatalf("expected bulk string, got %q", resp)
	}
	c.Close()
}

func TestServerEMBMULTISingle(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*3\r\n$9\r\nEMB.MULTI\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 3 || resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerEMBMULTIMultiple(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*5\r\n$9\r\nEMB.MULTI\r\n$4\r\ntest\r\n$1\r\na\r\n$4\r\ntest\r\n$1\r\nb\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 3 || resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerEMBMULTIOddArgs(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*4\r\n$9\r\nEMB.MULTI\r\n$4\r\ntest\r\n$1\r\na\r\n$11\r\nnonexistent\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 5 || resp[:2] != "-E" {
		t.Fatalf("expected error, got %q", resp)
	}
	c.Close()
}

func TestServerEMBMULTINoArgs(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$9\r\nEMB.MULTI\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 5 || resp[:2] != "-E" {
		t.Fatalf("expected error, got %q", resp)
	}
	c.Close()
}

func TestServerEMBMULTIUnknownModel(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*5\r\n$9\r\nEMB.MULTI\r\n$4\r\ntest\r\n$1\r\na\r\n$11\r\nnonexistent\r\n$1\r\nb\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 3 || resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

func TestServerEMBMULTIStats(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*5\r\n$9\r\nEMB.MULTI\r\n$4\r\ntest\r\n$1\r\na\r\n$4\r\ntest\r\n$1\r\nb\r\n"))
	readRESP(t, c)

	c.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
	resp := readRESP(t, c)
	if resp[0] != '*' {
		t.Fatalf("expected array, got %q", resp)
	}
	c.Close()
}

// countingSession reports the max number of concurrent Run calls, without serializing
// them, so tests can assert bounded fan-out (not just bounded worker count).
type countingSession struct {
	mu  sync.Mutex
	max int
	cur int
}

func (s *countingSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	s.mu.Lock()
	s.cur++
	if s.cur > s.max {
		s.max = s.cur
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.cur--
		s.mu.Unlock()
	}()

	time.Sleep(2 * time.Millisecond) // encourage overlap across workers
	data := make([]float32, batchSize*seqLen*dim)
	return data, nil
}

func (s *countingSession) Close() error { return nil }

func (s *countingSession) peekMax() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.max
}

// respCommand encodes arguments as a RESP array command.
func respCommand(args ...string) []byte {
	var b []byte
	b = append(b, '*')
	b = strconv.AppendInt(b, int64(len(args)), 10)
	b = append(b, '\r', '\n')
	for _, a := range args {
		b = append(b, '$')
		b = strconv.AppendInt(b, int64(len(a)), 10)
		b = append(b, '\r', '\n')
		b = append(b, a...)
		b = append(b, '\r', '\n')
	}
	return b
}

// respArrayElements parses a RESP array of bulk/null strings into its elements,
// using "<null>" for null bulk strings (for MGET-semantics assertions).
func respArrayElements(t *testing.T, raw string) []string {
	t.Helper()
	if len(raw) == 0 || raw[0] != '*' {
		t.Fatalf("expected RESP array, got %q", raw)
	}
	rest := raw[1:]
	end := strings.IndexByte(rest, '\r')
	count, err := strconv.Atoi(rest[:end])
	if err != nil {
		t.Fatalf("bad array count: %v", err)
	}
	stream := rest[end+2:]
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if stream[0] == '$' {
			if stream[1] == '-' { // $-1 null (5 bytes: $ - 1 \r \n)
				out = append(out, "<null>")
				stream = stream[5:]
				continue
			}
			end := strings.IndexByte(stream, '\r')
			l, err := strconv.Atoi(stream[1:end])
			if err != nil {
				t.Fatalf("bad bulk len: %v", err)
			}
			out = append(out, stream[end+2:end+2+l])
			stream = stream[end+2+l+2:]
		} else {
			t.Fatalf("unexpected element in array: %q", stream)
		}
	}
	return out
}

// TestServerEMBMULTIFanOutBounded proves a large EMB.MULTI does not spawn unbounded
// concurrency: with fanOut=2 and 64 pairs it completes correctly while the number of
// concurrent inference runs never exceeds 2 (bounded by fan-out, not by pair count).
func TestServerEMBMULTIFanOutBounded(t *testing.T) {
	sess := &countingSession{}
	addr, srv := serveWithPool(t, func() (onnx.Session, error) { return sess, nil }, 8, 0, 32)
	srv.fanOut = 2

	args := []string{"EMB.MULTI"}
	for i := 0; i < 64; i++ {
		args = append(args, "test", "x")
	}

	c := dial(t, addr)
	c.Write(respCommand(args...))
	resp := readRESP(t, c)
	c.Close()

	elements := respArrayElements(t, resp)
	if len(elements) != 64 {
		t.Fatalf("expected 64 elements, got %d", len(elements))
	}
	for i, el := range elements {
		if el == "<null>" {
			t.Fatalf("element %d unexpectedly null", i)
		}
	}
	if got := sess.peekMax(); got > 2 {
		t.Fatalf("fan-out not bounded: %d concurrent runs (cap 2)", got)
	}
}

// TestServerEMBMULTIMGETSemantics preserves per-pair nil (MGET) semantics under the
// bounded fan-out path.
func TestServerEMBMULTIMGETSemantics(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write(respCommand("EMB.MULTI", "test", "a", "nonexistent", "b", "test", "c"))
	resp := readRESP(t, c)
	c.Close()

	elements := respArrayElements(t, resp)
	if len(elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elements))
	}
	if elements[0] == "<null>" || elements[2] == "<null>" {
		t.Fatalf("known-model elements should be non-null")
	}
	if elements[1] != "<null>" {
		t.Fatalf("unknown-model element should be null, got %q", elements[1])
	}
}

// idBasedSession returns embeddings that depend only on the input ids, so the same text
// yields the same embedding whether it runs alone (EMB) or batched inside a MULTI window.
type idBasedSession struct{}

func (idBasedSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	data := make([]float32, batchSize*seqLen*dim)
	for b := 0; b < batchSize; b++ {
		var sum int64
		for j := 0; j < seqLen; j++ {
			sum += inputIDs[b*seqLen+j]
		}
		for d := 0; d < dim; d++ {
			data[b*seqLen*dim+d] = float32(sum)
		}
	}
	return data, nil
}

func (idBasedSession) Close() error { return nil }

// respBulk extracts the payload of a RESP bulk string (e.g. a single EMB reply).
func respBulk(t *testing.T, raw string) string {
	t.Helper()
	if len(raw) == 0 || raw[0] != '$' || raw[1] == '-' {
		t.Fatalf("expected RESP bulk, got %q", raw)
	}
	end := strings.IndexByte(raw, '\r')
	l, err := strconv.Atoi(raw[1:end])
	if err != nil {
		t.Fatalf("bad bulk len: %v", err)
	}
	return raw[end+2 : end+2+l]
}

// TestServerMULTIBatchingMatchesSequential verifies windowed MULTI (batching enabled)
// produces embeddings identical to the same texts via sequential EMB calls.
func TestServerMULTIBatchingMatchesSequential(t *testing.T) {
	addr, _ := serveWithPool(t, func() (onnx.Session, error) { return idBasedSession{}, nil }, 2, 1, 32)

	c := dial(t, addr)

	c.Write(respCommand("EMB.MULTI", "test", "alpha", "test", "beta"))
	multi := respArrayElements(t, readRESP(t, c))
	if len(multi) != 2 || multi[0] == "<null>" || multi[1] == "<null>" {
		t.Fatalf("multi batching produced invalid elements: %v", multi)
	}

	c.Write(respCommand("EMB", "test", "alpha"))
	seqA := respBulk(t, readRESP(t, c))
	c.Write(respCommand("EMB", "test", "beta"))
	seqB := respBulk(t, readRESP(t, c))
	c.Close()

	if multi[0] != seqA {
		t.Fatalf("windowed MULTI alpha != sequential EMB alpha")
	}
	if multi[1] != seqB {
		t.Fatalf("windowed MULTI beta != sequential EMB beta")
	}
}

func TestAUTHNoPassword(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$5\r\nhello\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "no password is set") {
		t.Fatalf("expected no password error, got %q", resp)
	}
	c.Close()
}

func TestAUTHWrongPassword(t *testing.T) {
	addr := serveTestWithAuth(t, "secret123")
	c := dial(t, addr)
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$6\r\nwrong!\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "invalid password") {
		t.Fatalf("expected invalid password error, got %q", resp)
	}
	c.Close()
}

func TestAUTHCorrectPassword(t *testing.T) {
	addr := serveTestWithAuth(t, "secret123")
	c := dial(t, addr)
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$9\r\nsecret123\r\n"))
	resp := readRESP(t, c)
	if resp != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", resp)
	}
	c.Close()
}

func TestCommandBeforeAuth(t *testing.T) {
	addr := serveTestWithAuth(t, "secret123")
	c := dial(t, addr)
	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "NOAUTH") {
		t.Fatalf("expected NOAUTH error, got %q", resp)
	}
	c.Close()
}

func TestPINGBeforeAuth(t *testing.T) {
	addr := serveTestWithAuth(t, "secret123")
	c := dial(t, addr)
	c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	resp := readRESP(t, c)
	if resp != "+PONG\r\n" {
		t.Fatalf("expected +PONG, got %q", resp)
	}
	c.Close()
}

func TestAUTHDouble(t *testing.T) {
	addr := serveTestWithAuth(t, "secret123")
	c := dial(t, addr)
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$9\r\nsecret123\r\n"))
	resp1 := readRESP(t, c)
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$9\r\nsecret123\r\n"))
	resp2 := readRESP(t, c)
	if resp1 != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", resp1)
	}
	if resp2 != "+OK\r\n" {
		t.Fatalf("expected +OK on second AUTH, got %q", resp2)
	}
	c.Close()
}

func TestCommandsWorkAfterAuth(t *testing.T) {
	addr := serveTestWithAuth(t, "secret123")
	c := dial(t, addr)
	c.Write([]byte("*2\r\n$4\r\nAUTH\r\n$9\r\nsecret123\r\n"))
	readRESP(t, c)
	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	resp := readRESP(t, c)
	if len(resp) < 3 || resp[0] != '$' {
		t.Fatalf("expected bulk string after auth, got %q", resp)
	}
	c.Close()
}

func serveTestWithAuth(t *testing.T, password string) string {
	t.Helper()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	addr := getFreeAddr()
	srv := New(addr, reg, password, "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr
}

func serveTest(t *testing.T) string {
	return serveTestWithAuth(t, "")
}

func serveTestWithServer(t *testing.T) (string, *Server) {
	t.Helper()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

// serveWithPool starts a server whose model "test" is backed by a pool built from the
// given session factory. numWorkers is the pool worker count (a value >1 here means the
// only bound on concurrent inference should come from the server's fan-out).
func serveWithPool(t *testing.T, factory func() (onnx.Session, error), numWorkers, timeoutMS, maxBatch int, opts ...Option) (string, *Server) {
	t.Helper()
	reg := registry.New()
	pool, err := pipeline.NewPool(factory, mockTokenizer{}, numWorkers, 4, 128, true, "mean", timeoutMS, maxBatch, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil, opts...)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

func serveTestWithOptions(t *testing.T, opts ...Option) (string, *Server) {
	t.Helper()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil, opts...)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

func serveTestEmpty(t *testing.T) string {
	t.Helper()
	reg := registry.New()

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr
}

func TestREADYWhenReady(t *testing.T) {
	addr, srv := serveTestWithServer(t)
	srv.SetReady()
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$9\r\nEMB.READY\r\n"))
	resp := readRESP(t, c)
	if resp != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", resp)
	}
	c.Close()
}

func TestREADYWhenLoading(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$9\r\nEMB.READY\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "loading") {
		t.Fatalf("expected loading error, got %q", resp)
	}
	c.Close()
}

func TestREADYDraining(t *testing.T) {
	addr, srv := serveTestWithServer(t)
	srv.SetDraining()
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$9\r\nEMB.READY\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "draining") {
		t.Fatalf("expected draining error, got %q", resp)
	}
	c.Close()
}

func TestREADYNoModels(t *testing.T) {
	addr := serveTestEmpty(t)
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$9\r\nEMB.READY\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "no models") {
		t.Fatalf("expected no models error, got %q", resp)
	}
	c.Close()
}

func serveTestWithCache(t *testing.T, cacheConfig string) string {
	t.Helper()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheConfig, nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr
}

func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func readRESP(t *testing.T, c net.Conn) string {
	t.Helper()
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf[:n])
}

func getFreeAddr() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	l.Close()
	return l.Addr().String()
}

func TestCacheGetSet(t *testing.T) {
	c := NewCache(1024 * 1024)
	key := "test:hello"
	val := []byte{1, 2, 3, 4}
	c.Set(key, val)
	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 4 || got[0] != 1 {
		t.Fatal("wrong cached value")
	}
	_, ok = c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestCacheEviction(t *testing.T) {
	c := NewCache(200)
	for i := range 20 {
		key := fmt.Sprintf("k:%d", i)
		val := []byte{byte(i)}
		c.Set(key, val)
	}
	st := c.Stats()
	if st.Evictions == 0 {
		t.Fatalf("expected evictions, got %d", st.Evictions)
	}
	if st.Entries > 15 {
		t.Fatalf("expected <=15 entries after eviction, got %d", st.Entries)
	}
}

func TestCacheHitCounts(t *testing.T) {
	c := NewCache(1024 * 1024)
	c.Set("m:hello", []byte{1})
	c.Set("m:world", []byte{2})
	c.Get("m:hello")
	c.Get("m:hello")
	c.Get("m:world")
	c.Get("m:nope")
	st := c.Stats()
	if st.Hits != 3 {
		t.Fatalf("expected 3 hits, got %d", st.Hits)
	}
	if st.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", st.Misses)
	}
}

func TestCachePartialHit(t *testing.T) {
	addr := serveTestWithCache(t, "1GB")
	c := dial(t, addr)

	c.Write([]byte("*4\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$1\r\na\r\n$1\r\nb\r\n"))
	resp1 := readRESP(t, c)
	if resp1[0] != '*' {
		t.Fatalf("expected array, got %q", resp1)
	}

	c.Write([]byte("*4\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$1\r\na\r\n$1\r\nc\r\n"))
	resp2 := readRESP(t, c)
	if resp2[0] != '*' {
		t.Fatalf("expected array, got %q", resp2)
	}

	c.Close()
}

func TestCacheOnINFO(t *testing.T) {
	addr := serveTestWithCache(t, "1GB")
	c := dial(t, addr)

	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	readRESP(t, c)

	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	readRESP(t, c)

	c.Write([]byte("*2\r\n$8\r\nEMB.INFO\r\n$4\r\ntest\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "cache_hits") {
		t.Fatalf("expected cache stats in INFO, got %q", resp)
	}

	c.Close()
}

func TestCacheDisabled(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	resp := readRESP(t, c)
	if resp[0] != '$' {
		t.Fatalf("expected bulk string, got %q", resp)
	}
	c.Close()
}

func parseRESPArrayCount(resp string) (declared, actual int) {
	if len(resp) == 0 || resp[0] != '*' {
		return -1, -1
	}

	i := 1
	for i < len(resp) && resp[i] >= '0' && resp[i] <= '9' {
		i++
	}
	declared, _ = strconv.Atoi(resp[1:i])

	if i+1 >= len(resp) || resp[i:i+2] != "\r\n" {
		return -1, -1
	}
	i += 2

	for i < len(resp) {
		switch {
		case resp[i] == '$':
			i++
			j := i
			for j < len(resp) && resp[j] >= '0' && resp[j] <= '9' {
				j++
			}
			strlen, _ := strconv.Atoi(resp[i:j])
			i = j + 2 + strlen + 2
			actual++

		case resp[i] == ':':
			i++
			for i < len(resp) && resp[i] != '\r' {
				i++
			}
			i += 2
			actual++

		default:
			return declared, actual
		}
	}

	return declared, actual
}

func TestServerINFOArrayCount(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)

	c.Write([]byte("*2\r\n$8\r\nEMB.INFO\r\n$4\r\ntest\r\n"))
	resp := readRESP(t, c)

	declared, actual := parseRESPArrayCount(resp)
	if declared != 30 {
		t.Fatalf("expected 30 declared elements, got %d: %q", declared, resp)
	}
	if actual != 30 {
		t.Fatalf("expected 30 actual elements, got %d: %q", actual, resp)
	}

	c.Close()
}

func TestCacheInfoArrayCount(t *testing.T) {
	addr := serveTestWithCache(t, "1GB")
	c := dial(t, addr)

	c.Write([]byte("*2\r\n$8\r\nEMB.INFO\r\n$4\r\ntest\r\n"))
	resp := readRESP(t, c)

	declared, actual := parseRESPArrayCount(resp)
	if declared != 44 {
		t.Fatalf("expected 44 declared elements, got %d: %q", declared, resp)
	}
	if actual != 44 {
		t.Fatalf("expected 44 actual elements, got %d: %q", actual, resp)
	}

	c.Close()
}

func TestTLSAcceptsPlainTCP(t *testing.T) {
	addr := serveTest(t)
	c := dial(t, addr)
	c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	resp := readRESP(t, c)
	if resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}
	c.Close()
}

func TestTLSEmptyConfigNew(t *testing.T) {
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)

	c := dial(t, addr)
	c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	resp := readRESP(t, c)
	if resp != "+PONG\r\n" {
		t.Fatalf("expected PONG with nil tlsConfig, got %q", resp)
	}
	c.Close()
}

func TestTLSConnection(t *testing.T) {
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})

	cert := generateTestCert(t)
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	addr := getFreeAddr()
	srv := New(addr, reg, "", "", tlsCfg)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("TLS dial failed: %v", err)
	}
	defer conn.Close()

	conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	resp := string(buf[:n])
	if resp != "+PONG\r\n" {
		t.Fatalf("expected +PONG, got %q", resp)
	}
}

func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
		Leaf:        cert,
	}
}

func TestParseCacheConfig(t *testing.T) {
	bytes, err := parseCacheConfig("")
	if err != nil || bytes != 0 {
		t.Fatalf("expected 0, got %d", bytes)
	}
	bytes, err = parseCacheConfig("512MB")
	if err != nil || bytes != 512000000 {
		t.Fatalf("expected 512000000, got %d", bytes)
	}
	_, err = parseCacheConfig("invalid")
	if err == nil {
		t.Fatal("expected error for invalid size")
	}
	bytes, err = parseCacheConfig("auto")
	if err != nil || bytes == 0 {
		t.Fatalf("expected positive auto-tune value, got %d", bytes)
	}
}

func TestAutoTuneCache(t *testing.T) {
	mem := registry.TotalSystemMemory()
	budget := autoTuneCache()

	if budget < 64*1024*1024 {
		t.Fatalf("auto budget %d below the 64MB floor", budget)
	}
	if budget > int64(mem/2) {
		t.Fatalf("auto budget %d above the mem/2 ceiling (mem %d)", budget, mem)
	}

	if mem >= 4*1024*1024*1024 {
		if budget <= 500*1024*1024 {
			t.Fatalf("auto budget %d did not exceed the old 500MB cap on a >=4GB machine", budget)
		}
	} else {
		t.Logf("skipping >500MB check: total mem %d < 4GB", mem)
	}
}

func TestParseCacheConfigPercent(t *testing.T) {
	mem := registry.TotalSystemMemory()

	for _, tt := range []struct {
		in   string
		want int64
		err  bool
	}{
		{in: "10%", want: int64(0.10 * float64(mem))},
		{in: "25%", want: int64(0.25 * float64(mem))},
		{in: "100%", want: int64(1.0 * float64(mem))},
		{in: "0%", err: true},
		{in: "-5%", err: true},
		{in: "150%", err: true},
		{in: "abc%", err: true},
		{in: "%", err: true},
	} {
		got, err := parseCacheConfig(tt.in)
		if tt.err {
			if err == nil {
				t.Errorf("%q: expected error, got %d", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%q: got %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestPercentCacheBudgetZeroMem(t *testing.T) {
	if _, err := percentCacheBudget(10, 0); err == nil {
		t.Fatal("expected error when system memory is unavailable")
	}
	got, err := percentCacheBudget(10, 8*1024*1024*1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mem8 := 8 * 1024 * 1024 * 1024
	if want := int64(0.10 * float64(mem8)); got != want {
		t.Fatalf("10%% of 8GB: got %d, want %d", got, want)
	}
}

func BenchmarkRESP(b *testing.B) {
	addr := getFreeAddr()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		b.Fatal(err)
	}
	reg.Add("test", &registry.ModelEntry{Pool: pool, Dim: 4, Name: "test"})
	srv := New(addr, reg, "", "", nil)
	go srv.ListenAndServe()
	b.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)

	cmd := []byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	buf := make([]byte, 4096)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Write(cmd)
		conn.Read(buf)
	}
}

func BenchmarkPoolEmbed(b *testing.B) {
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		4, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := pool.Embed([]string{"hello world"})
			if err != nil {
				b.Fatal(err)
			}
			if resp.Err != nil {
				b.Fatal(resp.Err)
			}
		}
	})
}

// statsIntField issues EMB.STATS on the given connection and returns the value
// of one integer field, so connection/active counts can be read without
// opening extra connections that would disturb the count.
func statsIntField(t *testing.T, c net.Conn, field string) int {
	t.Helper()
	c.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
	resp := readRESP(t, c)
	idx := strings.Index(resp, field)
	if idx < 0 {
		t.Fatalf("field %q not found in stats: %q", field, resp)
	}
	rest := resp[idx+len(field):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		t.Fatalf("no int value after %q in stats: %q", field, resp)
	}
	j := colon + 1
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	v, err := strconv.Atoi(rest[colon+1 : j])
	if err != nil {
		t.Fatalf("bad int value for %q in stats: %q", field, resp)
	}
	return v
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestConnAccounting(t *testing.T) {
	addr, _ := serveTestWithOptions(t)
	c1 := dial(t, addr)
	c2 := dial(t, addr)

	if got := statsIntField(t, c1, "connections"); got != 2 {
		t.Fatalf("expected 2 connections, got %d", got)
	}

	c2.Close()
	waitFor(t, func() bool {
		return statsIntField(t, c1, "connections") == 1
	})
}

func TestIdleClose(t *testing.T) {
	addr, _ := serveTestWithOptions(t, WithIdleTimeout(200*time.Millisecond))
	c := dial(t, addr)

	c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if resp := readRESP(t, c); resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}

	// Go idle: the server should reap us.
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	_, err := c.Read(buf)
	if err == nil {
		t.Fatalf("expected connection closed after idle timeout")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("connection was not reaped within 3s")
	}
}

func TestIdleCloseActiveSurvives(t *testing.T) {
	addr, _ := serveTestWithOptions(t, WithIdleTimeout(300*time.Millisecond))
	c := dial(t, addr)

	for i := range 4 {
		c.Write([]byte("*1\r\n$4\r\nPING\r\n"))
		if resp := readRESP(t, c); resp != "+PONG\r\n" {
			t.Fatalf("iteration %d: expected PONG, got %q", i, resp)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func TestMaxConnections(t *testing.T) {
	addr, _ := serveTestWithOptions(t, WithMaxConnections(1))
	c1 := dial(t, addr)

	c1.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if resp := readRESP(t, c1); resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}

	// Second connection at the cap is refused and closed.
	c2 := dial(t, addr)
	c2.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := c2.Read(buf); err == nil {
		t.Fatalf("expected refused connection to be closed, got data")
	}

	// First connection is unaffected, and only it is counted.
	c1.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if resp := readRESP(t, c1); resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}
	if got := statsIntField(t, c1, "connections"); got != 1 {
		t.Fatalf("expected 1 connection (refused socket uncounted), got %d", got)
	}

	// Slots free up after close.
	c1.Close()
	waitFor(t, func() bool {
		c3, err := net.Dial("tcp", addr)
		if err != nil {
			return false
		}
		defer c3.Close()
		c3.Write([]byte("*1\r\n$4\r\nPING\r\n"))
		c3.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := c3.Read(buf)
		return err == nil && strings.Contains(string(buf[:n]), "PONG")
	})
}

// blockingSession blocks in Run until released, so tests can hold a request
// in flight and observe the concurrency gate.
type blockingSession struct {
	gate chan struct{}
}

func (b *blockingSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	<-b.gate
	data := make([]float32, batchSize*seqLen*dim)
	for i := range data {
		data[i] = float32(i % dim)
	}
	return data, nil
}

func (b *blockingSession) Close() error { return nil }

func TestMaxConcurrentRequests(t *testing.T) {
	gate := make(chan struct{})
	addr, _ := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate}, nil },
		2, 0, 32,
		WithMaxConcurrentRequests(1),
	)
	c1 := dial(t, addr)
	c2 := dial(t, addr)

	// c1 occupies the single in-flight slot.
	c1.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nhello\r\n"))
	waitFor(t, func() bool {
		return statsIntField(t, c2, "active_requests") == 1
	})

	// Second EMB request is busy-errored while c1 is in flight.
	c2.Write([]byte("*3\r\n$3\r\nEMB\r\n$4\r\ntest\r\n$5\r\nworld\r\n"))
	resp := readRESP(t, c2)
	if !strings.HasPrefix(resp, "-ERR busy") {
		t.Fatalf("expected busy error, got %q", resp)
	}

	// Control commands still answer during saturation.
	c2.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if resp := readRESP(t, c2); resp != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", resp)
	}
	if got := statsIntField(t, c2, "active_requests"); got != 1 {
		t.Fatalf("expected active_requests 1, got %d", got)
	}

	// Release: c1 completes normally.
	close(gate)
	done := readRESP(t, c1)
	if len(done) < 3 || done[0] != '$' {
		t.Fatalf("expected bulk string for in-flight request, got %q", done)
	}
}

func TestStatsRESPParity(t *testing.T) {
	cacheCases := map[string]string{"no-cache": "", "cache": "1GB"}
	for name, cacheCfg := range cacheCases {
		t.Run(name, func(t *testing.T) {
			addr := serveTestWithCache(t, cacheCfg)
			c := dial(t, addr)
			c.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
			resp := readRESP(t, c)

			declared, actual := parseRESPArrayCount(resp)
			if declared != 28 {
				t.Fatalf("expected 28 declared elements, got %d: %q", declared, resp)
			}
			if actual != 28 {
				t.Fatalf("expected 28 actual elements, got %d: %q", actual, resp)
			}

			for _, f := range []string{
				"uptime_secs", "total_requests", "active_requests", "total_tokens",
				"total_errors", "models_loaded", "per_model", "connections",
				"idle_timeout_ms", "max_connections", "max_concurrent_requests",
				"cache_hits", "cache_misses", "cache_evictions",
			} {
				if !strings.Contains(resp, f) {
					t.Fatalf("missing field %q in stats: %q", f, resp)
				}
			}
			if !strings.Contains(resp, "active_requests\r\n:0") {
				t.Fatalf("expected active_requests 0 when idle: %q", resp)
			}
			c.Close()
		})
	}
}

func TestStatsPolicyEcho(t *testing.T) {
	addr, _ := serveTestWithOptions(t,
		WithIdleTimeout(5*time.Minute),
		WithMaxConnections(10),
		WithMaxConcurrentRequests(4),
	)
	c := dial(t, addr)
	c.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
	resp := readRESP(t, c)

	for _, want := range []string{
		"idle_timeout_ms\r\n:300000",
		"max_connections\r\n:10",
		"max_concurrent_requests\r\n:4",
		"connections\r\n:1",
		"active_requests\r\n:0",
	} {
		if !strings.Contains(resp, want) {
			t.Fatalf("expected %q in stats: %q", want, resp)
		}
	}
	c.Close()
}

func TestStatsDefaultIdleTimeout(t *testing.T) {
	// A server built without an idle-timeout option reports the default TTL.
	addr, _ := serveTestWithOptions(t)
	c := dial(t, addr)
	c.Write([]byte("*1\r\n$9\r\nEMB.STATS\r\n"))
	resp := readRESP(t, c)
	if !strings.Contains(resp, "idle_timeout_ms\r\n:900000") {
		t.Fatalf("expected default idle_timeout_ms 900000 in stats: %q", resp)
	}
	c.Close()
}
