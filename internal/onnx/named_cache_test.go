package onnx

import (
	"sync/atomic"
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

// namedInputsForSeq builds a single-sequence named input set with the given
// sequence length, for exercising the output-tensor cache across shapes.
func namedInputsForSeq(inputNames []string, seq int) []NamedTensor {
	out := make([]NamedTensor, 0, len(inputNames))
	for _, name := range inputNames {
		data := make([]int64, seq)
		switch name {
		case "input_ids":
			for i := range data {
				data[i] = 101
			}
		case "attention_mask":
			for i := range data {
				data[i] = 1
			}
		default:
			// token_type_ids and any other integral input: zeros.
		}
		out = append(out, NamedTensor{Name: name, Shape: []int64{1, int64(seq)}, DType: TensorInt64, Int64: data})
	}
	return out
}

// TestNamedSessionOutputCacheBoundedAndDestroyed verifies the per-session
// output-tensor cache stays at its documented cap and that each evicted entry
// has its tensors destroyed (rather than merely unlinked).
func TestNamedSessionOutputCacheBoundedAndDestroyed(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data := namedTestModel(t)
	inputNames, outNames := namedTestInputs(t)
	sess, err := NewNamedRuntimeSessionFromBytes(data, inputNames, outNames, 1, 2, ExecModeSequential)
	if err != nil {
		t.Fatalf("creating named session: %v", err)
	}
	defer func() { _ = sess.Close() }()

	orig := destroyValue
	var destroyed atomic.Int64
	destroyValue = func(v ort.Value) {
		destroyed.Add(1)
		orig(v)
	}
	defer func() { destroyValue = orig }()

	for _, seq := range []int{4, 5, 6, 7, 8} {
		if _, err := sess.RunNamed(namedInputsForSeq(inputNames, seq)); err != nil {
			t.Fatalf("run at seq=%d: %v", seq, err)
		}
	}
	if got := sess.CachedOutputSets(); got > maxCachedOutputShapes {
		t.Fatalf("cached output sets = %d, want <= %d", got, maxCachedOutputShapes)
	}
	if destroyed.Load() == 0 {
		t.Fatal("eviction did not destroy the evicted tensors")
	}
}

// TestNamedSessionCloseIdempotentAndEmptiesCache verifies Close releases the
// cached output tensors and is safe to call twice.
func TestNamedSessionCloseIdempotentAndEmptiesCache(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data := namedTestModel(t)
	inputNames, outNames := namedTestInputs(t)
	sess, err := NewNamedRuntimeSessionFromBytes(data, inputNames, outNames, 1, 2, ExecModeSequential)
	if err != nil {
		t.Fatalf("creating named session: %v", err)
	}
	defer func() { _ = sess.Close() }()

	if _, err := sess.RunNamed(namedInputsForSeq(inputNames, 4)); err != nil {
		t.Fatalf("populating run: %v", err)
	}
	if sess.CachedOutputSets() == 0 {
		t.Fatal("expected a populated output cache before close")
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if got := sess.CachedOutputSets(); got != 0 {
		t.Fatalf("cached output sets after close = %d, want 0", got)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("second close should be a no-op, got %v", err)
	}
}
