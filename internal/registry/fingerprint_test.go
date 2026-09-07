package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elcuervo/emb/internal/config"
)

func fingerprintEntry(t *testing.T, cfg config.ModelConfig, model, tokenizer []byte) string {
	t.Helper()
	dir := t.TempDir()
	cfg.ONNX = filepath.Join(dir, "model.onnx")
	cfg.Tokenizer = filepath.Join(dir, "tokenizer.json")
	if err := os.WriteFile(cfg.ONNX, model, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Tokenizer, tokenizer, 0o600); err != nil {
		t.Fatal(err)
	}
	e := &ModelEntry{cfg: cfg, Dim: cfg.Dim, Quantization: "fp32"}
	got, err := e.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestFingerprintIncludesOutputInputsAndExcludesScheduling(t *testing.T) {
	base := config.ModelConfig{OutputTensor: "out", Dim: 4, MaxLength: 128, Pooling: "mean", Normalize: true}
	want := fingerprintEntry(t, base, []byte("model"), []byte("tokenizer"))

	included := []struct {
		name      string
		configure func(*config.ModelConfig)
		model     []byte
		tokenizer []byte
	}{
		{"model content", func(*config.ModelConfig) {}, []byte("changed"), []byte("tokenizer")},
		{"tokenizer content", func(*config.ModelConfig) {}, []byte("model"), []byte("changed")},
		{"output tensor", func(c *config.ModelConfig) { c.OutputTensor = "other" }, []byte("model"), []byte("tokenizer")},
		{"dimension", func(c *config.ModelConfig) { c.Dim++ }, []byte("model"), []byte("tokenizer")},
		{"max length", func(c *config.ModelConfig) { c.MaxLength++ }, []byte("model"), []byte("tokenizer")},
		{"pooling", func(c *config.ModelConfig) { c.Pooling = "cls" }, []byte("model"), []byte("tokenizer")},
		{"normalize", func(c *config.ModelConfig) { c.Normalize = false }, []byte("model"), []byte("tokenizer")},
		{"pad output", func(c *config.ModelConfig) { c.PadOutput = true }, []byte("model"), []byte("tokenizer")},
	}
	for _, tc := range included {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.configure(&cfg)
			if got := fingerprintEntry(t, cfg, tc.model, tc.tokenizer); got == want {
				t.Fatal("output-affecting change did not alter fingerprint")
			}
		})
	}

	excluded := []struct {
		name      string
		configure func(*config.ModelConfig)
	}{
		{"workers", func(c *config.ModelConfig) { c.Workers = 8 }},
		{"tokenize workers", func(c *config.ModelConfig) { n := 8; c.TokenizeWorkers = &n }},
		{"batching", func(c *config.ModelConfig) { n := 5; c.Batching.Timeout = &n; c.Batching.MaxBatch = 99 }},
		{"threads", func(c *config.ModelConfig) { c.IntraOpThreads = 7; c.InterOpThreads = 3 }},
		{"execution mode", func(c *config.ModelConfig) { c.ExecutionMode = "parallel" }},
		{"preload", func(c *config.ModelConfig) { c.Preload = true }},
	}
	for _, tc := range excluded {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.configure(&cfg)
			if got := fingerprintEntry(t, cfg, []byte("model"), []byte("tokenizer")); got != want {
				t.Fatalf("scheduling-only change altered fingerprint: %s != %s", got, want)
			}
		})
	}
}

func TestFingerprintIsCached(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &ModelEntry{cfg: config.ModelConfig{ONNX: model}, Dim: 4}
	first, err := e.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(model); err != nil {
		t.Fatal(err)
	}
	second, err := e.Fingerprint()
	if err != nil || second != first {
		t.Fatalf("cached fingerprint = %q, %v; want %q", second, err, first)
	}
}
