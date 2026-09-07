package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/registry"
)

func lifecycleRegistry() *registry.Registry {
	reg := registry.New()
	reg.Add("test", &registry.ModelEntry{Name: "test", Dim: 4})
	return reg
}

func makeLifecycleSnapshot(t *testing.T, path string) {
	t.Helper()
	cache := NewCache(1 << 20)
	cache.Set("test:most-recent", make([]byte, 16))
	cache.Set("test:least-recent", make([]byte, 16))
	reg := lifecycleRegistry()
	models, err := reg.Fingerprints()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), models, 0); err != nil {
		t.Fatal(err)
	}
}

func TestServerSnapshotRestoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.embcache")
	makeLifecycleSnapshot(t, path)

	t.Run("valid", func(t *testing.T) {
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: true, RestoreLimit: "auto", RestoreReserve: "1MB",
		}))
		defer s.Close()
		// The "test" model is never loaded at boot, so its restored records
		// stay quarantined: nothing is published to the live cache yet.
		if got := s.cache.Stats().Entries; got != 0 {
			t.Fatalf("live cache entries = %d, want 0 (quarantined)", got)
		}
		if loaded, _ := s.reg.ModelsLoaded(); loaded != 0 {
			t.Fatalf("restore loaded %d lazy model sessions", loaded)
		}
		if status := s.snapshot.Status(); status.RestoreError != "" || status.RestoredEntries != 0 || status.QuarantinedEntries != 2 {
			t.Fatalf("restore status = %#v", status)
		}
	})

	t.Run("load disabled", func(t *testing.T) {
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{File: path, Load: false}))
		defer s.Close()
		if got := s.cache.Stats().Entries; got != 0 {
			t.Fatalf("load-disabled entries = %d", got)
		}
	})

	t.Run("missing is normal", func(t *testing.T) {
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: filepath.Join(t.TempDir(), "missing"), Load: true,
		}))
		defer s.Close()
		if status := s.snapshot.Status(); status.RestoreError != "" || status.RestoredEntries != 0 {
			t.Fatalf("missing restore status = %#v", status)
		}
	})

	t.Run("corrupt stays unpublished", func(t *testing.T) {
		corrupt := filepath.Join(t.TempDir(), "corrupt")
		if err := os.WriteFile(corrupt, []byte("broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{File: corrupt, Load: true}))
		defer s.Close()
		if got := s.cache.Stats().Entries; got != 0 {
			t.Fatalf("corrupt restore published %d entries", got)
		}
		if s.snapshot.Status().RestoreError == "" {
			t.Fatal("corrupt restore error was not observable")
		}
	})

	t.Run("memory ceiling", func(t *testing.T) {
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: true, RestoreLimit: "80B", RestoreReserve: "1MB",
		}))
		defer s.Close()
		status := s.snapshot.Status()
		if status.RestoredEntries != 0 || status.QuarantinedEntries != 1 || status.SkippedMemory != 1 {
			t.Fatalf("bounded restore status = %#v", status)
		}
	})
}

func TestServerSnapshotQuarantineAdmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.embcache")
	makeLifecycleSnapshot(t, path)
	addr, srv := serveTestWithCacheOptions(t, "1MB", WithPersistence(PersistenceConfig{
		File: path, Load: true, RestoreLimit: "auto", RestoreReserve: "1MB",
	}))
	srv.SetReady()

	// Boot: records for the never-loaded "test" model stay quarantined; the
	// live cache must not serve them before the model loads.
	if got := srv.cache.Stats().Entries; got != 0 {
		t.Fatalf("live cache entries = %d, want 0 before first load", got)
	}
	if status := srv.snapshot.Status(); status.RestoreError != "" || status.QuarantinedEntries != 2 {
		t.Fatalf("restore status = %#v", status)
	}

	// The first request loads the model, verifies the stored fingerprint, and
	// admits the two quarantined records before embedding the miss.
	if got := redisCmd(t, addr, "EMB", "test", "hello"); got.kind != "bulk" {
		t.Fatalf("EMB = %#v", got)
	}
	if got := srv.cache.Stats().Entries; got != 3 {
		t.Fatalf("cache entries after admission = %d, want 3 (2 restored + 1 miss)", got)
	}
	srv.quarantineMu.Lock()
	pending := len(srv.quarantine)
	srv.quarantineMu.Unlock()
	if pending != 0 {
		t.Fatalf("quarantine not consumed: %d buckets remain", pending)
	}

	// A restored record now serves as a plain cache hit.
	if got := redisCmd(t, addr, "EMB", "test", "most-recent"); got.kind != "bulk" {
		t.Fatalf("restored hit = %#v", got)
	}
	if srv.cache.Stats().Hits != 1 {
		t.Fatalf("restored hit was not served from cache")
	}
}

func TestShutdownSavesOnlyDirtyCache(t *testing.T) {
	t.Run("dirty", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cache.embcache")
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: false, SaveOnShutdown: true,
		}))
		s.cache.Set("test:dirty", make([]byte, 16))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil && err.Error() != "not serving" {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dirty shutdown snapshot: %v", err)
		}
	})

	t.Run("clean", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cache.embcache")
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: false, SaveOnShutdown: true,
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil && err.Error() != "not serving" {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("clean shutdown wrote snapshot: %v", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cache.embcache")
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: false, SaveOnShutdown: false,
		}))
		s.cache.Set("test:dirty", make([]byte, 16))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("disabled shutdown wrote snapshot: %v", err)
		}
	})

	t.Run("save failure is observable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cache.embcache")
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: false, SaveOnShutdown: true,
		}))
		s.cache.Set("test:dirty", make([]byte, 16))
		s.snapshot.write = func(context.Context, string, CacheSnapshot, map[string]registry.ModelFingerprint, int64) (snapshotWriteResult, error) {
			return snapshotWriteResult{}, errors.New("injected save failure")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
		if status := s.snapshot.Status(); status.Failures != 1 || status.Successes != 0 {
			t.Fatalf("failed shutdown status = %#v", status)
		}
	})

	t.Run("forced timeout cancels writer", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cache.embcache")
		s := New("", lifecycleRegistry(), "", "1MB", nil, WithPersistence(PersistenceConfig{
			File: path, Load: false, SaveOnShutdown: true,
		}))
		s.cache.Set("test:dirty", make([]byte, 16))
		cancelled := make(chan struct{})
		s.snapshot.write = func(ctx context.Context, _ string, _ CacheSnapshot, _ map[string]registry.ModelFingerprint, _ int64) (snapshotWriteResult, error) {
			<-ctx.Done()
			close(cancelled)
			return snapshotWriteResult{}, ctx.Err()
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = s.Shutdown(ctx)
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("shutdown timeout did not cancel snapshot writer")
		}
	})
}
