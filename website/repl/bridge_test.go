package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeEmb is a minimal RESP listener: enough of emb's surface for the bridge's
// contract tests, with every received argv recorded so a test can assert what
// was actually forwarded.
type fakeEmb struct {
	ln     net.Listener
	wg     sync.WaitGroup
	silent bool
	delay  time.Duration

	mu    sync.Mutex
	seen  [][]string
	conns []net.Conn
}

func startFakeEmb(t *testing.T) *fakeEmb {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeEmb{ln: ln}
	f.wg.Add(1)
	go f.accept()
	t.Cleanup(func() {
		_ = ln.Close()
		f.mu.Lock()
		for _, c := range f.conns {
			_ = c.Close()
		}
		f.mu.Unlock()
		f.wg.Wait()
	})
	return f
}

func (f *fakeEmb) addr() string { return f.ln.Addr().String() }

// killConns drops every accepted connection, standing in for a lost upstream.
func (f *fakeEmb) killConns() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		_ = c.Close()
	}
	f.conns = nil
}

func (f *fakeEmb) received() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.seen))
	copy(out, f.seen)
	return out
}

func (f *fakeEmb) accept() {
	defer f.wg.Done()
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns = append(f.conns, conn)
		f.mu.Unlock()
		f.wg.Add(1)
		go f.serve(conn)
	}
}

func (f *fakeEmb) serve(conn net.Conn) {
	defer f.wg.Done()
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		args, err := readCommand(r)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.seen = append(f.seen, args)
		f.mu.Unlock()
		if f.silent {
			continue
		}
		if f.delay > 0 {
			time.Sleep(f.delay)
		}
		if _, err := conn.Write(f.reply(args)); err != nil {
			return
		}
	}
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '*' {
		return nil, fmt.Errorf("bad array header %q", line)
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		hdr = strings.TrimRight(hdr, "\r\n")
		if len(hdr) == 0 || hdr[0] != '$' {
			return nil, fmt.Errorf("bad bulk header %q", hdr)
		}
		size, err := strconv.Atoi(hdr[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		out = append(out, string(buf[:size]))
	}
	return out, nil
}

// blob is the payload the fake returns for a BLOB embedding: 384 float32s
// whose bytes are deliberately not valid UTF-8, so a lossy text encoding would
// corrupt them.
func blob() []byte {
	b := make([]byte, 384*4)
	for i := range b {
		b[i] = byte(i*7 + 0x80)
	}
	return b
}

func (f *fakeEmb) reply(args []string) []byte {
	name := strings.ToLower(args[0])
	switch name {
	case "hello":
		if len(args) > 1 && args[1] == "3" {
			return []byte("%4\r\n$6\r\nserver\r\n$5\r\nredis\r\n$5\r\nproto\r\n:3\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n")
		}
		return []byte("*2\r\n$5\r\nproto\r\n:2\r\n")
	case "ping":
		return []byte("+PONG\r\n")
	case "emb":
		if len(args) > 1 && args[1] == "nosuch" {
			return []byte("-ERR model 'nosuch' not found\r\n")
		}
		if len(args) > 2 && strings.EqualFold(args[2], "values") {
			return []byte("*6\r\n$5\r\ndtype\r\n$5\r\nFLOAT\r\n$5\r\nshape\r\n*2\r\n:1\r\n:2\r\n$6\r\nvalues\r\n*2\r\n,0.5\r\n,0.25\r\n")
		}
		return bulkReply(blob())
	case "emb.multi":
		return append([]byte("*2\r\n"), append(bulkReply(blob()), bulkReply(blob())...)...)
	case "emb.evsha":
		return []byte("%3\r\n$5\r\nlabel\r\n$8\r\nPOSITIVE\r\n$10\r\nconfidence\r\n,0.99\r\n$6\r\nscores\r\n*2\r\n,0.01\r\n,0.99\r\n")
	case "emb.models":
		return []byte("*2\r\n$6\r\nminilm\r\n:384\r\n")
	case "info":
		return []byte("$11\r\n# Server\r\nX\r\n")
	default:
		return []byte("-ERR unknown command '" + args[0] + "'\r\n")
	}
}

func bulkReply(payload []byte) []byte {
	out := []byte("$" + strconv.Itoa(len(payload)) + "\r\n")
	out = append(out, payload...)
	return append(out, '\r', '\n')
}

func testLimits() Limits {
	l := DefaultLimits()
	l.PerClientRate = 1000
	l.PerClientBurst = 1000
	l.GlobalRate = 1000
	l.GlobalBurst = 1000
	l.WorkCeiling = 1_000_000
	l.Timeout = 2 * time.Second
	return l
}

func newTestBridge(t *testing.T, upstream string, p presets, l Limits, origins ...string) *Bridge {
	t.Helper()
	return NewBridge(upstream, p, l, origins, false)
}

func mustExec(t *testing.T, b *Bridge, args []string, proto int) (Envelope, int) {
	t.Helper()
	return b.Execute(args, proto, "test-client")
}

func TestClientKeyIgnoresForwardedForUnlessTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/exec", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	if got := (&Bridge{}).clientKey(req); got != "203.0.113.7" {
		t.Fatalf("untrusted clientKey = %q, want the peer address", got)
	}
	if got := (&Bridge{trustProxy: true}).clientKey(req); got != "10.0.0.1" {
		t.Fatalf("trusted clientKey = %q, want the forwarded address", got)
	}

	req.Header.Set("Fly-Client-IP", "198.51.100.9")
	if got := (&Bridge{}).clientKey(req); got != "198.51.100.9" {
		t.Fatalf("Fly clientKey = %q, want the Fly-Client-IP", got)
	}
}

