// Package driver is the open-loop RESP workload driver shared by the Fargate
// harness and the in-process inference benchmark.
//
// It offers EMB calls to an external server address on a fixed schedule that is
// independent of response completion, decodes every reply with internal/resp,
// and classifies each scheduled pair exactly once as success, error, timeout or
// dropped, so offered == success + errors + timeouts + dropped always holds.
// Throughput is successful pairs over wall elapsed time; latency is reported
// from the scheduled time and from the wire start so a late scheduler is
// visible. The driver never imports the server, so it measures any emb build.
package driver

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

// Dispatch is how the legs of one pair are issued.
type Dispatch string

const (
	Single   Dispatch = "single"   // one model
	Serial   Dispatch = "serial"   // legs one after another; stops at the first failure
	Parallel Dispatch = "parallel" // legs concurrently on separate connections
	Multi    Dispatch = "multi"    // one EMB.MULTI command carrying every leg
)

// Outcome is the single classification of a scheduled pair.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeError   Outcome = "error"
	OutcomeTimeout Outcome = "timeout"
	OutcomeDropped Outcome = "dropped"
)

// Error kinds, counted per failed leg.
const (
	KindServer    = "server"    // RESP error reply
	KindBusy      = "busy"      // RESP error reply starting "ERR busy": an explicit admission rejection
	KindNull      = "null"      // nil bulk/array/null
	KindMalformed = "malformed" // undecodable reply, wrong type or wrong blob length
	KindNonFinite = "nonfinite" // NaN or Inf in the embedding
	KindTransport = "transport" // dial, read or write failure
	KindTimeout   = "timeout"
)

// Config describes one open-loop pass against an external server.
type Config struct {
	Addr           string
	Models         []string      // one EMB leg per model
	Dispatch       Dispatch      // Single for one model
	Rate           float64       // pairs offered per second
	Duration       time.Duration // scheduling window
	MaxInFlight    int           // pairs in flight; a scheduled pair with no free slot is dropped
	RequestTimeout time.Duration // per-pair deadline from wire start, covering every leg
	DrainTimeout   time.Duration // after the window; unfinished pairs count as timeouts
	Dim            int           // expected embedding dimension
	Texts          func(i int) string
}

func (c Config) validate() error {
	switch {
	case c.Addr == "":
		return errors.New("driver: no server address")
	case len(c.Models) == 0:
		return errors.New("driver: no models")
	case c.Dispatch != Single && c.Dispatch != Serial && c.Dispatch != Parallel && c.Dispatch != Multi:
		return fmt.Errorf("driver: unknown dispatch %q", c.Dispatch)
	case c.Dispatch == Single && len(c.Models) != 1:
		return errors.New("driver: single dispatch takes exactly one model")
	case c.Rate <= 0 || c.Duration <= 0 || c.MaxInFlight <= 0:
		return errors.New("driver: rate, duration and max in-flight must be positive")
	case c.RequestTimeout <= 0 || c.DrainTimeout <= 0:
		return errors.New("driver: request and drain timeouts must be positive")
	case c.Dim <= 0:
		return errors.New("driver: expected dimension must be positive")
	case c.Texts == nil:
		return errors.New("driver: no text source")
	}
	return nil
}

