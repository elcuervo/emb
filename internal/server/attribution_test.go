package server

import (
	"image/color"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
)

// monitorFieldNames is the MONITOR event's field order under both encodings:
// the original six, then this request's attribution.
var monitorFieldNames = []string{
	"seq", "at_us", "model", "texts", "latency_us", "err",
	"cached_texts", "attributed", "dispatch_us", "admit_us", "run_us",
}

// monitorEvents reads one MONITOR reply and returns each event's fields by name.
func monitorEvents(t *testing.T, addr string) []map[string]respToken {
	t.Helper()
	got := redisCmd(t, addr, "MONITOR")
	if got.kind != "array" {
		t.Fatalf("MONITOR = %#v, want an array", got)
	}
	rows := got.val.([]respToken)
	out := make([]map[string]respToken, 0, len(rows))
	for _, row := range rows {
		if row.kind != "array" {
			t.Fatalf("event = %#v, want an array", row)
		}
		elems := row.val.([]respToken)
		if len(elems) != len(monitorFieldNames) {
			t.Fatalf("event has %d fields, want %d: %#v", len(elems), len(monitorFieldNames), elems)
		}
		byName := make(map[string]respToken, len(elems))
		for i, name := range monitorFieldNames {
			byName[name] = elems[i]
		}
		out = append(out, byName)
	}
	return out
}

// fetchEvent runs a command and returns the event it recorded.
func fetchEvent(t *testing.T, addr string, args ...string) map[string]respToken {
	t.Helper()
	before := len(monitorEvents(t, addr))
	redisCmd(t, addr, args...)
	events := monitorEvents(t, addr)
	if len(events) != before+1 {
		t.Fatalf("expected one new event, got %d (was %d)", len(events), before)
	}
	return events[len(events)-1]
}

func TestMonitorEventAttributesTheCachePath(t *testing.T) {
	addr := serveTestWithCache(t, "1MB")

	miss := fetchEvent(t, addr, "EMB", "test", "attribution-one")
	if got := intOf(t, miss["cached_texts"]); got != 0 {
		t.Errorf("uncached request reports cached_texts = %d, want 0", got)
	}
	if got := intOf(t, miss["attributed"]); got != 1 {
		t.Errorf("an embedding request is not attributed: attributed = %d", got)
	}
	if got := intOf(t, miss["run_us"]); got <= 0 {
		t.Errorf("an uncached run reports run_us = %d, want > 0", got)
	}

	hit := fetchEvent(t, addr, "EMB", "test", "attribution-one")
	if got := intOf(t, hit["cached_texts"]); got != 1 {
		t.Errorf("cache-served request reports cached_texts = %d, want 1", got)
	}
	if got := intOf(t, hit["attributed"]); got != 1 {
		t.Errorf("a cache-served request is not attributed: attributed = %d", got)
	}
	for _, phase := range []string{"dispatch_us", "admit_us", "run_us"} {
		if got := intOf(t, hit[phase]); got != 0 {
			t.Errorf("a cache-served request reports %s = %d, want 0", phase, got)
		}
	}
}

func TestMonitorEventPhasesFitWithinItsLatency(t *testing.T) {
	addr := serveTestWithCache(t, "1MB")
	ev := fetchEvent(t, addr, "EMB", "test", "attribution-two")

	latency := intOf(t, ev["latency_us"])
	sum := intOf(t, ev["dispatch_us"]) + intOf(t, ev["admit_us"]) + intOf(t, ev["run_us"])
	if sum > latency {
		t.Errorf("phases (%d) exceed the event's latency (%d)", sum, latency)
	}
}

func TestMonitorEventAttributesAPartlyCachedBatch(t *testing.T) {
	addr := serveTestWithCache(t, "1MB")
	// Warm one text, then ask for it beside a fresh one: the request is served
	// partly from the cache and its phases cover the text that ran.
	fetchEvent(t, addr, "EMB", "test", "attribution-cached")
	ev := fetchEvent(t, addr, "EMB", "test", "attribution-cached", "attribution-fresh")

	if got := intOf(t, ev["texts"]); got != 2 {
		t.Fatalf("texts = %d, want 2", got)
	}
	if got := intOf(t, ev["cached_texts"]); got != 1 {
		t.Errorf("partly cached batch reports cached_texts = %d, want 1", got)
	}
	if got := intOf(t, ev["run_us"]); got <= 0 {
		t.Errorf("a partly cached batch reports no run time: run_us = %d", got)
	}
	if got := intOf(t, ev["attributed"]); got != 1 {
		t.Errorf("a partly cached batch is not attributed: attributed = %d", got)
	}
}