// --- 3.1 health and readiness ---------------------------------------------

func TestHealthReportsOK(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %+v, want status ok", body)
	}
}

func TestReadyFalseUntilServerAnswers(t *testing.T) {
	// Grab a port, then close it: ready must be false while nothing listens.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	b := newTestBridge(t, addr, presets{}, testLimits())
	if got := readyState(t, b); got != "starting" {
		t.Fatalf("state before the server exists = %q, want starting", got)
	}

	// Now start the fake and the same probe must flip to ready.
	f := startFakeEmb(t)
	b2 := newTestBridge(t, f.addr(), presets{}, testLimits())
	if got := readyState(t, b2); got != "ready" {
		t.Fatalf("state after the server answers = %q, want ready", got)
	}
}

func readyState(t *testing.T, b *Bridge) string {
	t.Helper()
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/ready")
	if err != nil {
		t.Fatalf("get ready: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Ready bool   `json:"ready"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready: %v", err)
	}
	if body.Ready != (body.State == "ready") {
		t.Fatalf("ready=%v state=%q disagree", body.Ready, body.State)
	}
	return body.State
}

// --- 3.2 upstream client ---------------------------------------------------

func TestExecReportsStartingThenSucceeds(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	b := newTestBridge(t, addr, presets{}, testLimits())
	env, status := mustExec(t, b, []string{"PING"}, 2)
	if env.Kind != kindError || env.Code != codeStarting {
		t.Fatalf("before the server exists: %+v (status %d), want starting", env, status)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("starting status = %d, want 503", status)
	}

	f := startFakeEmb(t)
	b2 := newTestBridge(t, f.addr(), presets{}, testLimits())
	env, status = mustExec(t, b2, []string{"PING"}, 2)
	if env.Kind != kindStatus || env.Text != "PONG" || status != http.StatusOK {
		t.Fatalf("after the server exists: %+v (status %d), want PONG", env, status)
	}
}

func TestExecReconnectsAfterLoss(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())

	if env, _ := mustExec(t, b, []string{"PING"}, 2); env.Text != "PONG" {
		t.Fatalf("first PING = %+v", env)
	}
	f.killConns()
	if env, _ := mustExec(t, b, []string{"PING"}, 2); env.Text != "PONG" {
		t.Fatalf("PING after the connection dropped = %+v, want a reconnected PONG", env)
	}
}

