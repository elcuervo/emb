package pipeline

import (
	"math"
	"strconv"
	"testing"
)

func TestPadEncodings(t *testing.T) {
	encs := []Encoding{
		{InputIDs: []int64{101, 200, 300, 102}, AttentionMask: []int64{1, 1, 1, 1}},
		{InputIDs: []int64{101, 400, 102}, AttentionMask: []int64{1, 1, 1}},
		{InputIDs: []int64{101, 500, 600, 700, 102}, AttentionMask: []int64{1, 1, 1, 1, 1}},
	}

	ids, mask, seqLen := PadEncodings(encs)

	if seqLen != 5 {
		t.Fatalf("expected seqLen=5, got %d", seqLen)
	}

	// Check dimensions
	if len(ids) != 15 || len(mask) != 15 {
		t.Fatalf("expected 15 elements, got ids=%d mask=%d", len(ids), len(mask))
	}

	// First encoding padded to 5
	if ids[0] != 101 || ids[1] != 200 || ids[2] != 300 || ids[3] != 102 || ids[4] != 0 {
		t.Fatalf("bad padding for encoding 0: got %v", ids[0:5])
	}
	if mask[0] != 1 || mask[1] != 1 || mask[2] != 1 || mask[3] != 1 || mask[4] != 0 {
		t.Fatalf("bad mask for encoding 0")
	}

	// Second encoding padded to 5
	if ids[5] != 101 || ids[6] != 400 || ids[7] != 102 || ids[8] != 0 || ids[9] != 0 {
		t.Fatalf("bad padding for encoding 1: got %v", ids[5:10])
	}
	if mask[5] != 1 || mask[6] != 1 || mask[7] != 1 || mask[8] != 0 || mask[9] != 0 {
		t.Fatalf("bad mask for encoding 1")
	}

	// Third encoding fits exactly
	if ids[10] != 101 || ids[11] != 500 || ids[12] != 600 || ids[13] != 700 || ids[14] != 102 {
		t.Fatalf("bad padding for encoding 2: got %v", ids[10:15])
	}
	if mask[10] != 1 || mask[11] != 1 || mask[12] != 1 || mask[13] != 1 || mask[14] != 1 {
		t.Fatalf("bad mask for encoding 2")
	}
}

func TestPadEncodingsEmpty(t *testing.T) {
	ids, mask, seqLen := PadEncodings(nil)
	if seqLen != 0 {
		t.Fatalf("expected seqLen=0, got %d", seqLen)
	}
	if len(ids) != 0 || len(mask) != 0 {
		t.Fatal("expected empty slices")
	}
}

func TestMeanPoolBasic(t *testing.T) {
	// 2 tokens, 4 dims, batch=1
	// token 1: [1, 2, 3, 4], token 2: [5, 6, 7, 8]
	hidden := []float32{
		1, 2, 3, 4,
		5, 6, 7, 8,
	}
	mask := []int64{1, 1}
	dim := 4
	seqLen := 2
	batchSize := 1

	result := MeanPoolAndNormalize(hidden, mask, dim, seqLen, batchSize, false)

	if len(result) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(result))
	}

	got := result[0]
	// Mean should be [3, 4, 5, 6]
	expected := []float32{3, 4, 5, 6}
	for i := range expected {
		v := math.Float32frombits(uint32(got[i*4]) | uint32(got[i*4+1])<<8 | uint32(got[i*4+2])<<16 | uint32(got[i*4+3])<<24)
		if v != expected[i] {
			t.Fatalf("position %d: expected %f, got %f", i, expected[i], v)
		}
	}
}

func TestMeanPoolWithPadding(t *testing.T) {
	// 3 tokens, 2 dims, batch=1, but mask says only first 2 tokens count
	hidden := []float32{
		10, 20,
		30, 40,
		1000, 2000, // padding tokens that should be ignored
	}
	mask := []int64{1, 1, 0}
	dim := 2
	seqLen := 3
	batchSize := 1

	result := MeanPoolAndNormalize(hidden, mask, dim, seqLen, batchSize, false)

	got := result[0]
	v0 := math.Float32frombits(uint32(got[0]) | uint32(got[1])<<8 | uint32(got[2])<<16 | uint32(got[3])<<24)
	v1 := math.Float32frombits(uint32(got[4]) | uint32(got[5])<<8 | uint32(got[6])<<16 | uint32(got[7])<<24)

	if v0 != 20 || v1 != 30 {
		t.Fatalf("expected [20, 30], got [%f, %f]", v0, v1)
	}
}

