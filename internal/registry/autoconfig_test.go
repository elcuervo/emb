package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elcuervo/emb/internal/config"
)

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestResolveModelConfigDetectsFromFixture exercises every auto-detection
// branch: the sibling tokenizer path, max_length from config.json, dim from the
// graph, and output/pooling from the graph's output rank. The config.json value
// (256) is deliberately distinct from the 512 fallback so detection is proven.
func TestResolveModelConfigDetectsFromFixture(t *testing.T) {
	const model = "../../models/minilm/model.onnx"
	const tok = "../../models/minilm/tokenizer.json"
	if _, err := os.Stat(model); err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	initORT(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"max_position_embeddings": 256}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(mustAbs(t, model), filepath.Join(dir, "model.onnx")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(mustAbs(t, tok), filepath.Join(dir, "tokenizer.json")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	cfg := config.ModelConfig{ONNX: filepath.Join(dir, "model.onnx")}
	if err := resolveModelConfig(&cfg, "test"); err != nil {
		t.Fatalf("resolveModelConfig: %v", err)
	}
	if want := filepath.Join(dir, "tokenizer.json"); cfg.Tokenizer != want {
		t.Errorf("Tokenizer = %q, want the sibling %q", cfg.Tokenizer, want)
	}
	if cfg.MaxLength != 256 {
		t.Errorf("MaxLength = %d, want 256 detected from config.json", cfg.MaxLength)
	}
	if cfg.Dim != 384 {
		t.Errorf("Dim = %d, want 384 detected from the graph", cfg.Dim)
	}
	if cfg.OutputTensor != "last_hidden_state" {
		t.Errorf("OutputTensor = %q, want last_hidden_state", cfg.OutputTensor)
	}
	if cfg.Pooling != "mean" {
		t.Errorf("Pooling = %q, want mean for a rank-3 output", cfg.Pooling)
	}
}

// TestResolveModelConfigFallsBackWhenMetadataMissing verifies the documented
// fallbacks: 512 max length, last_hidden_state output, and mean pooling when
// the graph/config cannot supply values.
func TestResolveModelConfigFallsBackWhenMetadataMissing(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ModelConfig{ONNX: filepath.Join(dir, "missing.onnx")}
	if err := resolveModelConfig(&cfg, "test"); err != nil {
		t.Fatalf("resolveModelConfig: %v", err)
	}
	if cfg.MaxLength != 512 {
		t.Errorf("MaxLength = %d, want the 512 fallback", cfg.MaxLength)
	}
	if cfg.OutputTensor != "last_hidden_state" || cfg.Pooling != "mean" {
		t.Errorf("output/pooling fallback = (%q, %q), want (last_hidden_state, mean)", cfg.OutputTensor, cfg.Pooling)
	}
}

// TestResolveModelConfigBatchingDefaults pins the batching defaults: an unset
// timeout opts into a 1ms window with the documented caps, and an explicit
// timeout: 0 keeps count-only behavior (no token budget, no tokenize workers).
func TestResolveModelConfigBatchingDefaults(t *testing.T) {
	cfg := config.ModelConfig{ONNX: "/nonexistent/model.onnx"}
	if err := resolveModelConfig(&cfg, "test"); err != nil {
		t.Fatal(err)
	}
	if cfg.Batching.Timeout == nil || *cfg.Batching.Timeout != 1 {
		t.Errorf("Timeout = %v, want 1", cfg.Batching.Timeout)
	}
	if cfg.Batching.MaxBatch != 32 {
		t.Errorf("MaxBatch = %d, want 32", cfg.Batching.MaxBatch)
	}
	if cfg.Batching.MaxBatchTokens == nil || *cfg.Batching.MaxBatchTokens != 16384 {
		t.Errorf("MaxBatchTokens = %v, want 16384", cfg.Batching.MaxBatchTokens)
	}
	if cfg.TokenizeWorkers == nil || *cfg.TokenizeWorkers < 1 {
		t.Errorf("TokenizeWorkers = %v, want >= 1", cfg.TokenizeWorkers)
	}

	zero := 0
	cfg = config.ModelConfig{ONNX: "/nonexistent/model.onnx", Batching: config.BatchingConfig{Timeout: &zero}}
	if err := resolveModelConfig(&cfg, "test"); err != nil {
		t.Fatal(err)
	}
	if cfg.Batching.MaxBatchTokens != nil {
		t.Errorf("MaxBatchTokens = %v, want nil when timeout is 0", cfg.Batching.MaxBatchTokens)
	}
	if cfg.TokenizeWorkers != nil {
		t.Errorf("TokenizeWorkers = %v, want nil when timeout is 0", cfg.TokenizeWorkers)
	}
}
