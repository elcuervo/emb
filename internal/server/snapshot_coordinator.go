package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/go-units"

	"github.com/elcuervo/emb/internal/registry"
)

type PersistenceConfig struct {
	File           string
	Load           bool
	SaveInterval   time.Duration
	SaveOnShutdown bool
	RestoreLimit   string
	RestoreReserve string
	SaveRateBytes  int64
	SaveRateRaw    string
}

type SnapshotStatus struct {
	Enabled              bool
	InProgress           bool
	Successes            int64
	Failures             int64
	Skipped              int64
	LastSuccessUnix      int64
	LastDuration         time.Duration
	LastEntries          int64
	LastBytes            int64
	LastCaptureDuration  time.Duration
	RestoreLimitBytes    int64
	RestoreRSSBytes      uint64
	RestoreHeadroomBytes int64
	RestoredEntries      int64
	SkippedUnknown       int64
	SkippedFingerprint   int64
	SkippedMemory        int64
	RestoreError         string
}

type snapshotCoordinator struct {
	cache *Cache
	reg   *registry.Registry

	mu             sync.RWMutex
	file           string
	interval       time.Duration
	saveOnShutdown bool
	rateBytes      int64

	saving      atomic.Bool
	savedOnce   atomic.Bool
	lastGen     atomic.Uint64
	statusMu    sync.RWMutex
	status      SnapshotStatus
	currentMu   sync.Mutex
	currentDone chan struct{}

	wake        chan struct{}
	stop        chan struct{}
	stopOnce    sync.Once
	done        chan struct{}
	ready       chan struct{}
	ctx         context.Context
	stopContext context.CancelFunc

	write    func(context.Context, string, CacheSnapshot, map[string]registry.ModelFingerprint, int64) (snapshotWriteResult, error)
	newTimer func(time.Duration) coordinatorTimer
}

type coordinatorTimer interface {
	Chan() <-chan time.Time
	Stop() bool
}

type realCoordinatorTimer struct{ *time.Timer }

func (t realCoordinatorTimer) Chan() <-chan time.Time { return t.C }

func newSnapshotCoordinator(cache *Cache, reg *registry.Registry, cfg PersistenceConfig) *snapshotCoordinator {
	ctx, cancel := context.WithCancel(context.Background())
	c := &snapshotCoordinator{
		cache: cache, reg: reg, file: cfg.File, interval: cfg.SaveInterval,
		saveOnShutdown: cfg.SaveOnShutdown, rateBytes: cfg.SaveRateBytes,
		wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), ready: make(chan struct{}),
		ctx: ctx, stopContext: cancel, write: writeSnapshot,
		newTimer: func(d time.Duration) coordinatorTimer { return realCoordinatorTimer{time.NewTimer(d)} },
	}
	c.status.Enabled = cfg.File != ""
	go c.schedule()
	<-c.ready
	return c
}