func TestMonitorEventAttributesMULTIPairs(t *testing.T) {
	addr := serveTestWithCache(t, "1MB")

	miss := fetchEvent(t, addr, "EMB.MULTI", "test", "pair-one")
	if got := intOf(t, miss["cached_texts"]); got != 0 {
		t.Errorf("uncached pair reports cached_texts = %d, want 0", got)
	}
	if got := intOf(t, miss["attributed"]); got != 1 {
		t.Errorf("a pair that ran is not attributed: attributed = %d", got)
	}

	hit := fetchEvent(t, addr, "EMB.MULTI", "test", "pair-one")
	if got := intOf(t, hit["cached_texts"]); got != 1 {
		t.Errorf("cache-served pair reports cached_texts = %d, want 1", got)
	}
	if got := intOf(t, hit["run_us"]); got != 0 {
		t.Errorf("cache-served pair reports run_us = %d, want 0", got)
	}
}

func TestMonitorEventMarksTheImagePathUnattributed(t *testing.T) {
	addr, _, sessions := serveImage(t, "1MB")
	png := solidImagePNG(t, color.White)

	miss := fetchEvent(t, addr, "EMB.IMG", "imgA", png)
	if got := intOf(t, miss["attributed"]); got != 0 {
		t.Errorf("the image path claims an attribution it does not measure: attributed = %d", got)
	}
	for _, phase := range []string{"dispatch_us", "admit_us", "run_us"} {
		if got := intOf(t, miss[phase]); got != 0 {
			t.Errorf("unattributed image event reports %s = %d, want 0", phase, got)
		}
	}
	if got := intOf(t, miss["cached_texts"]); got != 0 {
		t.Errorf("uncached image request reports cached_texts = %d, want 0", got)
	}
	if sessions["imgA"].callCount() == 0 {
		t.Fatal("the image never ran")
	}

	hit := fetchEvent(t, addr, "EMB.IMG", "imgA", png)
	if got := intOf(t, hit["cached_texts"]); got != 1 {
		t.Errorf("cache-served image reports cached_texts = %d, want 1", got)
	}
}

func TestMonitorEventWithoutAttributionReadsAsUnattributed(t *testing.T) {
	// The event mapping itself: a request with no measured phases must not
	// present zeroes as a measured run.
	var zero MonitorEvent
	ev := zero.attributed(requestAttribution{})
	if ev.Attributed {
		t.Error("an unattributed request reports Attributed true")
	}
	if ev.DispatchUs != 0 || ev.AdmitUs != 0 || ev.RunUs != 0 {
		t.Errorf("unattributed phases are non-zero: %+v", ev)
	}

	attributed := zero.attributed(requestAttribution{
		cachedTexts:   2,
		attributed:    true,
		dispatchWait:  3 * time.Millisecond,
		admissionWait: 4 * time.Millisecond,
		run:           5 * time.Millisecond,
	})
	if !attributed.Attributed || attributed.CachedTexts != 2 {
		t.Errorf("attribution not carried: %+v", attributed)
	}
	if attributed.DispatchUs != 3000 || attributed.AdmitUs != 4000 || attributed.RunUs != 5000 {
		t.Errorf("phases in microseconds wrong: %+v", attributed)
	}
}

func TestInfoCacheCountersAreTheModels(t *testing.T) {
	addr, _, _ := serveImage(t, "1MB")
	png := solidImagePNG(t, color.White)

	// One model with a hit and a miss; the other untouched.
	redisCmd(t, addr, "EMB.IMG", "imgA", png)
	redisCmd(t, addr, "EMB.IMG", "imgA", png)

	hit, miss, entries, bytes := 0, 0, 0, 0
	imgA := replyPairs(t, redisCmd(t, addr, "EMB.INFO", "imgA"))
	if got := intOf(t, imgA["cache_hits"]); got != 1 {
		t.Errorf("imgA cache_hits = %d, want 1", got)
	} else {
		hit = got
	}
	if got := intOf(t, imgA["cache_misses"]); got != 1 {
		t.Errorf("imgA cache_misses = %d, want 1", got)
	} else {
		miss = got
	}
	if got := intOf(t, imgA["cache_entries"]); got != 1 {
		t.Errorf("imgA cache_entries = %d, want 1", got)
	} else {
		entries = got
	}
	if got := intOf(t, imgA["cache_memory_bytes"]); got <= 0 {
		t.Errorf("imgA cache_memory_bytes = %d, want > 0", got)
	} else {
		bytes = got
	}
	if got := bulkOf(t, imgA["cache_hit_rate"]); got != "50.0%" {
		t.Errorf("imgA cache_hit_rate = %q, want 50.0%%", got)
	}

	imgB := replyPairs(t, redisCmd(t, addr, "EMB.INFO", "imgB"))
	for _, key := range []string{"cache_hits", "cache_misses", "cache_entries", "cache_memory_bytes"} {
		if got := intOf(t, imgB[key]); got != 0 {
			t.Errorf("an untouched model reports %s = %d, want 0", key, got)
		}
	}
	// Two models never share one cache's numbers.
	if intOf(t, imgA["cache_hits"]) == intOf(t, imgB["cache_hits"]) &&
		intOf(t, imgA["cache_misses"]) == intOf(t, imgB["cache_misses"]) &&
		hit+miss+entries+bytes > 0 {
		t.Error("both models report the same cache counters")
	}

	// The aggregate stays process-wide on EMB.STATS.
	stats := replyPairs(t, redisCmd(t, addr, "EMB.STATS"))
	if got := intOf(t, stats["cache_hits"]); got != 1 {
		t.Errorf("EMB.STATS cache_hits = %d, want the aggregate 1", got)
	}
}

