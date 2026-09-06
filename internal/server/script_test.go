package server

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/pipeline"
	"github.com/elcuervo/emb/internal/registry"
)

// serveScriptTest starts a server with a scripted model "test" backed by the
// real minilm fixture, plus a blocking pool model "block" for gate tests. The
// minilm files are downloaded via `just download-model`; tests skip when the
// fixture is absent (e.g. fresh CI checkouts).
func serveScriptTest(t *testing.T, cacheCfg string, opts ...Option) (string, *Server) {
	t.Helper()
	reg := registry.New()
	entry, err := registry.LoadModel(config.ModelConfig{
		ONNX:      "../../models/minilm/model.onnx",
		Tokenizer: "../../models/minilm/tokenizer.json",
		Dim:       384,
		MaxLength: 128,
		Pooling:   "mean",
		Normalize: false,
	}, "test")
	if err != nil {
		t.Skipf("script test model not present: %v (run: just download-model)", err)
	}
	if !ortOK {
		t.Skip("onnx runtime unavailable (run inside nix develop)")
	}
	reg.Add("test", entry)

	gate := make(chan struct{})
	blockPool, err := pipeline.NewPool(
		func() (onnx.Session, error) { return &blockingSession{gate: gate}, nil },
		mockTokenizer{}, 1, 4, 128, true, "mean", 0, 32, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	reg.Add("block", &registry.ModelEntry{Pool: blockPool, Dim: 4, Name: "block"})
	t.Cleanup(func() { close(gate) })

	addr := getFreeAddr()
	srv := New(addr, reg, "", cacheCfg, nil, opts...)
	go srv.ListenAndServe()
	t.Cleanup(func() { srv.Close() })
	time.Sleep(50 * time.Millisecond)
	return addr, srv
}

// doCmd writes a raw RESP command and reads a complete RESP value.
func doCmd(t *testing.T, c net.Conn, args ...string) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := c.Write([]byte(b.String())); err != nil {
		t.Fatal(err)
	}
	return readRESPComplete(t, c)
}

const helloScript = `return KEYS[1] .. "|" .. ARGV[1]`

func TestScriptLoadExistsFlush(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)

	sha := doCmd(t, c, "EMB.SCRIPT", "LOAD", "test", helloScript)
	if !strings.HasPrefix(sha, "$40\r\n") {
		t.Fatalf("expected SHA1 bulk, got %q", sha)
	}
	shaVal := sha[5 : len(sha)-2]

	// EXISTS reflects the per-model cache.
	if got := doCmd(t, c, "EMB.SCRIPT", "EXISTS", "test", shaVal); got != "*1\r\n:1\r\n" {
		t.Fatalf("unexpected EXISTS reply %q", got)
	}
	if got := doCmd(t, c, "EMB.SCRIPT", "EXISTS", "block", shaVal); got != "*1\r\n:0\r\n" {
		t.Fatalf("unexpected EXISTS for other model %q", got)
	}

	// FLUSH per model clears only that model.
	if got := doCmd(t, c, "EMB.SCRIPT", "FLUSH", "test"); got != "+OK\r\n" {
		t.Fatalf("unexpected FLUSH reply %q", got)
	}
	if got := doCmd(t, c, "EMB.SCRIPT", "EXISTS", "test", shaVal); got != "*1\r\n:0\r\n" {
		t.Fatalf("expected flush to clear, got %q", got)
	}
	c.Close()
}

func TestScriptLoadRejectsInvalidScript(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	resp := doCmd(t, c, "EMB.SCRIPT", "LOAD", "test", "this is not lua (")
	if !strings.HasPrefix(resp, "-ERR") {
		t.Fatalf("expected error, got %q", resp)
	}
	c.Close()
}

func TestEvshaUnknownSHA(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	resp := doCmd(t, c, "EMB.EVSHA", "test", "deadbeef", "1", "hello")
	if resp != "-ERR no such script\r\n" {
		t.Fatalf("expected no-such-script error, got %q", resp)
	}
	c.Close()
}

func TestEvalArityErrors(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)

	cases := [][]string{
		{"EMB.EVAL", "test", helloScript},              // missing numtexts
		{"EMB.EVAL", "test", helloScript, "0"},         // numtexts must be >= 1
		{"EMB.EVAL", "test", helloScript, "abc"},       // non-numeric numtexts
		{"EMB.EVAL", "test", helloScript, "3", "only"}, // too few texts
	}
	for _, args := range cases {
		if resp := doCmd(t, c, args...); !strings.HasPrefix(resp, "-ERR") {
			t.Fatalf("args %v: expected error, got %q", args, resp)
		}
	}
	c.Close()
}

func TestEvalUnknownModel(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	resp := doCmd(t, c, "EMB.EVAL", "nope", helloScript, "1", "x")
	if !strings.Contains(resp, "not found") {
		t.Fatalf("expected model-not-found error, got %q", resp)
	}
	c.Close()
}

