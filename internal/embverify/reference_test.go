package embverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validReference() *Reference {
	r := &Reference{
		Model:      "minilm",
		Dim:        2,
		Sentences:  []string{"a", "b"},
		Embeddings: [][]float64{{1, 0}, {0.5, 0.5}},
		Generator:  "generate-reference.py",
		Version:    "1",
	}
	r.Checksum = r.ComputeChecksum()
	return r
}

func TestReferenceValidate(t *testing.T) {
	r := validReference()
	if err := r.Validate(); err != nil {
		t.Fatalf("valid reference rejected: %v", err)
	}
}

// TestChecksumMatchesPythonGenerator pins the cross-language checksum contract:
// generate-reference.py hashes the embeddings as little-endian float32 bytes and
// these constants were produced by that same formula in Python/numpy. If either
// side changes its byte layout, this fails.
func TestChecksumMatchesPythonGenerator(t *testing.T) {
	cases := []struct {
		name       string
		embeddings [][]float64
		want       string
	}{
		{"one vector", [][]float64{{1.0, 0.0}}, "434b26042aff3fb844a4c4c6be0d81a079b0ce84cfb8190679024404e5dc4822"},
		{"two vectors", [][]float64{{1.0, 0.0}, {0.5, 0.5}}, "4dc22ef583ef82e54c62018d1daa50a3d2ae10687a60e682dbe3cef353a594a3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Reference{Embeddings: tc.embeddings}
			if got := r.ComputeChecksum(); got != tc.want {
				t.Fatalf("checksum = %s, want %s (Python/numpy agreement broken)", got, tc.want)
			}
		})
	}
}

func TestReferenceValidateRejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Reference)
		wantSub string
	}{
		{"empty model", func(r *Reference) { r.Model = "" }, "model is empty"},
		{"bad dim", func(r *Reference) { r.Dim = 0 }, "dim must be positive"},
		{"empty sentences", func(r *Reference) { r.Sentences = nil; r.Embeddings = nil }, "sentence set is empty"},
		{"row count", func(r *Reference) { r.Embeddings = r.Embeddings[:1] }, "embeddings for"},
		{"row dim", func(r *Reference) { r.Embeddings[0] = []float64{1} }, "has dim"},
		{"missing checksum", func(r *Reference) { r.Checksum = "" }, "checksum is missing"},
		{"tampered", func(r *Reference) { r.Embeddings[0][0] = 9 }, "checksum mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validReference()
			tc.mutate(r)
			err := r.Validate()
			if err == nil {
				t.Fatalf("expected rejection")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

func TestLoadReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reference.json")

	data, err := json.Marshal(validReference())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Model != "minilm" || got.Dim != 2 {
		t.Fatalf("loaded %+v", got)
	}

	// Tampered artifact must be rejected, naming the checksum.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["checksum"] = strings.Repeat("0", 64)
	tampered, _ := json.Marshal(raw)
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestReferenceCheckInputs(t *testing.T) {
	r := validReference()
	if err := r.CheckInputs("minilm", 2, []string{"a", "b"}); err != nil {
		t.Fatalf("matching inputs rejected: %v", err)
	}
	if err := r.CheckInputs("bge", 0, nil); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("model mismatch: %v", err)
	}
	if err := r.CheckInputs("", 384, nil); err == nil || !strings.Contains(err.Error(), "dim") {
		t.Fatalf("dim mismatch: %v", err)
	}
	if err := r.CheckInputs("", 0, []string{"a", "c"}); err == nil || !strings.Contains(err.Error(), "sentence set") {
		t.Fatalf("sentence mismatch: %v", err)
	}
}
