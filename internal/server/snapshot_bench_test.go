package server

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/registry"
)

// BenchmarkCacheSnapshotLoad is the persistence acceptance harness. It drives
// a stable 50/50 cache hit/miss mix while a full streaming snapshot sink runs,
// and reports sampled tail latency, snapshot throughput, restore RSS delta,
// and the host-memory-derived restore ceiling.
func BenchmarkCacheSnapshotLoad(b *testing.B) {
	for _, mode := range []struct {
		name string
		rate int64
		save bool
	}{
		{name: "baseline"},
		{name: "snapshot-unlimited", save: true},
		{name: "snapshot-20MBps", save: true, rate: 20_000_000},
	} {
		b.Run(mode.name, func(b *testing.B) {
			benchmarkCacheSnapshotLoad(b, mode.save, mode.rate)
		})
	}
}

func benchmarkCacheSnapshotLoad(b *testing.B, save bool, rate int64) {
	const entries = 4096
	cache := NewCache(8 << 20)
	value := make([]byte, 256)
	for i := range entries {
		cache.Set(fmt.Sprintf("test:key-%04d", i), value)
	}
	models := map[string]registry.ModelFingerprint{"test": {Fingerprint: "benchmark", Dim: 64}}
	path := filepath.Join(b.TempDir(), "cache.embcache")

	var snapshotBytes atomic.Int64
	var snapshotNanos atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	startSave := make(chan struct{})
	if save {
		go func() {
			defer close(done)
			<-startSave
			started := time.Now()
			result, err := writeSnapshot(ctx, path, cache.Snapshot(), models, rate)
			if err == nil {
				snapshotBytes.Store(result.Bytes)
				snapshotNanos.Store(time.Since(started).Nanoseconds())
			}
		}()
	} else {
		close(startSave)
		close(done)
	}

	var samplesMu sync.Mutex
	samples := make([]int64, 0, b.N/256+1)
	var sequence atomic.Uint64
	b.ResetTimer()
	if save {
		close(startSave)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := sequence.Add(1)
			key := fmt.Sprintf("test:key-%04d", n%entries)
			if n&1 != 0 {
				key = "test:missing"
			}
			measure := n%256 == 0
			var at time.Time
			if measure {
				at = time.Now()
			}
			_, _ = cache.Get(key)
			if measure {
				samplesMu.Lock()
				samples = append(samples, time.Since(at).Nanoseconds())
				samplesMu.Unlock()
			}
		}
	})
	b.StopTimer()
	cancel()
	<-done

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	if len(samples) > 0 {
		b.ReportMetric(float64(samples[len(samples)/2]), "p50-ns")
		b.ReportMetric(float64(samples[min(len(samples)-1, len(samples)*99/100)]), "p99-ns")
	}
	if d := snapshotNanos.Load(); d > 0 {
		b.ReportMetric(float64(snapshotBytes.Load())/(float64(d)/float64(time.Second))/1_000_000, "snapshot-MB/s")
	}
	limit, _, _, _ := effectiveRestoreLimit(cache.Stats().MaxBytes, "auto", "10%")
	b.ReportMetric(float64(limit), "restore-limit-B")
	if save {
		before, _ := registry.CurrentMemoryUsage()
		_, _ = readSnapshot(path, limit, models)
		after, _ := registry.CurrentMemoryUsage()
		var delta uint64
		if after > before {
			delta = after - before
		}
		b.ReportMetric(float64(delta), "restore-rss-delta-B")
	}
}
