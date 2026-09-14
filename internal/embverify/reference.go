package embverify

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
)

// Reference is the stored Python reference artifact: the sentence set and the
// embedding each sentence produced, plus provenance and a checksum.
//
// The checksum is the sha256 over the embeddings as little-endian float32
// bytes, in row-major order. Both the Python generator and this package
// compute it the same way, so a value that crosses the language boundary in
// float64 hashes identically on both sides.
type Reference struct {
	Model      string            `json:"model"`
	Dim        int               `json:"dim"`
	Sentences  []string          `json:"sentences"`
	Embeddings [][]float64       `json:"embeddings"`
	Generator  string            `json:"generator,omitempty"`
	Version    string            `json:"generator_version,omitempty"`
	Created    string            `json:"created,omitempty"`
	Requires   map[string]string `json:"requirements,omitempty"`
	Checksum   string            `json:"checksum"`
}

// ComputeChecksum returns the sha256 of the embeddings as little-endian
// float32 bytes, row-major.
func (r *Reference) ComputeChecksum() string {
	h := sha256.New()
	var buf [4]byte
	for _, row := range r.Embeddings {
		for _, v := range row {
			binary.LittleEndian.PutUint32(buf[:], math.Float32bits(float32(v)))
			_, _ = h.Write(buf[:])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Validate checks the artifact is internally consistent and that its recorded
// checksum matches its contents.
func (r *Reference) Validate() error {
	if r.Model == "" {
		return errors.New("reference: model is empty")
	}
	if r.Dim <= 0 {
		return fmt.Errorf("reference: dim must be positive, got %d", r.Dim)
	}
	if len(r.Sentences) == 0 {
		return errors.New("reference: sentence set is empty")
	}
	if len(r.Embeddings) != len(r.Sentences) {
		return fmt.Errorf("reference: %d embeddings for %d sentences", len(r.Embeddings), len(r.Sentences))
	}
	for i, row := range r.Embeddings {
		if len(row) != r.Dim {
			return fmt.Errorf("reference: sentence %d has dim %d, want %d", i, len(row), r.Dim)
		}
	}
	if r.Checksum == "" {
		return errors.New("reference: checksum is missing")
	}
	if got := r.ComputeChecksum(); !strings.EqualFold(got, r.Checksum) {
		return fmt.Errorf("reference: checksum mismatch (recorded %s, computed %s)", r.Checksum, got)
	}
	return nil
}

// Load reads, parses and validates a reference artifact. Every failure names
// the path and the offending field.
func Load(path string) (*Reference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading reference %s: %w", path, err)
	}
	var ref Reference
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("parsing reference %s: %w", path, err)
	}
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &ref, nil
}

// CheckInputs compares the artifact's recorded model and dimension, and
// optionally its sentence set, against the values a verification run was asked
// for. Empty model/dim and a nil sentence slice mean "no expectation".
func (r *Reference) CheckInputs(model string, dim int, sentences []string) error {
	if model != "" && r.Model != model {
		return fmt.Errorf("reference: recorded model %q, requested %q", r.Model, model)
	}
	if dim > 0 && r.Dim != dim {
		return fmt.Errorf("reference: recorded dim %d, requested %d", r.Dim, dim)
	}
	if sentences != nil && !slices.Equal(r.Sentences, sentences) {
		return errors.New("reference: recorded sentence set does not match the requested set")
	}
	return nil
}
