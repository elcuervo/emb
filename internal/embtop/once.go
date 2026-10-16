package embtop

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
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
	// The node's own advisory goes with the node's own fields, before the model
	// sections: a reader that takes the aggregate as everything before the
	// first ` model:` would otherwise never see it.
	b = appendRec(b, RecommendNode(s.Window).Kind)
	b = appendAttribution(b, s)
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
	b = appendFleetAggregate(b, states, fleetRecommendation(states))
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
		b = appendCPUPlanFields(b, &p)
		b = appendRec(b, RecommendNode(st.sampler.Window).Kind)
		b = appendAttribution(b, st.sampler)
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
	b = appendCPUPlanFields(b, p)
	return b
}

// appendCPUPlanFields adds the process CPU plan, and nothing at all when the
// node did not report one: an absent plan must not read as budget 0. Every
// field is numeric because the headless line is parsed as key=float pairs;
// the allowance source name lives in the TUI, not here.
func appendCPUPlanFields(b []byte, p *Point) []byte {
	if !p.CPUPlanKnown {
		return b
	}
	b = appendF(b, "cpu_budget_enabled", boolToInt64(p.CPUBudgetEnabled))
	b = appendF(b, "cpu_budget", p.CPUBudget)
	b = appendF(b, "cpu_active_tokens", p.CPUActiveTokens)
	b = appendF(b, "cpu_peak_tokens", p.CPUPeakTokens)
	b = appendRate(b, "cpu_allowance", p.CPUAllowance)
	b = appendF(b, "cpu_admission_wait_us", p.CPUAdmissionWaitUs)
	b = appendF(b, "cpu_admission_waiters", p.CPUAdmissionWaiters)
	b = appendF(b, "cpu_admission_timeouts", p.CPUAdmissionTimeouts)
	return b
}

// appendFleetAggregate sums the sampled nodes' counters and rates into the
// aggregate field set. Percentiles are deliberately absent: they do not add
// across independent nodes.
func appendFleetAggregate(b []byte, states []*onceNode, rec FleetRecommendation) []byte {
	var totalRequests, totalTokens, totalErrors, active, conns, mem, goroutines, models int64
	var cacheHits, cacheMisses, cacheEvictions int64
	var req, tok, errRate, cpu float64
	var cpuBudget, cpuActive, cpuPeak int64
	var cpuAllowance float64
	var cpuWaiters, cpuTimeouts int64
	planKnown := false
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
		if p.CPUPlanKnown {
			planKnown = true
			cpuBudget += p.CPUBudget
			cpuActive += p.CPUActiveTokens
			cpuPeak += p.CPUPeakTokens
			cpuAllowance += p.CPUAllowance
			cpuWaiters += p.CPUAdmissionWaiters
			cpuTimeouts += p.CPUAdmissionTimeouts
		}
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
	// The fleet's own advisory, as tokens a script can match on. The per-node
	// sections carry each node's own kind.
	b = appendFleetRec(b, rec)
	// Each node plans its own process, so these are fleet sums of independent
	// budgets, not one shared pool.
	if planKnown {
		b = appendF(b, "cpu_budget", cpuBudget)
		b = appendF(b, "cpu_active_tokens", cpuActive)
		b = appendF(b, "cpu_peak_tokens", cpuPeak)
		b = appendRate(b, "cpu_allowance", cpuAllowance)
		b = appendF(b, "cpu_admission_waiters", cpuWaiters)
		b = appendF(b, "cpu_admission_timeouts", cpuTimeouts)
	}
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
		if ms.CPUBudgetEnabled {
			// The calibrated layout this model actually runs.
			b = appendF(b, "workers", int64(ms.Workers))
			b = appendF(b, "intra_op_threads", int64(ms.IntraOpThreads))
			b = appendF(b, "cpu_thread_share", int64(ms.CPUThreadShare))
		}
		b = appendF(b, "cache_served_reqs", mp.CachedReqs)
		if split := ModelPhaseSplit(s.ModHist[name]); !split.Empty() {
			// This model's own window: what its requests waited for and spent
			// running, and how many runs that covers.
			b = appendF(b, "runs", split.Runs)
			b = appendF(b, "wait_us", split.MeanUs(PhaseDispatch).Microseconds())
			b = appendF(b, "admit_us", split.MeanUs(PhaseAdmission).Microseconds())
			b = appendF(b, "run_us", split.MeanUs(PhaseRun).Microseconds())
		}
	}
	return b
}

