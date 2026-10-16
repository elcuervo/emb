package onnx

import (
	"errors"
	"os"
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

// TestRuntimeSessionFailedOutputCreationIsNotReused covers the reused-output
// requirement: when the session destroys its output tensor to re-create it for
// a new shape and that creation fails, the next run must create the tensor
// again rather than treat the destroyed one as still valid and hand out its
// slice. The creation is injected through the newEmptyTensor seam so the
// failure needs no host memory pressure.
func TestRuntimeSessionFailedOutputCreationIsNotReused(t *testing.T) {
	if err := InitEnvironment(""); err != nil {
		t.Skipf("onnx runtime unavailable: %v", err)
	}
	defer func() { _ = DestroyEnvironment() }()

	data, err := os.ReadFile("../../models/minilm/model.onnx")
	if err != nil {
		t.Skipf("test model not present: %v (run: just download-model)", err)
	}

	sess, err := NewRuntimeSessionFromBytes(
		data,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		384, 3, 1, 2, ExecModeSequential, true,
	)
	if err != nil {
		t.Skipf("session creation failed (model mismatch?): %v", err)
	}
	defer func() { _ = sess.Close() }()

	ids := func(n int) []int64 {
		out := make([]int64, n)
		for i := range out {
			out[i] = 101
		}
		return out
	}
	mask := func(n int) []int64 {
		out := make([]int64, n)
		for i := range out {
			out[i] = 1
		}
		return out
	}

	const seq = 4
	first, err := sess.Run(ids(seq), mask(seq), 1, seq, 384)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	firstCopy := make([]float32, len(first))
	copy(firstCopy, first)
	heldTensor := sess.outTensor

	// The next run asks for a different sequence length, so the session
	// destroys the held tensor and its replacement fails.
	injected := errors.New("injected: output creation failed")
	realNew := newEmptyTensor
	newEmptyTensor = func(ort.Shape) (*ort.Tensor[float32], error) { return nil, injected }
	t.Cleanup(func() { newEmptyTensor = realNew })

	const otherSeq = 8
	if _, err := sess.Run(ids(otherSeq), mask(otherSeq), 1, otherSeq, 384); !errors.Is(err, injected) {
		t.Fatalf("run with a failing output creation = %v, want the injected error", err)
	}

	// The failed creation must leave nothing behind that a later run could read
	// as a valid buffer: neither the tensor pointer nor the shape bookkeeping
	// that would make this shape look already allocated.
	if sess.outTensor != nil {
		t.Fatalf("failed output creation left the session holding %p (the destroyed tensor was %p)", sess.outTensor, heldTensor)
	}
	if sess.outShape != nil || sess.outFlat != 0 {
		t.Fatalf("failed output creation left outShape=%v outFlat=%d, want the reuse state cleared", sess.outShape, sess.outFlat)
	}

	// With creation working again, the shape the session used to hold must be
	// re-created (not reused from the destroyed tensor) and must reproduce the
	// first run's data.
	newEmptyTensor = realNew
	again, err := sess.Run(ids(seq), mask(seq), 1, seq, 384)
	if err != nil {
		t.Fatalf("run after the failed creation: %v", err)
	}
	if sess.outTensor == heldTensor {
		t.Fatal("run after the failed creation reused the destroyed output tensor")
	}
	if len(again) != len(firstCopy) {
		t.Fatalf("run after the failed creation returned %d floats, want %d", len(again), len(firstCopy))
	}
	for i := range again {
		if again[i] != firstCopy[i] {
			t.Fatalf("run after the failed creation[%d] = %v, want %v (data from the destroyed tensor)", i, again[i], firstCopy[i])
		}
	}
}