func TestProtocolIsNegotiatedPerRequest(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{"sst2": {"sha": "classify"}}, testLimits())

	env, _ := mustExec(t, b, []string{"EMB.EVSHA", "sst2", "sha", "1", "hi"}, 3)
	if env.Kind != kindMap {
		t.Fatalf("RESP3 reply = %+v, want a map", env)
	}
	env, _ = mustExec(t, b, []string{"EMB.MODELS"}, 2)
	if env.Kind != kindArray {
		t.Fatalf("RESP2 reply = %+v, want an array", env)
	}
	// Both HELLO negotiations happened on the same serialized connection.
	var hellos []string
	for _, args := range f.received() {
		if strings.EqualFold(args[0], "hello") && len(args) > 1 {
			hellos = append(hellos, args[1])
		}
	}
	if len(hellos) < 2 || hellos[0] != "3" || hellos[1] != "2" {
		t.Fatalf("HELLO versions seen = %v, want [3 2]", hellos)
	}
}

// --- 3.3 allowlist ---------------------------------------------------------

func TestValidateArgvRefusesOutsideSurface(t *testing.T) {
	p := presets{"sst2": {"gooddigest": "classify"}}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"configuration", []string{"CONFIG", "SET", "cache", "1GB"}},
		{"credentials", []string{"AUTH", "hunter2"}},
		{"traffic stream", []string{"MONITOR"}},
		{"shutdown", []string{"SHUTDOWN"}},
		{"persistence", []string{"EMB.SAVE"}},
		{"cache mutation", []string{"EMB.CACHE.FLUSH"}},
		{"script load", []string{"EMB.SCRIPT", "LOAD", "minilm", "return 1"}},
		{"script flush", []string{"EMB.SCRIPT", "FLUSH"}},
		{"raw lua", []string{"EMB.EVAL", "minilm", "return 1", "1", "hi"}},
		{"image decode", []string{"EMB.IMG", "minilm", "aGk="}},
		{"image multi", []string{"EMB.IMGMULTI", "minilm", "aGk="}},
		{"unknown", []string{"FLUSHALL"}},
		// Widened permitted commands.
		{"models widened", []string{"EMB.MODELS", "extra"}},
		{"info widened", []string{"EMB.INFO", "minilm", "extra"}},
		{"info missing model", []string{"EMB.INFO"}},
		{"ping widened", []string{"PING", "a", "b"}},
		{"hello wrong version", []string{"HELLO", "4"}},
		{"emb without text", []string{"EMB", "minilm"}},
		{"emb values without text", []string{"EMB", "minilm", "VALUES"}},
		{"emb.multi odd pairs", []string{"EMB.MULTI", "minilm", "a", "bge"}},
		{"emb.multi no pairs", []string{"EMB.MULTI", "minilm"}},
		{"evsha unknown digest", []string{"EMB.EVSHA", "sst2", "baddigest", "1", "hi"}},
		{"evsha zero texts", []string{"EMB.EVSHA", "sst2", "gooddigest", "0"}},
		{"evsha short texts", []string{"EMB.EVSHA", "sst2", "gooddigest", "2", "one"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateArgv(tc.args, p)
			if err == nil {
				t.Fatalf("validateArgv(%v) = nil, want a refusal", tc.args)
			}
			for _, want := range []string{"sandbox permits only", "EMB.MULTI", "EMB.EVSHA"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("refusal %q does not name the surface (missing %q)", err, want)
				}
			}
		})
	}
}