func (c *snapshotCoordinator) schedule() {
	defer close(c.done)
	var timer coordinatorTimer
	var timerC <-chan time.Time
	reset := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.Chan():
				default:
				}
			}
		}
		c.mu.RLock()
		interval, enabled := c.interval, c.file != ""
		c.mu.RUnlock()
		if enabled && interval > 0 {
			timer = c.newTimer(interval)
			timerC = timer.Chan()
		} else {
			timer = nil
			timerC = nil
		}
	}
	reset()
	close(c.ready)
	for {
		select {
		case <-timerC:
			c.periodicTick()
			reset()
		case <-c.wake:
			reset()
		case <-c.stop:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

func (c *snapshotCoordinator) periodicTick() {
	if !c.dirty() {
		c.addSkipped()
		return
	}
	if err := c.Save(c.ctx); err != nil && !errors.Is(err, errSnapshotInProgress) {
		log.Printf("periodic cache snapshot: %v", err)
	}
}

var errSnapshotInProgress = errors.New("snapshot already in progress")

func (c *snapshotCoordinator) dirty() bool {
	generation := c.cache.Stats().Generation
	if !c.savedOnce.Load() {
		return generation != 0
	}
	return generation != c.lastGen.Load()
}

func (c *snapshotCoordinator) Save(ctx context.Context) error {
	c.mu.RLock()
	file, rate := c.file, c.rateBytes
	c.mu.RUnlock()
	if file == "" {
		return errors.New("cache persistence is disabled (cache_file is empty)")
	}
	if !c.saving.CompareAndSwap(false, true) {
		c.addSkipped()
		return errSnapshotInProgress
	}
	done := make(chan struct{})
	c.currentMu.Lock()
	c.currentDone = done
	c.currentMu.Unlock()
	c.statusMu.Lock()
	c.status.InProgress = true
	c.status.Enabled = true
	c.statusMu.Unlock()
	go func() {
		defer close(done)
		defer c.saving.Store(false)
		started := time.Now()
		snapshot := c.cache.Snapshot()
		models, err := c.reg.Fingerprints()
		var result snapshotWriteResult
		if err == nil {
			result, err = c.write(ctx, file, snapshot, models, rate)
		}
		c.statusMu.Lock()
		c.status.InProgress = false
		c.status.LastDuration = time.Since(started)
		c.status.LastCaptureDuration = snapshot.CaptureDuration
		if err != nil {
			c.status.Failures++
			log.Printf("cache snapshot failed: %v", err)
		} else {
			c.status.Successes++
			c.status.LastSuccessUnix = time.Now().Unix()
			c.status.LastEntries = int64(result.Entries)
			c.status.LastBytes = result.Bytes
			c.lastGen.Store(snapshot.Generation)
			c.savedOnce.Store(true)
		}
		c.statusMu.Unlock()
	}()
	return nil
}

func (c *snapshotCoordinator) addSkipped() {
	c.statusMu.Lock()
	c.status.Skipped++
	c.statusMu.Unlock()
}

func (c *snapshotCoordinator) Status() SnapshotStatus {
	c.statusMu.RLock()
	status := c.status
	c.statusMu.RUnlock()
	return status
}

func (c *snapshotCoordinator) Configure(file string, interval time.Duration, shutdown bool, rate int64) {
	c.mu.Lock()
	c.file, c.interval, c.saveOnShutdown, c.rateBytes = file, interval, shutdown, rate
	c.mu.Unlock()
	c.statusMu.Lock()
	c.status.Enabled = file != ""
	c.statusMu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *snapshotCoordinator) Shutdown(ctx context.Context) {
	c.mu.RLock()
	enabled := c.file != "" && c.saveOnShutdown
	c.mu.RUnlock()
	if enabled && c.dirty() && !c.saving.Load() {
		_ = c.Save(ctx)
	}
	c.currentMu.Lock()
	done := c.currentDone
	c.currentMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			c.stopContext()
		}
	}
	c.stopContext()
	c.stopOnce.Do(func() { close(c.stop) })
	select {
	case <-c.done:
	case <-ctx.Done():
	}
}

func (c *snapshotCoordinator) Close() {
	c.stopContext()
	c.stopOnce.Do(func() { close(c.stop) })
	<-c.done
	c.currentMu.Lock()
	done := c.currentDone
	c.currentMu.Unlock()
	if done != nil {
		<-done
	}
}

func resolveSnapshotMemory(value string, total uint64, def int64, allowAuto bool) (int64, error) {
	v := strings.TrimSpace(value)
	if v == "" || (allowAuto && strings.EqualFold(v, "auto")) {
		return def, nil
	}
	if strings.HasSuffix(v, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil || pct <= 0 || pct > 100 {
			return 0, fmt.Errorf("invalid memory percentage %q", value)
		}
		if total == 0 {
			return def, nil
		}
		return int64(float64(total) * pct / 100), nil
	}
	n, err := units.FromHumanSize(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid memory size %q", value)
	}
	return n, nil
}

func effectiveRestoreLimit(cacheBytes int64, limitRaw, reserveRaw string) (limit int64, rss uint64, headroom int64, err error) {
	total := registry.TotalSystemMemory()
	rss, _ = registry.CurrentMemoryUsage()
	return effectiveRestoreLimitForMetrics(cacheBytes, limitRaw, reserveRaw, total, rss)
}

func effectiveRestoreLimitForMetrics(cacheBytes int64, limitRaw, reserveRaw string, total, rss uint64) (limit int64, sampledRSS uint64, headroom int64, err error) {
	limit, err = resolveSnapshotMemory(limitRaw, total, cacheBytes, true)
	if err != nil {
		return 0, rss, 0, err
	}
	if limit > cacheBytes {
		limit = cacheBytes
	}
	// If the platform cannot report total RAM, retain the bounded cache/restore
	// limit and publish unknown headroom. Startup remains useful without
	// pretending a percentage reserve can be resolved from missing metrics.
	if total == 0 {
		return limit, rss, -1, nil
	}
	reserveDefault := int64(total / 10)
	reserve, err := resolveSnapshotMemory(reserveRaw, total, reserveDefault, false)
	if err != nil {
		return 0, rss, 0, err
	}
	headroom = int64(total) - int64(rss) - reserve
	if headroom < 0 {
		headroom = 0
	}
	if limit > headroom {
		limit = headroom
	}
	return limit, rss, headroom, nil
}
