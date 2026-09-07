package script

import (
	"reflect"
	"testing"

	"github.com/elcuervo/emb/internal/onnx"
)

// countingSession records RunNamed invocations and returns logits that mirror
// the first input tensor's values (for parity checks).
type countingSession struct {
	calls int
}

func (c *countingSession) RunNamed(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	c.calls++
	var ids []int64
	var shape []int64
	for _, in := range inputs {
		if in.Name == "input_ids" {
			ids = in.Int64
			shape = in.Shape
		}
	}
	out := make([]float32, len(ids))
	for i, v := range ids {
		out[i] = float32(v) + 0.5
	}
	return map[string]onnx.NamedTensor{
		"logits": {Name: "logits", Shape: shape, DType: onnx.TensorFloat32, Float: out},
	}, nil
}

func (c *countingSession) Close() error { return nil }

func batchHosts(cc *countingSession) Hosts {
	return Hosts{Run: cc.RunNamed}
}

// runBatchScript evaluates a script that batches two input specs (seq 2 and
// seq 4) and joins per-item logits.
func TestRunBatchOneCallPerBatch(t *testing.T) {
	cc := &countingSession{}
	src := `
local outs = emb.run_batch({
  { input_ids = {shape = {1, 2}, data = {10, 20}}, attention_mask = {shape = {1, 2}, data = {1, 1}} },
  { input_ids = {shape = {1, 4}, data = {30, 40, 50, 60}}, attention_mask = {shape = {1, 4}, data = {1, 1, 1, 1}} }
})
local a, b = outs[1].logits, outs[2].logits
return a.shape[2] .. "|" .. a.data[1] .. "|" .. b.shape[2] .. "|" .. b.data[4]`
	v, err := EvalWithHosts(src, nil, nil, batchHosts(cc), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cc.calls != 1 {
		t.Fatalf("expected 1 session call for a 2-item batch, got %d", cc.calls)
	}
	// Per-item outputs carry the MERGED (padded) shape [1, 4].
	if v.String() != "4|10.5|4|60.5" {
		t.Fatalf("unexpected batch result %q", v.String())
	}
}

func TestRunBatchMatchesSingleRuns(t *testing.T) {
	cc := &countingSession{}
	src := `
local single = emb.run({ input_ids = {shape = {1, 2}, data = {10, 20}} })
local outs = emb.run_batch({
  { input_ids = {shape = {1, 2}, data = {10, 20}} },
  { input_ids = {shape = {1, 3}, data = {5, 6, 7}} }
})
return single.logits.data[1] .. "|" .. outs[1].logits.data[1] .. "|" .. outs[1].logits.data[2]`
	v, err := EvalWithHosts(src, nil, nil, batchHosts(cc), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Item 0 batched alone must equal its single-run output (10.5).
	if v.String() != "10.5|10.5|20.5" {
		t.Fatalf("unexpected parity result %q", v.String())
	}
	if cc.calls != 2 {
		t.Fatalf("expected single + batch = 2 calls, got %d", cc.calls)
	}
}

func TestRunBatchRejectsBatchDimGT1(t *testing.T) {
	// An item declaring a batch dim greater than 1 would have its data
	// mis-segmented by the merge (rows advance by the inner size only), so it
	// is rejected outright.
	src := `return emb.run_batch({ { x = {shape = {2, 2}, data = {1, 2, 3, 4}} } })`
	if _, err := EvalWithHosts(src, nil, nil, batchHosts(&countingSession{}), EvalOptions{}); err == nil {
		t.Fatal("expected batch-dim>1 item to be rejected")
	}
}

func TestRunBatchRank3Scatter(t *testing.T) {
	// Items with different inner extents are scattered into the padded merged
	// row layout: dims (1,2,3) and (1,3,2) merge into (2,3,3), and each
	// item's cells must land at their row-major offsets within [3,3].
	var merged onnx.NamedTensor
	run := func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		for _, in := range inputs {
			if in.Name == "x" {
				merged = in
			}
		}
		f := make([]float32, len(merged.Int64))
		for i, v := range merged.Int64 {
			f[i] = float32(v)
		}
		return map[string]onnx.NamedTensor{
			"logits": {Name: "logits", Shape: merged.Shape, DType: onnx.TensorFloat32, Float: f},
		}, nil
	}
	src := `return #emb.run_batch({ { x = {shape = {1, 2, 3}, data = {1,2,3,4,5,6}} }, { x = {shape = {1, 3, 2}, data = {7,8,9,10,11,12}} } })`
	if _, err := EvalWithHosts(src, nil, nil, Hosts{Run: run}, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(merged.Shape, []int64{2, 3, 3}) {
		t.Fatalf("merged shape = %v, want [2 3 3]", merged.Shape)
	}
	// item 0 occupies merged indices {0..2, 3..5} (prefix of its 3x3 row),
	// item 1 (3x2) scatters to {0,1}, {3,4}, {6,7} of its row; the remaining
	// cells stay zero.
	want := []int64{1, 2, 3, 4, 5, 6, 0, 0, 0, 7, 8, 0, 9, 10, 0, 11, 12, 0}
	if !reflect.DeepEqual(merged.Int64, want) {
		t.Fatalf("merged data = %v, want %v", merged.Int64, want)
	}
}

func TestRunBatchOutputShapeMismatch(t *testing.T) {
	// A graph output that does not carry a leading batch dim of size n must
	// produce a Lua error, not a panic (start+j would read past the data).
	run := func(_ []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
		return map[string]onnx.NamedTensor{
			"logits": {Name: "logits", Shape: []int64{1, 3}, DType: onnx.TensorFloat32, Float: []float32{1, 2, 3}},
		}, nil
	}
	src := `return emb.run_batch({ { x = {shape = {1, 3}, data = {1, 2, 3}} }, { x = {shape = {1, 3}, data = {4, 5, 6}} } })`
	if _, err := EvalWithHosts(src, nil, nil, Hosts{Run: run}, EvalOptions{}); err == nil {
		t.Fatal("expected output batch-dim mismatch to error")
	}
}

func TestRunBatchValidationErrors(t *testing.T) {
	cc := &countingSession{}
	for _, src := range []string{
		// empty batch
		`return emb.run_batch({})`,
		// mismatched names
		`return emb.run_batch({
			{ input_ids = {shape = {1, 2}, data = {1, 2}} },
			{ labels   = {shape = {1, 1}, data = {1}} }
		})`,
		// mismatched dtype
		`return emb.run_batch({
			{ input_ids = {shape = {1, 2}, data = {1, 2}} },
			{ input_ids = {shape = {1, 2}, data = {0.5, 2}} }
		})`,
		// mismatched rank
		`return emb.run_batch({
			{ input_ids = {shape = {1, 2}, data = {1, 2}} },
			{ input_ids = {shape = {2},   data = {3, 4}} }
		})`,
	} {
		if _, err := EvalWithHosts(src, nil, nil, batchHosts(cc), EvalOptions{}); err == nil {
			t.Fatalf("script should fail validation: %.60s", src)
		}
	}
}

func TestRunBatchFillItems(t *testing.T) {
	// Batch items may declare constant (fill) inputs; the merged tensor is the
	// per-dimension max shape with the constant in every row.
	var gotLabels onnx.NamedTensor
	src := `
local outs = emb.run_batch({
  { labels = {shape = {1, 2}, fill = 1, dtype = "i64"} },
  { labels = {shape = {1, 4}, fill = 1, dtype = "i64"} }
})
return outs[1].labels.shape[2] .. "|" .. outs[2].labels.shape[2]`
	v, err := EvalWithHosts(src, nil, nil, Hosts{
		Run: func(inputs []onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
			for _, in := range inputs {
				if in.Name == "labels" {
					gotLabels = in
				}
			}
			out := make([]int64, len(gotLabels.Int64))
			copy(out, gotLabels.Int64)
			return map[string]onnx.NamedTensor{
				"labels": {Name: "labels", Shape: gotLabels.Shape, DType: onnx.TensorInt64, Int64: out},
			}, nil
		},
	}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// merged shape [2, 4]: batch axis 2, sequence padded to 4. Item 0's two
	// ones occupy the head of row 0 (rest zero-padded); item 1 fills row 1.
	if gotLabels.DType != onnx.TensorInt64 {
		t.Fatalf("expected int64 fill tensor, got %v", gotLabels.DType)
	}
	if len(gotLabels.Int64) != 8 {
		t.Fatalf("expected 8 merged elements, got %d", len(gotLabels.Int64))
	}
	for i, x := range gotLabels.Int64 {
		want := int64(1)
		if i >= 2 && i < 4 { // item 0's padded tail
			want = 0
		}
		if x != want {
			t.Fatalf("merged element %d = %d, want %d", i, x, want)
		}
	}
	if v.String() != "4|4" {
		t.Fatalf("unexpected batch result %q", v.String())
	}
}