func TestValidateArgvPermitsSurface(t *testing.T) {
	p := presets{"sst2": {"gooddigest": "classify"}}
	for _, args := range [][]string{
		{"PING"},
		{"PING", "hello"},
		{"HELLO", "3"},
		{"HELLO"},
		{"INFO"},
		{"INFO", "server"},
		{"EMB.MODELS"},
		{"EMB.STATS"},
		{"EMB.READY"},
		{"EMB.HELP"},
		{"EMB.HELP", "EMB"},
		{"EMB.INFO", "minilm"},
		{"EMB", "minilm", "hello world"},
		{"EMB", "minilm", "VALUES", "hello world"},
		{"EMB.MULTI", "minilm", "a", "sst2", "b"},
		{"EMB.MULTI", "VALUES", "minilm", "a", "sst2", "b"},
		{"EMB.EVSHA", "sst2", "gooddigest", "1", "this is great", "NEGATIVE", "POSITIVE"},
	} {
		if err := validateArgv(args, p); err != nil {
			t.Fatalf("validateArgv(%v) = %v, want permitted", args, err)
		}
	}
}

func TestExecuteRefusalNeverReachesTheServer(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())

	env, status := mustExec(t, b, []string{"CONFIG", "SET", "cache", "1GB"}, 2)
	if env.Kind != kindError || env.Code != codeRefused || status != http.StatusOK {
		t.Fatalf("refusal = %+v (status %d), want a refused error envelope at 200", env, status)
	}
	if got := f.received(); len(got) != 0 {
		t.Fatalf("server received %v, want nothing", got)
	}
}

// --- 3.4 preset manifest ---------------------------------------------------

func TestPresetManifestForwardsKnownDigestAndRefusesOthers(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{"sst2": {"gooddigest": "classify"}}, testLimits())

	env, _ := mustExec(t, b, []string{"EMB.EVSHA", "sst2", "gooddigest", "1", "this is great"}, 3)
	if env.Kind != kindMap {
		t.Fatalf("known digest = %+v, want the preset's map reply", env)
	}
	env, _ = mustExec(t, b, []string{"EMB.EVSHA", "sst2", "otherdigest", "1", "this is great"}, 3)
	if env.Kind != kindError || env.Code != codeRefused {
		t.Fatalf("unknown digest = %+v, want a refusal", env)
	}
	// A digest valid for another model is not valid for this one.
	env, _ = mustExec(t, b, []string{"EMB.EVSHA", "minilm", "gooddigest", "1", "hi"}, 3)
	if env.Kind != kindError || env.Code != codeRefused {
		t.Fatalf("digest for the wrong model = %+v, want a refusal", env)
	}
	for _, args := range f.received() {
		if strings.EqualFold(args[0], "emb.evsha") && args[2] != "gooddigest" {
			t.Fatalf("server received refused digest %v", args)
		}
	}
}

// --- 3.5 reply envelope ----------------------------------------------------

func TestEnvelopeBlobRoundTripsByteExact(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())

	env, status := mustExec(t, b, []string{"EMB", "minilm", "hello world"}, 2)
	if status != http.StatusOK || env.Kind != kindBulk {
		t.Fatalf("blob reply = %+v (status %d), want a bulk", env, status)
	}
	if env.Text != "" {
		t.Fatalf("binary bulk carried text %q; bytes must not be text-encoded", env.Text)
	}
	got, err := base64.StdEncoding.DecodeString(env.B64)
	if err != nil {
		t.Fatalf("b64 decode: %v", err)
	}
	if !bytes.Equal(got, blob()) {
		t.Fatalf("round-tripped bytes differ: %d bytes, want %d", len(got), len(blob()))
	}
	if env.Vector == nil || env.Vector.Dtype != "float32" || env.Vector.Count != 384 {
		t.Fatalf("vector descriptor = %+v, want float32 × 384", env.Vector)
	}
}

