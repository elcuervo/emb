package pipeline

import (
	"errors"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

// invariantSession returns hidden rows whose values depend only on (token,
// dim), never on the row index or batch size — a position- and
// batch-invariant graph (fp32 / static-QDQ style). Row b in any batch is
// byte-identical to the same text in a 1-row run. When pooled is set the
// fake acts like a pre-pooled graph (pooling "none") and returns [batch, dim]
// instead of [batch, seq, dim].
type invariantSession struct {
	calls  []runCall
	pooled bool
}

func (s *invariantSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	s.calls = append(s.calls, runCall{batchSize, seqLen, dim})
	if s.pooled {
		hidden := make([]float32, batchSize*dim)
		for b := 0; b < batchSize; b++ {
			for d := 0; d < dim; d++ {
				hidden[b*dim+d] = float32(d)
			}
		}
		return hidden, nil
	}
	hidden := make([]float32, batchSize*seqLen*dim)
	for b := 0; b < batchSize; b++ {
		for t := 0; t < seqLen; t++ {
			for d := 0; d < dim; d++ {
				hidden[b*seqLen*dim+t*dim+d] = float32(t*10 + d)
			}
		}
	}
	return hidden, nil
}

func (s *invariantSession) Close() error { return nil }

var _ onnx.Session = (*invariantSession)(nil)

// batchSensitiveSession returns hidden rows whose values depend on the batch
// size, emulating a dynamic-quantized graph (DynamicQuantizeLinear derives a
// per-tensor scale from the whole batch, so every row shifts when other rows
// are present). The shift changes the vector's direction, so L2 normalization
// does not mask it. pooled mirrors the pre-pooled ([batch, dim]) output shape.
type batchSensitiveSession struct {
	pooled bool
}

func (s *batchSensitiveSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	if s.pooled {
		hidden := make([]float32, batchSize*dim)
		for b := 0; b < batchSize; b++ {
			for d := 0; d < dim; d++ {
				hidden[b*dim+d] = float32(d) + float32(batchSize)
			}
		}
		return hidden, nil
	}
	hidden := make([]float32, batchSize*seqLen*dim)
	for b := 0; b < batchSize; b++ {
		for t := 0; t < seqLen; t++ {
			for d := 0; d < dim; d++ {
				hidden[b*seqLen*dim+t*dim+d] = float32(t*10+d) + float32(batchSize)
			}
		}
	}
	return hidden, nil
}

func (s *batchSensitiveSession) Close() error { return nil }

var _ onnx.Session = (*batchSensitiveSession)(nil)

func TestProbeBatchDeterminismPassesInvariantGraph(t *testing.T) {
	for _, pooling := range []string{"mean", "cls", "none"} {
		sess := &invariantSession{pooled: pooling == "none"}
		err := ProbeBatchDeterminism(
			func() (onnx.Session, error) { return sess, nil },
			fakeTok{}, 4, 128, true, pooling,
		)
		if err != nil {
			t.Fatalf("pooling=%s: batch-invariant graph must pass, got %v", pooling, err)
		}
	}
}

func TestProbeBatchDeterminismFailsSensitiveGraph(t *testing.T) {
	for _, pooling := range []string{"mean", "cls", "none"} {
		sess := &batchSensitiveSession{pooled: pooling == "none"}
		err := ProbeBatchDeterminism(
			func() (onnx.Session, error) { return sess, nil },
			fakeTok{}, 4, 128, true, pooling,
		)
		if !errors.Is(err, ErrBatchDependence) {
			t.Fatalf("pooling=%s: batch-sensitive graph must fail with ErrBatchDependence, got %v", pooling, err)
		}
	}
}

// TestProbeBatchDeterminismRunsExactlyThreeRuns locks the probe's run shape:
// two 1-row runs at the probe texts' own lengths and one 2-row run padded to
// the longer text — the same shapes a solo request and a two-text request are
// served with. Comparing equal-sequence-length runs instead would hide graphs
// whose output depends on the padded sequence length.
func TestProbeBatchDeterminismRunsExactlyThreeRuns(t *testing.T) {
	sess := &invariantSession{}
	err := ProbeBatchDeterminism(
		func() (onnx.Session, error) { return sess, nil },
		fakeTok{}, 2, 128, true, "mean",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.calls) != 3 {
		t.Fatalf("probe runs = %d, want exactly 3", len(sess.calls))
	}
	if sess.calls[0].batchSize != 1 || sess.calls[1].batchSize != 1 || sess.calls[2].batchSize != 2 {
		t.Fatalf("probe batch sizes = %+v, want [1 1 2]", sess.calls)
	}
	if sess.calls[0].seqLen == sess.calls[1].seqLen {
		t.Fatalf("probe solo runs should keep their natural lengths: %+v", sess.calls)
	}
	wantCo := sess.calls[0].seqLen
	if sess.calls[1].seqLen > wantCo {
		wantCo = sess.calls[1].seqLen
	}
	if sess.calls[2].seqLen != wantCo {
		t.Fatalf("co-batched seqLen = %d, want %d (max of solo lengths)", sess.calls[2].seqLen, wantCo)
	}
}

func TestProbeBatchDeterminismRequiresValidDims(t *testing.T) {
	if err := ProbeBatchDeterminism(func() (onnx.Session, error) { return nil, nil }, fakeTok{}, 0, 128, true, "mean"); err == nil {
		t.Fatal("expected error for dim=0")
	}
	if err := ProbeBatchDeterminism(func() (onnx.Session, error) { return nil, nil }, fakeTok{}, 4, 0, true, "mean"); err == nil {
		t.Fatal("expected error for max_length=0")
	}
}
