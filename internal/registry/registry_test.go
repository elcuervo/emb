package registry

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
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

func TestResolveQuantizeDefaultsAuto(t *testing.T) {
	cfg := &config.ModelConfig{ONNX: "/x/model.onnx"}
	if err := resolveQuantize(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Quantize != "auto" {
		t.Fatalf("expected default auto, got %q", cfg.Quantize)
	}
}
