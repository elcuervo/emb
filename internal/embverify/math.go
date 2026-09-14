// Package embverify holds the shared embedding-verification helpers used by
// cmd/emb-verify, cmd/emb-multi-verify and cmd/emb-verify-performance: float32
// decoding, similarity/ranking math, the reference-artifact schema, and an
// embed client over internal/resp.
package embverify

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// DecodeFloat32 decodes little-endian float32 elements. It rejects a payload
// whose length is not a whole number of float32s.
func DecodeFloat32(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("embverify: float32 payload length %d is not a multiple of 4", len(b))
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out, nil
}

// ToFloat32 narrows a reference vector stored as float64 (JSON numbers) to
// float32. The narrowing is intentional: the wire format is float32, and both
// the reference checksum and the served bytes use that precision.
func ToFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}

// Cosine returns the cosine similarity of two vectors. Vectors of different
// lengths, or either empty/zero, yield 0.
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// RankDocuments returns document indices ordered by descending cosine to q.
// Ties keep their original order.
func RankDocuments(q []float32, docs [][]float32) []int {
	idx := make([]int, len(docs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return Cosine(q, docs[idx[i]]) > Cosine(q, docs[idx[j]])
	})
	return idx
}

// DCG scores a ranking against per-document gains, truncated to k. A document
// absent from gains contributes nothing.
func DCG(ranking []int, gains map[int]float64, k int) float64 {
	var sum float64
	for i, doc := range ranking {
		if i >= k {
			break
		}
		if g, ok := gains[doc]; ok {
			sum += g / math.Log2(float64(i+2))
		}
	}
	return sum
}

// MeanNDCG10 measures how well B's ranking retains A's top-10 results. For each
// query, A's top-10 documents are graded by reference rank (A's first result is
// worth the most) and B's ranking is scored against those gains with nDCG@10;
// the result averages over queries (1.0 means B ranks identically to A on A's
// top-10). Graded gains make a reordering of A's top-10 score below 1.0, so the
// gate can see an ordering regression and not only a missing document.
func MeanNDCG10(qA, docA, qB, docB [][]float32) float64 {
	k := min(10, len(docA))
	if k == 0 || len(qA) == 0 || len(qB) != len(qA) || len(docB) != len(docA) {
		return 0
	}
	var sum float64
	for qi := range qA {
		refRank := RankDocuments(qA[qi], docA)[:k]
		gains := make(map[int]float64, k)
		for i, d := range refRank {
			gains[d] = float64(k - i) // A's first result gets the largest gain
		}
		if ideal := DCG(refRank, gains, k); ideal != 0 {
			sum += DCG(RankDocuments(qB[qi], docB), gains, k) / ideal
		}
	}
	return sum / float64(len(qA))
}

// MeanCosine returns the mean of the per-index cosine similarities of two
// equal-length embedding sets, and the minimum. It returns (0, 0) when the
// sets are empty or mismatched in length.
func MeanCosine(a, b [][]float32) (mean, minPair float64) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, 0
	}
	minPair = 2
	for i := range a {
		c := Cosine(a[i], b[i])
		mean += c
		if c < minPair {
			minPair = c
		}
	}
	return mean / float64(len(a)), minPair
}
