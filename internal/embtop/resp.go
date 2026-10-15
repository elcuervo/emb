// Package embtop provides the polling client, rate sampler, and headless
// output shared by the emb-top TUI and its -once mode.
//
// It speaks RESP2 against a running emb node using only the commands the
// server already exposes (EMB.MODELS, EMB.INFO, EMB.STATS, MONITOR, AUTH),
// over the wire client in internal/resp. It imports nothing from
// internal/server, so the tool builds without CGo/onnxruntime.
package embtop

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

// Reply is one RESP2 reply, re-exported from internal/resp.
type Reply = resp.Reply

// DefaultTimeout bounds each poll's write/read round trip, re-exported from
// internal/resp.
const DefaultTimeout = resp.DefaultTimeout

// maxDepth is the reply-nesting bound enforced by the parser, kept here for
// the emb-top tests that assert the bound.
const maxDepth = resp.MaxDepth

// Client is the emb-top polling client: the shared RESP wire client plus the
// EMB.MODELS / EMB.INFO / EMB.STATS / MONITOR decoding the dashboard needs.
type Client struct {
	*resp.Client
}

// NewClient returns a Client for addr (host:port) with optional password
// (AUTH) and TLS transport.
func NewClient(addr, password string, useTLS bool) *Client {
	return &Client{Client: resp.NewClient(addr, password, useTLS)}
}

// Event is one completed-request record from MONITOR.
type Event struct {
	Seq       uint64
	AtUs      int64
	Model     string
	Texts     int
	LatencyUs int64
	Err       bool
	// The request's attribution: how many of its texts the reply cache
	// answered, whether the phases below were measured by the server, and the
	// time its texts that were not cache-answered waited for a session or
	// batch window, waited at the process CPU admission gate, and ran.
	CachedTexts int
	Attributed  bool
	DispatchUs  int64
	AdmitUs     int64
	RunUs       int64
}

// CacheServed reports whether every text of this event was answered from the
// reply cache, and Reran whether at least one text ran. Both require the
// server's attribution: an event without it says nothing about the path, and
// reading its absent counters as "nothing was cached" would count every request
// on an older server as a run.
func (e Event) CacheServed() bool { return e.Attributed && e.Texts > 0 && e.CachedTexts >= e.Texts }
func (e Event) Reran() bool       { return e.Attributed && e.CachedTexts < e.Texts }

// PollResult is one typed snapshot of EMB.MODELS + EMB.INFO + EMB.STATS.
type PollResult struct {
	UptimeSecs      int64
	TotalRequests   int64
	TotalTokens     int64
	TotalErrors     int64
	ActiveRequests  int64
	Connections     int64
	TruncatedTexts  int64
	TruncatedPairs  int64
	TruncatedImages int64
	ModelsLoaded    int
	MemMB           int64
	CPUUserUsec     int64
	CPUSysUsec      int64
	Goroutines      int64
	CacheHits       int64
	CacheMisses     int64
	CacheEvictions  int64
	GoMaxProcs      int // node's own parallelism from INFO cpu (0 when absent)

	// Process CPU plan, from EMB.STATS. Budget/active/peak are admission
	// tokens: how much intra-op work the process will admit at once, how much
	// is in use, and the highest seen. Allowance is the detected (possibly
	// fractional) CPU entitlement the budget was derived from.
	CPUBudgetEnabled  bool
	CPUBudget         int64
	CPUCapacity       int64
	CPUReserve        int64
	CPUActiveTokens   int64
	CPUPeakTokens     int64
	CPUAllowance      float64
	CPUAllowanceKnown bool
	CPUAllowanceSrc   string
	CPUPlanKnown      bool // the node reported the EMB.STATS CPU plan fields
	// Admission pressure at the CPU gate: cumulative wait, waiters now, and
	// waits that ended without a grant. Absent from older servers, so zero.
	CPUAdmissionWaitUs   int64
	CPUAdmissionWaiters  int64
	CPUAdmissionTimeouts int64
	// The kernel's own view of CPU the process could not use: cgroup throttling
	// and stolen time, cumulative. This is what separates "the budget is the
	// limit" from "the host is starving us". Zero on an unthrottled host.
	CPUThrottlePeriods  int64
	CPUThrottledPeriods int64
	CPUThrottleUsec     int64
	CPUStealUsec        int64
	// Cache-restore accounting: the limit the server derived for itself, what
	// headroom it had under that limit, and how many entries it dropped for
	// want of memory.
	CacheRestoreLimitBytes    int64
	CacheRestoreHeadroomBytes int64
	CacheRestoreSkippedMemory int64

	Models   []ModelListEntry // EMB.MODELS reply
	PerModel map[string]*ModelStats
	Events   []Event // MONITOR reply (newer than afterSeq)
	NextSeq  uint64  // highest event seq seen (afterSeq when no events)
}

