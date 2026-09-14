package server

import (
	"strings"
	"testing"
)

const (
	embedOneScript   = `return emb.math.float32_bytes(emb.embed(KEYS[1]))`
	embedPackScript  = `return emb.embed(KEYS[1], {bytes = true})`
	embedBatchScript = `return emb.math.float32_bytes(emb.embed({KEYS[1], ARGV[1]})[2])`
)

// TestEmbEmbedMatchesNativePath verifies the three emb.embed forms return
// exactly the vector EMB returns for the same text.
func TestEmbEmbedMatchesNativePath(t *testing.T) {
	addr, _ := serveScriptTest(t, "")

	// The packed byte form is directly comparable to the EMB reply.
	want := bulkOf(t, redisCmd(t, addr, "EMB", "test", "hello world"))

	for name, script := range map[string]string{
		"array form rescoped": embedOneScript,
		"packed form":         embedPackScript,
	} {
		got := bulkOf(t, redisCmd(t, addr, "EMB.EVAL", "test", script, "1", "hello world"))
		if got != want {
			t.Fatalf("%s: emb.embed vector differs from EMB reply (%d vs %d bytes)", name, len(got), len(want))
		}
	}

	// Batch form: the second vector must equal EMB of the second text.
	second := bulkOf(t, redisCmd(t, addr, "EMB", "test", "second text"))
	got := bulkOf(t, redisCmd(t, addr, "EMB.EVAL", "test", embedBatchScript, "1", "first text", "second text"))
	if got != second {
		t.Fatalf("batch emb.embed second vector differs from EMB reply")
	}
}

// TestEmbEmbedUnavailableForNonEmbeddableModel verifies the conditional
// binding: a model without an embedding configuration has no emb.embed.
func TestEmbEmbedUnavailableForNonEmbeddableModel(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	got := errorOf(t, redisCmd(t, addr, "EMB.EVAL", "block", `return emb.embed("x")`, "1", "x"))
	if !strings.Contains(got, "unavailable") {
		t.Fatalf("emb.embed on non-embeddable model = %q, want unavailable error", got)
	}
}

// TestEmbEmbedSharesEmbeddingCache verifies a text embedded by EMB is a cache
// hit for emb.embed, so no second inference runs.
func TestEmbEmbedSharesEmbeddingCache(t *testing.T) {
	addr, _ := serveScriptTest(t, "1GB")

	if got := bulkOf(t, redisCmd(t, addr, "EMB", "test", "shared text")); len(got) == 0 {
		t.Fatal("EMB reply empty")
	}
	before := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	beforeReq := intOf(t, before["requests"])

	got := bulkOf(t, redisCmd(t, addr, "EMB.EVAL", "test", embedOneScript, "1", "shared text"))
	if len(got) == 0 {
		t.Fatal("emb.embed reply empty")
	}

	after := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if afterReq := intOf(t, after["requests"]); afterReq != beforeReq {
		t.Fatalf("emb.embed ran inference on a cached text: requests %d -> %d", beforeReq, afterReq)
	}
}

// TestEmbEmbedOpensNoScriptSessions verifies the laziness requirement: an
// emb.embed-only script never opens the model's named-tensor session pool.
func TestEmbEmbedOpensNoScriptSessions(t *testing.T) {
	addr, _ := serveScriptTest(t, "")
	if got := bulkOf(t, redisCmd(t, addr, "EMB.EVAL", "test", embedPackScript, "1", "lazy")); len(got) == 0 {
		t.Fatal("emb.embed reply empty")
	}
	info := statsFields(t, redisCmd(t, addr, "EMB.INFO", "test"))
	if got := intOf(t, info["script_sessions"]); got != 0 {
		t.Fatalf("emb.embed-only script opened %d script sessions, want 0", got)
	}
}

// TestEmbEmbedRespectsTextCap verifies the maxTexts guard.
func TestEmbEmbedRespectsTextCap(t *testing.T) {
	addr, _ := serveScriptTest(t, "", WithMaxTexts(2))
	script := `local t = {}
for i = 1, 4 do t[i] = "text" .. i end
return emb.embed(t)`
	got := errorOf(t, redisCmd(t, addr, "EMB.EVAL", "test", script, "1", "x"))
	if !strings.Contains(got, "too many texts") {
		t.Fatalf("emb.embed over text cap = %q, want too-many-texts error", got)
	}
}

// TestEmbEmbedIsUsableFromScriptCache verifies the reply cache still applies to
// scripts that use emb.embed (the host is pure compute).
func TestEmbEmbedIsUsableFromScriptCache(t *testing.T) {
	addr, _ := serveScriptTest(t, "1GB")
	sha := bulkOf(t, redisCmd(t, addr, "EMB.SCRIPT", "LOAD", "test", embedPackScript))

	first := bulkOf(t, redisCmd(t, addr, "EMB.EVSHA", "test", sha, "1", "cache me"))
	second := bulkOf(t, redisCmd(t, addr, "EMB.EVSHA", "test", sha, "1", "cache me"))
	if first == "" || first != second {
		t.Fatalf("cached emb.embed reply mismatch: %d vs %d bytes", len(first), len(second))
	}
	if want := bulkOf(t, redisCmd(t, addr, "EMB", "test", "cache me")); first != want {
		t.Fatalf("cached emb.embed reply differs from EMB: %d vs %d bytes", len(first), len(want))
	}
}
