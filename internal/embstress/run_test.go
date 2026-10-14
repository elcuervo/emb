package embstress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

// fakeEndpoint is a scripted endpoint: it answers immediately, counts what it
// was offered, and can fail a fixed share of requests.
type fakeEndpoint struct {
	label  string
	models []embtop.ModelListEntry
	err    error // the Models call fails when set

	mu        sync.Mutex
	calls     int
	conns     int
	perModel  map[string]int
	texts     map[string]bool
	failEvery int
	closed    bool
}

func (f *fakeEndpoint) Label() string { return f.label }

func (f *fakeEndpoint) Models() ([]embtop.ModelListEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.models, nil
}

// Conn hands out one more connection. The fake shares the endpoint's counters,
// because the run's accounting is per endpoint however many workers drive it.
func (f *fakeEndpoint) Conn() (Conn, error) {
	f.mu.Lock()
	f.conns++
	f.mu.Unlock()
	return f, nil
}

func (f *fakeEndpoint) Embed(model, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.perModel == nil {
		f.perModel = map[string]int{}
	}
	if f.texts == nil {
		f.texts = map[string]bool{}
	}
	f.texts[text] = true
	f.perModel[model]++
	if f.failEvery > 0 && f.calls%f.failEvery == 0 {
		return errors.New("server said no")
	}
	return nil
}

func (f *fakeEndpoint) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeEndpoint) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeEndpoint) connCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
}

func (f *fakeEndpoint) sawText(text string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.texts[text]
}

func (f *fakeEndpoint) modelCount(model string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perModel[model]
}

func fakeFleetEndpoints(labels ...string) []Endpoint {
	out := make([]Endpoint, 0, len(labels))
	for _, l := range labels {
		out = append(out, &fakeEndpoint{
			label:  l,
			models: []embtop.ModelListEntry{{Name: "m", Dim: 8, Status: "ready"}},
		})
	}
	return out
}

func runOptions(levels ...int) Options {
	if len(levels) == 0 {
		levels = []int{2}
	}
	return Options{
		Target: "fake.internal",
		Models: []ConfiguredModel{{Name: "m", Dim: 8, MaxLength: 64}},
		Levels: levels,
		Dwell:  120 * time.Millisecond,
		Warmup: 40 * time.Millisecond,
		Seed:   9,
	}
}

// TestEveryEndpointReceivesLoad guards the fan-out: one worker pool per
// resolved endpoint, every endpoint offered work, and each endpoint's own
// numbers reported separately from the aggregate.
func TestEveryEndpointReceivesLoad(t *testing.T) {
	eps := fakeFleetEndpoints("a", "b", "c")
	res, err := Run(context.Background(), eps, runOptions(2, 4))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, ep := range eps {
		if got := ep.(*fakeEndpoint).count(); got == 0 {
			t.Errorf("endpoint %s received no load", ep.Label())
		}
	}
	if len(res.Endpoints) != 3 {
		t.Fatalf("result has %d endpoint sections, want 3", len(res.Endpoints))
	}
	if len(res.Fleet) != 2 {
		t.Fatalf("fleet has %d level results, want 2", len(res.Fleet))
	}
	for i, er := range res.Endpoints {
		if len(er.Levels) != 2 {
			t.Fatalf("endpoint %d has %d levels, want 2", i, len(er.Levels))
		}
		for li, l := range er.Levels {
			if l.Completed <= 0 {
				t.Errorf("%s level %d completed %.1f rps", er.Label, li, l.Completed)
			}
			if l.WindowSec <= 0 {
				t.Errorf("%s level %d reported a window of %.3fs", er.Label, li, l.WindowSec)
			}
		}
	}
	// The fleet aggregate is the sum over endpoints, so it counts each node's
	// work rather than one node's.
	if res.Fleet[0].Completed < res.Endpoints[0].Levels[0].Completed {
		t.Errorf("fleet completed %.1f is below a single endpoint's %.1f",
			res.Fleet[0].Completed, res.Endpoints[0].Levels[0].Completed)
	}
	if res.Fleet[0].Workers != 6 {
		t.Errorf("fleet workers = %d, want 2 workers × 3 endpoints", res.Fleet[0].Workers)
	}
}