// Quantiles are nearest-rank order statistics in milliseconds (p90 is the
// literal 90th percentile, never an approximation from a neighbour).
type Quantiles struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50_ms"`
	P90   float64 `json:"p90_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
	Max   float64 `json:"max_ms"`
}

// Summarize computes the quantiles of ds; it sorts a copy.
func Summarize(ds []time.Duration) Quantiles {
	if len(ds) == 0 {
		return Quantiles{}
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	at := func(p float64) float64 {
		idx := int(math.Ceil(p*float64(len(s)))) - 1
		return float64(s[max(0, min(len(s)-1, idx))].Microseconds()) / 1000
	}
	return Quantiles{Count: len(s), P50: at(0.50), P90: at(0.90), P95: at(0.95), P99: at(0.99), Max: at(1)}
}

// LegResult is one model's view of the pass; legs are counted when they start,
// so a serial pair that stopped early does not count its unstarted legs.
type LegResult struct {
	Success  int64     `json:"success"`
	Errors   int64     `json:"errors"`
	Timeouts int64     `json:"timeouts"`
	Latency  Quantiles `json:"wire_latency"`
}

// Result is the accounting of one pass.
type Result struct {
	Offered    int64                `json:"offered"`
	Success    int64                `json:"success"`
	Errors     int64                `json:"errors"`
	Timeouts   int64                `json:"timeouts"`
	Dropped    int64                `json:"dropped"`
	Seconds    float64              `json:"seconds"`    // wall elapsed, window plus drain
	Throughput float64              `json:"throughput"` // successful pairs per wall second
	Scheduled  Quantiles            `json:"pair_latency_from_scheduled"`
	Wire       Quantiles            `json:"pair_latency_from_wire_start"`
	Legs       map[string]LegResult `json:"legs"`
	ErrorKinds map[string]int64     `json:"error_kinds"`
}

// Accounted verifies that every offered pair was classified exactly once.
func (r Result) Accounted() error {
	if sum := r.Success + r.Errors + r.Timeouts + r.Dropped; sum != r.Offered {
		return fmt.Errorf("offered %d != success %d + errors %d + timeouts %d + dropped %d (= %d)",
			r.Offered, r.Success, r.Errors, r.Timeouts, r.Dropped, sum)
	}
	return nil
}

// Judge classifies one EMB reply. A leg succeeds only for a bulk holding dim
// float32 values that are all finite; everything else is an error or timeout.
func Judge(rep resp.Reply, err error, dim int) (Outcome, string) {
	if err != nil {
		var ne net.Error
		switch {
		case errors.Is(err, os.ErrDeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
			return OutcomeTimeout, KindTimeout
		case errors.As(err, &ne) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed):
			return OutcomeError, KindTransport
		default:
			return OutcomeError, KindMalformed
		}
	}
	switch {
	case rep.Type == '-':
		if strings.HasPrefix(rep.Str, "ERR busy") {
			return OutcomeError, KindBusy
		}
		return OutcomeError, KindServer
	case rep.Nil:
		return OutcomeError, KindNull
	case rep.Type != '$' || len(rep.Str) != dim*4:
		return OutcomeError, KindMalformed
	}
	for i := range dim {
		b := rep.Str[i*4 : i*4+4]
		bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		if f := math.Float32frombits(bits); math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return OutcomeError, KindNonFinite
		}
	}
	return OutcomeSuccess, ""
}

type legRec struct {
	model   string
	outcome Outcome
	kind    string
	wire    time.Duration
}

type pairRec struct {
	outcome Outcome
	sched   time.Duration
	wire    time.Duration
	legs    []legRec
}

// slot is one in-flight pair's connections, one per leg.
type slot struct{ clients []*resp.Client }

func (s *slot) close() {
	for _, c := range s.clients {
		_ = c.Close()
	}
}

// roundTrip sends one command and reads its reply under an absolute deadline,
// redialling a connection an earlier timeout or decode error closed.
func roundTrip(c *resp.Client, deadline time.Time, args ...string) (resp.Reply, error) {
	if _, err := c.EnsureConn(); err != nil {
		return resp.Reply{}, err
	}
	if err := c.SetDeadline(deadline); err != nil {
		return resp.Reply{}, err
	}
	if err := c.WriteArgv(args...); err != nil {
		return resp.Reply{}, err
	}
	if err := c.Flush(); err != nil {
		return resp.Reply{}, err
	}
	return c.ReadReply()
}

func doLeg(c *resp.Client, model, text string, dim int, deadline time.Time) legRec {
	t0 := time.Now()
	rep, err := roundTrip(c, deadline, "EMB", model, text)
	out, kind := Judge(rep, err, dim)
	return legRec{model: model, outcome: out, kind: kind, wire: time.Since(t0)}
}

// multiLegs validates an EMB.MULTI reply element by element: the array must have
// one valid embedding per model, in order. A transport failure or an error reply
// fails every leg the same way.
func multiLegs(models []string, rep resp.Reply, err error, dim int, wire time.Duration) []legRec {
	legs := make([]legRec, len(models))
	for i, m := range models {
		var out Outcome
		var kind string
		switch {
		case err != nil || rep.Type == '-' || rep.Nil:
			out, kind = Judge(rep, err, dim)
		case rep.Type != '*' || len(rep.Elems) != len(models):
			out, kind = OutcomeError, KindMalformed
		default:
			out, kind = Judge(rep.Elems[i], nil, dim)
		}
		legs[i] = legRec{model: m, outcome: out, kind: kind, wire: wire}
	}
	return legs
}

func runPair(cfg Config, s *slot, text string, scheduled, hardStop time.Time) pairRec {
	start := time.Now()
	deadline := start.Add(cfg.RequestTimeout)
	if deadline.After(hardStop) {
		deadline = hardStop
	}
	legs := make([]legRec, len(cfg.Models))
	switch cfg.Dispatch {
	case Multi:
		args := []string{"EMB.MULTI"}
		for _, m := range cfg.Models {
			args = append(args, m, text)
		}
		t0 := time.Now()
		rep, err := roundTrip(s.clients[0], deadline, args...)
		legs = multiLegs(cfg.Models, rep, err, cfg.Dim, time.Since(t0))
	case Parallel:
		var wg sync.WaitGroup
		for li := range cfg.Models {
			wg.Add(1)
			go func() {
				defer wg.Done()
				legs[li] = doLeg(s.clients[li], cfg.Models[li], text, cfg.Dim, deadline)
			}()
		}
		wg.Wait()
	default:
		for li := range cfg.Models {
			legs[li] = doLeg(s.clients[li], cfg.Models[li], text, cfg.Dim, deadline)
			if legs[li].outcome != OutcomeSuccess {
				legs = legs[:li+1]
				break
			}
		}
	}
	rec := pairRec{outcome: OutcomeSuccess, legs: legs}
	for _, l := range legs {
		switch {
		case l.outcome == OutcomeTimeout:
			rec.outcome = OutcomeTimeout
		case l.outcome == OutcomeError && rec.outcome != OutcomeTimeout:
			rec.outcome = OutcomeError
		}
	}
	end := time.Now()
	rec.sched, rec.wire = end.Sub(scheduled), end.Sub(start)
	return rec
}

// accumulator collects pair records until sealed; a record that arrives after
// the seal is dropped because the seal already counted that pair as a timeout.
type accumulator struct {
	mu                      sync.Mutex
	sealed                  bool
	started, recorded       int64
	success, errs, timeouts int64
	sched, wire             []time.Duration
	legs                    map[string]*legAcc
	kinds                   map[string]int64
}

type legAcc struct {
	success, errs, timeouts int64
	wire                    []time.Duration
}

// finish records a pair, then returns the slot for reuse; after the seal the
// slot is closed instead, so an orphaned pair never leaks its connections.
func (a *accumulator) finish(p pairRec, s *slot, free chan *slot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sealed {
		s.close()
		return
	}
	a.recorded++
	switch p.outcome {
	case OutcomeSuccess:
		a.success++
		a.sched = append(a.sched, p.sched)
		a.wire = append(a.wire, p.wire)
	case OutcomeTimeout:
		a.timeouts++
	default:
		a.errs++
	}
	for _, l := range p.legs {
		la := a.legs[l.model]
		if la == nil {
			la = &legAcc{}
			a.legs[l.model] = la
		}
		switch l.outcome {
		case OutcomeSuccess:
			la.success++
			la.wire = append(la.wire, l.wire)
		case OutcomeTimeout:
			la.timeouts++
			a.kinds[l.kind]++
		default:
			la.errs++
			a.kinds[l.kind]++
		}
	}
	free <- s
}

func (a *accumulator) seal(offered, dropped int64, elapsed time.Duration, drain func()) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sealed = true
	drain()
	// Pairs still in flight at the hard stop are timeouts: they were offered,
	// started, and never produced a reply within the declared bounds.
	a.timeouts += a.started - a.recorded
	r := Result{
		Offered: offered, Success: a.success, Errors: a.errs, Timeouts: a.timeouts, Dropped: dropped,
		Seconds:    elapsed.Seconds(),
		Scheduled:  Summarize(a.sched),
		Wire:       Summarize(a.wire),
		Legs:       map[string]LegResult{},
		ErrorKinds: map[string]int64{},
	}
	r.Throughput = float64(r.Success) / r.Seconds
	for m, la := range a.legs {
		r.Legs[m] = LegResult{Success: la.success, Errors: la.errs, Timeouts: la.timeouts, Latency: Summarize(la.wire)}
	}
	for k, v := range a.kinds {
		r.ErrorKinds[k] = v
	}
	return r
}

// Run offers pairs at cfg.Rate for cfg.Duration, then drains for at most
// cfg.DrainTimeout. Connection setup failures are returned as errors: a run
// that could not reach the server is invalid evidence, not a zero.
func Run(cfg Config) (Result, error) {
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	free := make(chan *slot, cfg.MaxInFlight)
	for range cfg.MaxInFlight {
		s := &slot{}
		for range cfg.Models {
			c := resp.NewClient(cfg.Addr, "", false)
			if err := c.Dial(); err != nil {
				s.close()
				for len(free) > 0 {
					(<-free).close()
				}
				return Result{}, err
			}
			s.clients = append(s.clients, c)
		}
		free <- s
	}

	count := max(1, int(cfg.Rate*cfg.Duration.Seconds()))
	interval := time.Duration(float64(time.Second) / cfg.Rate)
	acc := &accumulator{legs: map[string]*legAcc{}, kinds: map[string]int64{}}
	started := time.Now()
	hardStop := started.Add(cfg.Duration + cfg.DrainTimeout)
	var offered, dropped int64
	var wg sync.WaitGroup
	for i := range count {
		scheduled := started.Add(time.Duration(i) * interval)
		if wait := time.Until(scheduled); wait > 0 {
			time.Sleep(wait)
		}
		offered++
		select {
		case s := <-free:
			text := cfg.Texts(i)
			acc.mu.Lock()
			acc.started++
			acc.mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				p := runPair(cfg, s, text, scheduled, hardStop)
				acc.finish(p, s, free)
			}()
		default:
			dropped++
		}
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	timer := time.NewTimer(time.Until(hardStop))
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	// The last pair is scheduled one interval before the window ends; elapsed
	// covers the whole window so throughput cannot exceed the offered rate.
	res := acc.seal(offered, dropped, max(time.Since(started), cfg.Duration), func() {
		for len(free) > 0 {
			(<-free).close()
		}
	})
	return res, res.Accounted()
}
