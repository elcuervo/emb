package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/registry"
)

func testFingerprints() map[string]registry.ModelFingerprint {
	// Loaded: true so unit tests exercise the admit-now path (a real server
	// would quarantine entries for unloaded lazy models instead).
	return map[string]registry.ModelFingerprint{
		"alpha": {Fingerprint: "alpha-fingerprint", Dim: 2, Loaded: true},
		"beta":  {Fingerprint: "beta-fingerprint", Dim: 1, Loaded: true},
	}
}

func TestSnapshotRoundTripDeterministicAndOrdered(t *testing.T) {
	cache := NewCache(1 << 20)
	cache.Set("alpha:first", []byte{1, 2, 3, 4, 5, 6, 7, 8})
	cache.Set("beta:\x00unicode-🦊", []byte{9, 10, 11, 12})
	cache.Set("alpha:last", []byte{13, 14, 15, 16, 17, 18, 19, 20})
	snapshot := cache.Snapshot()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.embcache")
	second := filepath.Join(dir, "second.embcache")
	models := testFingerprints()
	if _, err := writeSnapshot(context.Background(), first, snapshot, models, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := writeSnapshot(context.Background(), second, snapshot, models, 0); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("identical snapshots produced different bytes")
	}
	const goldenSHA256 = "34ce4da38c846c08b70f9aee3c0c0fc46ea5af67c239aa6b7cce97b6222c3d7a"
	sum := sha256.Sum256(a)
	if got := hex.EncodeToString(sum[:]); got != goldenSHA256 {
		t.Fatalf("v1 golden SHA-256 = %s, want %s", got, goldenSHA256)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("snapshot permissions = %o, want 600", got)
	}

	restored, err := readSnapshot(first, 1<<20, models)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored != 3 {
		t.Fatalf("restored %d entries, want 3", restored.Restored)
	}
	got := restored.Cache.Snapshot()
	if !reflect.DeepEqual(got.Entries, snapshot.Entries) {
		t.Fatalf("LRU order/value mismatch:\n got %#v\nwant %#v", got.Entries, snapshot.Entries)
	}
}

func TestSnapshotEmptyAndCompatibilityFiltering(t *testing.T) {
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.embcache")
	if _, err := writeSnapshot(context.Background(), emptyPath, NewCache(1024).Snapshot(), testFingerprints(), 0); err != nil {
		t.Fatal(err)
	}
	empty, err := readSnapshot(emptyPath, 1024, testFingerprints())
	if err != nil || empty.Restored != 0 {
		t.Fatalf("empty restore = %#v, %v", empty, err)
	}

	cache := NewCache(1 << 20)
	cache.Set("alpha:ok", make([]byte, 8))
	cache.Set("beta:changed", make([]byte, 4))
	cache.Set("removed:unknown", make([]byte, 4))
	path := filepath.Join(dir, "mixed.embcache")
	stored := testFingerprints()
	stored["removed"] = registry.ModelFingerprint{Fingerprint: "old", Dim: 1}
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), stored, 0); err != nil {
		t.Fatal(err)
	}
	current := testFingerprints()
	// A fingerprint mismatch on a loaded model is skipped, not quarantined:
	// quarantine applies only to models that have not loaded yet.
	current["beta"] = registry.ModelFingerprint{Fingerprint: "new", Dim: 1, Loaded: true}
	result, err := readSnapshot(path, 1<<20, current)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 1 || result.SkippedUnknown != 1 || result.SkippedFingerprint != 1 {
		t.Fatalf("unexpected compatibility counts: %#v", result)
	}
}