// ModelListEntry is one [name, dim, status] row from EMB.MODELS.
type ModelListEntry struct {
	Name   string
	Dim    int
	Status string
}

// ModelStats is the typed EMB.INFO <model> reply. Cache fields are present
// only when the server runs with a cache.
type ModelStats struct {
	Dim              int
	MaxLength        int
	Workers          int
	Requests         int64
	AvgLatencyUs     int64
	Tokens           int64
	Errors           int64
	Pooling          string
	Normalize        bool
	BatchingTimeout  int
	BatchingMaxBatch int
	BatchingMaxToks  int
	PaddingEff       string
	Quantization     string
	ModelBytes       int64
	CacheHits        int64
	CacheMisses      int64
	CacheHitRate     string
	CacheEvictions   int64
	CacheEntries     int64
	CacheMaxBytes    int64
	CacheCurBytes    int64

	// CPU plan for this model: the selected session width, its share of the
	// process budget, and whether ORT spinning is on.
	CPUBudgetEnabled bool
	CPUBudget        int
	CPUThreadShare   int
	IntraOpThreads   int
	AllowSpinning    bool

	// The embedding path's cumulative phases and session gauges: what this
	// model's requests waited for and spent running, and how many runs and
	// sessions those were. Admission wait is kept out of the run time.
	EmbedDispatchWaitUs  int64
	EmbedAdmissionWaitUs int64
	EmbedRunUs           int64
	EmbedRuns            int64
	EmbedSessionsBusy    int64
	EmbedSessionsTotal   int64
}

// Poll runs one pipelined poll: EMB.MODELS, EMB.INFO for each known model,
// then EMB.STATS — all written in a single round trip, replies read in
// order. known lists the models to poll EMB.INFO for; newly announced
// models are returned in Models and picked up on the next poll. afterSeq
// fetches MONITOR events newer than that sequence (0 = all buffered).
func (c *Client) Poll(known []string, afterSeq uint64) (*PollResult, error) {
	// Bound the whole round trip so a stalled peer cannot block the caller.
	_ = c.SetDeadline(time.Now().Add(c.Timeout()))
	if err := c.writePoll(known, afterSeq); err != nil {
		return nil, err
	}
	if err := c.Flush(); err != nil {
		return nil, err
	}

	modelsRep, err := c.ReadReply()
	if err != nil {
		return nil, err
	}
	if err := modelsRep.Err(); err != nil {
		// Remaining pipelined replies are unread: drop the connection so the
		// next poll starts clean instead of misparsing stale replies.
		_ = c.Close()
		return nil, fmt.Errorf("EMB.MODELS: %w", err)
	}

	perModel := make(map[string]*ModelStats, len(known))
	for _, name := range known {
		rep, err := c.ReadReply()
		if err != nil {
			return nil, err
		}
		if err := rep.Err(); err != nil {
			// Model disappeared between polls; treat as absent this round.
			continue
		}
		perModel[name] = parseModelStats(rep)
	}

	statsRep, err := c.ReadReply()
	if err != nil {
		return nil, err
	}
	if err := statsRep.Err(); err != nil {
		// MONITOR's reply is still unread: drop the connection.
		_ = c.Close()
		return nil, fmt.Errorf("EMB.STATS: %w", err)
	}

	cpuRep, err := c.ReadReply()
	if err != nil {
		return nil, err
	}

	monRep, err := c.ReadReply()
	if err != nil {
		return nil, err
	}
	if err := monRep.Err(); err != nil {
		return nil, fmt.Errorf("MONITOR: %w", err)
	}

	res := parseStats(statsRep)
	res.GoMaxProcs = parseInfoCPU(cpuRep)
	res.Models = parseModelList(modelsRep)
	res.PerModel = perModel
	res.Events = parseEvents(monRep)
	if len(res.Events) > 0 {
		res.NextSeq = res.Events[len(res.Events)-1].Seq
	} else {
		res.NextSeq = afterSeq
	}
	return res, nil
}

