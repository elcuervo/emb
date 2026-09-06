package server

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/registry"
)

type fakeCoordinatorTimer struct {
	ch chan time.Time
}

func (t *fakeCoordinatorTimer) Chan() <-chan time.Time { return t.ch }
func (t *fakeCoordinatorTimer) Stop() bool             { return true }

func TestSnapshotCoordinatorSingleFlightDirtyAndAvailability(t *testing.T) {
	cache := NewCache(1 << 20)
	cache.Set("test:before", make([]byte, 16))
	reg := registry.New()
	reg.Add("test", &registry.ModelEntry{Name: "test", Dim: 4})
	c := newSnapshotCoordinator(cache, reg, PersistenceConfig{
		File: filepath.Join(t.TempDir(), "cache.embcache"), SaveOnShutdown: true,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		c.Shutdown(ctx)
	})

	captured := make(chan CacheSnapshot, 1)
	release := make(chan struct{})
	c.write = func(ctx context.Context, _ string, snapshot CacheSnapshot, _ map[string]registry.ModelFingerprint, _ int64) (snapshotWriteResult, error) {
		captured <- snapshot
		select {
		case <-release:
			return snapshotWriteResult{Entries: len(snapshot.Entries), Bytes: snapshot.CurBytes}, nil
		case <-ctx.Done():
			return snapshotWriteResult{}, ctx.Err()
		}
	}

	// periodicTick is the exact path used by the automatic timer.
	c.periodicTick()
	snapshot := <-captured
	if !c.Status().InProgress {
		t.Fatal("automatic snapshot did not enter in-progress state")
	}
	if err := c.Save(context.Background()); !errors.Is(err, errSnapshotInProgress) {
		t.Fatalf("overlapping save error = %v", err)
	}

	// Encoding and storage are gated. Cache operations must still complete;
	// this also proves the snapshot's post-capture work owns no cache lock.
	operationsDone := make(chan struct{})
	go func() {
		if _, ok := cache.Get("test:before"); !ok {
			t.Errorf("cache read failed during snapshot")
		}
		cache.Set("test:during", make([]byte, 16))
		if removed := cache.FlushModel("missing"); removed != 0 {
			t.Errorf("unexpected scoped flush count %d", removed)
		}
		close(operationsDone)
	}()
	select {
	case <-operationsDone:
	case <-time.After(time.Second):
		t.Fatal("cache operations waited for post-capture snapshot work")
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("captured snapshot changed after concurrent mutation: %#v", snapshot.Entries)
	}

	close(release)
	select {
	case <-c.currentDone:
	case <-time.After(time.Second):
		t.Fatal("snapshot did not finish")
	}
	status := c.Status()
	if status.InProgress || status.Successes != 1 || status.LastEntries != 1 {
		t.Fatalf("unexpected completed status: %#v", status)
	}
	// The mutation during save makes the cache dirty relative to the captured
	// generation, so another automatic tick starts another save.
	capturedAgain := make(chan CacheSnapshot, 1)
	c.write = func(_ context.Context, _ string, snapshot CacheSnapshot, _ map[string]registry.ModelFingerprint, _ int64) (snapshotWriteResult, error) {
		capturedAgain <- snapshot
		return snapshotWriteResult{Entries: len(snapshot.Entries)}, nil
	}
	c.periodicTick()
	select {
	case <-capturedAgain:
	case <-time.After(time.Second):
		t.Fatal("dirty cache did not trigger the next automatic save")
	}
	select {
	case <-c.currentDone:
	case <-time.After(time.Second):
		t.Fatal("second snapshot did not finish")
	}
	skipped := c.Status().Skipped
	c.periodicTick()
	if c.Status().Skipped != skipped+1 {
		t.Fatal("clean automatic tick was not coalesced")
	}
}

