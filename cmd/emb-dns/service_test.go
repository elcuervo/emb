package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/emoji"
)

// fixtureVocab is a vocabulary small enough to reason about, with descriptions
// that map onto three clearly separated vectors.
const fixtureVocab = `{"source":"test","license":"test","entries":[
 {"glyph":"🦈","slug":"shark","name":"shark","description":"shark"},
 {"glyph":"😭","slug":"loudly_crying_face","name":"loudly crying face","description":"loudly crying face"},
 {"glyph":"🎉","slug":"party_popper","name":"party popper","description":"party popper"},
 {"glyph":"🐟","slug":"fish","name":"fish","description":"fish"}]}`

// fixtureVectors place each description on its own axis, so a fake composition
// lands on a known entry:
//
//	shark             [1 0 0]
//	loudly crying     [0 1 0]
//	party popper      [0 0 1]
//	fish              [0.8 0.6 0]
var fixtureVectors = map[string][]float32{
	"shark":              {1, 0, 0},
	"loudly crying face": {0, 1, 0},
	"party popper":       {0, 0, 1},
	"fish":               {0.8, 0.6, 0},
}

// fakeEmbedder stands in for the emb server: it answers the index build from a
// table and returns one composed vector, recording the query it was handed so a
// test can prove what crossed the boundary.
type fakeEmbedder struct {
	mu         sync.Mutex
	vectors    map[string][]float32
	composed   []float32
	queries    []emoji.Query
	embedErr   error
	compErr    error
	embedCalls int
}

func newFakeEmbedder() *fakeEmbedder {
	return &fakeEmbedder{vectors: fixtureVectors, composed: []float32{1, 0, 0}}
}

func (f *fakeEmbedder) Embed(texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.embedCalls++
	if f.embedErr != nil {
		return nil, f.embedErr
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vector, ok := f.vectors[text]
		if !ok {
			// Text the fake does not model still gets a direction, so a free
			// sentence produces a deterministic ranking rather than a NaN.
			vector = []float32{0.5, 0.5, 0.5}
		}
		out[i] = append([]float32(nil), vector...)
	}
	return out, nil
}

func (f *fakeEmbedder) Compose(query emoji.Query) ([]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.compErr != nil {
		return nil, f.compErr
	}
	f.queries = append(f.queries, query)
	return append([]float32(nil), f.composed...), nil
}

func (f *fakeEmbedder) lastQuery() emoji.Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) == 0 {
		return emoji.Query{}
	}
	return f.queries[len(f.queries)-1]
}

// errTestUpstream is a failure the service must report as its own, never as an
// answer about the name.
var errTestUpstream = errors.New("test: the upstream failed")

func testConfig() Config {
	cfg := Default()
	cfg.ListenUDP = ""
	cfg.ListenTCP = ""
	cfg.ListenHTTP = "127.0.0.1:0"
	cfg.TopK = 3
	return cfg
}

// newTestService builds a ready service over the fixture vocabulary.
func newTestService(t *testing.T, tweak func(*Config)) (*Service, *fakeEmbedder) {
	t.Helper()
	cfg := testConfig()
	if tweak != nil {
		tweak(&cfg)
	}
	vocab, err := emoji.LoadBytes([]byte(fixtureVocab))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	fake := newFakeEmbedder()
	service := NewService(cfg, vocab, fake)
	if err := service.BuildIndex(context.Background()); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return service, fake
}

func TestBuildIndexEmbedsEveryDescription(t *testing.T) {
	service, fake := newTestService(t, nil)
	if !service.Ready() {
		t.Fatal("the service is not ready after a successful index build")
	}
	if fake.embedCalls != 1 {
		t.Fatalf("the index was built in %d calls, want one batched call", fake.embedCalls)
	}
	if dim := service.index.Load().Dim(); dim != 3 {
		t.Fatalf("index dimension is %d, want 3", dim)
	}
}

func TestBuildIndexRefusesWhatItCannotIndex(t *testing.T) {
	vocab, err := emoji.LoadBytes([]byte(fixtureVocab))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	cases := map[string]*fakeEmbedder{
		"the upstream failed":      {vectors: fixtureVectors, embedErr: fmt.Errorf("boom")},
		"a vector is empty":        {vectors: map[string][]float32{"shark": {}}},
		"a vector has wrong width": {vectors: map[string][]float32{"shark": {1, 0, 0, 0}, "fish": {1, 0, 0}}},
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			service := NewService(testConfig(), vocab, fake)
			if err := service.BuildIndex(context.Background()); err == nil {
				t.Fatal("BuildIndex accepted an index it could not build")
			}
			if service.Ready() {
				t.Fatal("a failed build left the service ready")
			}
		})
	}
}

