package embstress

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

// Options configures one run.
type Options struct {
	// Target is the specification the endpoints came from, for the report.
	Target string
	// Models are the configured models the fleet is reconciled against.
	Models []ConfiguredModel
	// Levels are the concurrent workers per endpoint, in the order they run.
	Levels []int
	// Dwell is how long each level is held.
	Dwell time.Duration
	// Warmup is the leading portion of each dwell that is discarded.
	Warmup time.Duration
	// Seed drives the text generator; the same seed offers the same texts.
	Seed int64
	// Corpus, when set, replaces generation.
	Corpus []string
}

// Run reconciles the configured models against the fleet, then offers load to
// every endpoint concurrently: one worker pool per endpoint, each level held
// for a dwell with its warm-up discarded.
//
// A pair that fails reconciliation is dropped from the workload and named in
// the result rather than exercised. An endpoint with no pair left is left out
// of the load entirely.
func Run(ctx context.Context, endpoints []Endpoint, opt Options) (*Result, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints to run against")
	}
	if len(opt.Levels) == 0 {
		return nil, fmt.Errorf("no load levels to run")
	}
	if opt.Dwell <= 0 {
		return nil, fmt.Errorf("dwell must be positive")
	}
	if opt.Warmup < 0 || opt.Warmup >= opt.Dwell {
		return nil, fmt.Errorf("warm-up %s must be shorter than the dwell %s", opt.Warmup, opt.Dwell)
	}

	reported := make(map[string][]embtop.ModelListEntry, len(endpoints))
	for _, ep := range endpoints {
		models, err := ep.Models()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ep.Label(), err)
		}
		reported[ep.Label()] = models
	}
	pairs, drift := Reconcile(opt.Models, reported)

	// One workload per endpoint: its own pairs and its own worker pool, so no
	// endpoint's share is an artifact of another's scheduling and a stalled node
	// cannot hide behind its peers.
	modelsByEndpoint := map[string][]string{}
	for _, p := range pairs {
		modelsByEndpoint[p.Endpoint] = append(modelsByEndpoint[p.Endpoint], p.Model)
	}
	var active []Endpoint
	for _, ep := range endpoints {
		if len(modelsByEndpoint[ep.Label()]) > 0 {
			active = append(active, ep)
		}
	}

	res := &Result{Seed: opt.Seed, Target: opt.Target, Drift: drift}
	for _, cm := range opt.Models {
		res.Models = append(res.Models, cm.Name)
	}
	for _, ep := range active {
		res.Endpoints = append(res.Endpoints, EndpointResult{Label: ep.Label(), Addr: endpointAddr(ep)})
	}

	maxLength := runMaxLength(opt.Models)
	if len(active) == 0 {
		// Nothing reconciled: the drift names why. Holding levels against an
		// empty fleet would report zeroes as if they were a measurement.
		return res, nil
	}
	for _, workers := range opt.Levels {
		// Every endpoint is driven at the same level over the same dwell, so the
		// fleet aggregate is a fleet at one load, not endpoints at several.
		start := time.Now()
		measured, end := start.Add(opt.Warmup), start.Add(opt.Dwell)

		recs := make([][]sample, len(active))
		errs := make([]error, len(active))
		var wg sync.WaitGroup
		for i, ep := range active {
			wg.Add(1)
			go func(i int, ep Endpoint) {
				defer wg.Done()
				recs[i], errs[i] = driveLevel(ctx, ep, modelsByEndpoint[ep.Label()], workers, end, opt.Seed, maxLength, opt.Corpus)
			}(i, ep)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return res, err
			}
		}

		for i := range active {
			res.Endpoints[i].Levels = append(res.Endpoints[i].Levels,
				summarize(recs[i], workers, measured, end))
		}
		res.Fleet = append(res.Fleet, mergeLevels(recs, workers, measured, end))
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	return res, nil
}

// endpointAddr reports an endpoint's dial address when it is a RESP client, so
// the result names what was actually dialed.
func endpointAddr(ep Endpoint) string {
	if c, ok := ep.(*Client); ok {
		return c.addr
	}
	return ""
}

// generatorFor is the text source for one worker: the corpus when the run was
// given one, the seeded generator otherwise.
func generatorFor(seed int64, worker, maxLength int, corpus []string) *Generator {
	if len(corpus) > 0 {
		return NewCorpusGenerator(corpus)
	}
	return NewGenerator(seed, worker, maxLength)
}

// runMaxLength is the longest text the run will offer: the smallest maximum
// length among the configured models, so one run's texts fit every model it
// exercises. 0 when the configuration states none anywhere.
func runMaxLength(models []ConfiguredModel) int {
	best := 0
	for _, m := range models {
		if m.MaxLength > 0 && (best == 0 || m.MaxLength < best) {
			best = m.MaxLength
		}
	}
	return best
}

// driveLevel runs one endpoint's worker pool for one level and returns every
// sample it took, warm-up included: the caller's window decides what counts.
// Each worker gets its own connection, so the pool is genuinely concurrent
// rather than several goroutines interleaving on one socket.
func driveLevel(ctx context.Context, ep Endpoint, models []string, workers int, end time.Time, seed int64, maxLength int, corpus []string) ([]sample, error) {
	if workers < 1 {
		workers = 1
	}
	conns := make([]Conn, 0, workers)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for range workers {
		c, err := ep.Conn()
		if err != nil {
			// Named as the client's own connection, so a local limit (open
			// files, a port range) does not read as the endpoint being down.
			return nil, fmt.Errorf("%s: open a worker connection: %w", ep.Label(), err)
		}
		conns = append(conns, c)
	}

	var mu sync.Mutex
	var recs []sample
	var wg sync.WaitGroup

	for w := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			conn := conns[worker]
			// Every level replays the same texts, so levels are comparable; the
			// worker index keeps the workers of one level from offering
			// identical requests in lockstep.
			gen := generatorFor(seed, worker, maxLength, corpus)
			for {
				if ctx.Err() != nil || !time.Now().Before(end) {
					return
				}
				// A closed loop: the next request leaves only when the previous
				// reply arrived, so no arrival scheduler is needed and the
				// server's own batcher still sees the concurrency.
				s := sample{start: time.Now()}
				s.err = conn.Embed(models[worker%len(models)], gen.Next()) != nil
				s.lat = time.Since(s.start)
				mu.Lock()
				recs = append(recs, s)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return recs, nil
}