func TestEnvelopePreservesKindsAndOrder(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{"sst2": {"sha": "classify"}}, testLimits())

	// A RESP3 map keeps kind and order: label, confidence (typed double), scores.
	env, _ := mustExec(t, b, []string{"EMB.EVSHA", "sst2", "sha", "1", "hi"}, 3)
	if env.Kind != kindMap || len(env.Elems) != 6 {
		t.Fatalf("map = %+v, want 3 ordered pairs", env)
	}
	if env.Elems[0].Kind != kindBulk || env.Elems[0].Text != "label" {
		t.Fatalf("first key = %+v", env.Elems[0])
	}
	if env.Elems[1].Kind != kindBulk || env.Elems[1].Text != "POSITIVE" {
		t.Fatalf("first value = %+v", env.Elems[1])
	}
	if env.Elems[3].Kind != kindDouble || env.Elems[3].Float == nil || *env.Elems[3].Float != 0.99 {
		t.Fatalf("typed double = %+v, want 0.99", env.Elems[3])
	}
	if env.Elems[5].Kind != kindArray || len(env.Elems[5].Elems) != 2 {
		t.Fatalf("nested scores = %+v, want a 2-element array", env.Elems[5])
	}

	// The same command under RESP2 stays a flat array of the same kinds: the
	// VALUES envelope is a key/value list whose shape and values are nested
	// arrays of typed doubles.
	env, _ = mustExec(t, b, []string{"EMB", "minilm", "VALUES", "hi"}, 2)
	if env.Kind != kindArray || len(env.Elems) != 6 {
		t.Fatalf("VALUES array = %+v, want a 6-element array", env)
	}
	if env.Elems[3].Kind != kindArray || len(env.Elems[3].Elems) != 2 || *env.Elems[3].Elems[0].Int != 1 {
		t.Fatalf("VALUES shape = %+v, want [1 2]", env.Elems[3])
	}
	vals := env.Elems[5]
	if vals.Kind != kindArray || len(vals.Elems) != 2 {
		t.Fatalf("VALUES values = %+v, want a 2-element array", vals)
	}
	if vals.Elems[1].Kind != kindDouble || *vals.Elems[1].Float != 0.25 {
		t.Fatalf("VALUES double = %+v, want 0.25", vals.Elems[1])
	}
}

func TestEnvelopeErrorIsNotAValue(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())

	env, status := mustExec(t, b, []string{"EMB", "nosuch", "hi"}, 2)
	if env.Kind != kindError || env.Code != "" {
		t.Fatalf("server error = %+v, want an error kind without a bridge code", env)
	}
	if !strings.Contains(env.Text, "not found") {
		t.Fatalf("server error text = %q, want the server's own message", env.Text)
	}
	if status != http.StatusOK {
		t.Fatalf("server error status = %d, want 200 (it is a command result)", status)
	}
}

func TestReplyCarriesServerMeasuredElapsedTime(t *testing.T) {
	f := startFakeEmb(t)
	f.delay = 10 * time.Millisecond
	b := newTestBridge(t, f.addr(), presets{}, testLimits())

	env, status := mustExec(t, b, []string{"EMB", "minilm", "hi"}, 2)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	// The bridge times the server's answer on the loopback connection, so a
	// reply the server took ~10 ms to produce reports at least that, and the
	// client's round trip never enters the number.
	if env.ElapsedUs < 5000 {
		t.Fatalf("elapsed_us = %d, want the server's ~10 ms answer time", env.ElapsedUs)
	}
}

func TestRefusalCarriesNoElapsedTime(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())

	env, _ := mustExec(t, b, []string{"CONFIG", "SET", "cache", "1GB"}, 2)
	if env.Code != codeRefused || env.ElapsedUs != 0 {
		t.Fatalf("refusal = %+v, want no elapsed value", env)
	}
	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// omitempty keeps the field off the wire, so the client renders no trailer.
	if bytes.Contains(payload, []byte("elapsed_us")) {
		t.Fatalf("refusal payload %s carries an elapsed value", payload)
	}
}

// --- 3.6 spend bounds ------------------------------------------------------

func TestRateBoundRefusesIndependently(t *testing.T) {
	l := testLimits()
	l.PerClientRate = 0
	l.PerClientBurst = 1
	b := newTestBridge(t, "127.0.0.1:1", presets{}, l)

	if err := b.lim.rate("a", time.Now()); err != nil {
		t.Fatalf("first request refused: %v", err)
	}
	err := b.lim.rate("a", time.Now())
	if err == nil {
		t.Fatal("second request allowed, want a rate refusal")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("refusal %q does not name the rate limit", err)
	}
	if err := b.lim.rate("b", time.Now()); err != nil {
		t.Fatalf("a different client was refused: %v", err)
	}
}

