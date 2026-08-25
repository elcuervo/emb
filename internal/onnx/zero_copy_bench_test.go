package onnx

import (
	"os"
	"strconv"
	"testing"
)

// BenchmarkRuntimeSessionRun measures per-run allocation of the real ONNX
// session path. The pre-zero-copy code allocated two flatSize float slices per
// run (output tensor backing + result copy); steady-state should now be 0.
func BenchmarkRuntimeSessionRun(b *testing.B) {
	if err := InitEnvironment(""); err != nil {
		b.Fatal(err)
	}
	defer func() { _ = DestroyEnvironment() }()

	sess, err := NewRuntimeSessionFromBytes(
		modelBytes(b),
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		384, 3, 1, 2,
	)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = sess.Close() }()

	for _, tc := range []struct {
		batch, seq int
	}{
		{1, 512},  // single text
		{32, 512}, // full window
		{1, 16},   // short text
	} {
		b.Run(batchSeqName(tc.batch, tc.seq), func(b *testing.B) {
			ids := make([]int64, tc.batch*tc.seq)
			mask := make([]int64, tc.batch*tc.seq)
			for i := range mask {
				mask[i] = 1
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sess.Run(ids, mask, tc.batch, tc.seq, 384); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func batchSeqName(batch, seq int) string {
	return "batch=" + strconv.Itoa(batch) + "_seq=" + strconv.Itoa(seq)
}

func modelBytes(b *testing.B) []byte {
	data, err := os.ReadFile("../../models/minilm/model.onnx")
	if err != nil {
		b.Fatal(err)
	}
	return data
}
