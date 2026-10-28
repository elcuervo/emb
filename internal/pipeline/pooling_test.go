package pipeline

import (
	"bytes"
	"testing"
)

// TestExtractTokenRowsMatchesScalar covers the token-grid extractor against a
// scalar reference: rows from start onward, entire batch, with and without
// per-row normalization.
func TestExtractTokenRowsMatchesScalar(t *testing.T) {
	const (
		batch = 3
		seq   = 9
		dim   = 5
	)
	hidden := make([]float32, batch*seq*dim)
	for i := range hidden {
		hidden[i] = float32(i%17) - 6
	}
	for _, start := range []int{0, 4} {
		for _, normalize := range []bool{false, true} {
			got := ExtractTokenRows(hidden, batch, dim, seq, start, normalize)
			want := extractTokenRowsScalar(hidden, batch, dim, seq, start, normalize)
			if len(got) != batch {
				t.Fatalf("start=%d normalize=%v: %d blobs, want %d", start, normalize, len(got), batch)
			}
			for b := range got {
				if !bytes.Equal(got[b], want[b]) {
					t.Fatalf("start=%d normalize=%v image %d: rows differ from the scalar reference", start, normalize, b)
				}
				if want := (seq - start) * dim * 4; len(got[b]) != want {
					t.Fatalf("start=%d image %d: %d bytes, want %d", start, b, len(got[b]), want)
				}
			}
		}
	}

	// An out-of-range start yields nothing rather than reading past the tensor.
	if got := ExtractTokenRows(hidden, batch, dim, seq, seq+1, true); got != nil {
		t.Fatalf("start past seqLen returned %d blobs, want nil", len(got))
	}
}