func TestAnswerRanksTheComposedVector(t *testing.T) {
	service, fake := newTestService(t, nil)
	fake.composed = []float32{0, 0, 1} // the party-popper axis
	outcome, err := service.Answer([]string{"anything", "at", "all"}, "192.0.2.1")
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(outcome.Results) != 3 {
		t.Fatalf("got %d results, want top_k=3", len(outcome.Results))
	}
	if outcome.Results[0].Entry.Slug != "party_popper" {
		t.Fatalf("best result is %q, want party_popper", outcome.Results[0].Entry.Slug)
	}
	if outcome.Name != "anything.at.all.dns.emb.is." {
		t.Fatalf("name is %q", outcome.Name)
	}
	var want strings.Builder
	for _, result := range outcome.Results {
		want.WriteString(result.Entry.Glyph)
	}
	if outcome.Phrase != want.String() {
		t.Fatalf("a sentence's phrase is %q, want the ranked glyphs joined (%q)", outcome.Phrase, want.String())
	}
	if service.stats.Served.Load() != 1 {
		t.Fatalf("served counter is %d, want 1", service.stats.Served.Load())
	}
}

func TestAnswerRefusesBadNamesAndCountsThem(t *testing.T) {
	service, fake := newTestService(t, nil)
	cases := []struct {
		name   string
		labels []string
		want   error
	}{
		{"empty", []string{""}, emoji.ErrEmpty},
		{"operators only", []string{"-"}, emoji.ErrNoTerms},
		{"over-long label", []string{strings.Repeat("a", emoji.MaxLabelBytes+1)}, emoji.ErrTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Answer(tc.labels, "192.0.2.1"); !errors.Is(err, tc.want) {
				t.Fatalf("Answer gave %v, want %v", err, tc.want)
			}
		})
	}
	if got := service.stats.Unparseable.Load(); got != int64(len(cases)) {
		t.Fatalf("unparseable counter is %d, want %d", got, len(cases))
	}
	if len(fake.queries) != 0 {
		t.Fatalf("a refused name still reached the model: %+v", fake.queries)
	}
	if service.stats.Served.Load() != 0 {
		t.Fatalf("a refused name counted as served")
	}
}

func TestAnswerRefusesBeforeTheIndexIsReady(t *testing.T) {
	vocab, err := emoji.LoadBytes([]byte(fixtureVocab))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	service := NewService(testConfig(), vocab, newFakeEmbedder())
	if _, err := service.Answer([]string{"shark"}, "192.0.2.1"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Answer gave %v, want ErrNotReady", err)
	}
	if service.stats.Served.Load() != 0 {
		t.Fatal("a query answered before the index existed counted as served")
	}
}

func TestAnswerCountsUpstreamFailures(t *testing.T) {
	service, fake := newTestService(t, nil)
	fake.compErr = fmt.Errorf("upstream is down")
	if _, err := service.Answer([]string{"shark"}, "192.0.2.1"); err == nil {
		t.Fatal("Answer swallowed an upstream failure")
	}
	snapshot := service.stats.Snapshot()
	if snapshot.UpstreamErrors != 1 {
		t.Fatalf("upstream_errors is %d, want 1", snapshot.UpstreamErrors)
	}
	if snapshot.Served != 0 {
		t.Fatal("a failed query counted as served")
	}
}

func TestLimiterBoundsASource(t *testing.T) {
	service, _ := newTestService(t, func(cfg *Config) { cfg.RateLimit = 2 })
	now := time.Now()
	source := "192.0.2.1"
	allowed := 0
	for range 10 {
		if service.limiter.allow(source, now) {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("allowed %d immediate queries, want the burst of 2", allowed)
	}
	if !service.limiter.allow(source, now.Add(2*time.Second)) {
		t.Fatal("the bucket did not refill")
	}
	if !service.limiter.allow("198.51.100.7", now) {
		t.Fatal("one source's flood consumed another's allowance")
	}
}

func TestMetaReportsTheServedFacts(t *testing.T) {
	service, _ := newTestService(t, nil)
	meta := service.Meta()
	for key, want := range map[string]any{
		"zone":       "dns.emb.is.",
		"entries":    4,
		"dimensions": 3,
		"top_k":      3,
		"ready":      true,
	} {
		if got := meta[key]; got != want {
			t.Errorf("meta[%q] is %v, want %v", key, got, want)
		}
	}
}