func TestMeanPoolBatch(t *testing.T) {
	// batch=2, seq_len=2, dim=3
	hidden := []float32{
		// batch 0: token1=[1,1,1], token2=[2,2,2]
		1, 1, 1,
		2, 2, 2,
		// batch 1: token1=[3,3,3], token2=padded
		3, 3, 3,
		0, 0, 0,
	}
	mask := []int64{
		1, 1,
		1, 0,
	}
	dim := 3
	seqLen := 2
	batchSize := 2

	result := MeanPoolAndNormalize(hidden, mask, dim, seqLen, batchSize, false)

	if len(result) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(result))
	}

	// batch 0: mean of [1,1,1] and [2,2,2] = [1.5,1.5,1.5]
	got0 := result[0]
	for i := 0; i < 3; i++ {
		v := math.Float32frombits(uint32(got0[i*4]) | uint32(got0[i*4+1])<<8 | uint32(got0[i*4+2])<<16 | uint32(got0[i*4+3])<<24)
		if v != 1.5 {
			t.Fatalf("batch0 pos %d: expected 1.5, got %f", i, v)
		}
	}

	// batch 1: mean of [3,3,3] only (ignore padded) = [3,3,3]
	got1 := result[1]
	for i := 0; i < 3; i++ {
		v := math.Float32frombits(uint32(got1[i*4]) | uint32(got1[i*4+1])<<8 | uint32(got1[i*4+2])<<16 | uint32(got1[i*4+3])<<24)
		if v != 3.0 {
			t.Fatalf("batch1 pos %d: expected 3.0, got %f", i, v)
		}
	}
}

func TestL2Normalize(t *testing.T) {
	vec := []float32{3, 0, 4}
	L2Normalize(vec)

	// expected: [3/5, 0, 4/5] = [0.6, 0, 0.8]
	eps := float32(0.0001)
	if abs(vec[0]-0.6) > eps {
		t.Fatalf("expected 0.6, got %f", vec[0])
	}
	if abs(vec[1]) > eps {
		t.Fatalf("expected 0, got %f", vec[1])
	}
	if abs(vec[2]-0.8) > eps {
		t.Fatalf("expected 0.8, got %f", vec[2])
	}
}

func TestL2NormalizeUnitLength(t *testing.T) {
	vec := []float32{1, 2, 3, 4, 5}
	L2Normalize(vec)

	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	if sumSq < 0.999 || sumSq > 1.001 {
		t.Fatalf("expected unit length, got %f", sumSq)
	}
}

func TestL2NormalizeZero(t *testing.T) {
	vec := []float32{0, 0, 0}
	L2Normalize(vec)
	for _, v := range vec {
		if v != 0 {
			t.Fatalf("expected all zeros, got %f", v)
		}
	}
}

func TestMeanPoolAndNormalizeBatch(t *testing.T) {
	// batch=2, seq_len=2, dim=2
	hidden := []float32{
		1, 1,
		1, 1,
		2, 2,
		0, 0,
	}
	mask := []int64{
		1, 1,
		1, 0,
	}
	dim := 2
	seqLen := 2
	batchSize := 2

	result := MeanPoolAndNormalize(hidden, mask, dim, seqLen, batchSize, true)

	if len(result) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(result))
	}

	// Both should be unit length
	for i, emb := range result {
		var sumSq float64
		for j := 0; j < dim; j++ {
			v := math.Float32frombits(uint32(emb[j*4]) | uint32(emb[j*4+1])<<8 | uint32(emb[j*4+2])<<16 | uint32(emb[j*4+3])<<24)
			sumSq += float64(v) * float64(v)
		}
		if sumSq < 0.999 || sumSq > 1.001 {
			t.Fatalf("embedding %d: expected unit length, got %f", i, sumSq)
		}
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

var (
	poolingResult [][]byte
	l2Result      []float32
)

func BenchmarkMeanPool(b *testing.B) {
	dim := 768
	seqLen := 128
	batchSizes := []int{1, 4, 16, 64}

	for _, bs := range batchSizes {
		b.Run("batch="+strconv.Itoa(bs), func(b *testing.B) {
			hidden := make([]float32, bs*seqLen*dim)
			mask := make([]int64, bs*seqLen)
			for i := range mask {
				if i%seqLen < seqLen/2 {
					mask[i] = 1
				}
			}
			b.ResetTimer()
			var r [][]byte
			for i := 0; i < b.N; i++ {
				r = MeanPoolAndNormalize(hidden, mask, dim, seqLen, bs, false)
			}
			poolingResult = r
		})
	}
}

func BenchmarkL2Normalize(b *testing.B) {
	dims := []int{384, 768, 1024}
	for _, d := range dims {
		b.Run("dim="+strconv.Itoa(d), func(b *testing.B) {
			vec := make([]float32, d)
			for i := range vec {
				vec[i] = float32(i)
			}
			b.ResetTimer()
			var r []float32
			for i := 0; i < b.N; i++ {
				L2Normalize(vec)
				r = vec
			}
			l2Result = r
		})
	}
}
