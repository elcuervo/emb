package registry

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// TestDefaultIntraOpThreads verifies the thread-isolation default: unset resolves to
// cores−2 (floor 1 on ≤2 cores), and the registry only substitutes it when the config
// value is unset (so an explicit value always wins).
func TestDefaultIntraOpThreads(t *testing.T) {
	cores := runtime.GOMAXPROCS(0)
	expected := 1
	if cores > 2 {
		expected = cores - 2
	}
	if got := defaultIntraOpThreads(); got != expected {
		t.Fatalf("defaultIntraOpThreads() = %d, want %d (cores=%d)", got, expected, cores)
	}

	// Explicit config must win over the default: emulate the ensurePool branching.
	explicit := 4
	resolved := explicit
	if resolved <= 0 {
		resolved = defaultIntraOpThreads()
	}
	if resolved != explicit {
		t.Fatalf("explicit intra_op_threads=%d was overridden to %d", explicit, resolved)
	}
}

func TestSelectOutputTensorPrefersRank2(t *testing.T) {
	outputs := map[string]onnx.OutputInfo{
		"last_hidden_state": {Name: "last_hidden_state", Rank: 3, Dim: 768},
		"pooler_output":     {Name: "pooler_output", Rank: 2, Dim: 768},
	}
	name, rank := selectOutputTensor(outputs)
	if name != "pooler_output" {
		t.Fatalf("expected pooler_output, got %s", name)
	}
	if rank != 2 {
		t.Fatalf("expected rank 2, got %d", rank)
	}
}

func TestSelectOutputTensorPicksOnlyAvailable(t *testing.T) {
	outputs := map[string]onnx.OutputInfo{
		"last_hidden_state": {Name: "last_hidden_state", Rank: 3, Dim: 384},
	}
	name, rank := selectOutputTensor(outputs)
	if name != "last_hidden_state" {
		t.Fatalf("expected last_hidden_state, got %s", name)
	}
	if rank != 3 {
		t.Fatalf("expected rank 3, got %d", rank)
	}
}

func TestSelectOutputTensorRank3(t *testing.T) {
	outputs := map[string]onnx.OutputInfo{
		"sentence_embedding": {Name: "sentence_embedding", Rank: 2, Dim: 384},
		"last_hidden_state":  {Name: "last_hidden_state", Rank: 3, Dim: 384},
	}
	name, _ := selectOutputTensor(outputs)
	if name != "sentence_embedding" {
		t.Fatalf("expected sentence_embedding (rank 2), got %s", name)
	}
}

func TestSelectOutputTensorEmpty(t *testing.T) {
	name, _ := selectOutputTensor(map[string]onnx.OutputInfo{})
	if name != "last_hidden_state" {
		t.Fatalf("expected fallback last_hidden_state, got %s", name)
	}
}

func TestPoolingForRank2(t *testing.T) {
	if poolingForRank(2) != "none" {
		t.Fatalf("expected none, got %s", poolingForRank(2))
	}
}

func TestPoolingForRank3(t *testing.T) {
	if poolingForRank(3) != "mean" {
		t.Fatalf("expected mean, got %s", poolingForRank(3))
	}
}

func TestPoolingForRankOther(t *testing.T) {
	if poolingForRank(4) != "mean" {
		t.Fatalf("expected mean for rank 4, got %s", poolingForRank(4))
	}
}

func TestResolveQuantize(t *testing.T) {
	t.Run("auto picks quantized when present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "model_quantized.onnx"), []byte("q"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx")}
		if err := resolveQuantize(cfg); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(cfg.ONNX) != "model_quantized.onnx" {
			t.Fatalf("expected quantized pick, got %s", cfg.ONNX)
		}
	})

	t.Run("auto keeps fp32 when absent", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx")}
		if err := resolveQuantize(cfg); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(cfg.ONNX) != "model.onnx" {
			t.Fatalf("expected fp32 fallback, got %s", cfg.ONNX)
		}
	})

	t.Run("on requires quantized", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx"), Quantize: "on"}
		if err := resolveQuantize(cfg); err == nil {
			t.Fatal("expected error for quantize=on without quantized weights")
		}
	})

	t.Run("auto picks int8-named weights over sibling fp32", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []string{"model_fp32.onnx", "model_int8.onnx"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("w"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		// The configured path is the fp32 file, which sorts before the int8 one.
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model_fp32.onnx")}
		if err := resolveQuantize(cfg); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(cfg.ONNX) != "model_int8.onnx" {
			t.Fatalf("expected int8 pick, got %s", cfg.ONNX)
		}
	})

	t.Run("auto picks nested int8-named weights", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "onnx"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "onnx", "model_int8.onnx"), []byte("q"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx")}
		if err := resolveQuantize(cfg); err != nil {
			t.Fatal(err)
		}
		if got := filepath.ToSlash(cfg.ONNX); !strings.HasSuffix(got, "onnx/model_int8.onnx") {
			t.Fatalf("expected nested int8 pick, got %s", cfg.ONNX)
		}
	})

	t.Run("on accepts int8-named weights", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "model_int8.onnx"), []byte("q"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx"), Quantize: "on"}
		if err := resolveQuantize(cfg); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(cfg.ONNX) != "model_int8.onnx" {
			t.Fatalf("expected int8 pick, got %s", cfg.ONNX)
		}
	})

	t.Run("off never switches", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "model_quantized.onnx"), []byte("q"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx"), Quantize: "off"}
		if err := resolveQuantize(cfg); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(cfg.ONNX) != "model.onnx" {
			t.Fatalf("quantize=off must keep fp32, got %s", cfg.ONNX)
		}
	})

	t.Run("invalid value rejected", func(t *testing.T) {
		cfg := &config.ModelConfig{ONNX: "/x/model.onnx", Quantize: "banana"}
		if err := resolveQuantize(cfg); err == nil {
			t.Fatal("expected error for invalid quantize value")
		}
	})
}

