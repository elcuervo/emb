package registry

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

func TestConcurrentColdLoadIsRaceFree(t *testing.T) {
	reg, _ := loadModelFixture(t, config.ModelConfig{})
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			<-start
			if _, err := reg.GetOrInit("test"); err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
}

func intPtr(v int) *int { return &v }

const parityModelPath = "../../models/minilm/model.onnx"
const parityTokenizerPath = "../../models/minilm/tokenizer.json"

// loadModelFixture loads minilm under a custom config and registers it for
// cleanup (Registry.Close before the ONNX environment is destroyed). Skips when
// the fixture is absent, mirroring scriptFixture.
func loadModelFixture(t *testing.T, cfg config.ModelConfig) (*Registry, *ModelEntry) {
	t.Helper()
	if err := onnx.InitEnvironment(""); err != nil {
		t.Skipf("onnx runtime unavailable: %v", err)
	}
	reg := New()
	t.Cleanup(func() {
		_ = reg.Close()
		_ = onnx.DestroyEnvironment()
	})
	cfg.ONNX = parityModelPath
	cfg.Tokenizer = parityTokenizerPath
	if cfg.Dim == 0 {
		cfg.Dim = 384
	}
	if cfg.MaxLength == 0 {
		cfg.MaxLength = 128
	}
	if cfg.Pooling == "" {
		cfg.Pooling = "mean"
	}
	entry, err := LoadModel(cfg, "test")
	if err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	reg.Add("test", entry)
	return reg, entry
}

