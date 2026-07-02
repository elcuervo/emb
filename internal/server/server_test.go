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
		2, 4, 128, true, "mean", 0, 32,
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
		2, 4, 128, true, "mean", 0, 32,
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
		2, 4, 128, true, "mean", 0, 32,
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
	if declared != 22 {
		t.Fatalf("expected 22 declared elements, got %d: %q", declared, resp)
	}
	if actual != 22 {
		t.Fatalf("expected 22 actual elements, got %d: %q", actual, resp)
	}

	c.Close()
}

func TestCacheInfoArrayCount(t *testing.T) {
	addr := serveTestWithCache(t, "1GB")
	c := dial(t, addr)

	c.Write([]byte("*2\r\n$8\r\nEMB.INFO\r\n$4\r\ntest\r\n"))
	resp := readRESP(t, c)

	declared, actual := parseRESPArrayCount(resp)
	if declared != 36 {
		t.Fatalf("expected 36 declared elements, got %d: %q", declared, resp)
	}
	if actual != 36 {
		t.Fatalf("expected 36 actual elements, got %d: %q", actual, resp)
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
		2, 4, 128, true, "mean", 0, 32,
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
		2, 4, 128, true, "mean", 0, 32,
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

func BenchmarkRESP(b *testing.B) {
	addr := getFreeAddr()
	reg := registry.New()
	pool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &mockSession{}, nil },
		mockTokenizer{},
		2, 4, 128, true, "mean", 0, 32,
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
		4, 4, 128, true, "mean", 0, 32,
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