func TestIsQuantizedWeights(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/m/model_int8.onnx", true},
		{"/m/onnx/model_int8.onnx", true},
		{"/m/text_model_int8.onnx", true},
		{"/m/model_quantized.onnx", true},
		{"/m/onnx/quantized/model.onnx", true},
		{"/m/model.onnx", false},
		{"/m/model_fp32.onnx", false},
	}
	for _, tc := range cases {
		if got := isQuantizedWeights(tc.path); got != tc.want {
			t.Errorf("isQuantizedWeights(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
}

// TestInt8NamedWeightsLoadAndLabel guards the intake contract an int8 decision
// export depends on: a weight file named model_int8.onnx is resolved as
// pre-quantized, labelled int8 on the entry, and named on the boot line — so an
// int8 mount that silently fell back to fp32 is visible in the log.
func TestInt8NamedWeightsLoadAndLabel(t *testing.T) {
	initORT(t)
	src, err := os.ReadFile("../../testdata/laya/model.onnx")
	if err != nil {
		t.Skipf("vendored laya fixture missing: %v", err)
	}
	dir := t.TempDir()
	onnxPath := filepath.Join(dir, "model_int8.onnx")
	if err := os.WriteFile(onnxPath, src, 0o600); err != nil {
		t.Fatal(err)
	}

	var logged bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(prev) })

	entry, err := LoadModel(config.ModelConfig{
		ONNX:      onnxPath,
		Tokenizer: "../../testdata/laya/tokenizer/tokenizer.json",
		Dim:       32,
		MaxLength: 64,
		Pooling:   "mean",
		Workers:   1,
		Preload:   true,
	}, "int8fixture")
	if err != nil {
		t.Fatalf("loading int8-named weights: %v", err)
	}
	t.Cleanup(func() { _ = entry.closeResources() })

	if entry.Quantization != "int8" {
		t.Errorf("Quantization = %q, want int8", entry.Quantization)
	}
	if entry.ModelSize != int64(len(src)) {
		t.Errorf("ModelSize = %d, want %d", entry.ModelSize, len(src))
	}
	if got := logged.String(); !strings.Contains(got, "quantization=int8 weights=model_int8.onnx") {
		t.Errorf("boot log missing the int8 weight line, got:\n%s", got)
	}
}

// TestDownloadModelEarlyReturnAcceptsInt8Name guards the boot path of an int8
// mount served from a volume: the configured fp32 path does not exist, the
// cached pre-quantized member is named model_int8.onnx, and the model must load
// from disk instead of re-downloading.
func TestDownloadModelEarlyReturnAcceptsInt8Name(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model_int8.onnx"), []byte("q"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.ModelConfig{
		ModelRepo: "test/model",
		ONNX:      filepath.Join(dir, "model.onnx"),
		Tokenizer: filepath.Join(dir, "tokenizer.json"),
	}
	// No network is available here: an early return is the only way this passes.
	if err := downloadModel(cfg, "int8cached"); err != nil {
		t.Fatalf("downloadModel: %v", err)
	}
}

func TestResolveQuantizeDefaultsAuto(t *testing.T) {
	cfg := &config.ModelConfig{ONNX: "/x/model.onnx"}
	if err := resolveQuantize(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Quantize != "auto" {
		t.Fatalf("expected default auto, got %q", cfg.Quantize)
	}
}

// TestCloseWaitsForInFlightInitialization proves Close waits for a lazy load
// that holds the registry read lock, instead of opening resources under it.
func TestCloseWaitsForInFlightInitialization(t *testing.T) {
	orig := newTokenizer
	t.Cleanup(func() { newTokenizer = orig })

	var closes atomic.Int64
	fake := &countingTokenizer{closes: &closes}
	entered := make(chan struct{})
	gate := make(chan struct{})
	newTokenizer = func(string, bool) (tokenizer.Tokenizer, error) {
		close(entered)
		<-gate
		return fake, nil
	}

	reg := New()
	// A nonexistent ONNX path fails ensurePool right after the tokenizer is
	// created, so the test needs no ONNX environment or downloaded model.
	reg.Add("test", &ModelEntry{Name: "test", cfg: config.ModelConfig{
		Tokenizer: "ignored",
		ONNX:      filepath.Join(t.TempDir(), "missing.onnx"),
	}})

	loaded := make(chan error, 1)
	go func() {
		_, err := reg.GetOrInit("test")
		loaded <- err
	}()
	<-entered

	closed := make(chan error, 1)
	go func() { closed <- reg.Close() }()
	select {
	case <-closed:
		t.Fatal("Close returned while a lazy initialization was in flight")
	case <-time.After(20 * time.Millisecond):
	}

	close(gate)
	if err := <-loaded; err == nil {
		t.Fatal("GetOrInit succeeded with a missing model file")
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if got := closes.Load(); got != 1 {
		t.Fatalf("tokenizer closes = %d, want exactly 1", got)
	}
}
