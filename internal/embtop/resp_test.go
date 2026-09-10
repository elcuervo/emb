package embtop

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer is a canned RESP server for client tests. It reads all
// expected commands before answering (proving clients pipeline), either
// from a fixed reply list or from a per-command responder.
type fakeServer struct {
	ln net.Listener

	mu      sync.Mutex
	got     [][]string
	replies [][]byte
	respond func(cmd []string, n int) []byte
	closed  bool
}

func startFake(t *testing.T, replies ...[]byte) (*fakeServer, string) {
	t.Helper()
	s, addr := startScripted(t, nil, replies...)
	return s, addr
}

// startScripted starts a fake server whose replies come from respond (called
// with the command and its index in the connection); replies are used when
// respond is nil.
func startScripted(t *testing.T, respond func(cmd []string, n int) []byte, replies ...[]byte) (*fakeServer, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeServer{ln: ln, replies: replies, respond: respond}
	go s.serve()
	t.Cleanup(func() { s.mu.Lock(); s.closed = true; s.mu.Unlock(); _ = ln.Close() })
	return s, ln.Addr().String()
}

func (s *fakeServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n := len(s.replies) // commands to collect before replying
	if s.respond != nil {
		n = 1 << 30 // scripted: reply per command
	}
	idx := 0
	for i := 0; i < n; i++ {
		cmd, err := readRESPCommand(r)
		if err != nil {
			s.mu.Lock()
			defer s.mu.Unlock()
			return
		}
		s.mu.Lock()
		s.got = append(s.got, cmd)
		s.mu.Unlock()
		var reply []byte
		if s.respond != nil {
			reply = s.respond(cmd, idx)
		} else {
			reply = s.replies[i]
		}
		if _, err := w.Write(reply); err != nil {
			return
		}
		_ = w.Flush()
		idx++
	}
}

// readRESPCommand reads one RESP array-of-bulk-strings command.
func readRESPCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("expected array, got %q", line)
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(hdr, "$") {
			return nil, fmt.Errorf("expected bulk, got %q", hdr)
		}
		l, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(hdr, "$")))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, l)
		if _, err := ioReadFull(r, buf); err != nil {
			return nil, err
		}
		if _, err := r.Discard(2); err != nil {
			return nil, err
		}
		args = append(args, string(buf))
	}
	return args, nil
}

func (s *fakeServer) commands() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]string(nil), s.got...)
}

// ---- RESP encoders for the fake server ----

func encodeOK() []byte         { return []byte("+OK\r\n") }
func encodeInt(n int64) []byte { return []byte(":" + strconv.FormatInt(n, 10) + "\r\n") }
func encodeNil() []byte        { return []byte("$-1\r\n") }

func encodeBulk(s string) []byte {
	if s == "" {
		return []byte("$0\r\n\r\n")
	}
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s))
}

func encodeArray(parts ...[]byte) []byte {
	var b strings.Builder
	b.WriteString("*" + strconv.Itoa(len(parts)) + "\r\n")
	for _, p := range parts {
		b.Write(p)
	}
	return []byte(b.String())
}

func encodeError(msg string) []byte { return []byte("-" + msg + "\r\n") }

// statsReply builds a canned EMB.STATS reply from a map.
func statsReply(fields map[string]int64) []byte {
	parts := make([][]byte, 0, len(fields)*2)
	keys := []string{
		"uptime_secs", "total_requests", "total_tokens", "total_errors",
		"active_requests", "connections", "models_loaded", "mem",
		"cpu_user_usec", "cpu_sys_usec", "goroutines",
		"cache_hits", "cache_misses", "cache_evictions",
		"truncated_texts", "truncated_pairs",
	}
	for _, k := range keys {
		parts = append(parts, encodeBulk(k), encodeInt(fields[k]))
	}
	return encodeArray(parts...)
}

// infoReply builds a canned EMB.INFO <model> reply.
func infoReply(overrides map[string]string, ints map[string]int64) []byte {
	strs := map[string]string{
		"pooling": "mean", "normalize": "true", "quantization": "none",
		"padding_efficiency": "1.0000",
	}
	for k, v := range overrides {
		strs[k] = v
	}
	allInts := map[string]int64{
		"dim": 384, "max_length": 512, "workers": 2,
		"requests": 0, "avg_latency_us": 0, "tokens": 0, "errors": 0,
		"model_bytes": 0, "batching_timeout_ms": 0,
		"batching_max_batch": 0, "batching_max_tokens": 0,
	}
	for k, v := range ints {
		allInts[k] = v
	}
	var parts [][]byte
	for k, v := range allInts {
		parts = append(parts, encodeBulk(k), encodeInt(v))
	}
	for k, v := range strs {
		parts = append(parts, encodeBulk(k), encodeBulk(v))
	}
	return encodeArray(parts...)
}

func modelsReply(models ...[]string) []byte {
	rows := make([][]byte, 0, len(models))
	for _, m := range models {
		rows = append(rows, encodeArray(encodeBulk(m[0]), encodeInt(parseDim(m[1])), encodeBulk("ready")))
	}
	return encodeArray(rows...)
}

