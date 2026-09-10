package embtop

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

// RunOnce polls the node `samples` times at `interval`, printing one
// machine-readable line per poll to out, and returns when done. It returns
// an error if the node is unreachable at start or if a poll fails mid-run.
func RunOnce(c *Client, interval time.Duration, samples int, out io.Writer) error {
	if err := c.Dial(); err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	sampler := NewSampler(samples)
	known := []string{}
	lastSeq := uint64(0)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	first := true
	for i := 0; i < samples; {
		if !first {
			<-ticker.C
		}
		first = false

		res, err := c.Poll(known, lastSeq)
		if err != nil {
			return fmt.Errorf("poll %d: %w", i+1, err)
		}
		lastSeq = res.NextSeq
		// Reconcile model list; new models are polled from the next round.
		known = known[:0]
		for _, m := range res.Models {
			known = append(known, m.Name)
		}
		sampler.PushEvents(res.Events)
		p := sampler.Push(res)
		writeOnceLine(out, p, res, sampler)
		i++
	}
	return nil
}

// writeOnceLine emits one key=value line for the given poll.
func writeOnceLine(out io.Writer, p Point, res *PollResult, s *Sampler) {
	var b []byte
	b = append(b, "t="...)
	b = strconv.AppendInt(b, p.At.Unix(), 10)
	b = appendF(b, "uptime_secs", p.UptimeSecs)
	b = appendF(b, "total_requests", p.TotalRequests)
	b = appendF(b, "total_tokens", p.TotalTokens)
	b = appendF(b, "total_errors", p.TotalErrors)
	b = appendF(b, "models_loaded", int64(res.ModelsLoaded))
	b = appendRate(b, "req_rate", p.ReqRate)
	b = appendRate(b, "tok_rate", p.TokRate)
	b = appendRate(b, "err_rate", p.ErrRate)
	b = appendF(b, "active_requests", p.Active)
	b = appendF(b, "connections", p.Conns)
	b = appendF(b, "mem_mb", p.MemMB)
	b = appendRate(b, "cpu_pct", p.CPUPercent)
	b = appendF(b, "goroutines", p.Goroutines)
	b = appendF(b, "cache_hits", res.CacheHits)
	b = appendF(b, "cache_misses", res.CacheMisses)
	b = appendF(b, "cache_evictions", res.CacheEvictions)
	b = appendRate(b, "cache_hit_rate", p.CacheHitRate)

	// Latency percentiles from MONITOR events.
	if p50, p95, p99, ok := s.Latency(); ok {
		b = appendF(b, "lat_p50_us", p50)
		b = appendF(b, "lat_p95_us", p95)
		b = appendF(b, "lat_p99_us", p99)
	}

	// One section per model with cumulative + derived rates.
	for _, m := range res.Models {
		ms, ok := res.PerModel[m.Name]
		if !ok {
			continue
		}
		b = append(b, " model:"...)
		b = append(b, m.Name...)
		b = appendF(b, "dim", int64(ms.Dim))
		b = appendF(b, "reqs", ms.Requests)
		b = appendF(b, "toks", ms.Tokens)
		b = appendF(b, "errs", ms.Errors)
		mp := s.LatestModels[m.Name]
		b = appendRate(b, "req_rate", mp.ReqRate)
		b = appendRate(b, "tok_rate", mp.TokRate)
		b = appendRate(b, "err_rate", mp.ErrRate)
		b = appendF(b, "avg_latency_us", ms.AvgLatencyUs)
		b = append(b, " pooling="...)
		b = append(b, ms.Pooling...)
		b = append(b, " quant="...)
		b = append(b, ms.Quantization...)
	}
	b = append(b, '\n')
	_, _ = out.Write(b)
}

func appendF(b []byte, key string, v int64) []byte {
	b = append(b, ' ')
	b = append(b, key...)
	b = append(b, '=')
	return strconv.AppendInt(b, v, 10)
}

func appendRate(b []byte, key string, v float64) []byte {
	b = append(b, ' ')
	b = append(b, key...)
	b = append(b, '=')
	return strconv.AppendFloat(b, v, 'f', 1, 64)
}
