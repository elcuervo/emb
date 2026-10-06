package server

import (
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/script"
)

// warmKey returns the reply-cache key a client call with these arguments reads,
// taken from the loaded script entry the way a client call would.
func warmKey(t *testing.T, srv *Server, model, sha, text string, args []string) string {
	t.Helper()
	entry, ok := srv.scripts.Get(model, sha)
	if !ok {
		t.Fatalf("model %q has no script %s", model, sha)
	}
	return script.CacheKeyConfig(model, sha, args, 1, text, entry.digest)
}

// TestWarmScriptsCachesDeclaredPayloads verifies a declared payload lands in the
// reply cache under the client's key, that the warm is not counted as a client
// evaluation (or it would stop itself), and that the client call then reads it.
func TestWarmScriptsCachesDeclaredPayloads(t *testing.T) {
	addr, srv := serveScriptTest(t, "1GB")
	sha, err := srv.PreloadScript("test", `return KEYS[1] .. "|" .. ARGV[1]`)
	if err != nil {
		t.Fatal(err)
	}

	<-srv.WarmScripts([]ScriptWarmup{{
		Model: "test",
		SHA:   sha,
		Payloads: []WarmPayload{
			{Text: "warm-state", Args: []string{"q"}},
		},
	}})

	key := warmKey(t, srv, "test", sha, "warm-state", []string{"q"})
	got, ok := srv.cache.Get(key)
	if !ok {
		t.Fatal("warm did not cache the declared payload")
	}
	if string(got) != "$12\r\nwarm-state|q\r\n" {
		t.Fatalf("cached reply = %q", got)
	}

	// The warm must not advance the counter the yield rule watches.
	stats := statsFields(t, redisCmd(t, addr, "EMB.STATS"))
	if n := intOf(t, stats["script_requests"]); n != 0 {
		t.Fatalf("script_requests after warm = %d, want 0", n)
	}

	c := dial(t, addr)
	defer c.Close()
	before := statsFields(t, redisCmd(t, addr, "EMB.STATS"))
	missesBefore := intOf(t, before["cache_misses"])
	if reply := doCmd(t, c, "EMB.EVSHA", "test", sha, "1", "warm-state", "q"); reply != "$12\r\nwarm-state|q\r\n" {
		t.Fatalf("client reply = %q", reply)
	}
	// The client call must be a hit: a warmed payload is never re-evaluated.
	after := statsFields(t, redisCmd(t, addr, "EMB.STATS"))
	if misses := intOf(t, after["cache_misses"]); misses != missesBefore {
		t.Fatalf("client call missed the warm entry: cache_misses %d -> %d", missesBefore, misses)
	}
}

// TestWarmScriptsSchedulesWithoutBlocking verifies the boot path cannot gate
// readiness: the schedule returns while the warm is parked, and the channel
// closes once it finishes.
func TestWarmScriptsSchedulesWithoutBlocking(t *testing.T) {
	addr, srv := serveScriptTest(t, "1GB")
	srv.SetReady()
	sha, err := srv.PreloadScript("test", `return KEYS[1]`)
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	served := func(string) int64 { <-release; return 0 }
	done := srv.warmScriptsAsync([]ScriptWarmup{{
		Model:    "test",
		SHA:      sha,
		Payloads: []WarmPayload{{Text: "parked", Args: []string{"a"}}},
	}}, served)

	select {
	case <-done:
		t.Fatal("the warm ran to completion before the schedule returned; it would gate readiness")
	default:
	}
	// A warm parked mid-flight must not keep the server from reporting ready.
	c := dial(t, addr)
	defer c.Close()
	if got := doCmd(t, c, "EMB.READY"); got != "+OK\r\n" {
		t.Fatalf("EMB.READY during the warm = %q, want +OK", got)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("warm did not finish after the signal was released")
	}
}

// TestWarmScriptsYieldsToClientTraffic verifies the loop stops at the first
// payload that saw a client request since the baseline, rather than making a
// visitor wait behind the whole declared set.
func TestWarmScriptsYieldsToClientTraffic(t *testing.T) {
	_, srv := serveScriptTest(t, "1GB")
	sha, err := srv.PreloadScript("test", `return KEYS[1]`)
	if err != nil {
		t.Fatal(err)
	}

	// Unchanged for the baseline and the first payload, then risen: what a
	// client request arriving during the first payload looks like.
	calls := 0
	served := func(string) int64 {
		calls++
		if calls <= 2 {
			return 0
		}
		return 1
	}
	srv.warmScripts([]ScriptWarmup{{Model: "test", SHA: sha, Payloads: []WarmPayload{
		{Text: "first", Args: []string{"a"}},
		{Text: "second", Args: []string{"a"}},
		{Text: "third", Args: []string{"a"}},
	}}}, served)

	if _, ok := srv.cache.Get(warmKey(t, srv, "test", sha, "first", []string{"a"})); !ok {
		t.Fatal("the first payload should have warmed")
	}
	if _, ok := srv.cache.Get(warmKey(t, srv, "test", sha, "second", []string{"a"})); ok {
		t.Fatal("the warm should have stopped before the second payload")
	}
}

// TestWarmScriptsContinuesAfterFailure verifies a failing payload is skipped
// without stopping the rest, so a drifted declaration degrades to a cold call.
func TestWarmScriptsContinuesAfterFailure(t *testing.T) {
	_, srv := serveScriptTest(t, "1GB")
	sha, err := srv.PreloadScript("test", `if KEYS[1] == "bad" then error("boom") end return KEYS[1]`)
	if err != nil {
		t.Fatal(err)
	}

	<-srv.WarmScripts([]ScriptWarmup{{Model: "test", SHA: sha, Payloads: []WarmPayload{
		{Text: "bad", Args: []string{"a"}},
		{Text: "good", Args: []string{"a"}},
	}}})

	if _, ok := srv.cache.Get(warmKey(t, srv, "test", sha, "bad", []string{"a"})); ok {
		t.Fatal("a failed payload should not be cached")
	}
	if _, ok := srv.cache.Get(warmKey(t, srv, "test", sha, "good", []string{"a"})); !ok {
		t.Fatal("a failing payload must not stop the remaining payloads")
	}
}
