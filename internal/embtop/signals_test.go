package embtop

import (
	"strings"
	"testing"
	"time"
)

// signalResponder answers a poll with the given EMB.STATS field set, so the
// decode of the kernel and cache-restore counters is exercised over the wire.
func signalResponder(stats func() []byte) func([]string, int) []byte {
	return func(cmd []string, _ int) []byte {
		switch strings.ToUpper(cmd[0]) {
		case "EMB.MODELS":
			return modelsReply()
		case "EMB.STATS":
			return stats()
		case "INFO":
			return encodeBulk("gomaxprocs:4\r\n")
		case "MONITOR":
			return encodeArray()
		default:
			return encodeError("ERR unknown " + cmd[0])
		}
	}
}

// TestPollDecodesThrottleAndRestore covers the counters the sizing
// recommendation reads: every one must reach both the decoded PollResult and
// the derived Point.
func TestPollDecodesThrottleAndRestore(t *testing.T) {
	fields := map[string]int64{
		"cpu_throttle_periods":         1200,
		"cpu_throttled_periods":        340,
		"cpu_throttle_usec":            56_000_000,
		"cpu_steal_usec":               7_000,
		"cache_restore_limit_bytes":    1 << 30,
		"cache_restore_headroom_bytes": 1 << 28,
		"cache_restore_skipped_memory": 42,
	}
	_, addr := startScripted(t, signalResponder(func() []byte { return statsReply(fields) }))
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	res, err := c.Poll(nil, 0)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	got := map[string]int64{
		"cpu_throttle_periods":         res.CPUThrottlePeriods,
		"cpu_throttled_periods":        res.CPUThrottledPeriods,
		"cpu_throttle_usec":            res.CPUThrottleUsec,
		"cpu_steal_usec":               res.CPUStealUsec,
		"cache_restore_limit_bytes":    res.CacheRestoreLimitBytes,
		"cache_restore_headroom_bytes": res.CacheRestoreHeadroomBytes,
		"cache_restore_skipped_memory": res.CacheRestoreSkippedMemory,
	}
	for k, w := range fields {
		if got[k] != w {
			t.Errorf("PollResult %s = %d, want %d", k, got[k], w)
		}
	}

	s := NewSampler(4)
	s.SetClock(newFakeClock(time.Unix(0, 0), time.Second))
	p := s.Push(res)
	gotPoint := map[string]int64{
		"cpu_throttle_periods":         p.CPUThrottlePeriods,
		"cpu_throttled_periods":        p.CPUThrottledPeriods,
		"cpu_throttle_usec":            p.CPUThrottleUsec,
		"cpu_steal_usec":               p.CPUStealUsec,
		"cache_restore_limit_bytes":    p.CacheRestoreLimitBytes,
		"cache_restore_headroom_bytes": p.CacheRestoreHeadroomBytes,
		"cache_restore_skipped_memory": p.CacheRestoreSkippedMemory,
	}
	for k, w := range fields {
		if gotPoint[k] != w {
			t.Errorf("Point %s = %d, want %d", k, gotPoint[k], w)
		}
	}
}

// TestPollMissingSignalsDecayToZero covers an older server: the fields it
// never published must read zero rather than fail the poll.
func TestPollMissingSignalsDecayToZero(t *testing.T) {
	bare := encodeArray(
		encodeBulk("uptime_secs"), encodeInt(3),
		encodeBulk("cpu_budget_enabled"), encodeInt(1),
	)
	_, addr := startScripted(t, signalResponder(func() []byte { return bare }))
	c := NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	res, err := c.Poll(nil, 0)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if res.CPUThrottlePeriods != 0 || res.CPUThrottledPeriods != 0 ||
		res.CPUThrottleUsec != 0 || res.CPUStealUsec != 0 ||
		res.CacheRestoreLimitBytes != 0 || res.CacheRestoreHeadroomBytes != 0 ||
		res.CacheRestoreSkippedMemory != 0 {
		t.Fatalf("a reply without the new fields did not decay to zero: %+v", res)
	}
	if res.UptimeSecs != 3 {
		t.Fatalf("the fields it did carry were lost: %+v", res)
	}
}

// TestPointCacheBytesSumsModelCaches covers the occupancy the growth check
// reads: the cache bytes of a poll are its models' caches added up.
func TestPointCacheBytesSumsModelCaches(t *testing.T) {
	res := &PollResult{PerModel: map[string]*ModelStats{
		"a": {CacheCurBytes: 1000},
		"b": {CacheCurBytes: 24},
		"c": {}, // a model with no cache contributes nothing
	}}
	p := NewSampler(2).Push(res)
	if p.CacheBytes != 1024 {
		t.Fatalf("CacheBytes = %d, want 1024", p.CacheBytes)
	}
}
