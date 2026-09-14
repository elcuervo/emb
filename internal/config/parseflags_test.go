package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseFlagsModelSections covers the order-dependent -model/-model-* block
// and the model-scoped flags that were previously unreachable.
func TestParseFlagsModelSections(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "minilm",
		"--model-onnx", "./minilm.onnx", "-model-tokenizer", "./tok.json",
		"-model-dim", "384", "-model-max-length", "256",
		"-model-pooling", "cls", "-model-normalize",
		"-model-output-tensor", "emb", "-model-quantize", "off",
		"-model-pad-output",
		"-model", "bge", "-model-repo", "Xenova/bge-small-en-v1.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	minilm := fc.Models["minilm"]
	if minilm.ONNX != "./minilm.onnx" || minilm.Tokenizer != "./tok.json" {
		t.Fatalf("minilm paths not parsed: %#v", minilm)
	}
	if minilm.Dim != 384 || minilm.MaxLength != 256 {
		t.Fatalf("minilm numerics not parsed: dim=%d max=%d", minilm.Dim, minilm.MaxLength)
	}
	if minilm.Pooling != "cls" || !minilm.Normalize || minilm.OutputTensor != "emb" || minilm.Quantize != "off" || !minilm.PadOutput {
		t.Fatalf("minilm options not parsed: %#v", minilm)
	}
	if fc.Models["bge"].ModelRepo != "Xenova/bge-small-en-v1.5" {
		t.Fatalf("bge repo not parsed: %#v", fc.Models["bge"])
	}
}

// TestParseFlagsImplicitModel covers -model-* with no preceding -model, and the
// --flag form the emb-server-distribution spec requires.
func TestParseFlagsImplicitModel(t *testing.T) {
	fc, err := ParseFlags([]string{"--model-repo", "Xenova/all-MiniLM-L6-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if fc.Models["model"].ModelRepo != "Xenova/all-MiniLM-L6-v2" {
		t.Fatalf("implicit model section not populated: %#v", fc.Models)
	}
}

// TestParseFlagsLenientInt keeps the historical behavior where a malformed
// numeric flag silently becomes 0 instead of failing.
func TestParseFlagsLenientInt(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "m", "-model-onnx", "./m.onnx",
		"-max-connections", "abc", "-max-concurrent-requests", "12",
		"-model-workers", "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.MaxConnections != 0 {
		t.Fatalf("malformed -max-connections = %d, want 0", fc.MaxConnections)
	}
	if fc.MaxConcurrentRequests != 12 {
		t.Fatalf("-max-concurrent-requests = %d, want 12", fc.MaxConcurrentRequests)
	}
	if fc.Models["m"].Workers != 0 {
		t.Fatalf("malformed -model-workers = %d, want 0", fc.Models["m"].Workers)
	}
}

func TestParseFlagsTokenizeWorkersPointer(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-model", "m", "-model-onnx", "./m.onnx", "-model-tokenize-workers", "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.Models["m"].TokenizeWorkers == nil || *fc.Models["m"].TokenizeWorkers != 3 {
		t.Fatalf("tokenize_workers not parsed: %#v", fc.Models["m"].TokenizeWorkers)
	}
}

// TestParseFlagsUnknownFlag documents the intentional strictness: unknown flags
// and flags missing a value are now errors rather than silently ignored.
func TestParseFlagsUnknownFlag(t *testing.T) {
	if _, err := ParseFlags([]string{"-bogus", "x"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if _, err := ParseFlags([]string{"-model", "m", "-model-onnx"}); err == nil {
		t.Fatal("expected error for -model-onnx missing a value")
	}
}

func TestParseFlagsVersionSentinel(t *testing.T) {
	_, err := ParseFlags([]string{"-version"})
	if err == nil || err.Error() != "__version__" {
		t.Fatalf("expected __version__ sentinel, got %v", err)
	}
}

func TestParseFlagsListenAndOrtLib(t *testing.T) {
	fc, err := ParseFlags([]string{
		"-listen", ":6380", "-ort-lib", "/nix/lib",
		"-model", "m", "-model-onnx", "./m.onnx",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc.Listen != ":6380" || fc.OrtLib != "/nix/lib" {
		t.Fatalf("listen/ort-lib not parsed: %q %q", fc.Listen, fc.OrtLib)
	}
}

// TestParseFlagsConfigFile verifies -config loads a file and that later flags
// override it, including a config file with no models plus a -model flag (the
// map must not be nil).
func TestParseFlagsConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("listen: \":6380\"\nmodels:\n  test:\n    onnx: ./model.onnx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fc, err := ParseFlags([]string{"-config", path, "-listen", ":7000", "-password", "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if fc.Models["test"].ONNX != "./model.onnx" {
		t.Fatalf("config models not loaded: %#v", fc.Models)
	}
	if fc.Listen != ":7000" || fc.Password != "pw" {
		t.Fatalf("post-config flags not applied: listen=%q pw=%q", fc.Listen, fc.Password)
	}

	// A config with no models must not panic when a -model flag follows.
	empty := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(empty, []byte("listen: \":6380\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fc, err = ParseFlags([]string{"-config", empty, "-model", "m", "-model-onnx", "./m.onnx"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fc.Models["m"]; !ok {
		t.Fatalf("model after empty config not registered: %#v", fc.Models)
	}
}

func TestParseFlagsNoModels(t *testing.T) {
	_, err := ParseFlags(nil)
	if err == nil || !strings.Contains(err.Error(), "no models configured") {
		t.Fatalf("expected no-models error, got %v", err)
	}
}