func TestGlobalRateBoundRefusesIndependently(t *testing.T) {
	l := testLimits()
	l.GlobalRate = 0
	l.GlobalBurst = 1
	b := newTestBridge(t, "127.0.0.1:1", presets{}, l)

	if err := b.lim.rate("a", time.Now()); err != nil {
		t.Fatalf("first request refused: %v", err)
	}
	err := b.lim.rate("b", time.Now())
	if err == nil || !strings.Contains(err.Error(), "global rate") {
		t.Fatalf("second client err = %v, want the global rate limit", err)
	}
}

func TestWorkCeilingIsDistinctFromRate(t *testing.T) {
	l := testLimits()
	l.WorkCeiling = 2
	b := newTestBridge(t, "127.0.0.1:1", presets{}, l)
	now := time.Now()

	if err := b.lim.charge(2, now); err != nil {
		t.Fatalf("first charge refused: %v", err)
	}
	err := b.lim.charge(1, now)
	if err == nil {
		t.Fatal("charge above the ceiling allowed")
	}
	if !strings.Contains(err.Error(), "at capacity") {
		t.Fatalf("ceiling refusal %q does not say at capacity", err)
	}
	// The rate bound is a different refusal: a rate refusal names the rate,
	// never the ceiling, so a client can tell them apart.
	l2 := testLimits()
	l2.PerClientRate = 0
	l2.PerClientBurst = 1
	b2 := newTestBridge(t, "127.0.0.1:1", presets{}, l2)
	if err := b2.lim.rate("a", now); err != nil {
		t.Fatalf("first rate request refused: %v", err)
	}
	rateErr := b2.lim.rate("a", now)
	if rateErr == nil || !strings.Contains(rateErr.Error(), "rate limit") {
		t.Fatalf("rate refusal = %v, want a rate-limit message", rateErr)
	}
	if rateErr.Error() == err.Error() {
		t.Fatal("ceiling and rate refusals are identical; they must be distinguishable")
	}
}

func TestConcurrencyBoundRefusesIndependently(t *testing.T) {
	l := testLimits()
	l.MaxConcurrent = 1
	b := newTestBridge(t, "127.0.0.1:1", presets{}, l)

	release, err := b.lim.acquireSlot()
	if err != nil {
		t.Fatalf("first slot refused: %v", err)
	}
	if _, err := b.lim.acquireSlot(); err == nil {
		t.Fatal("second slot allowed, want a capacity refusal")
	}
	release()
	if _, err := b.lim.acquireSlot(); err != nil {
		t.Fatalf("slot after release refused: %v", err)
	}
}

func TestRequestAndTextCapsRefuseIndependently(t *testing.T) {
	l := testLimits()
	l.MaxArgs = 4
	l.MaxTexts = 1
	l.MaxTextBytes = 8
	b := newTestBridge(t, "127.0.0.1:1", presets{}, l)

	if _, err := b.lim.chargeShape([]string{"PING", "a", "b", "c", "d"}); err == nil || !strings.Contains(err.Error(), "arguments") {
		t.Fatalf("argv cap err = %v, want an argument-count refusal", err)
	}
	if _, err := b.lim.chargeShape([]string{"EMB", "minilm", "a", "b"}); err == nil || !strings.Contains(err.Error(), "texts") {
		t.Fatalf("text count cap err = %v, want a text-count refusal", err)
	}
	if _, err := b.lim.chargeShape([]string{"EMB", "minilm", "0123456789"}); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("text byte cap err = %v, want a byte refusal", err)
	}
	if _, err := b.lim.chargeShape([]string{"EMB", "minilm", "ok"}); err != nil {
		t.Fatalf("a within-caps request was refused: %v", err)
	}
}

