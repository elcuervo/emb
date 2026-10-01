package onnx

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInferMaxLength verifies the max length is read from config.json.
func TestInferMaxLength(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"max_position_embeddings": 256}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := InferMaxLength(dir)
	if err != nil {
		t.Fatalf("InferMaxLength: %v", err)
	}
	if got != 256 {
		t.Fatalf("InferMaxLength = %d, want 256", got)
	}
}

// TestInferMaxLengthErrors covers the fallback-triggering failures: a missing
// file, malformed JSON, and a non-positive value each return an error so the
// caller can fall back rather than silently using zero.
func TestInferMaxLengthErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := InferMaxLength(t.TempDir()); err == nil {
			t.Fatal("expected an error for a missing config.json")
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := InferMaxLength(dir); err == nil {
			t.Fatal("expected an error for malformed config.json")
		}
	})
	t.Run("non-positive", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"max_position_embeddings": 0}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := InferMaxLength(dir); err == nil {
			t.Fatal("expected an error for a non-positive max_position_embeddings")
		}
	})
}

// TestInferDimAndInputInfoFromFixture reads the graph metadata the registry's
// auto-configuration uses. Skips when the minilm fixture or the runtime is
// absent, mirroring named_test.go.
func TestInferDimAndInputInfoFromFixture(t *testing.T) {
	const path = "../../models/minilm/model.onnx"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}
	if err := InitEnvironment(""); err != nil {
		t.Skipf("onnx runtime unavailable: %v", err)
	}
	t.Cleanup(func() { _ = DestroyEnvironment() })

	dim, err := InferDim(path)
	if err != nil {
		t.Fatalf("InferDim: %v", err)
	}
	if dim != 384 {
		t.Fatalf("InferDim = %d, want 384", dim)
	}

	infos, err := GetInputInfo(path)
	if err != nil {
		t.Fatalf("GetInputInfo: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("GetInputInfo returned no inputs")
	}
	for _, in := range infos {
		if in.Name == "" || in.Rank != len(in.Dimensions) {
			t.Fatalf("malformed input info: %+v", in)
		}
	}
}
