package embtop

import (
	"fmt"
	"io"
	"slices"
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
		known = reconcileKnown(known, res)
		sampler.PushEvents(res.Events)
		p := sampler.Push(res)
		if err := writeOnceLine(out, p, res, known, sampler); err != nil {
			return fmt.Errorf("poll %d: %w", i+1, err)
		}
		i++
	}
	return nil
}

// onceNode is one fleet member's headless polling state.
type onceNode struct {
	c       *Client
	label   string
	sampler *Sampler
	known   []string
	lastSeq uint64

	res     *PollResult
	sampled bool
}

// RunOnceFleet polls several nodes in lockstep, printing one line per poll:
// the summed fleet aggregate followed by one `node:<label>` section per node.
// Latency percentiles stay per node, because they do not add across nodes. A
// node that cannot be reached is skipped; the run fails only when no node
// produced a sample.
func RunOnceFleet(clients []*Client, labels []string, interval time.Duration, samples int, out io.Writer) error {
	if len(clients) != len(labels) {
		return fmt.Errorf("RunOnceFleet: %d clients, %d labels", len(clients), len(labels))
	}
	states := make([]*onceNode, len(clients))
	for i, c := range clients {
		states[i] = &onceNode{c: c, label: labels[i], sampler: NewSampler(samples)}
	}
	defer func() {
		for _, st := range states {
			_ = st.c.Close()
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	first := true
	for i := 0; i < samples; {
		if !first {
			<-ticker.C
		}
		first = false

		sampled := 0
		for _, st := range states {
			st.sampled = false
			if _, err := st.c.EnsureConn(); err != nil {
				continue
			}
			res, err := st.c.Poll(st.known, st.lastSeq)
			if err != nil {
				_ = st.c.Close()
				continue
			}
			st.lastSeq = res.NextSeq
			st.known = reconcileKnown(st.known, res)
			st.sampler.PushEvents(res.Events)
			st.sampler.Push(res)
			st.res = res
			st.sampled = true
			sampled++
		}
		if sampled == 0 {
			return fmt.Errorf("no node produced a sample")
		}
		if err := writeFleetOnceLine(out, states); err != nil {
			return fmt.Errorf("poll %d: %w", i+1, err)
		}
		i++
	}
	return nil
}

// reconcileKnown folds a poll's announced models into a stable first-seen
// order, so the per-model sections do not follow the server's per-call
// EMB.MODELS enumeration order.
func reconcileKnown(known []string, res *PollResult) []string {
	announced := make(map[string]bool, len(res.Models))
	for _, m := range res.Models {
		announced[m.Name] = true
	}
	kept := known[:0]
	for _, name := range known {
		if announced[name] {
			kept = append(kept, name)
		}
	}
	for _, m := range res.Models {
		if !slices.Contains(kept, m.Name) {
			kept = append(kept, m.Name)
		}
	}
	return kept
}

// writeOnceLine emits one key=value line for the given poll, returning any
// write error (a closed pipe must not look like a successful run). order is
// the stable first-seen model order the per-model sections follow.
func writeOnceLine(out io.Writer, p Point, res *PollResult, order []string, s *Sampler) error {
	var b []byte
	b = append(b, "t="...)
	b = strconv.AppendInt(b, p.At.Unix(), 10)
	b = appendAggregateFields(b, &p, res)
	if p50, p95, p99, ok := s.Latency(); ok {
		b = appendF(b, "lat_p50_us", p50)
		b = appendF(b, "lat_p95_us", p95)
		b = appendF(b, "lat_p99_us", p99)
	}
	b = appendModelSections(b, order, res, s)
	b = append(b, '\n')
	_, err := out.Write(b)
	return err
}

// writeFleetOnceLine emits the fleet aggregate followed by one section per
// node that produced a sample this poll.
func writeFleetOnceLine(out io.Writer, states []*onceNode) error {
	var b []byte
	b = append(b, "t="...)
	now := time.Now()
	for _, st := range states {
		if st.sampled {
			now = st.sampler.Latest.At
			break
		}
	}
	b = strconv.AppendInt(b, now.Unix(), 10)
	b = appendFleetAggregate(b, states)
	for _, st := range states {
		if !st.sampled {
			continue
		}
		b = append(b, " node:"...)
		b = append(b, st.label...)
		p := st.sampler.Latest
		b = appendF(b, "uptime_secs", p.UptimeSecs)
		b = appendF(b, "total_requests", p.TotalRequests)
		b = appendF(b, "total_tokens", p.TotalTokens)
		b = appendF(b, "total_errors", p.TotalErrors)
		b = appendRate(b, "req_rate", p.ReqRate)
		b = appendRate(b, "tok_rate", p.TokRate)
		b = appendRate(b, "err_rate", p.ErrRate)
		b = appendRate(b, "cpu_pct", p.CPUPercent)
		b = appendF(b, "cache_hits", st.res.CacheHits)
		b = appendF(b, "cache_misses", st.res.CacheMisses)
		b = appendRate(b, "cache_hit_rate", p.CacheHitRate)
		if p50, p95, p99, ok := st.sampler.Latency(); ok {
			b = appendF(b, "lat_p50_us", p50)
			b = appendF(b, "lat_p95_us", p95)
			b = appendF(b, "lat_p99_us", p99)
		}
		b = appendModelSections(b, st.known, st.res, st.sampler)
	}
	b = append(b, '\n')
	_, err := out.Write(b)
	return err
}

// appendAggregateFields writes one node's aggregated rate/gauges. It is the
// single-node line's field set, so a fleet of one renders identically.
func appendAggregateFields(b []byte, p *Point, res *PollResult) []byte {
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
	return b
}

// appendFleetAggregate sums the sampled nodes' counters and rates into the
// aggregate field set. Percentiles are deliberately absent: they do not add
// across independent nodes.
func appendFleetAggregate(b []byte, states []*onceNode) []byte {
	var totalRequests, totalTokens, totalErrors, active, conns, mem, goroutines, models int64
	var cacheHits, cacheMisses, cacheEvictions int64
	var req, tok, errRate, cpu float64
	for _, st := range states {
		if !st.sampled {
			continue
		}
		p := st.sampler.Latest
		totalRequests += p.TotalRequests
		totalTokens += p.TotalTokens
		totalErrors += p.TotalErrors
		active += p.Active
		conns += p.Conns
		mem += p.MemMB
		goroutines += p.Goroutines
		models += int64(st.res.ModelsLoaded)
		cacheHits += st.res.CacheHits
		cacheMisses += st.res.CacheMisses
		cacheEvictions += st.res.CacheEvictions
		req += p.ReqRate
		tok += p.TokRate
		errRate += p.ErrRate
		cpu += p.CPUPercent
	}
	b = appendF(b, "uptime_secs", states[0].sampler.Latest.UptimeSecs)
	b = appendF(b, "total_requests", totalRequests)
	b = appendF(b, "total_tokens", totalTokens)
	b = appendF(b, "total_errors", totalErrors)
	b = appendF(b, "models_loaded", models)
	b = appendRate(b, "req_rate", req)
	b = appendRate(b, "tok_rate", tok)
	b = appendRate(b, "err_rate", errRate)
	b = appendF(b, "active_requests", active)
	b = appendF(b, "connections", conns)
	b = appendF(b, "mem_mb", mem)
	b = appendRate(b, "cpu_pct", cpu)
	b = appendF(b, "goroutines", goroutines)
	b = appendF(b, "cache_hits", cacheHits)
	b = appendF(b, "cache_misses", cacheMisses)
	b = appendF(b, "cache_evictions", cacheEvictions)
	// A ratio of summed counters, not a sum of per-node percentages, which
	// could exceed 100 across nodes.
	hitRate := 0.0
	if t := cacheHits + cacheMisses; t > 0 {
		hitRate = float64(cacheHits) / float64(t) * 100
	}
	b = appendRate(b, "cache_hit_rate", hitRate)
	return b
}

// appendModelSections writes the per-model key=value sections in the stable
// first-seen order.
func appendModelSections(b []byte, order []string, res *PollResult, s *Sampler) []byte {
	for _, name := range order {
		ms, ok := res.PerModel[name]
		if !ok {
			continue
		}
		b = append(b, " model:"...)
		b = append(b, name...)
		b = appendF(b, "dim", int64(ms.Dim))
		b = appendF(b, "reqs", ms.Requests)
		b = appendF(b, "toks", ms.Tokens)
		b = appendF(b, "errs", ms.Errors)
		mp := s.LatestModels[name]
		b = appendRate(b, "req_rate", mp.ReqRate)
		b = appendRate(b, "tok_rate", mp.TokRate)
		b = appendRate(b, "err_rate", mp.ErrRate)
		b = appendF(b, "avg_latency_us", ms.AvgLatencyUs)
		b = append(b, " pooling="...)
		b = append(b, ms.Pooling...)
		b = append(b, " quant="...)
		b = append(b, ms.Quantization...)
	}
	return b
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