func TestExecuteAppliesBounds(t *testing.T) {
	f := startFakeEmb(t)
	l := testLimits()
	l.MaxTexts = 1
	b := newTestBridge(t, f.addr(), presets{}, l)

	env, status := mustExec(t, b, []string{"EMB", "minilm", "a", "b"}, 2)
	if env.Kind != kindError || env.Code != codeCapacity || status != http.StatusTooManyRequests {
		t.Fatalf("over-cap request = %+v (status %d), want capacity at 429", env, status)
	}
}

// --- 3.7 error mapping -----------------------------------------------------

func TestErrorMappingDistinguishesFourConditions(t *testing.T) {
	t.Run("not ready", func(t *testing.T) {
		b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
		env, status := mustExec(t, b, []string{"PING"}, 2)
		if env.Code != codeStarting || status != http.StatusServiceUnavailable {
			t.Fatalf("got %+v (status %d), want starting/503", env, status)
		}
	})

	t.Run("transport failure", func(t *testing.T) {
		b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
		b.everOK.Store(true)
		env, status := mustExec(t, b, []string{"PING"}, 2)
		if env.Code != codeUnavailable || status != http.StatusBadGateway {
			t.Fatalf("got %+v (status %d), want unavailable/502", env, status)
		}
	})

	t.Run("capacity", func(t *testing.T) {
		l := testLimits()
		l.MaxConcurrent = 1
		f := startFakeEmb(t)
		b := newTestBridge(t, f.addr(), presets{}, l)
		release, err := b.lim.acquireSlot()
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		defer release()
		env, status := mustExec(t, b, []string{"PING"}, 2)
		if env.Code != codeCapacity || status != http.StatusTooManyRequests {
			t.Fatalf("got %+v (status %d), want capacity/429", env, status)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		f := startFakeEmb(t)
		f.silent = true
		l := testLimits()
		l.Timeout = 150 * time.Millisecond
		b := newTestBridge(t, f.addr(), presets{}, l)
		env, status := mustExec(t, b, []string{"PING"}, 2)
		if env.Code != codeTimeout || status != http.StatusGatewayTimeout {
			t.Fatalf("got %+v (status %d), want timeout/504", env, status)
		}
	})
}

// --- 3.8 CORS --------------------------------------------------------------

func TestCORSAllowlist(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits(), "https://emb.is", "https://preview.emb.is")
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	preflight := func(origin string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/exec", nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		req.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("preflight: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	if got := preflight("https://evil.example"); got.StatusCode != http.StatusForbidden {
		t.Fatalf("preflight from a disallowed origin = %d, want 403", got.StatusCode)
	}
	ok := preflight("https://emb.is")
	if ok.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight from the site = %d, want 204", ok.StatusCode)
	}
	if got := ok.Header.Get("Access-Control-Allow-Origin"); got != "https://emb.is" {
		t.Fatalf("allowed origin header = %q, want https://emb.is", got)
	}
}

// --- request decoding ------------------------------------------------------

