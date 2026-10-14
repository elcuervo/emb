package embstress

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

// Conn is one dedicated connection to an endpoint. A worker of a load level
// owns exactly one, because a RESP socket carries one request and one reply at
// a time: two workers interleaving their writes and reads on one socket would
// corrupt the stream and report the corruption as load.
type Conn interface {
	// Embed performs one embedding request. It measures nothing itself: the
	// caller times the call.
	Embed(model, text string) error
	// Close releases the connection.
	Close() error
}

// Endpoint is one node the run offers load to. The production implementation is
// Client; tests supply fakes.
type Endpoint interface {
	// Label is the endpoint's identity in the result.
	Label() string
	// Models returns the models the endpoint reports.
	Models() ([]embtop.ModelListEntry, error)
	// Conn opens one more connection for one worker of a pool.
	Conn() (Conn, error)
	// Close releases the endpoint's own connection.
	Close() error
}

// Client is the RESP endpoint a run drives: the shared embtop poll client plus
// the EMB hot path, and one fresh connection per worker it is asked for.
type Client struct {
	addr     string
	label    string
	password string
	useTLS   bool
	tlsName  string

	poll *embtop.Client // the connection reconciliation and readiness use
}

// NewClient returns an Endpoint that dials addr, labelled label in the result.
func NewClient(addr, label, password string, useTLS bool) *Client {
	return &Client{
		addr:     addr,
		label:    label,
		password: password,
		useTLS:   useTLS,
		poll:     embtop.NewClient(addr, password, useTLS),
	}
}

// SetTLSServerName overrides the name the TLS handshake verifies, on the poll
// connection and on every worker connection opened afterwards.
func (c *Client) SetTLSServerName(name string) {
	c.tlsName = name
	c.poll.SetTLSServerName(name)
}

// Label returns the endpoint's identity in the result.
func (c *Client) Label() string { return c.label }

// Models dials when needed and returns the models the endpoint reports.
func (c *Client) Models() ([]embtop.ModelListEntry, error) {
	if _, err := c.poll.EnsureConn(); err != nil {
		return nil, err
	}
	return c.poll.Models()
}

// Conn opens one more connection to the same address, for one worker.
func (c *Client) Conn() (Conn, error) {
	w := embtop.NewClient(c.addr, c.password, c.useTLS)
	if c.tlsName != "" {
		w.SetTLSServerName(c.tlsName)
	}
	if err := w.Dial(); err != nil {
		return nil, err
	}
	return &clientConn{c: w}, nil
}

// Close closes the poll connection.
func (c *Client) Close() error { return c.poll.Close() }

// clientConn is one worker's dedicated connection. It re-dials when the
// connection was dropped, so one lost socket costs one error rather than every
// remaining request of the level.
type clientConn struct{ c *embtop.Client }

func (w *clientConn) Embed(model, text string) error {
	if _, err := w.c.EnsureConn(); err != nil {
		return err
	}
	return w.c.Embed(model, text)
}

func (w *clientConn) Close() error { return w.c.Close() }

// sample is one completed request, as the worker observed it.
type sample struct {
	start time.Time
	lat   time.Duration
	err   bool
}

// LevelResult is one load level's numbers, drawn only from the samples after
// its warm-up.
type LevelResult struct {
	Workers   int     `json:"workers"`
	WindowSec float64 `json:"window_secs"`
	Offered   float64 `json:"offered_rps"`   // requests attempted per second
	Completed float64 `json:"completed_rps"` // successful requests per second
	Errors    int64   `json:"errors"`
	P50Us     int64   `json:"lat_p50_us"`
	P95Us     int64   `json:"lat_p95_us"`
	P99Us     int64   `json:"lat_p99_us"`
}

// EndpointResult is one endpoint's numbers across every level of the run.
type EndpointResult struct {
	Label  string        `json:"label"`
	Addr   string        `json:"addr"`
	Levels []LevelResult `json:"levels"`
}

// Result is one run's outcome: every endpoint's own numbers, the fleet
// aggregate per level, and the reconciliation drift that kept a pair out of the
// workload.
type Result struct {
	Seed      int64            `json:"seed"`
	Target    string           `json:"target"`
	Models    []string         `json:"models"`
	Endpoints []EndpointResult `json:"endpoints"`
	Fleet     []LevelResult    `json:"fleet"`
	Drift     []Drift          `json:"drift,omitempty"`
}

// summarize reduces one level's samples to its result over the measured window
// [from, to): the warm-up before from is discarded, and nothing after to
// belongs to this level.
func summarize(recs []sample, workers int, from, to time.Time) LevelResult {
	res := LevelResult{Workers: workers, WindowSec: to.Sub(from).Seconds()}
	if res.WindowSec <= 0 {
		return res
	}
	var lats []int64
	for _, s := range recs {
		if s.start.Before(from) || !s.start.Before(to) {
			continue
		}
		if s.err {
			res.Errors++
			continue
		}
		lats = append(lats, s.lat.Microseconds())
	}
	res.Offered = float64(len(lats)+int(res.Errors)) / res.WindowSec
	res.Completed = float64(len(lats)) / res.WindowSec
	res.P50Us, res.P95Us, res.P99Us = percentiles(lats)
	return res
}

// mergeLevels adds several endpoints' samples of the same level into the fleet
// aggregate. It sums counters and rates, and recomputes the percentiles over
// every endpoint's samples, which is the fleet's own latency rather than an
// average of per-node percentiles.
func mergeLevels(perEndpoint [][]sample, workersPerEndpoint int, from, to time.Time) LevelResult {
	var all []sample
	for _, recs := range perEndpoint {
		all = append(all, recs...)
	}
	return summarize(all, workersPerEndpoint*len(perEndpoint), from, to)
}

// percentiles returns the nearest-rank p50/p95/p99 of a microsecond sample set.
func percentiles(lats []int64) (p50, p95, p99 int64) {
	if len(lats) == 0 {
		return 0, 0, 0
	}
	sorted := append([]int64(nil), lats...)
	slices.Sort(sorted)
	return sorted[rank(len(sorted), 0.50)], sorted[rank(len(sorted), 0.95)], sorted[rank(len(sorted), 0.99)]
}

// rank maps a fraction to a sorted index using the nearest-rank definition,
// matching the dashboard's percentiles so the two tools agree: idx =
// ceil(n*f) - 1.
func rank(n int, f float64) int {
	idx := int(math.Ceil(float64(n)*f)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return idx
}

// Describe renders one level as a single human line.
func (l LevelResult) Describe() string {
	return fmt.Sprintf("workers %d · offered %.1f/s · completed %.1f/s · p50 %s · p95 %s · p99 %s · %d errors",
		l.Workers, l.Offered, l.Completed,
		embtop.FormatLatency(l.P50Us), embtop.FormatLatency(l.P95Us), embtop.FormatLatency(l.P99Us), l.Errors)
}
