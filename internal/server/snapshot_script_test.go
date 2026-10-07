package server

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/registry"
	"github.com/elcuervo/emb/internal/script"
)

// opaqueKey builds a script-reply cache key for a model: the shape modelOf
// recognizes as "<model>:<sha1>:<sha256>:<text>".
func opaqueKey(model, text string) string {
	return model + ":" + strings.Repeat("a", sha1HexLen) + ":" + strings.Repeat("b", sha256HexLen) + ":" + text
}

// TestSnapshotRestoresEntriesByKeyFamily verifies the restore validation branch:
// an opaque script reply is gated by fingerprint alone, while an embedding value
// still has to match the model's dimension.
func TestSnapshotRestoresEntriesByKeyFamily(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "family.embcache")

	scriptKey := opaqueKey("laya", "state")
	goodText := textCacheKey("emb", "hello")
	shortText := textCacheKey("emb", "oops")

	cache := NewCache(1 << 20)
	cache.Set(scriptKey, []byte(`{"answers":{"category":{"choice":"billing"}}}`))
	cache.Set(goodText, make([]byte, 8))  // emb dim 2 -> 8 bytes
	cache.Set(shortText, make([]byte, 7)) // wrong length for emb

	models := map[string]registry.ModelFingerprint{
		"laya": {Fingerprint: "laya-fp", Dim: -1, Loaded: true},
		"emb":  {Fingerprint: "emb-fp", Dim: 2, Loaded: true},
	}
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), models, 0); err != nil {
		t.Fatal(err)
	}

	result, err := readSnapshot(path, 1<<20, models)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 2 {
		t.Fatalf("restored = %d, want 2 (script reply + good embedding): %#v", result.Restored, result)
	}
	if result.SkippedFingerprint != 1 {
		t.Fatalf("skipped = %d, want 1 (wrong-length embedding): %#v", result.SkippedFingerprint, result)
	}
	if got, ok := result.Cache.Get(scriptKey); !ok || !strings.Contains(string(got), "billing") {
		t.Fatalf("script reply not restored: %q ok=%v", got, ok)
	}
	if _, ok := result.Cache.Get(shortText); ok {
		t.Fatal("a wrong-length embedding must not be restored")
	}
}

// TestSnapshotQuarantinesAndAdmitsScriptReplies verifies the lazy-model path: a
// script reply for a model that has not loaded is quarantined, then published
// when the model's fingerprint is verified — the case a Dim=-1 model previously
// lost at the dimension check.
func TestSnapshotQuarantinesAndAdmitsScriptReplies(t *testing.T) {
	entry := &registry.ModelEntry{Name: "laya", Dim: -1}
	fp, err := entry.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "lazy.embcache")
	key := script.CacheKey("laya", strings.Repeat("a", sha1HexLen), nil, 1, "state")
	value := []byte(`{"answers":{}}`)
	snap := CacheSnapshot{Entries: []CacheSnapshotEntry{{Key: key, Value: value}}}
	stored := map[string]registry.ModelFingerprint{"laya": {Fingerprint: fp, Dim: -1, Loaded: true}}
	if _, err := writeSnapshot(context.Background(), path, snap, stored, 0); err != nil {
		t.Fatal(err)
	}

	current := map[string]registry.ModelFingerprint{"laya": {Dim: -1, Loaded: false}}
	result, err := readSnapshot(path, 1<<20, current)
	if err != nil {
		t.Fatal(err)
	}
	if result.QuarantinedCount != 1 || result.Restored != 0 {
		t.Fatalf("restore counts = %#v, want 1 quarantined / 0 restored", result)
	}

	srv := &Server{cache: result.Cache, quarantine: result.Quarantine, quarantineBytes: result.QuarantineBytes}
	srv.admitQuarantine("laya", entry)
	if got, ok := result.Cache.Get(key); !ok || string(got) != string(value) {
		t.Fatalf("admitted value = %q ok=%v, want %q", got, ok, value)
	}
}

