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
}

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

	monRep, err := c.ReadReply()
	if err != nil {
		return nil, err
	}
	if err := monRep.Err(); err != nil {
		return nil, fmt.Errorf("MONITOR: %w", err)
	}

	res := parseStats(statsRep)
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
// texts, latency_us, err] arrays.
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
	return c.WriteArgv("MONITOR", strconv.FormatUint(afterSeq, 10), "512")
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
	}
	return res
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
	}
	return ms
}