func parseDim(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func TestClientAuth(t *testing.T) {
	s, addr := startFake(t, encodeOK())
	c := NewClient(addr, "secret", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	cmds := s.commands()
	if len(cmds) != 1 || cmds[0][0] != "AUTH" || cmds[0][1] != "secret" {
		t.Fatalf("expected AUTH secret, got %v", cmds)
	}
}

func TestClientAuthRejected(t *testing.T) {
	_, addr := startFake(t, encodeError("ERR invalid password"))
	c := NewClient(addr, "nope", false)
	if err := c.Dial(); err == nil {
		t.Fatal("expected dial error on bad auth")
	} else if !strings.Contains(err.Error(), "invalid password") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientPollPipelinesInOneWrite(t *testing.T) {
	models := modelsReply([]string{"a", "384"})
	info := infoReply(nil, map[string]int64{"requests": 12, "tokens": 300, "errors": 1})
	stats := statsReply(map[string]int64{
		"uptime_secs": 42, "total_requests": 12, "total_tokens": 300,
		"total_errors": 1, "active_requests": 2, "connections": 3,
		"models_loaded": 1, "mem": 512, "cpu_user_usec": 1000,
		"cpu_sys_usec": 500, "goroutines": 17,
		"cache_hits": 9, "cache_misses": 1, "cache_evictions": 0,
	})
	s, addr := startFake(t, models, info, stats, encodeArray())

	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	res, err := c.Poll([]string{"a"}, 0)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}

	cmds := s.commands()
	got := make([]string, 0, 7)
	for _, cmd := range cmds {
		for _, a := range cmd {
			got = append(got, strings.ToUpper(a))
		}
	}
	want := []string{"EMB.MODELS", "EMB.INFO", "A", "EMB.STATS", "MONITOR", "0", "512"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("pipeline mismatch: got %v want %v", got, want)
	}

	if res.UptimeSecs != 42 || res.TotalRequests != 12 || res.TotalTokens != 300 {
		t.Fatalf("bad stats parse: %+v", res)
	}
	if res.ActiveRequests != 2 || res.Connections != 3 || res.MemMB != 512 {
		t.Fatalf("bad gauges parse: %+v", res)
	}
	if res.CacheHits != 9 || res.CacheMisses != 1 {
		t.Fatalf("bad cache parse: %+v", res)
	}
	if len(res.Models) != 1 || res.Models[0].Name != "a" || res.Models[0].Dim != 384 {
		t.Fatalf("bad models parse: %+v", res.Models)
	}
	ms, ok := res.PerModel["a"]
	if !ok {
		t.Fatal("missing per-model stats")
	}
	if ms.Requests != 12 || ms.Tokens != 300 || ms.Errors != 1 {
		t.Fatalf("bad per-model parse: %+v", ms)
	}
	if ms.Pooling != "mean" || !ms.Normalize || ms.Quantization != "none" {
		t.Fatalf("bad per-model metadata: %+v", ms)
	}
}

func TestClientPollEmptyModels(t *testing.T) {
	models := modelsReply()
	stats := statsReply(map[string]int64{"uptime_secs": 1, "total_requests": 0})
	s, addr := startFake(t, models, stats, encodeArray())
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	res, err := c.Poll(nil, 0)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(res.Models) != 0 || len(res.PerModel) != 0 {
		t.Fatalf("expected no models, got %+v", res)
	}
	if cmds := s.commands(); len(cmds) != 3 ||
		len(cmds[0]) != 1 || cmds[0][0] != "EMB.MODELS" ||
		len(cmds[1]) != 1 || cmds[1][0] != "EMB.STATS" ||
		len(cmds[2]) != 3 || cmds[2][0] != "MONITOR" {
		t.Fatalf("unexpected pipeline: %v", cmds)
	}
}

func TestClientPollErrorReply(t *testing.T) {
	_, addr := startFake(t, encodeError("ERR something"))
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if _, err := c.Poll(nil, 0); err == nil {
		t.Fatal("expected poll error")
	}
}

func TestSanitizeStripsControlCharacters(t *testing.T) {
	// Server-provided strings must not be able to inject terminal escapes.
	got := sanitize("mini\x1b[31mlm\x07\x00")
	if got != "mini[31mlm" {
		t.Fatalf("sanitize = %q, want %q", got, "mini[31mlm")
	}
	// Printable Unicode (accents, CJK) is preserved.
	if got := sanitize("modèle-模型"); got != "modèle-模型" {
		t.Fatalf("sanitize mangled unicode: %q", got)
	}
}

func TestClientParsesSanitizedModelNames(t *testing.T) {
	models := modelsReply([]string{"evil\x1b[2Jname", "384"})
	stats := statsReply(map[string]int64{"uptime_secs": 1})
	_, addr := startFake(t, models, stats, encodeArray())
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	res, err := c.Poll(nil, 0)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(res.Models) != 1 || strings.ContainsRune(res.Models[0].Name, 0x1b) {
		t.Fatalf("model name not sanitized: %+v", res.Models)
	}
}

func TestReadReplyBoundsOversizedBulk(t *testing.T) {
	// A peer must not be able to make us allocate an arbitrary buffer.
	s, addr := startScripted(t, func(cmd []string, n int) []byte {
		return []byte("$99999999999\r\n")
	})
	_ = s
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if _, err := c.Poll(nil, 0); err == nil {
		t.Fatal("expected oversized bulk to be rejected")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadReplyBoundsDeepNesting(t *testing.T) {
	deep := strings.Repeat("*1\r\n", maxDepth+2) + ":1\r\n"
	_, addr := startScripted(t, func(cmd []string, n int) []byte { return []byte(deep) })
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if _, err := c.Poll(nil, 0); err == nil {
		t.Fatal("expected over-deep reply to be rejected")
	}
}

func TestEnsureConnReportsDial(t *testing.T) {
	_, addr := startScripted(t, func(cmd []string, n int) []byte { return encodeArray() })
	c := NewClient(addr, "", false)
	defer c.Close()

	dialed, err := c.EnsureConn()
	if err != nil || !dialed {
		t.Fatalf("first EnsureConn: dialed=%v err=%v (want true, nil)", dialed, err)
	}
	dialed, err = c.EnsureConn()
	if err != nil || dialed {
		t.Fatalf("second EnsureConn: dialed=%v err=%v (want false, nil)", dialed, err)
	}
}
