package driver

import (
	"fmt"
	"strconv"
	"time"

	"github.com/elcuervo/emb/internal/resp"
)

// Snapshot is the server's own counters at one instant: the flat EMB.STATS map
// and, when models are named, one EMB.INFO map per model. EMB.INFO initializes
// a lazy model, so a cold-first-use pass observes with no models (STATS never
// loads anything).
type Snapshot struct {
	Stats  map[string]string            `json:"stats"`
	Models map[string]map[string]string `json:"models,omitempty"`
}

// fields flattens a RESP2 pair array or a RESP3 map into string values.
func fields(rep resp.Reply) (map[string]string, error) {
	if rep.Type == '-' {
		return nil, rep.Err()
	}
	if (rep.Type != '*' && rep.Type != '%') || len(rep.Elems)%2 != 0 {
		return nil, fmt.Errorf("driver: expected a key/value reply, got type %q with %d elements", rep.Type, len(rep.Elems))
	}
	out := make(map[string]string, len(rep.Elems)/2)
	for i := 0; i+1 < len(rep.Elems); i += 2 {
		v := rep.Elems[i+1]
		switch v.Type {
		case ':':
			out[rep.Elems[i].Str] = strconv.FormatInt(v.Int, 10)
		case ',':
			out[rep.Elems[i].Str] = strconv.FormatFloat(v.Float, 'g', -1, 64)
		default:
			out[rep.Elems[i].Str] = v.Str
		}
	}
	return out, nil
}

// Observe reads EMB.STATS and, per named model, EMB.INFO from the server.
func Observe(addr string, models []string) (Snapshot, error) {
	c := resp.NewClient(addr, "", false)
	if err := c.Dial(); err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = c.Close() }()
	read := func(args ...string) (map[string]string, error) {
		rep, err := roundTrip(c, time.Now().Add(10*time.Second), args...)
		if err != nil {
			return nil, err
		}
		return fields(rep)
	}
	snap := Snapshot{}
	var err error
	if snap.Stats, err = read("EMB.STATS"); err != nil {
		return Snapshot{}, fmt.Errorf("EMB.STATS: %w", err)
	}
	for _, m := range models {
		info, err := read("EMB.INFO", m)
		if err != nil {
			return Snapshot{}, fmt.Errorf("EMB.INFO %s: %w", m, err)
		}
		if snap.Models == nil {
			snap.Models = map[string]map[string]string{}
		}
		snap.Models[m] = info
	}
	return snap, nil
}

// Counters are the integer counter deltas between two snapshots. A field the
// server did not report in both snapshots is absent: older releases lack newer
// counters, and the bundle records them unavailable rather than zero.
type Counters struct {
	Stats  map[string]int64            `json:"stats"`
	Models map[string]map[string]int64 `json:"models,omitempty"`
}

func deltaInts(before, after map[string]string) map[string]int64 {
	out := map[string]int64{}
	for k, av := range after {
		a, err := strconv.ParseInt(av, 10, 64)
		if err != nil {
			continue
		}
		b, err := strconv.ParseInt(before[k], 10, 64)
		if err != nil {
			continue
		}
		out[k] = a - b
	}
	return out
}

// Delta subtracts before from after.
func Delta(before, after Snapshot) Counters {
	c := Counters{Stats: deltaInts(before.Stats, after.Stats)}
	for m, info := range after.Models {
		if c.Models == nil {
			c.Models = map[string]map[string]int64{}
		}
		c.Models[m] = deltaInts(before.Models[m], info)
	}
	return c
}

// PhaseConfig is one pass over a corpus with the cache state it asserts.
type PhaseConfig struct {
	Config
	Cache  string  // CacheWarm or CacheCold
	Corpus *Corpus // text source; Config.Texts is set from it
	Info   bool    // observe EMB.INFO per model (initializes lazy models)
}

// PhaseResult is the pass accounting plus the server counters around it.
type PhaseResult struct {
	Cache    string   `json:"cache"`
	Result   Result   `json:"result"`
	Counters Counters `json:"counters"`
	Before   Snapshot `json:"before"`
	After    Snapshot `json:"after"`
}

// Prewarm sends every warm text to every model once, so a warm pass measures
// cache hits and not first inferences. Any failed reply is an error: a corpus
// that could not be warmed is not a warm phase.
func Prewarm(cfg Config, c *Corpus) error {
	cl := resp.NewClient(cfg.Addr, "", false)
	if err := cl.Dial(); err != nil {
		return err
	}
	defer func() { _ = cl.Close() }()
	for _, text := range c.Warm() {
		for _, m := range cfg.Models {
			rep, err := roundTrip(cl, time.Now().Add(cfg.RequestTimeout), "EMB", m, text)
			if out, kind := Judge(rep, err, cfg.Dim); out != OutcomeSuccess {
				return fmt.Errorf("prewarm %s: %s (%s)", m, out, kind)
			}
		}
	}
	return nil
}

// RunPhase prewarms a warm phase, snapshots the server, runs the open-loop
// pass, and snapshots again. The returned error covers setup, observation and
// accounting; outcomes of the pass itself live in the result.
func RunPhase(pc PhaseConfig) (PhaseResult, error) {
	if pc.Cache != CacheWarm && pc.Cache != CacheCold {
		return PhaseResult{}, fmt.Errorf("driver: unknown cache phase %q", pc.Cache)
	}
	pc.Texts = func(i int) string { return pc.Corpus.Text(pc.Cache, i) }
	if err := pc.validate(); err != nil {
		return PhaseResult{}, err
	}
	if pc.Cache == CacheWarm {
		if err := Prewarm(pc.Config, pc.Corpus); err != nil {
			return PhaseResult{}, err
		}
	}
	var models []string
	if pc.Info {
		models = pc.Models
	}
	before, err := Observe(pc.Addr, models)
	if err != nil {
		return PhaseResult{}, err
	}
	res, runErr := Run(pc.Config)
	if runErr != nil && res.Offered == 0 {
		return PhaseResult{}, runErr
	}
	after, err := Observe(pc.Addr, models)
	if err != nil {
		return PhaseResult{}, err
	}
	return PhaseResult{Cache: pc.Cache, Result: res, Counters: Delta(before, after), Before: before, After: after}, runErr
}

// CheckCache asserts the cache state the phase claims, from the server's own
// counters: a warm phase has zero misses and a hit per successful leg; a cold
// phase has zero hits and a miss per successful leg.
func (p PhaseResult) CheckCache() error {
	hits, okH := p.Counters.Stats["cache_hits"]
	misses, okM := p.Counters.Stats["cache_misses"]
	if !okH || !okM {
		return fmt.Errorf("server reports no cache counters (is the cache enabled?)")
	}
	var legs int64
	for _, l := range p.Result.Legs {
		legs += l.Success
	}
	switch p.Cache {
	case CacheWarm:
		if misses != 0 || hits < legs {
			return fmt.Errorf("warm phase: %d cache misses, %d hits for %d successful legs; want 0 misses and a hit per leg", misses, hits, legs)
		}
	case CacheCold:
		if hits != 0 || misses < legs {
			return fmt.Errorf("cold phase: %d cache hits, %d misses for %d successful legs; want 0 hits and a miss per leg", hits, misses, legs)
		}
	}
	return nil
}