func TestExecHTTPRoundTrip(t *testing.T) {
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	body := strings.NewReader(`{"args":["EMB","minilm","hello world"],"proto":2}`)
	resp, err := http.Post(srv.URL+"/api/exec", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var env Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Kind != kindBulk || env.Vector == nil || env.Vector.Count != 384 {
		t.Fatalf("reply = %+v, want a 384-element bulk", env)
	}
}

func TestLoadPresetsMatchesTheServerRule(t *testing.T) {
	dir := t.TempDir()
	src := []byte("return { label = 'POSITIVE' }\n")
	script := filepath.Join(dir, "preset.lua")
	if err := os.WriteFile(script, src, 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := "models:\n  sst2:\n    onnx: /dev/null\n    tokenizer: /dev/null\n    scripts:\n      - preset.lua\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	p, err := loadPresets(cfgPath)
	if err != nil {
		t.Fatalf("loadPresets: %v", err)
	}
	//nolint:gosec // cache identity only, per Redis EVALSHA semantics
	sum := sha1.Sum(src)
	if p["sst2"][hex.EncodeToString(sum[:])] != "preset" {
		t.Fatalf("manifest = %v, want the digest of the preloaded bytes", p)
	}
	if len(p["sst2"]) != 1 {
		t.Fatalf("manifest = %v, want exactly one digest", p)
	}
}

// --- 7.2 the landing page's module reference -------------------------------

func TestLandingPageTerminalReferenceIsServed(t *testing.T) {
	// The landing page loads the client module from the sandbox origin. Assert
	// the reference exists and that this bridge serves exactly that path, so the
	// console cannot silently lose its client.
	page, err := os.ReadFile(filepath.Join("..", "index.html"))
	if err != nil {
		t.Fatalf("read the landing page: %v", err)
	}
	ref := regexp.MustCompile(`src="(https?://[^"]*/terminal\.js)"`).FindSubmatch(page)
	if ref == nil {
		t.Fatal("the landing page does not reference a served terminal.js module")
	}
	path := "/" + "terminal.js"
	if u, err := url.Parse(string(ref[1])); err != nil {
		t.Fatalf("parse %q: %v", ref[1], err)
	} else {
		path = u.Path
	}

	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("terminal.js content type = %q, want javascript", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Contains(body, []byte("embTerminal")) {
		t.Fatal("the served module does not define window.embTerminal")
	}
}

func TestStandaloneTerminalIsServed(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("/terminal.js")) {
		t.Fatal("the standalone terminal does not load the shared client module")
	}
	missing, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("get /nope: %v", err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nope = %d, want 404", missing.StatusCode)
	}
}

func TestCORSSameOriginIsAllowed(t *testing.T) {
	// The standalone terminal is served by this bridge, and a browser sends
	// Origin even on that same-origin POST: refusing it would break the
	// sandbox's own page.
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits(), "https://emb.is")
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/exec",
		strings.NewReader(`{"args":["PING"],"proto":2}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same-origin POST = %d, want 200", resp.StatusCode)
	}
	var env Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Kind != kindStatus || env.Text != "PONG" {
		t.Fatalf("reply = %+v, want PONG", env)
	}
}

func TestCORSForwardedSchemeMatchesOrigin(t *testing.T) {
	// Behind the platform proxy the request arrives over http with the scheme
	// forwarded; the browser's Origin is the https one.
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits(), "https://emb.is")
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/exec",
		strings.NewReader(`{"args":["PING"],"proto":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://cli.emb.is")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "cli.emb.is"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxied same-origin POST = %d, want 200", resp.StatusCode)
	}
}

func TestPresetsEndpointPublishesTheManifest(t *testing.T) {
	b := newTestBridge(t, "127.0.0.1:1", presets{
		"minilm": {"aaaa": "embed"},
		"sst2":   {"bbbb": "classify"},
	}, testLimits())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/presets")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Presets []presetInfo `json:"presets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Presets) != 2 {
		t.Fatalf("presets = %+v, want both", body.Presets)
	}
	if body.Presets[0].Model != "minilm" || body.Presets[0].Name != "embed" || body.Presets[0].SHA != "aaaa" {
		t.Fatalf("first preset = %+v", body.Presets[0])
	}
}

func TestCORSSameHostDifferentPortIsAllowed(t *testing.T) {
	// The local dev loop serves the page and the bridge from one machine on two
	// ports (`just website-dev`, reachable from the LAN), so the page's Origin
	// never equals the bridge's own. Another host is still refused.
	f := startFakeEmb(t)
	b := newTestBridge(t, f.addr(), presets{}, testLimits(), "https://emb.is")
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/exec",
		strings.NewReader(`{"args":["PING"],"proto":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://192.168.1.20:8080")
	req.Host = "192.168.1.20:8081"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same-host, other-port POST = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://192.168.1.20:8080" {
		t.Fatalf("allow-origin = %q, want the page's origin", got)
	}

	other, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/exec",
		strings.NewReader(`{"args":["PING"],"proto":2}`))
	other.Header.Set("Content-Type", "application/json")
	other.Header.Set("Origin", "http://192.168.1.99:8080")
	other.Host = "192.168.1.20:8081"
	denied, err := http.DefaultClient.Do(other)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("different-host POST = %d, want 403", denied.StatusCode)
	}
}