func TestSnapshotCoordinatorDisabledIsDormant(t *testing.T) {
	cache := NewCache(1024)
	reg := registry.New()
	// Server construction, rather than the coordinator constructor, owns the
	// dormant guarantee when cache_file is empty.
	s := New("", reg, "", "1KB", nil, WithPersistence(PersistenceConfig{Load: true, SaveOnShutdown: true}))
	if s.snapshot != nil {
		t.Fatal("empty cache_file created a snapshot coordinator")
	}
	if cache.Stats().Generation != 0 {
		t.Fatal("unrelated cache mutated")
	}
}

func TestSnapshotCoordinatorLiveTimerAndRateReplacement(t *testing.T) {
	cache := NewCache(1024)
	cache.Set("test:key", make([]byte, 16))
	reg := registry.New()
	reg.Add("test", &registry.ModelEntry{Name: "test", Dim: 4})
	c := newSnapshotCoordinator(cache, reg, PersistenceConfig{File: "snapshot"})
	t.Cleanup(c.Close)

	created := make(chan struct {
		duration time.Duration
		timer    *fakeCoordinatorTimer
	}, 4)
	c.newTimer = func(d time.Duration) coordinatorTimer {
		timer := &fakeCoordinatorTimer{ch: make(chan time.Time, 1)}
		created <- struct {
			duration time.Duration
			timer    *fakeCoordinatorTimer
		}{d, timer}
		return timer
	}
	writes := make(chan int64, 1)
	c.write = func(_ context.Context, _ string, snapshot CacheSnapshot, _ map[string]registry.ModelFingerprint, rate int64) (snapshotWriteResult, error) {
		writes <- rate
		return snapshotWriteResult{Entries: len(snapshot.Entries)}, nil
	}

	c.Configure("snapshot", time.Minute, false, 1234)
	first := <-created
	if first.duration != time.Minute {
		t.Fatalf("first timer = %v", first.duration)
	}
	first.timer.ch <- time.Now()
	if rate := <-writes; rate != 1234 {
		t.Fatalf("live rate = %d", rate)
	}
	select {
	case <-c.currentDone:
	case <-time.After(time.Second):
		t.Fatal("automatic save did not complete")
	}
	// A new interval replaces the timer rather than adding another scheduler.
	c.Configure("snapshot", 2*time.Minute, false, 5678)
	for {
		next := <-created
		if next.duration == 2*time.Minute {
			break
		}
	}
	c.mu.RLock()
	rate := c.rateBytes
	c.mu.RUnlock()
	if rate != 5678 {
		t.Fatalf("replacement rate = %d", rate)
	}
}

func TestEffectiveRestoreLimitUsesSmallestCeiling(t *testing.T) {
	limit, _, headroom, err := effectiveRestoreLimit(1<<30, "64MB", "1MB")
	if err != nil {
		t.Fatal(err)
	}
	if limit <= 0 || limit > 64_000_000 {
		t.Fatalf("effective limit = %d", limit)
	}
	if headroom >= 0 && limit > headroom {
		t.Fatalf("effective limit %d exceeds host headroom %d", limit, headroom)
	}
}

func TestEffectiveRestoreLimitFallbackWithoutHostMetrics(t *testing.T) {
	limit, rss, headroom, err := effectiveRestoreLimitForMetrics(128_000_000, "auto", "10%", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if limit != 128_000_000 || rss != 0 || headroom != -1 {
		t.Fatalf("fallback = limit %d rss %d headroom %d", limit, rss, headroom)
	}
}

func TestSnapshotCoordinatorEnableDisableRaceAndCleanup(t *testing.T) {
	cache := NewCache(1024)
	reg := registry.New()
	c := newSnapshotCoordinator(cache, reg, PersistenceConfig{File: "snapshot"})
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				c.Configure("", 0, false, int64(i))
			} else {
				c.Configure("snapshot", 0, true, int64(i))
			}
			_ = c.Status()
		}()
	}
	wg.Wait()
	c.Close()
	select {
	case <-c.done:
	default:
		t.Fatal("coordinator scheduler did not exit")
	}
}
