package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type faultSnapshotFS struct {
	fail string
	osSnapshotFS
}

func (f faultSnapshotFS) CreateTemp(dir, pattern string) (snapshotFile, error) {
	if f.fail == "create" {
		return nil, errors.New("injected create failure")
	}
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &faultSnapshotFile{File: file, fail: f.fail, temp: true}, nil
}

func (f faultSnapshotFS) Open(path string) (snapshotFile, error) {
	if f.fail == "directory-open" {
		return nil, errors.New("injected directory open failure")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &faultSnapshotFile{File: file, fail: f.fail}, nil
}

func (f faultSnapshotFS) Rename(oldPath, newPath string) error {
	if f.fail == "rename" && filepath.Ext(oldPath) != ".previous" {
		return errors.New("injected rename failure")
	}
	return os.Rename(oldPath, newPath)
}

type faultSnapshotFile struct {
	*os.File
	fail string
	temp bool
}

func (f *faultSnapshotFile) Write(p []byte) (int, error) {
	if f.temp && f.fail == "write" {
		return 0, errors.New("injected write failure")
	}
	if f.temp && f.fail == "short-write" && len(p) > 1 {
		return f.File.Write(p[:len(p)-1])
	}
	return f.File.Write(p)
}

func (f *faultSnapshotFile) Chmod(mode os.FileMode) error {
	if f.temp && f.fail == "chmod" {
		return errors.New("injected chmod failure")
	}
	return f.File.Chmod(mode)
}

func (f *faultSnapshotFile) Sync() error {
	if f.temp && f.fail == "file-sync" {
		return errors.New("injected file sync failure")
	}
	if !f.temp && f.fail == "directory-sync" {
		return errors.New("injected directory sync failure")
	}
	return f.File.Sync()
}

func (f *faultSnapshotFile) Close() error {
	err := f.File.Close()
	if f.temp && f.fail == "close" {
		return errors.New("injected close failure")
	}
	if !f.temp && f.fail == "directory-close" {
		return errors.New("injected directory close failure")
	}
	return err
}

func TestSnapshotFilesystemFaultsPreservePreviousAndCleanup(t *testing.T) {
	tests := []string{"create", "chmod", "write", "short-write", "file-sync", "close", "rename", "directory-open", "directory-sync", "directory-close"}
	for _, failure := range tests {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "cache.embcache")
			previous := []byte("previous-snapshot")
			if err := os.WriteFile(path, previous, 0o600); err != nil {
				t.Fatal(err)
			}
			cache := NewCache(1024)
			cache.Set("alpha:key", make([]byte, 8))
			_, err := writeSnapshotFS(context.Background(), path, cache.Snapshot(), testFingerprints(), 0, faultSnapshotFS{fail: failure})
			if err == nil {
				t.Fatal("injected filesystem fault did not fail")
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("previous snapshot disappeared: %v", readErr)
			}
			if !reflect.DeepEqual(got, previous) {
				t.Fatalf("previous snapshot changed: %q", got)
			}
			matches, globErr := filepath.Glob(filepath.Join(dir, ".cache.embcache.tmp-*"))
			if globErr != nil {
				t.Fatal(globErr)
			}
			if len(matches) != 0 {
				t.Fatalf("temporary files leaked: %v", matches)
			}
		})
	}
}

func TestRateWriterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &rateWriter{w: io.Discard, limit: 1, started: time.Now(), ctx: ctx, now: time.Now, wait: waitContext}
	if _, err := w.Write(make([]byte, 1024)); !errors.Is(err, context.Canceled) {
		t.Fatalf("rate writer cancellation = %v", err)
	}
}

func TestRateWriterUsesInjectedClockOutsideCache(t *testing.T) {
	started := time.Unix(100, 0)
	var waited time.Duration
	w := &rateWriter{
		w: io.Discard, limit: 100, started: started, ctx: context.Background(),
		now:  func() time.Time { return started },
		wait: func(_ context.Context, d time.Duration) error { waited = d; return nil },
	}
	if _, err := w.Write(make([]byte, 50)); err != nil {
		t.Fatal(err)
	}
	if waited != 500*time.Millisecond {
		t.Fatalf("injected throttle delay = %v", waited)
	}
}
