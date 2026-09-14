package script

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/pipeline"
)

func TestMathPairwiseArrayAndPackedAgree(t *testing.T) {
	for _, op := range []string{"dot", "cosine", "l2"} {
		array := evalNum(t, `return emb.math.`+op+`({1, 2, 3}, {4, 5, 6})`)
		packed := evalNum(t, `return emb.math.`+op+`(emb.math.float32_bytes({1, 2, 3}), emb.math.float32_bytes({4, 5, 6}))`)
		if math.Abs(array-packed) > 1e-6 {
			t.Fatalf("emb.math.%s: array %v != packed %v", op, array, packed)
		}
	}
	// Expected values.
	if got := evalNum(t, `return emb.math.dot({1, 2, 3}, {4, 5, 6})`); math.Abs(got-32) > 1e-6 {
		t.Fatalf("dot = %v, want 32", got)
	}
	if got := evalNum(t, `return emb.math.l2({0, 0}, {3, 4})`); math.Abs(got-5) > 1e-6 {
		t.Fatalf("l2 = %v, want 5", got)
	}
	if got := evalNum(t, `return emb.math.cosine({1, 0}, {1, 0})`); math.Abs(got-1) > 1e-6 {
		t.Fatalf("cosine = %v, want 1", got)
	}
}

func TestMathNorm(t *testing.T) {
	if got := evalNum(t, `return emb.math.norm({3, 4})`); math.Abs(got-5) > 1e-6 {
		t.Fatalf("norm = %v, want 5", got)
	}
	packed := evalNum(t, `return emb.math.norm(emb.math.float32_bytes({3, 4}))`)
	if math.Abs(packed-5) > 1e-6 {
		t.Fatalf("packed norm = %v, want 5", packed)
	}
}

func TestMathPairwiseMisalignedPacked(t *testing.T) {
	msg := evalErr(t, `return emb.math.dot(string.char(1, 2, 3), {1})`)
	if !strings.Contains(msg, "multiple of 4") {
		t.Fatalf("misaligned packed error = %q", msg)
	}
}

func TestMathMeanPoolExcludesPadding(t *testing.T) {
	// One row, seq=2, dim=2; mask keeps only the first position.
	got := evalNum(t, `return emb.math.mean_pool({2, 2, 4, 4}, {1, 2, 2}, {1, 0})[1][1]`)
	want := 2.0 / math.Sqrt(8)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("mean_pool first component = %v, want %v", got, want)
	}
}

func TestMathMeanPoolMatchesEmbeddingPipeline(t *testing.T) {
	hidden := []float32{0.5, -1, 2, 3, 4, -2, 1, 0.25}
	const dim, seq, batch = 2, 2, 2
	masks := []int64{1, 1, 1, 0}
	want := pipeline.MeanPoolAndNormalize(hidden, masks, dim, seq, batch, true)

	v, err := EvalWithHosts(`
local h = emb.math.float32_bytes({0.5, -1, 2, 3, 4, -2, 1, 0.25})
local pooled = emb.math.mean_pool(h, {2, 2, 2}, {1, 1, 1, 0})
return {pooled[1][1], pooled[1][2], pooled[2][1], pooled[2][2]}`, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := luaNumList(t, v)
	for i := 0; i < batch; i++ {
		for d := 0; d < dim; d++ {
			wantF := math.Float32frombits(binary.LittleEndian.Uint32(want[i][d*4:]))
			if math.Abs(got[i*dim+d]-float64(wantF)) > 1e-6 {
				t.Fatalf("mean_pool[%d][%d] = %v, pipeline = %v", i+1, d+1, got[i*dim+d], wantF)
			}
		}
	}
}

func TestMathCLSNormalized(t *testing.T) {
	// [1, seq=2, dim=2]; CLS takes position 0 = {3, 4} -> {0.6, 0.8}.
	got := evalNum(t, `return emb.math.cls({3, 4, 9, 9}, {1, 2, 2})[1][1]`)
	if math.Abs(got-0.6) > 1e-6 {
		t.Fatalf("cls first component = %v, want 0.6", got)
	}
}

func TestMathTopkDescending(t *testing.T) {
	v, err := EvalWithHosts(`
local top = emb.math.topk({3, 7, 1, 7}, 2)
return {top[1].index, top[1].value, top[2].index, top[2].value}`, nil, nil, Hosts{}, EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := luaNumList(t, v)
	// Descending values; the tie is broken toward the lower index.
	if got[0] != 2 || got[1] != 7 || got[2] != 4 || got[3] != 7 {
		t.Fatalf("topk = %v, want index2/7 then index4/7", got)
	}
}

func TestMathGatherAndSliceAndScaleAndAdd(t *testing.T) {
	if got := evalNum(t, `return emb.math.gather({10, 20, 30}, {3, 1})[1]`); got != 30 {
		t.Fatalf("gather = %v, want 30", got)
	}
	if got := evalNum(t, `return emb.math.slice({1, 2, 3, 4}, {2, 2}, 2, 2)[1]`); got != 2 {
		t.Fatalf("slice = %v, want 2", got)
	}
	if got := evalNum(t, `return emb.math.scale({1, 2}, 3)[2]`); got != 6 {
		t.Fatalf("scale = %v, want 6", got)
	}
	if got := evalNum(t, `return emb.math.add({1, 2}, {10, 20})[2]`); got != 22 {
		t.Fatalf("add = %v, want 22", got)
	}
}

func TestMathShapeValidation(t *testing.T) {
	msg := evalErr(t, `return emb.math.mean_pool({1, 2}, {1, 2, 2}, {1, 1})`)
	if !strings.Contains(msg, "needs 4 elements") {
		t.Fatalf("mean_pool shape error = %q", msg)
	}
	msg = evalErr(t, `return emb.math.slice({1, 2, 3, 4}, {2, 2}, 3, 4)`)
	if !strings.Contains(msg, "out of range") {
		t.Fatalf("slice range error = %q", msg)
	}
	msg = evalErr(t, `return emb.math.mean_pool({1, 2, 3, 4}, {1, 2, 2}, {0, 0})`)
	if !strings.Contains(msg, "all-zero mask") {
		t.Fatalf("mean_pool zero-mask error = %q", msg)
	}
}

func TestMathSigmoidSoftmaxArgmaxAcceptPacked(t *testing.T) {
	array := evalNum(t, `return emb.math.sigmoid({0, 1, -1})[2]`)
	packed := evalNum(t, `return emb.math.sigmoid(emb.math.float32_bytes({0, 1, -1}))[2]`)
	if math.Abs(array-packed) > 1e-9 {
		t.Fatalf("sigmoid packed %v != array %v", packed, array)
	}
	smArray := evalNum(t, `return emb.math.softmax({1, 2, 3})[3]`)
	smPacked := evalNum(t, `return emb.math.softmax(emb.math.float32_bytes({1, 2, 3}))[3]`)
	if math.Abs(smArray-smPacked) > 1e-9 {
		t.Fatalf("softmax packed %v != array %v", smPacked, smArray)
	}
	if got := evalNum(t, `local i = emb.math.argmax(emb.math.float32_bytes({3, 7, 1})); return i`); got != 2 {
		t.Fatalf("argmax packed = %v, want 2", got)
	}
}
