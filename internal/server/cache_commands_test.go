package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/registry"
)

func TestCacheFlushCommand(t *testing.T) {
	addr := serveTestWithCache(t, "1MB")
	redisCmd(t, addr, "EMB", "test", "one", "two")
	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH", "test"); got.kind != "int" || got.val.(int) != 2 {
		t.Fatalf("scoped flush = %#v, want integer 2", got)
	}
	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH", "missing"); !strings.Contains(errorOf(t, got), "not found") {
		t.Fatalf("unknown model error = %#v", got)
	}
	redisCmd(t, addr, "EMB", "test", "three")
	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH"); got.kind != "int" || got.val.(int) != 1 {
		t.Fatalf("whole flush = %#v, want integer 1", got)
	}
	if got := errorOf(t, redisCmd(t, addr, "EMB.CACHE.FLUSH", "test", "extra")); !strings.Contains(got, "wrong number") {
		t.Fatalf("arity error = %q", got)
	}
}

func TestCacheFlushDisabledIsIdempotent(t *testing.T) {
	addr := serveTest(t)
	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH"); got.kind != "int" || got.val.(int) != 0 {
		t.Fatalf("disabled flush = %#v, want integer 0", got)
	}
}

func TestSaveCommandIsAsyncAndCommandsRemainAvailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.embcache")
	addr, srv := serveTestWithCacheOptions(t, "1MB", WithPersistence(PersistenceConfig{
		File: path, Load: false, SaveOnShutdown: false,
	}))
	srv.SetReady()
	redisCmd(t, addr, "EMB", "test", "before")

	captured := make(chan struct{})
	release := make(chan struct{})
	srv.snapshot.write = func(ctx context.Context, _ string, snap CacheSnapshot, _ map[string]registry.ModelFingerprint, _ int64) (snapshotWriteResult, error) {
		close(captured)
		select {
		case <-release:
			return snapshotWriteResult{Entries: len(snap.Entries)}, nil
		case <-ctx.Done():
			return snapshotWriteResult{}, ctx.Err()
		}
	}
	if got := redisCmd(t, addr, "EMB.SAVE"); got.kind != "status" || got.val != "OK" {
		t.Fatalf("EMB.SAVE = %#v", got)
	}
	<-captured
	if got := errorOf(t, redisCmd(t, addr, "EMB.SAVE")); !strings.Contains(got, "already in progress") {
		t.Fatalf("overlap error = %q", got)
	}

	// The post-capture save is still blocked, but control, inference and cache
	// administration commands all remain usable.
	if got := redisCmd(t, addr, "PING"); got.kind != "status" || got.val != "PONG" {
		t.Fatalf("PING during save = %#v", got)
	}
	if got := redisCmd(t, addr, "EMB", "test", "during"); got.kind != "bulk" {
		t.Fatalf("EMB during save = %#v", got)
	}
	if got := redisCmd(t, addr, "EMB.CACHE.FLUSH", "test"); got.kind != "int" || got.val.(int) < 1 {
		t.Fatalf("flush during save = %#v", got)
	}
	info := bulkOf(t, redisCmd(t, addr, "INFO", "cache"))
	if !strings.Contains(info, "cache_snapshot_in_progress:true") {
		t.Fatalf("INFO omitted active snapshot:\n%s", info)
	}
	stats := arrayOf(t, redisCmd(t, addr, "EMB.STATS"))
	if len(stats) != 88 {
		t.Fatalf("EMB.STATS length = %d, want 88", len(stats))
	}

	close(release)
	select {
	case <-srv.snapshot.currentDone:
	case <-time.After(time.Second):
		t.Fatal("snapshot did not finish")
	}
	if got := errorOf(t, redisCmd(t, addr, "EMB.SAVE", "extra")); !strings.Contains(got, "wrong number") {
		t.Fatalf("arity error = %q", got)
	}
}

func TestSaveDisabledAndAuthentication(t *testing.T) {
	addr := serveTest(t)
	if got := errorOf(t, redisCmd(t, addr, "EMB.SAVE")); !strings.Contains(got, "disabled") {
		t.Fatalf("disabled save error = %q", got)
	}

	authAddr := serveTestWithAuth(t, "secret")
	c := dial(t, authAddr)
	c.Write(respCommand("EMB.CACHE.FLUSH"))
	if got := readRESP(t, c); !strings.HasPrefix(got, "-NOAUTH") {
		t.Fatalf("flush without auth = %q", got)
	}
	c.Write(respCommand("EMB.SAVE"))
	if got := readRESP(t, c); !strings.HasPrefix(got, "-NOAUTH") {
		t.Fatalf("save without auth = %q", got)
	}
}
