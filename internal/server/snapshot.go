package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/elcuervo/emb/internal/registry"
)

var snapshotMagic = [8]byte{'E', 'M', 'B', 'C', 'A', 'C', 'H', 'E'}

const (
	snapshotVersion      = uint16(1)
	snapshotHeaderSize   = 8 + 2 + 8
	snapshotChecksumSize = sha256.Size
	maxSnapshotModels    = 10_000
	maxSnapshotString    = 16 << 20
	snapshotFloatFP32    = byte(1)
	snapshotLittleEndian = byte(1)
)

type snapshotWriteResult struct {
	Entries int
	Bytes   int64
}

type snapshotFile interface {
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

type snapshotFS interface {
	CreateTemp(string, string) (snapshotFile, error)
	Open(string) (snapshotFile, error)
	Rename(string, string) error
	Link(string, string) error
	Remove(string) error
}

type osSnapshotFS struct{}

func (osSnapshotFS) CreateTemp(dir, pattern string) (snapshotFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osSnapshotFS) Open(path string) (snapshotFile, error) { return os.Open(path) }
func (osSnapshotFS) Rename(oldPath, newPath string) error   { return os.Rename(oldPath, newPath) }
func (osSnapshotFS) Link(oldPath, newPath string) error     { return os.Link(oldPath, newPath) }
func (osSnapshotFS) Remove(path string) error               { return os.Remove(path) }

type snapshotRestoreResult struct {
	Cache              *Cache
	Found              bool
	Restored           int64
	SkippedUnknown     int64
	SkippedFingerprint int64
	SkippedMemory      int64
}

type rateWriter struct {
	w       io.Writer
	limit   int64
	started time.Time
	written int64
	ctx     context.Context
	now     func() time.Time
	wait    func(context.Context, time.Duration) error
}

func (w *rateWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.written += int64(n)
	if w.limit > 0 {
		want := time.Duration(float64(w.written) / float64(w.limit) * float64(time.Second))
		if delay := want - w.now().Sub(w.started); delay > 0 {
			if err := w.wait(w.ctx, delay); err != nil {
				return n, err
			}
		}
	}
	return n, err
}

func snapshotPayloadSize(s CacheSnapshot, models map[string]registry.ModelFingerprint) (uint64, error) {
	// Numeric representation + byte order + model count + entry count.
	size := uint64(2 + 4 + 8)
	for name, model := range models {
		size += uint64(4 + len(name) + 4 + len(model.Fingerprint) + 4)
	}
	for _, entry := range s.Entries {
		size += uint64(4 + len(entry.Key) + 4 + len(entry.Value))
		if size > ^uint64(0)-snapshotHeaderSize-snapshotChecksumSize {
			return 0, errors.New("snapshot size overflow")
		}
	}
	return size, nil
}

func writeU16(w io.Writer, v uint16) error { return binary.Write(w, binary.LittleEndian, v) }
func writeU32(w io.Writer, v uint32) error { return binary.Write(w, binary.LittleEndian, v) }
func writeU64(w io.Writer, v uint64) error { return binary.Write(w, binary.LittleEndian, v) }

func writeSized(w io.Writer, value []byte) error {
	if len(value) > int(^uint32(0)) {
		return errors.New("snapshot field too large")
	}
	if err := writeU32(w, uint32(len(value))); err != nil {
		return err
	}
	_, err := w.Write(value)
	return err
}

func writeSnapshot(ctx context.Context, path string, snapshot CacheSnapshot, models map[string]registry.ModelFingerprint, bytesPerSecond int64) (snapshotWriteResult, error) {
	return writeSnapshotFS(ctx, path, snapshot, models, bytesPerSecond, osSnapshotFS{})
}

func writeSnapshotFS(ctx context.Context, path string, snapshot CacheSnapshot, models map[string]registry.ModelFingerprint, bytesPerSecond int64, fs snapshotFS) (snapshotWriteResult, error) {
	var result snapshotWriteResult
	if path == "" {
		return result, errors.New("cache_file is empty")
	}
	payloadSize, err := snapshotPayloadSize(snapshot, models)
	if err != nil {
		return result, err
	}
	dir := filepath.Dir(path)
	tmp, err := fs.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return result, fmt.Errorf("creating snapshot temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = fs.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return result, fmt.Errorf("setting snapshot permissions: %w", err)
	}

	hash := sha256.New()
	rate := &rateWriter{
		w: tmp, limit: bytesPerSecond, started: time.Now(), ctx: ctx,
		now: time.Now, wait: waitContext,
	}
	writer := bufio.NewWriterSize(io.MultiWriter(rate, hash), 64<<10)
	if _, err := writer.Write(snapshotMagic[:]); err != nil {
		return result, err
	}
	if err := writeU16(writer, snapshotVersion); err != nil {
		return result, err
	}
	if err := writeU64(writer, payloadSize); err != nil {
		return result, err
	}
	if _, err := writer.Write([]byte{snapshotFloatFP32, snapshotLittleEndian}); err != nil {
		return result, err
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	if err := writeU32(writer, uint32(len(names))); err != nil {
		return result, err
	}
	for _, name := range names {
		model := models[name]
		if err := writeSized(writer, []byte(name)); err != nil {
			return result, err
		}
		if err := writeSized(writer, []byte(model.Fingerprint)); err != nil {
			return result, err
		}
		if err := writeU32(writer, uint32(model.Dim)); err != nil {
			return result, err
		}
	}
	if err := writeU64(writer, uint64(len(snapshot.Entries))); err != nil {
		return result, err
	}
	for _, entry := range snapshot.Entries {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		if err := writeSized(writer, []byte(entry.Key)); err != nil {
			return result, err
		}
		if err := writeSized(writer, entry.Value); err != nil {
			return result, err
		}
		result.Entries++
	}
	if err := writer.Flush(); err != nil {
		return result, fmt.Errorf("flushing snapshot: %w", err)
	}
	if _, err := tmp.Write(hash.Sum(nil)); err != nil {
		return result, fmt.Errorf("writing snapshot checksum: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return result, fmt.Errorf("syncing snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return result, fmt.Errorf("closing snapshot: %w", err)
	}
	backupPath := tmpPath + ".previous"
	hadPrevious := false
	if err := fs.Link(path, backupPath); err == nil {
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("preserving previous snapshot: %w", err)
	}
	defer func() { _ = fs.Remove(backupPath) }()
	if err := fs.Rename(tmpPath, path); err != nil {
		return result, fmt.Errorf("replacing snapshot: %w", err)
	}
	d, err := fs.Open(dir)
	if err != nil {
		rollbackSnapshot(fs, path, backupPath, hadPrevious)
		return result, fmt.Errorf("opening snapshot directory: %w", err)
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil || closeErr != nil {
		rollbackSnapshot(fs, path, backupPath, hadPrevious)
		if syncErr != nil {
			return result, fmt.Errorf("syncing snapshot directory: %w", syncErr)
		}
		return result, fmt.Errorf("closing snapshot directory: %w", closeErr)
	}
	committed = true
	_ = fs.Remove(backupPath)
	result.Bytes = int64(snapshotHeaderSize) + int64(payloadSize) + snapshotChecksumSize
	return result, nil
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func rollbackSnapshot(fs snapshotFS, path, backupPath string, hadPrevious bool) {
	if hadPrevious {
		_ = fs.Rename(backupPath, path)
	} else {
		_ = fs.Remove(path)
	}
}

func readU16(r io.Reader) (uint16, error) {
	var v uint16
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}
func readU32(r io.Reader) (uint32, error) {
	var v uint32
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}
func readU64(r io.Reader) (uint64, error) {
	var v uint64
	err := binary.Read(r, binary.LittleEndian, &v)
	return v, err
}

func readSized(r io.Reader, max uint32) ([]byte, error) {
	n, err := readU32(r)
	if err != nil {
		return nil, err
	}
	if n > max {
		return nil, fmt.Errorf("snapshot field length %d exceeds limit %d", n, max)
	}
	value := make([]byte, n)
	_, err = io.ReadFull(r, value)
	return value, err
}

func readSnapshot(path string, maxBytes int64, current map[string]registry.ModelFingerprint) (snapshotRestoreResult, error) {
	result := snapshotRestoreResult{Cache: NewCache(maxBytes)}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer func() { _ = f.Close() }()
	result.Found = true
	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if info.Size() < snapshotHeaderSize+snapshotChecksumSize {
		return result, errors.New("snapshot is truncated")
	}
	dataLen := info.Size() - snapshotChecksumSize
	hash := sha256.New()
	stream := io.TeeReader(io.LimitReader(f, dataLen), hash)
	var magic [8]byte
	if _, err := io.ReadFull(stream, magic[:]); err != nil {
		return result, err
	}
	if magic != snapshotMagic {
		return result, errors.New("invalid snapshot magic")
	}
	version, err := readU16(stream)
	if err != nil || version != snapshotVersion {
		return result, fmt.Errorf("unsupported snapshot version %d", version)
	}
	payloadSize, err := readU64(stream)
	if err != nil {
		return result, err
	}
	if payloadSize != uint64(dataLen-snapshotHeaderSize) {
		return result, errors.New("snapshot payload length mismatch")
	}
	var representation [2]byte
	if _, err := io.ReadFull(stream, representation[:]); err != nil {
		return result, err
	}
	if representation != [2]byte{snapshotFloatFP32, snapshotLittleEndian} {
		return result, errors.New("unsupported snapshot numeric representation")
	}
	modelCount, err := readU32(stream)
	if err != nil || modelCount > maxSnapshotModels {
		return result, errors.New("invalid snapshot model count")
	}
	compatible := make(map[string]registry.ModelFingerprint)
	for range modelCount {
		name, err := readSized(stream, maxSnapshotString)
		if err != nil {
			return result, err
		}
		fingerprint, err := readSized(stream, maxSnapshotString)
		if err != nil {
			return result, err
		}
		dim, err := readU32(stream)
		if err != nil {
			return result, err
		}
		cur, ok := current[string(name)]
		if ok && cur.Fingerprint == string(fingerprint) && cur.Dim == int(dim) {
			compatible[string(name)] = cur
		}
	}
	entryCount, err := readU64(stream)
	if err != nil || entryCount > uint64(maxBytes/48)+1 {
		return result, errors.New("invalid snapshot entry count")
	}
	seen := make(map[string]struct{})
	for range entryCount {
		keyBytes, err := readSized(stream, maxSnapshotString)
		if err != nil {
			return result, err
		}
		value, err := readSized(stream, uint32(min(int64(^uint32(0)), maxBytes)))
		if err != nil {
			return result, err
		}
		key := string(keyBytes)
		model := modelOf(key)
		cur, known := current[model]
		if !known {
			result.SkippedUnknown++
			continue
		}
		if _, ok := compatible[model]; !ok || len(value) != cur.Dim*4 {
			result.SkippedFingerprint++
			continue
		}
		if _, exists := seen[key]; exists {
			return result, errors.New("duplicate snapshot key")
		}
		seen[key] = struct{}{}
		if result.Cache.restoreAppendMRU(key, value) {
			result.Restored++
		} else {
			result.SkippedMemory++
		}
	}
	var extra [1]byte
	if n, err := stream.Read(extra[:]); n != 0 || err != io.EOF {
		return result, errors.New("snapshot contains trailing payload data")
	}
	want := make([]byte, snapshotChecksumSize)
	if _, err := io.ReadFull(f, want); err != nil {
		return result, err
	}
	if !bytes.Equal(hash.Sum(nil), want) {
		return result, errors.New("snapshot checksum mismatch")
	}
	return result, nil
}