// parseEvents decodes the MONITOR reply: array of [seq, at_us, model,
// texts, latency_us, err, cached_texts, attributed, dispatch_us, admit_us,
// run_us] arrays. A server predating the attribution sends the first six
// fields only: such an event is read without an attribution rather than as a
// run that spent no time.
func parseEvents(r Reply) []Event {
	if r.Type != '*' {
		return nil
	}
	evs := make([]Event, 0, len(r.Elems))
	for _, row := range r.Elems {
		if row.Type != '*' || len(row.Elems) < 6 {
			continue
		}
		e := Event{
			Seq:  uint64(row.Elems[0].Int),
			AtUs: row.Elems[1].Int,
		}
		if row.Elems[2].Type == '$' || row.Elems[2].Type == '+' {
			e.Model = sanitize(row.Elems[2].Str)
		}
		e.Texts = int(row.Elems[3].Int)
		e.LatencyUs = row.Elems[4].Int
		e.Err = row.Elems[5].Int != 0
		if len(row.Elems) >= 11 {
			e.CachedTexts = int(row.Elems[6].Int)
			e.Attributed = row.Elems[7].Int != 0
			e.DispatchUs = row.Elems[8].Int
			e.AdmitUs = row.Elems[9].Int
			e.RunUs = row.Elems[10].Int
		}
		evs = append(evs, e)
	}
	return evs
}

// writePoll appends the pipelined EMB.MODELS / EMB.INFO / EMB.STATS commands
// and the MONITOR incremental fetch.
func (c *Client) writePoll(known []string, afterSeq uint64) error {
	if err := c.WriteArgv("EMB.MODELS"); err != nil {
		return err
	}
	for _, name := range known {
		if err := c.WriteArgv("EMB.INFO", name); err != nil {
			return err
		}
	}
	if err := c.WriteArgv("EMB.STATS"); err != nil {
		return err
	}
	// INFO cpu carries the node's own gomaxprocs, so CPU is measured against
	// the node's parallelism rather than the machine running emb-top.
	if err := c.WriteArgv("INFO", "cpu"); err != nil {
		return err
	}
	return c.WriteArgv("MONITOR", strconv.FormatUint(afterSeq, 10), "512")
}

// Models sends EMB.MODELS and returns the models the node reports. The
// connection must already be established (EnsureConn).
func (c *Client) Models() ([]ModelListEntry, error) {
	_ = c.SetDeadline(time.Now().Add(c.Timeout()))
	if err := c.WriteArgv("EMB.MODELS"); err != nil {
		return nil, err
	}
	if err := c.Flush(); err != nil {
		return nil, err
	}
	rep, err := c.ReadReply()
	if err != nil {
		// The stream is no longer at a reply boundary: drop the connection so
		// the next call dials clean instead of misparsing what is left.
		_ = c.Close()
		return nil, err
	}
	if err := rep.Err(); err != nil {
		return nil, fmt.Errorf("EMB.MODELS: %w", err)
	}
	return parseModelList(rep), nil
}

