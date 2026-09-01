package pipeline

import (
	"encoding/binary"
	"math"
	"testing"
)

// decodeRow decodes dim little-endian float32 values from bytes.
func decodeRow(t *testing.T, b []byte, dim int) []float32 {
	t.Helper()
	if len(b) != dim*4 {
		t.Fatalf("row bytes = %d, want %d", len(b), dim*4)
	}
	row := make([]float32, dim)
	for i := range dim {
		row[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return row
}

func cosine(a, b []float32) float32 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// TestMeanPoolFastBitIdentical asserts the SIMD mean pool is byte-identical to
// the scalar reference across varied shapes and mask patterns.
func TestMeanPoolFastBitIdentical(t *testing.T) {
	cases := []struct {
		batch, seq, dim int
		normalize       bool
	}{
		{1, 2, 4, false},
		{2, 3, 8, false},
		{4, 7, 16, false},
		{3, 5, 6, true},
		{8, 10, 32, true},
		{1, 1, 1, true},
	}
	for _, c := range cases {
		hidden := make([]float32, c.batch*c.seq*c.dim)
		masks := make([]int64, c.batch*c.seq)
		for i := range hidden {
			hidden[i] = float32((i*17+3)%97) - 48 // noisy, non-trivial values
		}
		for b := range c.batch {
			for s := range c.seq {
				// ~2/3 of each sequence is real, rest padded
				if s%3 != 2 {
					masks[b*c.seq+s] = 1
				}
			}
		}
		fast := MeanPoolAndNormalize(hidden, masks, c.dim, c.seq, c.batch, c.normalize)
		scalar := meanPoolAndNormalizeScalar(hidden, masks, c.dim, c.seq, c.batch, c.normalize)
		if len(fast) != len(scalar) {
			t.Fatalf("case %+v: got %d rows, want %d", c, len(fast), len(scalar))
		}
		for b := range fast {
			if string(fast[b]) != string(scalar[b]) {
				t.Fatalf("case %+v row %d: not byte-identical\nfast=%v\nscal=%v", c, b, fast[b], scalar[b])
			}
		}
	}
}

// TestExtractPrePooledFastVsScalar asserts the fast pre-pooled extraction
// (normalize on and off) matches the scalar reference.
func TestExtractPrePooledFastVsScalar(t *testing.T) {
	hidden := make([]float32, 4*7)
	for i := range hidden {
		hidden[i] = float32((i*13+5)%37) - 18
	}
	for _, norm := range []bool{false, true} {
		fast := ExtractPrePooled(hidden, 4, 7, norm)
		scalar := extractPrePooledScalar(hidden, 4, 7, norm)
		for b := range fast {
			if string(fast[b]) != string(scalar[b]) {
				t.Fatalf("normalize=%v row %d: not byte-identical", norm, b)
			}
		}
	}
}

// TestExtractPrePooledNormalizeOffPureExport verifies the normalize=false path
// is a pure little-endian export of the input tensor (no arithmetic).
func TestExtractPrePooledNormalizeOffPureExport(t *testing.T) {
	dim, batch := 6, 2
	hidden := []float32{1, -2, 3, 0, 5.5, -0.25, 7, 8, -9, 0.5, 11, 12}
	out := ExtractPrePooled(hidden, batch, dim, false)
	exported := decodeRow(t, out[1], dim)
	for i := range dim {
		if exported[i] != hidden[dim+i] {
			t.Fatalf("pos %d: exported %v != input %v", i, exported[i], hidden[dim+i])
		}
		// Bytes must be the raw little-endian float32 bits.
		want := hidden[dim+i]
		got := math.Float32frombits(binary.LittleEndian.Uint32(out[1][i*4:]))
		if math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("pos %d: bit mismatch", i)
		}
	}
}

// TestExtractCLS verifies CLS extraction takes the first token of each row.
func TestExtractCLS(t *testing.T) {
	dim, seq, batch := 2, 3, 2
	hidden := []float32{
		1, 2, 100, 200, 300, 400, // seq 0: tokens [1,2],[100,200],[300,400]
		5, 6, 700, 800, 900, 1000, // seq 1: tokens [5,6],...
	}
	out := ExtractCLS(hidden, batch, dim, seq, false)
	r0 := decodeRow(t, out[0], dim)
	r1 := decodeRow(t, out[1], dim)
	if r0[0] != 1 || r0[1] != 2 {
		t.Fatalf("row0 = %v, want [1 2]", r0)
	}
	if r1[0] != 5 || r1[1] != 6 {
		t.Fatalf("row1 = %v, want [5 6]", r1)
	}
}

// TestFastPathsCosineGate asserts fast results stay within 0.99 cosine of the
// scalar reference over a larger randomized workload (the harness gate).
func TestFastPathsCosineGate(t *testing.T) {
	// ExtractPrePooled (normalize on): fast vs scalar — L2 arithmetic is
	// identical (float64 accumulation), expect cosine 1.
	hidden := make([]float32, 16*64)
	for i := range hidden {
		hidden[i] = float32((i*29+7)%101) - 50
	}
	fast := ExtractPrePooled(hidden, 16, 64, true)
	scalar := extractPrePooledScalar(hidden, 16, 64, true)
	for b := range fast {
		fa, sc := decodeRow(t, fast[b], 64), decodeRow(t, scalar[b], 64)
		if c := cosine(fa, sc); c < 0.99 {
			t.Fatalf("row %d cosine %f < 0.99", b, c)
		}
	}
}

// BenchmarkMeanPoolFast vs the scalar reference.
func BenchmarkMeanPoolFast(b *testing.B) {
	dim, seqLen := 384, 128
	for _, bs := range []int{1, 4, 32} {
		b.Run("batch="+itoa(bs), func(b *testing.B) {
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
				r = MeanPoolAndNormalize(hidden, mask, dim, seqLen, bs, true)
			}
			poolingResult = r
		})
	}
}

func BenchmarkExtractPrePooledFast(b *testing.B) {
	dim := 768
	for _, bs := range []int{1, 4, 32} {
		b.Run("batch="+itoa(bs), func(b *testing.B) {
			hidden := make([]float32, bs*dim)
			for i := range hidden {
				hidden[i] = float32(i%7) / 3
			}
			b.ResetTimer()
			var r [][]byte
			for i := 0; i < b.N; i++ {
				r = ExtractPrePooled(hidden, bs, dim, true)
			}
			poolingResult = r
		})
	}
}

func itoa(n int) string {
	if n == 1 {
		return "1"
	}
	if n == 4 {
		return "4"
	}
	return "32"
}