// TestScriptEvaluationAdmitsQuarantineBeforeLookup verifies the request path: a
// quarantined reply for a model whose embedding pool never loads is served on
// the first scripted evaluation without running the script.
func TestScriptEvaluationAdmitsQuarantineBeforeLookup(t *testing.T) {
	reg := registry.New()
	entry := &registry.ModelEntry{Name: "m", Dim: -1}
	reg.Add("m", entry)

	srv := New(getFreeAddr(), reg, "", "1GB", nil)
	const src = `return "fresh"`
	sha, err := srv.PreloadScript("m", src)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := srv.scripts.Get("m", sha)
	if !ok {
		t.Fatal("script not cached")
	}
	fp, err := entry.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}

	// The restored reply differs from what the script returns, so serving it can
	// only come from admission.
	restored := []byte("$8\r\nrestored\r\n")
	key := script.CacheKeyConfig("m", sha, nil, 1, "state", loaded.digest)
	path := filepath.Join(t.TempDir(), "current.embcache")
	cache := NewCache(1 << 20)
	cache.Set(key, restored)
	stored := map[string]registry.ModelFingerprint{"m": {Fingerprint: fp, Dim: -1, Loaded: true}}
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), stored, 0); err != nil {
		t.Fatal(err)
	}
	result, err := readSnapshot(path, 1<<20, map[string]registry.ModelFingerprint{"m": {Dim: -1, Loaded: false}})
	if err != nil {
		t.Fatal(err)
	}
	if result.QuarantinedCount != 1 {
		t.Fatalf("expected lazy restore: %+v", result)
	}
	srv.cache, srv.quarantine, srv.quarantineBytes = result.Cache, result.Quarantine, result.QuarantineBytes

	replies, err := srv.evalScripted("m", src, sha, loaded.config, loaded.digest, []string{"state"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 1 || string(replies[0]) != string(restored) {
		t.Fatalf("reply = %q, want the restored reply %q", replies, restored)
	}
}

// TestScriptEvaluationWithoutPersistence verifies admission is a no-op when no
// snapshot was restored, so an ordinary server is unaffected.
func TestScriptEvaluationWithoutPersistence(t *testing.T) {
	reg := registry.New()
	reg.Add("m", &registry.ModelEntry{Name: "m", Dim: -1})
	srv := New(getFreeAddr(), reg, "", "1GB", nil)
	sha, err := srv.PreloadScript("m", `return "fresh"`)
	if err != nil {
		t.Fatal(err)
	}
	replies, err := srv.evalScripted("m", `return "fresh"`, sha, nil, "", []string{"state"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 1 || string(replies[0]) != "$5\r\nfresh\r\n" {
		t.Fatalf("reply = %q, want the script's own value", replies)
	}
}

func TestSnapshotLegacyScriptIdentity(t *testing.T) {
	addr, srv := serveTestWithCacheOptions(t, "1MB")
	imageAddr, imageSrv, _ := serveImage(t, "1MB")
	const src = `return KEYS[1]`
	sha, err := srv.PreloadScript("test", src)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit legacy metadata fixture: API 1.3.0, one text, no arguments.
	const legacyMetadata = "916b0b7d3e5eba509cc49f1c31e93025ec144ac6963e10909b98d6486f36efad"
	legacyKey := "test:" + sha + ":" + legacyMetadata + ":hello"
	textKey := textCacheKey("test", "hello")
	imageKey := imageCacheKey("imgA", []byte("image"))
	textValue, imageValue := bytes.Repeat([]byte{7}, 16), bytes.Repeat([]byte{8}, testImageDim*4)
	cache := NewCache(1 << 20)
	cache.Set(legacyKey, []byte("$5\r\nwrong\r\n"))
	cache.Set(textKey, textValue)
	cache.Set(imageKey, imageValue)
	models := map[string]registry.ModelFingerprint{}
	for model, reg := range map[string]*registry.Registry{"test": srv.reg, "imgA": imageSrv.reg} {
		entry, err := reg.Resolve(model)
		if err != nil {
			t.Fatal(err)
		}
		fp, err := entry.Fingerprint()
		if err != nil {
			t.Fatal(err)
		}
		models[model] = registry.ModelFingerprint{Fingerprint: fp, Dim: entry.Dim, Loaded: true}
	}
	path := filepath.Join(t.TempDir(), "legacy.embcache")
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), models, 0); err != nil {
		t.Fatal(err)
	}
	result, err := readSnapshot(path, 1<<20, models)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 3 {
		t.Fatalf("restore = %+v", result)
	}
	srv.cache.replaceStorageFrom(result.Cache)
	// Read a second staging cache so the running servers do not share storage.
	imageResult, err := readSnapshot(path, 1<<20, models)
	if err != nil {
		t.Fatal(err)
	}
	imageSrv.cache.replaceStorageFrom(imageResult.Cache)
	before := srv.cache.Stats()
	for range 2 {
		if got := bulkOf(t, redisCmd(t, addr, "EMB.EVSHA", "test", sha, "1", "hello")); got != "hello" {
			t.Fatalf("legacy reply served: %q", got)
		}
	}
	after := srv.cache.Stats()
	if after.Misses != before.Misses+1 || after.Hits != before.Hits+1 {
		t.Fatalf("script counters: %+v -> %+v", before, after)
	}
	if got := bulkOf(t, redisCmd(t, addr, "EMB", "test", "hello")); got != string(textValue) {
		t.Fatal("restored text missed")
	}
	if got := bulkOf(t, redisCmd(t, imageAddr, "EMB.IMG", "imgA", "image")); got != string(imageValue) {
		t.Fatal("restored image missed")
	}
	if srv.cache.Stats().Hits != after.Hits+1 || imageSrv.cache.Stats().Hits != 1 {
		t.Fatal("embedding requests did not hit")
	}
}