// TestEffectiveEmbeddingSessions pins the single source of truth for the
// auto-tuned scripted bound: a batcher pool holds exactly one session, a worker
// pool holds its configured workers, and an unset worker count auto-tunes.
func TestEffectiveEmbeddingSessions(t *testing.T) {
	autoTuned := func() int {
		n := autoTuneWorkers(parityModelPath, 0)
		if n < 1 {
			n = 1
		}
		return n
	}
	cases := []struct {
		name string
		cfg  config.ModelConfig
		want int
	}{
		{"nil timeout is a batcher", config.ModelConfig{ONNX: parityModelPath, Workers: 4}, 1},
		{"positive timeout is a batcher", config.ModelConfig{ONNX: parityModelPath, Workers: 4, Batching: config.BatchingConfig{Timeout: intPtr(1)}}, 1},
		{"configured workers", config.ModelConfig{ONNX: parityModelPath, Workers: 2, Batching: config.BatchingConfig{Timeout: intPtr(0)}}, 2},
		{"auto-tuned", config.ModelConfig{ONNX: parityModelPath, Batching: config.BatchingConfig{Timeout: intPtr(0)}}, autoTuned()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveEmbeddingSessions(&tc.cfg); got != tc.want {
				t.Fatalf("effectiveEmbeddingSessions = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScriptSessionsBoundedByBatcherDefault verifies the auto-tuned default is
// exactly the embedding pool's session count (1) under the batching default.
func TestScriptSessionsBoundedByBatcherDefault(t *testing.T) {
	_, entry := loadModelFixture(t, config.ModelConfig{})
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Sessions()); got != 1 {
		t.Fatalf("auto-tuned script sessions = %d, want 1 (batcher pool holds one)", got)
	}
}

// TestScriptWorkersOverrideNotClamped verifies an explicit script_workers is
// honoured verbatim even though the embedding pool holds one session.
func TestScriptWorkersOverrideNotClamped(t *testing.T) {
	_, entry := loadModelFixture(t, config.ModelConfig{ScriptWorkers: 4})
	res, err := entry.ScriptResources()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Sessions()); got != 4 {
		t.Fatalf("explicit script_workers sessions = %d, want 4 (not clamped)", got)
	}
}

// TestScriptSessionsMatchPoolSessions verifies scripted sessions and embedding
// pool sessions agree for both pool shapes.
func TestScriptSessionsMatchPoolSessions(t *testing.T) {
	cases := []struct {
		name         string
		cfg          config.ModelConfig
		wantSessions int
	}{
		{"batcher", config.ModelConfig{}, 1},
		{"workers", config.ModelConfig{Workers: 2, Batching: config.BatchingConfig{Timeout: intPtr(0)}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, entry := loadModelFixture(t, tc.cfg)
			if err := entry.ensurePool(); err != nil {
				t.Fatal(err)
			}
			poolWorkers := entry.Pool.Stats().NumWorkers
			res, err := entry.ScriptResources()
			if err != nil {
				t.Fatal(err)
			}
			if poolWorkers != tc.wantSessions {
				t.Errorf("pool sessions = %d, want %d", poolWorkers, tc.wantSessions)
			}
			if got := len(res.Sessions()); got != tc.wantSessions {
				t.Errorf("script sessions = %d, want %d", got, tc.wantSessions)
			}
		})
	}
}

// countingSession is a NamedSession that records Close calls, so a partial
// construction failure can be proven to release what it opened.
type countingSession struct {
	closes *atomic.Int64
}

func (c *countingSession) RunNamed([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	return nil, nil
}

func (c *countingSession) Close() error {
	c.closes.Add(1)
	return nil
}

// TestPartialScriptPoolConstructionClosesOpened verifies a session-creation
// failure partway through the pool closes every session already opened and
// surfaces the error, leaving no sessions recorded.
func TestPartialScriptPoolConstructionClosesOpened(t *testing.T) {
	orig := newNamedSession
	t.Cleanup(func() { newNamedSession = orig })

	var created, closed atomic.Int64
	injected := errors.New("injected session failure")
	newNamedSession = func([]byte, []string, []string, int, int, int) (onnx.NamedSession, error) {
		if created.Add(1) == 3 {
			return nil, injected
		}
		return &countingSession{closes: &closed}, nil
	}

	_, entry := loadModelFixture(t, config.ModelConfig{ScriptWorkers: 4})
	if _, err := entry.ScriptResources(); !errors.Is(err, injected) {
		t.Fatalf("ScriptResources error = %v, want the injected failure", err)
	}
	if got := created.Load(); got != 3 {
		t.Fatalf("sessions attempted = %d, want 3", got)
	}
	if got := closed.Load(); got != 2 {
		t.Fatalf("sessions closed after failure = %d, want 2", got)
	}
	if sessions, tokenizer := entry.ScriptFootprint(); sessions != 0 || tokenizer {
		t.Fatalf("footprint after failed construction = (%d, %t), want (0, false)", sessions, tokenizer)
	}
}

// countingTokenizer is a Tokenizer that records how many times it is closed.
type countingTokenizer struct {
	closes *atomic.Int64
}

func (c *countingTokenizer) Encode(string, int) ([]int64, []int64, error) { return nil, nil, nil }
func (c *countingTokenizer) Close() error {
	c.closes.Add(1)
	return nil
}

// TestSharedTokenizerCreatedOnceClosedOnce verifies the tokenizer shared by the
// embedding pool and the scripted path is a single instance, released exactly
// once by Registry.Close.
func TestSharedTokenizerCreatedOnceClosedOnce(t *testing.T) {
	orig := newTokenizer
	t.Cleanup(func() { newTokenizer = orig })

	var closes, created atomic.Int64
	fake := &countingTokenizer{closes: &closes}
	newTokenizer = func(string, bool) (tokenizer.Tokenizer, error) {
		created.Add(1)
		return fake, nil
	}

	reg := New()
	entry := &ModelEntry{Name: "test", cfg: config.ModelConfig{Tokenizer: "ignored"}}
	reg.Add("test", entry)

	// The pool path and the scripted path both borrow the shared tokenizer.
	a, err := entry.sharedTokenizer()
	if err != nil {
		t.Fatal(err)
	}
	b, err := entry.sharedTokenizer()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("sharedTokenizer returned two different instances")
	}
	if got := created.Load(); got != 1 {
		t.Fatalf("tokenizer constructions = %d, want 1", got)
	}

	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	if got := closes.Load(); got != 1 {
		t.Fatalf("tokenizer closes = %d, want exactly 1", got)
	}
}

// TestImagePlanResolvedWithoutSessions verifies the preprocessing plan can be
// resolved (for emb.image.info / emb.image.preprocess) without opening any
// image session.
func TestImagePlanResolvedWithoutSessions(t *testing.T) {
	_, entry := loadModelFixture(t, config.ModelConfig{
		Image: &config.ImageConfig{
			Input:  "pixel_values",
			Size:   4,
			Output: "last_hidden_state",
			Std:    []float64{1, 1, 1},
		},
	})
	if !entry.HasImageSurface() {
		t.Fatal("image-configured model reports no image surface")
	}
	if entry.ImageRes != nil {
		t.Fatal("image sessions were opened at load")
	}
	plan, err := entry.ImagePlan()
	if err != nil {
		t.Fatal(err)
	}
	if plan.Input != "pixel_values" || plan.Size != 4 {
		t.Fatalf("plan = {input:%q size:%d}, want {pixel_values 4}", plan.Input, plan.Size)
	}
	if entry.ImageRes != nil || entry.ImageFootprint() != 0 {
		t.Fatalf("plan resolution opened image sessions (res=%v footprint=%d)", entry.ImageRes, entry.ImageFootprint())
	}
}