// Embed sends one EMB request and reads its reply on the current connection
// (EnsureConn). The reply's values are not decoded: a caller driving load
// measures the round trip, not the vector. A server-side error is returned so it
// is counted rather than read as success, and a reply that cannot be read closes
// the connection so the caller's next request starts from a clean stream.
func (c *Client) Embed(model, text string) error {
	_ = c.SetDeadline(time.Now().Add(c.Timeout()))
	if err := c.WriteArgv("EMB", model, text); err != nil {
		return err
	}
	if err := c.Flush(); err != nil {
		return err
	}
	rep, err := c.ReadReply()
	if err != nil {
		// A read that failed (timeout, reset, unparseable reply) leaves the
		// connection mid-stream. Closing it makes the next request re-dial, so
		// one failure cannot become an unbounded run of spurious ones.
		_ = c.Close()
		return err
	}
	if err := rep.Err(); err != nil {
		return fmt.Errorf("EMB %s: %w", model, err)
	}
	return nil
}

// parseModelList decodes the EMB.MODELS reply: array of [name, dim, status].
func parseModelList(r Reply) []ModelListEntry {
	var out []ModelListEntry
	for _, row := range r.Elems {
		if row.Type != '*' || len(row.Elems) < 3 {
			continue
		}
		dim := 0
		if row.Elems[1].Type == ':' {
			dim = int(row.Elems[1].Int)
		}
		out = append(out, ModelListEntry{
			Name:   sanitize(row.Elems[0].String()),
			Dim:    dim,
			Status: sanitize(row.Elems[2].String()),
		})
	}
	return out
}

// sanitize strips control characters from server-provided display strings so a
// hostile or corrupted node cannot inject terminal escape sequences into the
// dashboard (CWE-150). Printable Unicode is preserved.
func sanitize(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// pairMap flattens a field/value RESP array into a map.
func pairMap(r Reply) map[string]Reply {
	m := make(map[string]Reply)
	elems := r.Elems
	for i := 0; i+1 < len(elems); i += 2 {
		m[elems[i].String()] = elems[i+1]
	}
	return m
}

func intField(m map[string]Reply, key string) int64 {
	if r, ok := m[key]; ok && r.Type == ':' {
		return r.Int
	}
	return 0
}

func strField(m map[string]Reply, key string) string {
	if r, ok := m[key]; ok && (r.Type == '$' || r.Type == '+') {
		return sanitize(r.Str)
	}
	return ""
}

// parseStats decodes the EMB.STATS reply into a PollResult.
func parseStats(r Reply) *PollResult {
	m := pairMap(r)
	res := &PollResult{
		UptimeSecs:      intField(m, "uptime_secs"),
		TotalRequests:   intField(m, "total_requests"),
		TotalTokens:     intField(m, "total_tokens"),
		TotalErrors:     intField(m, "total_errors"),
		ActiveRequests:  intField(m, "active_requests"),
		Connections:     intField(m, "connections"),
		TruncatedTexts:  intField(m, "truncated_texts"),
		TruncatedPairs:  intField(m, "truncated_pairs"),
		TruncatedImages: intField(m, "truncated_images"),
		ModelsLoaded:    int(intField(m, "models_loaded")),
		MemMB:           intField(m, "mem"),
		CPUUserUsec:     intField(m, "cpu_user_usec"),
		CPUSysUsec:      intField(m, "cpu_sys_usec"),
		Goroutines:      intField(m, "goroutines"),
		CacheHits:       intField(m, "cache_hits"),
		CacheMisses:     intField(m, "cache_misses"),
		CacheEvictions:  intField(m, "cache_evictions"),

		CPUBudgetEnabled: intField(m, "cpu_budget_enabled") != 0,
		CPUBudget:        intField(m, "cpu_budget"),
		CPUCapacity:      intField(m, "cpu_capacity"),
		CPUReserve:       intField(m, "cpu_reserve"),
		CPUActiveTokens:  intField(m, "cpu_active_tokens"),
		CPUPeakTokens:    intField(m, "cpu_peak_tokens"),
		CPUAllowanceSrc:  strField(m, "cpu_allowance_source"),

		CPUAdmissionWaitUs:   intField(m, "cpu_admission_wait_us"),
		CPUAdmissionWaiters:  intField(m, "cpu_admission_waiters"),
		CPUAdmissionTimeouts: intField(m, "cpu_admission_timeouts"),

		CPUThrottlePeriods:  intField(m, "cpu_throttle_periods"),
		CPUThrottledPeriods: intField(m, "cpu_throttled_periods"),
		CPUThrottleUsec:     intField(m, "cpu_throttle_usec"),
		CPUStealUsec:        intField(m, "cpu_steal_usec"),

		CacheRestoreLimitBytes:    intField(m, "cache_restore_limit_bytes"),
		CacheRestoreHeadroomBytes: intField(m, "cache_restore_headroom_bytes"),
		CacheRestoreSkippedMemory: intField(m, "cache_restore_skipped_memory"),
	}
	if _, ok := m["cpu_budget_enabled"]; ok {
		res.CPUPlanKnown = true
	}
	if raw := strField(m, "cpu_allowance"); raw != "" {
		if allowance, err := strconv.ParseFloat(raw, 64); err == nil {
			res.CPUAllowance, res.CPUAllowanceKnown = allowance, true
		}
	}
	return res
}

// parseInfoCPU extracts the node's own gomaxprocs from an INFO cpu reply (a
// bulk string of key:value lines). 0 when absent or unparseable, so callers
// fall back to their local processor count.
func parseInfoCPU(r Reply) int {
	if r.Type != '$' && r.Type != '+' {
		return 0
	}
	for _, line := range strings.Split(r.Str, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "gomaxprocs:"); ok {
			n, _ := strconv.Atoi(strings.TrimSpace(v))
			if n > 0 {
				return n
			}
		}
	}
	return 0
}