// TestRunOnlyDrivesReconciledModels guards that a node is never exercised under
// a name it does not serve.
func TestRunOnlyDrivesReconciledModels(t *testing.T) {
	a := &fakeEndpoint{label: "a", models: []embtop.ModelListEntry{{Name: "m", Dim: 8}, {Name: "other", Dim: 4}}}
	b := &fakeEndpoint{label: "b", models: []embtop.ModelListEntry{{Name: "other", Dim: 4}}}
	res, err := Run(context.Background(), []Endpoint{a, b}, runOptions(1))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := a.modelCount("m"); got == 0 {
		t.Error("the endpoint that serves m was not exercised for it")
	}
	if got := b.modelCount("m"); got != 0 {
		t.Errorf("the endpoint that does not serve m was exercised for it %d times", got)
	}
	if got := b.modelCount("other"); got != 0 {
		t.Errorf("a model the configuration does not name was exercised %d times", got)
	}
	if len(res.Drift) != 1 || res.Drift[0].Endpoint != "b" {
		t.Fatalf("drift = %v, want b named", res.Drift)
	}
	if len(res.Endpoints) != 1 || res.Endpoints[0].Label != "a" {
		t.Fatalf("result sections = %+v, want only a", res.Endpoints)
	}
}

// TestRunFailsWhenTheFleetCannotBeRead guards that a run never reports numbers
// against a fleet it could not reconcile.
func TestRunFailsWhenTheFleetCannotBeRead(t *testing.T) {
	eps := []Endpoint{&fakeEndpoint{label: "a", err: errors.New("connection refused")}}
	if _, err := Run(context.Background(), eps, runOptions(1)); err == nil {
		t.Fatal("a run against an unreadable endpoint did not fail")
	}
}

func TestRunValidatesItsOptions(t *testing.T) {
	eps := fakeFleetEndpoints("a")
	cases := []struct {
		name string
		opt  Options
		eps  []Endpoint
	}{
		{"no endpoints", runOptions(1), nil},
		{"no levels", Options{Models: []ConfiguredModel{{Name: "m", Dim: 8}}, Dwell: time.Second, Levels: nil}, eps},
		{"zero dwell", Options{Models: []ConfiguredModel{{Name: "m", Dim: 8}}, Levels: []int{1}}, eps},
		{"warm-up longer than the dwell", Options{Models: []ConfiguredModel{{Name: "m", Dim: 8}},
			Levels: []int{1}, Dwell: time.Second, Warmup: 2 * time.Second}, eps},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Run(context.Background(), tc.eps, tc.opt); err == nil {
				t.Fatal("invalid options did not fail")
			}
		})
	}
}

func TestRunStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opt := runOptions(1, 1, 1)
	opt.Dwell = 5 * time.Second
	opt.Warmup = 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := Run(ctx, fakeFleetEndpoints("a"), opt)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestSummarizeDiscardsWarmupAndKeepsItsOwnDwell is the level-window contract:
// a level's numbers come only from the samples after its warm-up, and no other
// level's samples leak in.
func TestSummarizeDiscardsWarmupAndKeepsItsOwnDwell(t *testing.T) {
	base := time.Unix(1000, 0)
	rec := func(offset time.Duration, lat time.Duration, err bool) sample {
		return sample{start: base.Add(offset), lat: lat, err: err}
	}
	recs := []sample{
		rec(-time.Second, time.Millisecond, false),         // the previous level's last sample
		rec(0, time.Millisecond, false),                    // this level's warm-up
		rec(400*time.Millisecond, time.Millisecond, false), // measured
		rec(600*time.Millisecond, 3*time.Millisecond, false),
		rec(900*time.Millisecond, 0, true),          // a failure in the window
		rec(2*time.Second, time.Millisecond, false), // the next level's first sample
	}
	from, to := base.Add(500*time.Millisecond), base.Add(time.Second)
	got := summarize(recs, 2, from, to)

	if got.WindowSec != 0.5 {
		t.Errorf("WindowSec = %v, want 0.5 (the measured dwell)", got.WindowSec)
	}
	if got.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (only the failure inside the window)", got.Errors)
	}
	if got.Offered != 4 { // 2 successes + 1 failure in the window, over 0.5s
		t.Errorf("Offered = %v, want 4/s", got.Offered)
	}
	if got.Completed != 2 {
		t.Errorf("Completed = %v, want 2/s (the failure is not a completion)", got.Completed)
	}
	if got.P50Us != 3000 || got.P95Us != 3000 {
		t.Errorf("latency = %d/%d, want the window's only sample, 3000µs", got.P50Us, got.P95Us)
	}
}