// SamplerAdviceInput gathers one node's advice evidence from its sampler: its
// series, its models' phases, its event window by path and the tail of the
// requests that ran. Both the dashboard and the headless output read the node
// through it, so the two cannot disagree about the same poll.
func SamplerAdviceInput(s *Sampler) AdviceInput {
	in := AdviceInput{
		Window: s.Window,
		Models: s.ModHist,
		Raw:    s.RawModels(),
		Split:  NodePhaseSplit(s.ModHist),
		Paths:  s.PathCounts(),
	}
	if _, _, p95, n, ok := s.UncachedLatency(); ok {
		in.UncachedP95Us = p95
		in.UncachedN = n
	}
	return in
}

// SamplerFleetRecInput is one node's contribution to the fleet
// recommendation, gathered from the sampler both surfaces read: its series, the
// tail its client measured and its own advice. The fleet view and the headless
// fleet line both build their record inputs through it, so one poll cannot be
// attributed two ways and a node's reservable repair cannot be lost between
// them.
func SamplerFleetRecInput(label string, s *Sampler) FleetNodeRecInput {
	_, _, p95, _ := s.Latency()
	advice := SamplerAdviceInput(s)
	return FleetNodeRecInput{Label: label, Hist: s.Window, P95Us: p95, Advice: &advice}
}

// appendAttribution writes the node's course of action and the evidence behind
// it: the action and the action it rejects, the cache's share of the window's
// traffic, the p95 of the requests that ran and the phases they spent it in.
// Every token the line already carried keeps its name; these are additions.
func appendAttribution(b []byte, s *Sampler) []byte {
	in := SamplerAdviceInput(s)
	adv := Advise(in)
	if !adv.Empty() {
		b = append(b, " act="...)
		b = append(b, adv.Action...)
		if adv.Rejected != "" {
			b = append(b, " act_rejected="...)
			b = append(b, adv.Rejected...)
		}
	}
	// The share the dashboard states beside the advice, from the same window
	// function it renders: the event ring's own share describes the band's
	// legend, and reading it here would let the two surfaces disagree about one
	// poll. Absent exactly where the panel states nothing.
	if adv.CachedPct > 0 {
		b = appendRate(b, "cached_pct", adv.CachedPct)
	}
	if in.UncachedP95Us > 0 {
		b = appendF(b, "lat_p95_run_us", in.UncachedP95Us)
		b = appendF(b, "lat_p95_run_n", int64(in.UncachedN))
	}
	if !adv.Phases.Empty() {
		b = append(b, " phase="...)
		b = append(b, string(adv.Phase)...)
		b = appendRate(b, "phase_share", adv.Share*100)
		b = appendF(b, "phase_wait_us", adv.Phases.MeanUs(PhaseDispatch).Microseconds())
		b = appendF(b, "phase_admit_us", adv.Phases.MeanUs(PhaseAdmission).Microseconds())
		b = appendF(b, "phase_run_us", adv.Phases.MeanUs(PhaseRun).Microseconds())
	}
	return b
}

// appendRec writes one advisory sizing kind as a single token — a headless
// consumer must see the recommendation the dashboard renders. Nothing is
// written when there is nothing to recommend.
func appendRec(b []byte, kind string) []byte {
	if kind == "" {
		return b
	}
	b = append(b, " rec="...)
	return append(b, kind...)
}

// appendFleetRec writes a fleet recommendation as its tokens: the binding
// resources joined into one, and the node to inspect first as its own field.
// The node needs its own field because a recommendation can name a node without
// naming any resource — the "one peer is slow, nothing else is near a
// threshold" case — and dropping it would make the headless line say there is
// nothing to recommend while the dashboard names a node.
func appendFleetRec(b []byte, rec FleetRecommendation) []byte {
	b = appendRec(b, strings.Join(rec.Kinds, "+"))
	if rec.First != "" {
		b = append(b, " rec_first="...)
		b = append(b, rec.First...)
	}
	if rec.Action != "" && rec.Action != ActionNone {
		b = append(b, " act="...)
		b = append(b, rec.Action...)
	}
	return b
}

// fleetRecommendation is the fleet advisory for the nodes that produced a
// sample this poll.
func fleetRecommendation(states []*onceNode) FleetRecommendation {
	var in []FleetNodeRecInput
	for _, st := range states {
		if !st.sampled {
			continue
		}
		in = append(in, SamplerFleetRecInput(st.label, st.sampler))
	}
	return RecommendFleet(in)
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

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
