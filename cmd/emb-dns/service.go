package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/emoji"
)

// Errors the service reports to its surfaces. Each maps to a reply a resolver
// understands: a refusal, a server failure, or a name that does not exist.
var (
	// ErrNotReady is a query that arrives before the vocabulary is indexed.
	// It is a server failure, never a negative answer: the name may be
	// perfectly good, the service just cannot rank it yet.
	ErrNotReady = errors.New("zone: the vocabulary is not ready")
	// ErrRateLimited is a query past its source's allowance.
	ErrRateLimited = errors.New("zone: rate limited")
)

// Embedder is what the zone needs from the emb server beside it: the vectors
// for the vocabulary's descriptions, and the vector a query composes to.
type Embedder interface {
	Embed(texts []string) ([][]float32, error)
	Compose(query emoji.Query) ([]float32, error)
}

// Outcome is one answered query.
type Outcome struct {
	// Name is the query name as it was asked.
	Name string
	// Results are the ranked emoji, best first.
	Results []emoji.Result
	// Phrase is the ranked glyphs joined with no separator, and is empty for a
	// query that is not a sentence: ask in words and the answer is a sentence of
	// emoji, ask in emoji and the answer is one emoji.
	Phrase string
}

// Stats counts what the zone served, so the statistics route can report it
// without reading a log. Nothing here records a query name.
type Stats struct {
	Served         atomic.Int64
	Unparseable    atomic.Int64
	RateLimited    atomic.Int64
	UpstreamErrors atomic.Int64
	UpstreamCalls  atomic.Int64
	UpstreamNanos  atomic.Int64
}

// StatsSnapshot is the statistics route's body: what the zone served, and the
// mean upstream latency those calls averaged.
type StatsSnapshot struct {
	Served         int64 `json:"served"`
	Unparseable    int64 `json:"unparseable"`
	RateLimited    int64 `json:"rate_limited"`
	UpstreamErrors int64 `json:"upstream_errors"`
	UpstreamCalls  int64 `json:"upstream_calls"`
	UpstreamMeanUS int64 `json:"upstream_mean_us"`
}

// Snapshot renders the counters for the statistics route.
func (s *Stats) Snapshot() StatsSnapshot {
	calls := s.UpstreamCalls.Load()
	snapshot := StatsSnapshot{
		Served:         s.Served.Load(),
		Unparseable:    s.Unparseable.Load(),
		RateLimited:    s.RateLimited.Load(),
		UpstreamErrors: s.UpstreamErrors.Load(),
		UpstreamCalls:  calls,
	}
	if calls > 0 {
		snapshot.UpstreamMeanUS = s.UpstreamNanos.Load() / calls / int64(time.Microsecond)
	}
	return snapshot
}

// Service answers queries: it owns the vocabulary, the index, and the policy
// (readiness, rate limiting) that decides whether a query is answered at all.
type Service struct {
	cfg      Config
	vocab    emoji.Vocab
	upstream Embedder
	stats    Stats
	limiter  *limiter

	// index is nil until the vocabulary has been embedded. A query that
	// arrives before that is refused rather than answered from a partial one.
	index atomic.Pointer[emoji.Index]
}

// NewService wires a service over a loaded vocabulary and a live upstream.
func NewService(cfg Config, vocab emoji.Vocab, upstream Embedder) *Service {
	s := &Service{cfg: cfg, vocab: vocab, upstream: upstream}
	if cfg.RateLimit > 0 {
		s.limiter = newLimiter(cfg.RateLimit)
	}
	return s
}

