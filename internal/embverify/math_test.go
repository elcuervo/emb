package embverify

import (
	"math"
	"testing"
)

func TestDecodeFloat32(t *testing.T) {
	got, err := DecodeFloat32(encodeFloat32(1, -2, 3.5))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []float32{1, -2, 3.5}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if _, err := DecodeFloat32([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected an error for a 3-byte payload")
	}
}

func encodeFloat32(vals ...float32) []byte {
	b := make([]byte, len(vals)*4)
	for i, v := range vals {
		b[i*4] = byte(math.Float32bits(v))
		b[i*4+1] = byte(math.Float32bits(v) >> 8)
		b[i*4+2] = byte(math.Float32bits(v) >> 16)
		b[i*4+3] = byte(math.Float32bits(v) >> 24)
	}
	return b
}

func TestCosine(t *testing.T) {
	if c := Cosine([]float32{1, 0}, []float32{1, 0}); math.Abs(c-1) > 1e-9 {
		t.Fatalf("identical cosine = %v, want 1", c)
	}
	if c := Cosine([]float32{1, 0}, []float32{0, 1}); math.Abs(c) > 1e-9 {
		t.Fatalf("orthogonal cosine = %v, want 0", c)
	}
	if c := Cosine([]float32{0, 0}, []float32{1, 1}); c != 0 {
		t.Fatalf("zero-vector cosine = %v, want 0", c)
	}
	if c := Cosine([]float32{1}, []float32{1, 2}); c != 0 {
		t.Fatalf("mismatched-length cosine = %v, want 0", c)
	}
}

func TestRankDocuments(t *testing.T) {
	q := []float32{1, 0}
	docs := [][]float32{
		{0, 1},  // cosine 0
		{1, 0},  // cosine 1
		{-1, 0}, // cosine -1
	}
	got := RankDocuments(q, docs)
	want := []int{1, 0, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranking = %v, want %v", got, want)
		}
	}
}

func TestMeanNDCG10IdenticalRankings(t *testing.T) {
	q := [][]float32{{1, 0}}
	docs := [][]float32{{1, 0.1}, {0.9, 0.2}, {0, 1}, {0.4, 0.4}}
	got := MeanNDCG10(q, docs, q, docs)
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("identical rankings nDCG = %v, want 1", got)
	}
}

func TestMeanNDCG10DegenerateInputs(t *testing.T) {
	if got := MeanNDCG10(nil, nil, nil, nil); got != 0 {
		t.Fatalf("empty inputs = %v, want 0", got)
	}
	q := [][]float32{{1, 0}}
	docs := [][]float32{{1, 0}}
	if got := MeanNDCG10(q, docs, q, nil); got != 0 {
		t.Fatalf("mismatched B = %v, want 0", got)
	}
}

func TestMeanNDCG10DetectsReordering(t *testing.T) {
	// A's top-10, reversed by B: same documents, different order. Graded gains
	// make this score below 1, so the gate can see an ordering regression.
	q := [][]float32{{1, 0}}
	docA := make([][]float32, 0, 10)
	docB := make([][]float32, 0, 10)
	for i := 0; i < 10; i++ {
		docA = append(docA, []float32{1, float32(i)})     // cosine decreases with i
		docB = append(docB, []float32{1, float32(9 - i)}) // reversed ranking
	}
	got := MeanNDCG10(q, docA, q, docB)
	if got >= 1 {
		t.Fatalf("reversed top-10 nDCG = %v, want < 1", got)
	}
	if got <= 0 {
		t.Fatalf("reversed top-10 nDCG = %v, want > 0", got)
	}
}

func TestMeanCosine(t *testing.T) {
	a := [][]float32{{1, 0}, {0, 1}}
	b := [][]float32{{1, 0}, {1, 0}}
	mean, minPair := MeanCosine(a, b)
	if math.Abs(mean-0.5) > 1e-9 {
		t.Fatalf("mean = %v, want 0.5", mean)
	}
	if math.Abs(minPair) > 1e-9 {
		t.Fatalf("min = %v, want 0", minPair)
	}
	if m, n := MeanCosine(nil, nil); m != 0 || n != 0 {
		t.Fatalf("empty = (%v,%v), want (0,0)", m, n)
	}
}
