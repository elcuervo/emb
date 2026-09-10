// Package embtop provides the polling client, rate sampler, and headless
// output shared by the emb-top TUI and its -once mode.
//
// It speaks RESP2 against a running emb node using only the commands the
// server already exposes (EMB.MODELS, EMB.INFO, EMB.STATS, AUTH). It imports
// nothing from internal/server, so the tool builds without CGo/onnxruntime.
package embtop

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Reply is one RESP2 reply. Only the shapes emb produces are decoded:
// bulk strings ('$'), status ('+'), integers (':'), arrays ('*'), errors
// ('-'), and nil (empty bulk/array).
type Reply struct {
	Type  byte    // '+', '-', ':', '$', '*'
	Str   string  // payload for + / - / $
	Int   int64   // payload for :
	Elems []Reply // payload for *
	Nil   bool    // nil bulk or array
}

// Err returns the error for an error reply, else nil.
func (r Reply) Err() error {
	if r.Type == '-' {
		return errors.New(r.Str)
	}
	return nil
}

// String returns the string payload (bulk/status) or "" for other types.
func (r Reply) String() string { return r.Str }

// Client is a minimal RESP2 client for an emb node.
type Client struct {
	addr      string
	password  string
	useTLS    bool
	ioTimeout time.Duration

	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

// NewClient returns a Client for addr (host:port) with optional password
// (AUTH) and TLS transport.
func NewClient(addr, password string, useTLS bool) *Client {
	return &Client{addr: addr, password: password, useTLS: useTLS}
}

// Addr returns the configured address.
func (c *Client) Addr() string { return c.addr }

// DefaultTimeout bounds the TLS handshake, each poll's write/read round trip,
// and the AUTH exchange, so a stalled peer surfaces as an error (and the
// dashboard's reconnect path) instead of hanging forever.
const DefaultTimeout = 10 * time.Second

// Dial connects (plain TCP or TLS), authenticates if a password is set, and
// waits for the AUTH reply when applicable.
func (c *Client) Dial() error {
	if c.conn != nil {
		_ = c.Close()
	}
	timeout := c.timeout()
	nc, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect %s: %w", c.addr, err)
	}
	// Bound the handshake and AUTH exchange; Poll refreshes the deadline.
	_ = nc.SetDeadline(time.Now().Add(timeout))
	if c.useTLS {
		host := c.addr
		if h, _, err := net.SplitHostPort(c.addr); err == nil {
			host = h
		}
		tc := tls.Client(nc, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tc.Handshake(); err != nil {
			_ = nc.Close()
			return fmt.Errorf("tls handshake %s: %w", c.addr, err)
		}
		nc = tc
	}
	c.conn = nc
	c.r = bufio.NewReader(nc)
	c.w = bufio.NewWriter(nc)

	if c.password != "" {
		if err := c.WriteArgv("AUTH", c.password); err != nil {
			_ = c.Close()
			return err
		}
		if err := c.Flush(); err != nil {
			_ = c.Close()
			return err
		}
		rep, err := c.ReadReply()
		if err != nil {
			_ = c.Close()
			return err
		}
		if err := rep.Err(); err != nil {
			_ = c.Close()
			return fmt.Errorf("auth: %w", err)
		}
	}
	return nil
}

// EnsureConn dials the node when not currently connected, reporting whether a
// new connection was established (so callers can rebase sequence cursors).
func (c *Client) EnsureConn() (dialed bool, err error) {
	if c.conn == nil {
		if err := c.Dial(); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// timeout returns the effective per-operation deadline.
func (c *Client) timeout() time.Duration {
	if c.ioTimeout > 0 {
		return c.ioTimeout
	}
	return DefaultTimeout
}

// SetTimeout overrides the per-operation deadline (tests use short values).
func (c *Client) SetTimeout(d time.Duration) { c.ioTimeout = d }

// closeOnErr closes the connection when err is non-nil, so a broken
// transport is re-established on the next EnsureConn.
func (c *Client) closeOnErr(err error) error {
	if err != nil {
		_ = c.Close()
	}
	return err
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// WriteArgv appends one RESP2 command (array of bulk strings) to the write
// buffer. Call Flush to send. Multiple WriteArgv calls batch into a single
// round trip.
func (c *Client) WriteArgv(args ...string) error {
	w := c.w
	if _, err := w.WriteString("*" + strconv.Itoa(len(args)) + "\r\n"); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := w.WriteString("$" + strconv.Itoa(len(a)) + "\r\n" + a + "\r\n"); err != nil {
			return err
		}
	}
	return nil
}

// Flush sends the buffered commands.
func (c *Client) Flush() error { return c.closeOnErr(c.w.Flush()) }

// Parser bounds: the peer controls bulk lengths, array counts and nesting, so
// cap them before allocating or recursing (a hostile or broken server must not
// be able to exhaust emb-top's memory or stack).
const (
	maxBulkBytes = 16 << 20 // 16 MiB per bulk string
	maxArrayLen  = 1 << 20  // 1M elements per array
	maxDepth     = 32       // nested reply depth
)

// ReadReply decodes one RESP2 reply.
func (c *Client) ReadReply() (Reply, error) { return c.readReply(0) }

func (c *Client) readReply(depth int) (Reply, error) {
	if depth > maxDepth {
		return Reply{}, fmt.Errorf("resp: reply nested deeper than %d", maxDepth)
	}
	prefix, err := c.r.ReadByte()
	if err != nil {
		return Reply{}, err
	}
	switch prefix {
	case '+', '-':
		line, err := c.readLine()
		return Reply{Type: prefix, Str: line}, err
	case ':':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.ParseInt(line, 10, 64)
		return Reply{Type: ':', Int: n}, err
	case '$':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return Reply{}, err
		}
		if n < 0 {
			return Reply{Type: '$', Nil: true}, nil
		}
		if n > maxBulkBytes {
			return Reply{}, fmt.Errorf("resp: bulk length %d exceeds %d", n, maxBulkBytes)
		}
		payload := make([]byte, n)
		if _, err := ioReadFull(c.r, payload); err != nil {
			return Reply{}, err
		}
		if _, err := c.r.Discard(2); err != nil { // trailing CRLF
			return Reply{}, err
		}
		return Reply{Type: '$', Str: string(payload)}, nil
	case '*':
		line, err := c.readLine()
		if err != nil {
			return Reply{}, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return Reply{}, err
		}
		if n < 0 {
			return Reply{Type: '*', Nil: true}, nil
		}
		if n > maxArrayLen {
			return Reply{}, fmt.Errorf("resp: array length %d exceeds %d", n, maxArrayLen)
		}
		rep := Reply{Type: '*', Elems: make([]Reply, 0, n)}
		for i := 0; i < n; i++ {
			el, err := c.readReply(depth + 1)
			if err != nil {
				return Reply{}, err
			}
			rep.Elems = append(rep.Elems, el)
		}
		return rep, nil
	default:
		return Reply{}, fmt.Errorf("resp: unexpected prefix %q", prefix)
	}
}

func (c *Client) readLine() (string, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ioReadFull mirrors io.ReadFull using the bufio.Reader.
func ioReadFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
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
	UptimeSecs     int64
	TotalRequests  int64
	TotalTokens    int64
	TotalErrors    int64
	ActiveRequests int64
	Connections    int64
	TruncatedTexts int64
	TruncatedPairs int64
	ModelsLoaded   int
	MemMB          int64
	CPUUserUsec    int64
	CPUSysUsec     int64
	Goroutines     int64
	CacheHits      int64
	CacheMisses    int64
	CacheEvictions int64

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
	_ = c.conn.SetDeadline(time.Now().Add(c.timeout()))
	if err := c.writePoll(known, afterSeq); err != nil {
		return nil, c.closeOnErr(err)
	}
	if err := c.Flush(); err != nil {
		return nil, err
	}

	modelsRep, err := c.ReadReply()
	if err != nil {
		return nil, c.closeOnErr(err)
	}
	if err := modelsRep.Err(); err != nil {
		// Remaining pipelined replies are unread: drop the connection so the
		// next poll starts clean instead of misparsing stale replies.
		return nil, c.closeOnErr(fmt.Errorf("EMB.MODELS: %w", err))
	}

	perModel := make(map[string]*ModelStats, len(known))
	for _, name := range known {
		rep, err := c.ReadReply()
		if err != nil {
			return nil, c.closeOnErr(err)
		}
		if err := rep.Err(); err != nil {
			// Model disappeared between polls; treat as absent this round.
			continue
		}
		perModel[name] = parseModelStats(rep)
	}

	statsRep, err := c.ReadReply()
	if err != nil {
		return nil, c.closeOnErr(err)
	}
	if err := statsRep.Err(); err != nil {
		// MONITOR's reply is still unread: drop the connection.
		return nil, c.closeOnErr(fmt.Errorf("EMB.STATS: %w", err))
	}

	monRep, err := c.ReadReply()
	if err != nil {
		return nil, c.closeOnErr(err)
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
		UptimeSecs:     intField(m, "uptime_secs"),
		TotalRequests:  intField(m, "total_requests"),
		TotalTokens:    intField(m, "total_tokens"),
		TotalErrors:    intField(m, "total_errors"),
		ActiveRequests: intField(m, "active_requests"),
		Connections:    intField(m, "connections"),
		TruncatedTexts: intField(m, "truncated_texts"),
		TruncatedPairs: intField(m, "truncated_pairs"),
		ModelsLoaded:   int(intField(m, "models_loaded")),
		MemMB:          intField(m, "mem"),
		CPUUserUsec:    intField(m, "cpu_user_usec"),
		CPUSysUsec:     intField(m, "cpu_sys_usec"),
		Goroutines:     intField(m, "goroutines"),
		CacheHits:      intField(m, "cache_hits"),
		CacheMisses:    intField(m, "cache_misses"),
		CacheEvictions: intField(m, "cache_evictions"),
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
