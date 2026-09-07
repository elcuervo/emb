package script

import (
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