func TestSnapshotRejectsCorruptionWithoutPublishing(t *testing.T) {
	cache := NewCache(1024)
	cache.Set("alpha:key", make([]byte, 8))
	path := filepath.Join(t.TempDir(), "cache.embcache")
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), testFingerprints(), 0); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func([]byte) []byte{
		"bad magic":          func(b []byte) []byte { b[0] ^= 0xff; return b },
		"bad version":        func(b []byte) []byte { b[8] = 99; return b },
		"bad representation": func(b []byte) []byte { b[snapshotHeaderSize] = 99; return b },
		"bad checksum":       func(b []byte) []byte { b[len(b)-1] ^= 0xff; return b },
		"truncated":          func(b []byte) []byte { return b[:len(b)-1] },
		"trailing":           func(b []byte) []byte { return append(b, 0) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := append([]byte(nil), original...)
			candidate = mutate(candidate)
			corrupt := filepath.Join(t.TempDir(), "corrupt.embcache")
			if err := os.WriteFile(corrupt, candidate, 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := readSnapshot(corrupt, 1024, testFingerprints())
			if err == nil {
				t.Fatal("corrupt snapshot was accepted")
			}
			if result.Cache.Stats().Entries != 0 && name != "bad checksum" {
				t.Fatalf("corrupt restore exposed %d entries", result.Cache.Stats().Entries)
			}
		})
	}
}

func TestSnapshotRestoreHonorsMemoryCeiling(t *testing.T) {
	cache := NewCache(1024)
	cache.Set("alpha:one", make([]byte, 8))
	cache.Set("alpha:two", make([]byte, 8))
	path := filepath.Join(t.TempDir(), "cache.embcache")
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), testFingerprints(), 0); err != nil {
		t.Fatal(err)
	}
	// One entry costs 48 bytes plus key and value. This ceiling admits one.
	result, err := readSnapshot(path, int64(48+len("alpha:two")+8), testFingerprints())
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 1 || result.SkippedMemory != 1 {
		t.Fatalf("memory admission counts = %#v", result)
	}
}

func TestSnapshotMaximumLegalKeyLength(t *testing.T) {
	key := "beta:" + strings.Repeat("x", maxSnapshotString-len("beta:"))
	cache := NewCache(20 << 20)
	cache.Set(key, []byte{1, 2, 3, 4})
	path := filepath.Join(t.TempDir(), "maximum.embcache")
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), testFingerprints(), 0); err != nil {
		t.Fatal(err)
	}
	result, err := readSnapshot(path, 20<<20, testFingerprints())
	if err != nil || result.Restored != 1 {
		t.Fatalf("maximum-length restore = %#v, %v", result, err)
	}
}

func rewriteSnapshotChecksum(data []byte) {
	sum := sha256.Sum256(data[:len(data)-snapshotChecksumSize])
	copy(data[len(data)-snapshotChecksumSize:], sum[:])
}

func TestSnapshotRejectsBoundedFieldsAndDuplicateKeys(t *testing.T) {
	cache := NewCache(1 << 20)
	cache.Set("alpha:one", make([]byte, 8))
	cache.Set("alpha:two", make([]byte, 8))
	path := filepath.Join(t.TempDir(), "base.embcache")
	if _, err := writeSnapshot(context.Background(), path, cache.Snapshot(), testFingerprints(), 0); err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func([]byte){
		"payload integer overflow": func(data []byte) {
			binary.LittleEndian.PutUint64(data[10:18], ^uint64(0))
		},
		"excessive model count": func(data []byte) {
			binary.LittleEndian.PutUint32(data[snapshotHeaderSize+2:], maxSnapshotModels+1)
			rewriteSnapshotChecksum(data)
		},
		"excessive string length": func(data []byte) {
			binary.LittleEndian.PutUint32(data[snapshotHeaderSize+2+4:], maxSnapshotString+1)
			rewriteSnapshotChecksum(data)
		},
		"duplicate key": func(data []byte) {
			at := bytes.Index(data, []byte("alpha:two"))
			if at < 0 {
				t.Fatal("fixture key not found")
			}
			copy(data[at:at+len("alpha:two")], []byte("alpha:one"))
			rewriteSnapshotChecksum(data)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			data := append([]byte(nil), base...)
			mutate(data)
			candidate := filepath.Join(t.TempDir(), "candidate.embcache")
			if err := os.WriteFile(candidate, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readSnapshot(candidate, 1<<20, testFingerprints()); err == nil {
				t.Fatal("invalid bounded field was accepted")
			}
		})
	}
}

func FuzzReadSnapshot(f *testing.F) {
	f.Add([]byte("not-a-snapshot"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "fuzz.embcache")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = readSnapshot(path, 1<<20, testFingerprints())
	})
}