// replyPairs views an EMB.INFO/EMB.STATS flat key/value reply as a map.
func replyPairs(t *testing.T, tok respToken) map[string]respToken {
	t.Helper()
	if tok.kind != "array" {
		t.Fatalf("reply = %#v, want an array of pairs", tok)
	}
	elems := tok.val.([]respToken)
	if len(elems)%2 != 0 {
		t.Fatalf("reply has %d elements, want key/value pairs", len(elems))
	}
	out := make(map[string]respToken, len(elems)/2)
	for i := 0; i < len(elems); i += 2 {
		out[bulkOf(t, elems[i])] = elems[i+1]
	}
	return out
}

// fetchEventEval runs an EMB.EVAL, fails on a script error, and returns the
// event it recorded.
func fetchEventEval(t *testing.T, addr, script string, args ...string) map[string]respToken {
	t.Helper()
	cmd := append([]string{"EMB.EVAL", "test", script}, args...)
	if got := redisCmd(t, addr, cmd...); got.kind == "error" {
		t.Fatalf("EMB.EVAL failed: %v", got.val)
	}
	events := monitorEvents(t, addr)
	if len(events) == 0 {
		t.Fatal("no event recorded for the evaluation")
	}
	return events[len(events)-1]
}

// The scripted path reports its phases through the model's cumulative counters,
// which are the model's and not one request's. A single run in the window is
// therefore this request's time; anything else is marked unattributed rather
// than reported as if it were.
func TestMonitorEventAttributesASingleScriptedRun(t *testing.T) {
	addr, _ := serveScriptModel(t, config.ModelConfig{}, "")
	// A scripted run, not an embedding-pool run: emb.embed is served by the
	// embedding pool and its runs are counted there, so only emb.run exercises
	// the scripted path's own dispatch and run counters.
	const oneRun = `local e = emb.tokenize.encode(KEYS[1], 128)
local out = emb.run({
  input_ids = {shape = {1, #e.ids}, data = e.ids},
  attention_mask = {shape = {1, #e.mask}, data = e.mask},
  token_type_ids = {shape = {1, #e.ids}, fill = 0, dtype = "i64"},
}, {bytes = true, outputs = {"last_hidden_state"}})
local h = out.last_hidden_state
return emb.math.mean_pool(h.bytes, h.shape, e.mask)[1][1]`

	ev := fetchEventEval(t, addr, oneRun, "1", "hello world")
	if got := intOf(t, ev["attributed"]); got != 1 {
		t.Fatalf("a single-run evaluation is not attributed: attributed = %d (%v)", got, ev)
	}
	if got := intOf(t, ev["run_us"]); got <= 0 {
		t.Errorf("a single-run evaluation reports run_us = %d, want > 0", got)
	}
	latency := intOf(t, ev["latency_us"])
	if sum := intOf(t, ev["dispatch_us"]) + intOf(t, ev["admit_us"]) + intOf(t, ev["run_us"]); sum > latency {
		t.Errorf("scripted phases (%d) exceed the event's latency (%d)", sum, latency)
	}
}

func TestMonitorEventLeavesAMultiRunEvaluationUnattributed(t *testing.T) {
	addr, _ := serveScriptModel(t, config.ModelConfig{}, "")
	// Two sequential scripted runs: the counter delta covers both, so it is not
	// this request's own time.
	const twoRuns = `local function one(text)
  local e = emb.tokenize.encode(text, 128)
  local out = emb.run({
    input_ids = {shape = {1, #e.ids}, data = e.ids},
    attention_mask = {shape = {1, #e.mask}, data = e.mask},
    token_type_ids = {shape = {1, #e.ids}, fill = 0, dtype = "i64"},
  }, {bytes = true, outputs = {"last_hidden_state"}})
  return emb.math.mean_pool(out.last_hidden_state.bytes, out.last_hidden_state.shape, e.mask)[1][1]
end
return one(KEYS[1]) + one(ARGV[1])`

	ev := fetchEventEval(t, addr, twoRuns, "1", "hello world", "second text")
	if got := intOf(t, ev["attributed"]); got != 0 {
		t.Errorf("a two-run evaluation claims an attribution it cannot support: attributed = %d (%v)", got, ev)
	}
	for _, phase := range []string{"dispatch_us", "admit_us", "run_us"} {
		if got := intOf(t, ev[phase]); got != 0 {
			t.Errorf("unattributed scripted event reports %s = %d, want 0", phase, got)
		}
	}
}