func TestEvalInlineAndEvshaRoundTrip(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)

	// Inline EVAL, single text → single bulk.
	resp := doCmd(t, c, "EMB.EVAL", "test", helloScript, "1", "hello world", "PERSON")
	if resp != "$18\r\nhello world|PERSON\r\n" {
		t.Fatalf("unexpected inline eval reply %q", resp)
	}

	// LOAD then EVSHA with two texts → array of bulks.
	sha := doCmd(t, c, "EMB.SCRIPT", "LOAD", "test", helloScript)
	shaVal := sha[5 : len(sha)-2]
	resp = doCmd(t, c, "EMB.EVSHA", "test", shaVal, "2", "a", "b", "ORG")
	if resp != "*2\r\n$5\r\na|ORG\r\n$5\r\nb|ORG\r\n" {
		t.Fatalf("unexpected evsha reply %q", resp)
	}
	c.Close()
}

func TestEvalHashReply(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)

	// Entities table → flat field/value pairs (HGETALL shape), keys sorted.
	resp := doCmd(t, c, "EMB.EVAL", "test", `return {PERSON = {"Tim Cook"}, ORG = {"Apple"}}`, "1", "ignored")
	want := "*4\r\n$3\r\nORG\r\n*1\r\n$5\r\nApple\r\n$6\r\nPERSON\r\n*1\r\n$8\r\nTim Cook\r\n"
	if resp != want {
		t.Fatalf("unexpected hash reply\n got %q\nwant %q", resp, want)
	}
	c.Close()
}

func TestEvalErrTable(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	resp := doCmd(t, c, "EMB.EVAL", "test", `return {err = "bad labels"}`, "1", "x")
	if resp != "-bad labels\r\n" {
		t.Fatalf("unexpected err-table reply %q", resp)
	}
	c.Close()
}

func TestEvshaCacheHit(t *testing.T) {
	addr, srv := serveScriptTest(t, "8mb")
	c := dial(t, addr)

	sha := doCmd(t, c, "EMB.SCRIPT", "LOAD", "test", helloScript)
	shaVal := sha[5 : len(sha)-2]

	first := doCmd(t, c, "EMB.EVSHA", "test", shaVal, "1", "hello", "PERSON")
	second := doCmd(t, c, "EMB.EVSHA", "test", shaVal, "1", "hello", "PERSON")
	if first != second {
		t.Fatalf("cache should reply identically:\n%q\n%q", first, second)
	}
	stats := srv.cache.Stats()
	if stats.Hits < 1 {
		t.Fatalf("expected >=1 cache hit after repeated call, got %d", stats.Hits)
	}

	// Distinct args → different cache entries (miss again).
	if got := doCmd(t, c, "EMB.EVSHA", "test", shaVal, "1", "hello", "ORG"); got != "$9\r\nhello|ORG\r\n" {
		t.Fatalf("unexpected distinct-args reply %q", got)
	}
	if stats = srv.cache.Stats(); stats.Hits < 1 {
		t.Fatalf("expected hits preserved, got %d", stats.Hits)
	}
	c.Close()
}

// TestEvalDeadlineIsolated verifies a runaway script errors only its own
// request: the server stays reachable and cooperative requests keep working
// (spec: budgets reply per-request, no global blocking).
func TestEvalDeadlineIsolated(t *testing.T) {
	addr, _ := serveScriptTest(t, "", WithScriptDeadline(200*time.Millisecond))
	c := dial(t, addr)

	resp := doCmd(t, c, "EMB.EVAL", "test", "local x = 0 while true do x = x + 1 end", "1", "hi")
	if !strings.Contains(resp, "deadline exceeded") {
		t.Fatalf("expected deadline error, got %q", resp)
	}

	// The same connection keeps working.
	if got := doCmd(t, c, "EMB.EVAL", "test", helloScript, "1", "world", "PERSON"); got != "$12\r\nworld|PERSON\r\n" {
		t.Fatalf("cooperative eval failed after runaway: %q", got)
	}
	c.Close()
}

func TestEvalConcurrentGate(t *testing.T) {
	addr, _ := serveScriptTest(t, "", WithMaxConcurrentRequests(1))
	c1 := dial(t, addr)
	c2 := dial(t, addr)

	// c1 occupies the single slot with a blocking EMB on the "block" model.
	c1.Write([]byte("*3\r\n$3\r\nEMB\r\n$5\r\nblock\r\n$5\r\nhello\r\n"))
	waitFor(t, func() bool {
		return statsIntField(t, c2, "active_requests") == 1
	})

	// A scripted evaluation on another model is busy-errored under the gate.
	resp := doCmd(t, c2, "EMB.EVAL", "test", helloScript, "1", "world", "PERSON")
	if !strings.HasPrefix(resp, "-ERR busy") {
		t.Fatalf("expected busy error, got %q", resp)
	}

	// Control commands stay reachable.
	if got := doCmd(t, c2, "PING"); got != "+PONG\r\n" {
		t.Fatalf("expected PONG, got %q", got)
	}
	c1.Close()
	c2.Close()
}

func TestHelpDocumentsScriptFamily(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	resp := doCmd(t, c, "EMB.HELP")
	for _, want := range []string{
		"EMB.EVAL", "EMB.EVSHA", "EMB.SCRIPT LOAD", "EMB.SCRIPT EXISTS",
		"EMB.SCRIPT FLUSH", "field/value pairs", "emb.math",
	} {
		if !strings.Contains(resp, want) {
			t.Fatalf("EMB.HELP missing %q", want)
		}
	}
	c.Close()
}
