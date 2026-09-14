package server

import (
	"strings"
	"testing"
)

// statsFields flattens an EMB.STATS / EMB.INFO flat pair reply into a map.
func statsFields(t *testing.T, tok respToken) map[string]respToken {
	t.Helper()
	arr := arrayOf(t, tok)
	if len(arr)%2 != 0 {
		t.Fatalf("stats reply has odd element count %d", len(arr))
	}
	m := make(map[string]respToken, len(arr)/2)
	for i := 0; i+1 < len(arr); i += 2 {
		key, ok := arr[i].val.(string)
		if !ok {
			t.Fatalf("stats key %d is not a string: %#v", i, arr[i])
		}
		m[key] = arr[i+1]
	}
	return m
}

func intOf(t *testing.T, tok respToken) int {
	t.Helper()
	if tok.kind != "int" {
		t.Fatalf("expected int, got %s", tok.kind)
	}
	return tok.val.(int)
}

// TestScriptedEvaluationRecordedInMonitor verifies a completed evaluation
// appends a MONITOR event carrying the model and text count and no payload.
func TestScriptedEvaluationRecordedInMonitor(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	defer c.Close()

	if got := doCmd(t, c, "EMB.EVAL", "test", "return 42", "1", "secret-text"); got != ":42\r\n" {
		t.Fatalf("evaluation reply = %q", got)
	}

	resp := doCmd(t, c, "MONITOR")
	if !strings.HasPrefix(resp, "*1\r\n") {
		t.Fatalf("monitor = %q, want exactly one event", resp)
	}
	if !strings.Contains(resp, "$4\r\ntest\r\n") {
		t.Fatalf("event missing model: %q", resp)
	}
	// seq, at_us, model, texts=1, latency, err. No text or script payload.
	if strings.Contains(resp, "secret-text") || strings.Contains(resp, "return 42") {
		t.Fatalf("event leaked payload: %q", resp)
	}
}

// TestScriptedEvaluationCounters verifies the cumulative counters, including
// the error count for a script that raises.
func TestScriptedEvaluationCounters(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	defer c.Close()

	if got := doCmd(t, c, "EMB.EVAL", "test", "return 1", "1", "a"); got != ":1\r\n" {
		t.Fatalf("first evaluation = %q", got)
	}
	if got := doCmd(t, c, "EMB.EVAL", "test", "error('boom')", "1", "b"); !strings.HasPrefix(got, "-ERR") {
		t.Fatalf("failing evaluation = %q, want error", got)
	}
	if got := doCmd(t, c, "EMB.EVAL", "test", "return 3", "1", "c"); got != ":3\r\n" {
		t.Fatalf("third evaluation = %q", got)
	}

	stats := statsFields(t, redisCmd(t, addr, "EMB.STATS"))
	if got := intOf(t, stats["script_requests"]); got != 3 {
		t.Fatalf("script_requests = %d, want 3", got)
	}
	if got := intOf(t, stats["script_errors"]); got != 1 {
		t.Fatalf("script_errors = %d, want 1", got)
	}
	if got := intOf(t, stats["script_avg_latency_us"]); got < 0 {
		t.Fatalf("script_avg_latency_us = %d, want >= 0", got)
	}
}

// TestScriptedPerModelCountsAreSeparate verifies embedding and scripted counts
// are reported independently per model.
func TestScriptedPerModelCountsAreSeparate(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	defer c.Close()

	// One embedding request, two scripted evaluations, on the same model.
	if got := doCmd(t, c, "EMB", "test", "hello"); !strings.HasPrefix(got, "$") {
		t.Fatalf("EMB reply = %q", got)
	}
	for i := 0; i < 2; i++ {
		if got := doCmd(t, c, "EMB.EVAL", "test", "return 1", "1", "x"); got != ":1\r\n" {
			t.Fatalf("evaluation %d = %q", i, got)
		}
	}

	stats := statsFields(t, redisCmd(t, addr, "EMB.STATS"))
	perModel := bulkOf(t, stats["per_model"])
	if !strings.Contains(perModel, "test: req=1") {
		t.Fatalf("per_model = %q, want one embedding request", perModel)
	}
	perModelScripts := bulkOf(t, stats["per_model_scripts"])
	if !strings.Contains(perModelScripts, "test: req=2 err=0") {
		t.Fatalf("per_model_scripts = %q, want two scripted evaluations", perModelScripts)
	}
}

// TestScriptedFootprintReported verifies the scripted resource fields exist and
// that resources are created lazily: a constant-returning script opens no
// named-tensor sessions, while a script that calls emb.run does.
func TestScriptedFootprintReported(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	c := dial(t, addr)
	defer c.Close()

	info := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	for _, field := range []string{"script_requests", "script_errors", "script_sessions", "script_tokenizer"} {
		if _, ok := info[field]; !ok {
			t.Fatalf("EMB.INFO missing %q: %#v", field, info)
		}
	}
	if got := intOf(t, info["script_sessions"]); got != 0 {
		t.Fatalf("script_sessions before any script = %d, want 0", got)
	}

	// A script that never touches emb.run must not open sessions.
	if got := doCmd(t, c, "EMB.EVAL", "test", "return 1", "1", "x"); got != ":1\r\n" {
		t.Fatalf("constant evaluation = %q", got)
	}
	info = statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if got := intOf(t, info["script_sessions"]); got != 0 {
		t.Fatalf("script_sessions after constant script = %d, want 0", got)
	}

	// A script that calls emb.run does open sessions.
	runScript := `local e = emb.tokenize.encode(KEYS[1], 8)
local out = emb.run({input_ids = {shape = {1, #e.ids}, data = e.ids},
  attention_mask = {shape = {1, #e.mask}, data = e.mask},
  token_type_ids = {shape = {1, #e.ids}, fill = 0, dtype = "i64"}})
return #out.last_hidden_state.shape`
	if got := doCmd(t, c, "EMB.EVAL", "test", runScript, "1", "hello"); !strings.HasPrefix(got, ":") {
		t.Fatalf("emb.run evaluation = %q", got)
	}
	info = statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if got := intOf(t, info["script_sessions"]); got < 1 {
		t.Fatalf("script_sessions after emb.run script = %d, want >= 1", got)
	}
}