// BuildIndex embeds every vocabulary description through the model and holds
// the resulting matrix. It is the only work the zone does at boot, and a
// failure here leaves the service unready rather than serving a partial index.
func (s *Service) BuildIndex(ctx context.Context) error {
	if s.vocab.Len() == 0 {
		return fmt.Errorf("zone: the vocabulary is empty")
	}
	descriptions := make([]string, s.vocab.Len())
	for i, entry := range s.vocab.Entries {
		descriptions[i] = entry.Description
	}

	started := time.Now()
	vectors, err := s.upstream.Embed(descriptions)
	if err != nil {
		return fmt.Errorf("zone: build index: %w", err)
	}
	if len(vectors) != s.vocab.Len() {
		return fmt.Errorf("zone: build index: %d vectors for %d entries", len(vectors), s.vocab.Len())
	}
	dim := 0
	for i, vector := range vectors {
		if len(vector) == 0 {
			return fmt.Errorf("zone: build index: entry %q has no vector", s.vocab.Entries[i].Slug)
		}
		if dim == 0 {
			dim = len(vector)
		} else if len(vector) != dim {
			return fmt.Errorf("zone: build index: entry %q has %d dimensions, want %d", s.vocab.Entries[i].Slug, len(vector), dim)
		}
	}

	matrix := make([]float32, 0, dim*s.vocab.Len())
	for _, vector := range vectors {
		matrix = append(matrix, vector...)
	}
	index, err := emoji.NewIndex(s.vocab, dim, matrix)
	if err != nil {
		return fmt.Errorf("zone: build index: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.index.Store(index)
	// #nosec G706 -- counts and a duration, and the vocabulary is operator configuration.
	log.Printf("zone: indexed %d entries at %d dimensions in %s", s.vocab.Len(), dim, time.Since(started).Round(time.Millisecond))
	return nil
}

// Ready reports whether the vocabulary has been indexed.
func (s *Service) Ready() bool { return s.index.Load() != nil }

// Answer ranks a query's labels. The name is a sentence, a composition of
// terms, or a conjunction of them; the answer is the vocabulary ordered by
// similarity to the composed vector, or by joint similarity to every term of a
// conjunction.
func (s *Service) Answer(labels []string, source string) (Outcome, error) {
	name := s.Name(labels)
	if s.cfg.LogQueries {
		// #nosec G706 -- %q quotes the name, and this line only runs when the operator asked for it.
		log.Printf("zone: query %q from %s", name, source)
	}
	index := s.index.Load()
	if index == nil {
		return Outcome{Name: name}, ErrNotReady
	}
	parsed, err := emoji.Parse(labels)
	if err != nil {
		// A name the grammar cannot read is refused, and nothing is ranked.
		// This runs before the limiter: reading a name costs nothing, and a
		// caller who sent nonsense should be told so rather than told to slow
		// down.
		s.stats.Unparseable.Add(1)
		return Outcome{Name: name}, err
	}
	query := s.vocab.Spell(parsed)
	if s.limiter != nil && !s.limiter.allow(source, time.Now()) {
		// Past its allowance, the caller gets nothing: the vector is the work
		// being protected, and it is the only thing the limit guards.
		s.stats.RateLimited.Add(1)
		return Outcome{Name: name}, ErrRateLimited
	}

	// A conjunction asks what its terms have in common and needs every term's
	// own vector, so it embeds each of them; a composition is one vector and
	// stays on the preset that folds the terms.
	started := time.Now()
	var (
		vector []float32
		terms  [][]float32
	)
	if query.Conjunction {
		texts := make([]string, len(query.Terms))
		for i, term := range query.Terms {
			texts[i] = term.Text
		}
		terms, err = s.upstream.Embed(texts)
	} else {
		vector, err = s.upstream.Compose(query)
	}
	s.stats.UpstreamCalls.Add(1)
	s.stats.UpstreamNanos.Add(int64(time.Since(started)))
	if err != nil {
		s.stats.UpstreamErrors.Add(1)
		return Outcome{Name: name}, err
	}

	var results []emoji.Result
	if query.Conjunction {
		results, err = index.RankJoint(terms, s.vocab.Indices(parsed), s.cfg.TopK)
	} else {
		results, err = index.Rank(vector, s.cfg.TopK)
	}
	if err != nil {
		s.stats.UpstreamErrors.Add(1)
		return Outcome{Name: name}, err
	}
	s.stats.Served.Add(1)
	outcome := Outcome{Name: name, Results: results}
	if query.Sentence {
		var phrase strings.Builder
		for _, result := range results {
			phrase.WriteString(result.Entry.Glyph)
		}
		outcome.Phrase = phrase.String()
	}
	return outcome, nil
}

// Name is the query name as the zone would spell it, which is the labels
// joined by the dots the transport carried them in.
func (s *Service) Name(labels []string) string {
	name := strings.Join(labels, ".")
	if name == "" {
		return s.cfg.FQDN()
	}
	return name + "." + s.cfg.FQDN()
}

// Meta is what a client surface reads instead of typing its own numbers.
func (s *Service) Meta() map[string]any {
	meta := map[string]any{
		"zone":         s.cfg.FQDN(),
		"model":        s.cfg.Model,
		"entries":      s.vocab.Len(),
		"top_k":        s.cfg.TopK,
		"ttl":          s.cfg.TTL,
		"negative_ttl": s.cfg.NegativeTTL,
		"ready":        s.Ready(),
		"license":      s.vocab.License,
		"source":       s.vocab.Source,
	}
	if index := s.index.Load(); index != nil {
		meta["dimensions"] = index.Dim()
	}
	return meta
}

// limiter is a per-source token bucket with a one-second burst. The map is
// guarded by one lock: a query costs a map lookup and a few float operations,
// so a per-source lock would cost more than it saves. Swap it for a sharded
// map if the zone ever serves enough sources to make the lock the bottleneck.
type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxSources bounds the bucket map, so an attacker rotating source addresses
// cannot grow it without bound. Past this many tracked sources the stalest
// entries are dropped, which at worst refunds a flooder's allowance.
const maxSources = 4096

func newLimiter(rate float64) *limiter {
	return &limiter{
		rate:    rate,
		burst:   max(rate, 1),
		buckets: make(map[string]*bucket),
	}
}

// allow reports whether a query from source may be answered now.
func (l *limiter) allow(source string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) >= maxSources {
		l.prune(now)
	}
	b, ok := l.buckets[source]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[source] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// prune drops buckets that have refilled completely, which are the ones whose
// source has stopped sending. The caller holds the lock.
func (l *limiter) prune(now time.Time) {
	for source, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, source)
		}
	}
}