// parseModelStats decodes the EMB.INFO <model> reply.
func parseModelStats(r Reply) *ModelStats {
	m := pairMap(r)
	ms := &ModelStats{
		Dim:              int(intField(m, "dim")),
		MaxLength:        int(intField(m, "max_length")),
		Workers:          int(intField(m, "workers")),
		Requests:         intField(m, "requests"),
		AvgLatencyUs:     intField(m, "avg_latency_us"),
		Tokens:           intField(m, "tokens"),
		Errors:           intField(m, "errors"),
		Pooling:          strField(m, "pooling"),
		Normalize:        strField(m, "normalize") == "true",
		BatchingTimeout:  int(intField(m, "batching_timeout_ms")),
		BatchingMaxBatch: int(intField(m, "batching_max_batch")),
		BatchingMaxToks:  int(intField(m, "batching_max_tokens")),
		PaddingEff:       strField(m, "padding_efficiency"),
		Quantization:     strField(m, "quantization"),
		ModelBytes:       intField(m, "model_bytes"),
		CacheHits:        intField(m, "cache_hits"),
		CacheMisses:      intField(m, "cache_misses"),
		CacheHitRate:     strField(m, "cache_hit_rate"),
		CacheEvictions:   intField(m, "cache_evictions"),
		CacheEntries:     intField(m, "cache_entries"),
		CacheMaxBytes:    intField(m, "cache_max_bytes"),
		CacheCurBytes:    intField(m, "cache_memory_bytes"),

		CPUBudgetEnabled: intField(m, "cpu_budget_enabled") != 0,
		CPUBudget:        int(intField(m, "cpu_budget")),
		CPUThreadShare:   int(intField(m, "cpu_thread_share")),
		IntraOpThreads:   int(intField(m, "intra_op_threads")),
		AllowSpinning:    intField(m, "allow_spinning") != 0,

		EmbedDispatchWaitUs:  int64(intField(m, "embed_dispatch_wait_us")),
		EmbedAdmissionWaitUs: int64(intField(m, "embed_admission_wait_us")),
		EmbedRunUs:           int64(intField(m, "embed_run_us")),
		EmbedRuns:            int64(intField(m, "embed_runs")),
		EmbedSessionsBusy:    int64(intField(m, "embed_sessions_busy")),
		EmbedSessionsTotal:   int64(intField(m, "embed_sessions_total")),
	}
	return ms
}