// TestErrorsAreCountedNotHidden guards that a failure is reported beside the
// successes and never counted as one.
func TestErrorsAreCountedNotHidden(t *testing.T) {
	ep := &fakeEndpoint{
		label:     "a",
		models:    []embtop.ModelListEntry{{Name: "m", Dim: 8}},
		failEvery: 2, // every other request fails
	}
	opt := runOptions(1)
	opt.Dwell = 200 * time.Millisecond
	opt.Warmup = 0
	res, err := Run(context.Background(), []Endpoint{ep}, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	l := res.Endpoints[0].Levels[0]
	if l.Errors == 0 {
		t.Fatal("failures were not counted")
	}
	if l.Completed >= l.Offered {
		t.Errorf("completed %.1f/s is not below offered %.1f/s: a failure was counted as done", l.Completed, l.Offered)
	}
	if got := float64(ep.count()) / l.WindowSec; got < l.Completed {
		t.Errorf("reported %.1f completed/s exceeds the %d requests attempted", l.Completed, ep.count())
	}
}

// TestResultIsMachineReadable guards the JSON form: every endpoint plus the
// aggregate, so a consumer needs no parsing of the human table.
func TestResultIsMachineReadable(t *testing.T) {
	res, err := Run(context.Background(), fakeFleetEndpoints("a", "b"), runOptions(1))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back Result
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(back.Endpoints) != 2 || len(back.Fleet) != 1 {
		t.Fatalf("round trip lost sections: %+v", back)
	}
	if back.Seed != res.Seed || back.Target != res.Target {
		t.Errorf("round trip lost the run's identity: %+v", back)
	}
	for _, er := range back.Endpoints {
		if len(er.Levels) != 1 || er.Levels[0].Workers != 1 {
			t.Errorf("endpoint %s round-tripped as %+v", er.Label, er)
		}
	}
	for _, key := range []string{`"offered_rps"`, `"completed_rps"`, `"lat_p95_us"`, `"errors"`, `"fleet"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("JSON is missing %s: %s", key, raw)
		}
	}
}

// TestEachWorkerGetsItsOwnConnection guards the concurrency model: a level's
// workers must not share one RESP socket, or their writes and reads interleave
// and the run reports the corruption as load.
func TestEachWorkerGetsItsOwnConnection(t *testing.T) {
	ep := &fakeEndpoint{label: "a", models: []embtop.ModelListEntry{{Name: "m", Dim: 8}}}
	opt := runOptions(4, 8)
	if _, err := Run(context.Background(), []Endpoint{ep}, opt); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := ep.connCount(); got != 12 {
		t.Fatalf("connections opened = %d, want 4 + 8 (one per worker of every level)", got)
	}
}

// TestRunOffersTheCorpusWhenGivenOne guards that -corpus actually replaces
// generation: a corpus accepted and then ignored would drive texts nobody asked
// for.
func TestRunOffersTheCorpusWhenGivenOne(t *testing.T) {
	corpus := []string{"first corpus text", "second corpus text"}
	ep := &fakeEndpoint{label: "a", models: []embtop.ModelListEntry{{Name: "m", Dim: 8}}}
	opt := runOptions(1)
	opt.Corpus = corpus
	if _, err := Run(context.Background(), []Endpoint{ep}, opt); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, text := range corpus {
		if !ep.sawText(text) {
			t.Errorf("the corpus text %q was never offered", text)
		}
	}
}

// Without a corpus the generated texts are offered instead, and none of them
// exceeds the configured maximum.
func TestRunOffersGeneratedTextsWithoutACorpus(t *testing.T) {
	ep := &fakeEndpoint{label: "a", models: []embtop.ModelListEntry{{Name: "m", Dim: 8}}}
	opt := runOptions(1)
	opt.Models[0].MaxLength = 32
	if _, err := Run(context.Background(), []Endpoint{ep}, opt); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if len(ep.texts) == 0 {
		t.Fatal("no text was offered")
	}
	for text := range ep.texts {
		if n := len(strings.Fields(text)); n > 32 {
			t.Errorf("offered a %d-word text for a max_length of 32: %q", n, text)
		}
	}
}
