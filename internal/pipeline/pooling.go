package pipeline

import (
	"encoding/binary"
	"math"
)

func MeanPoolAndNormalize(hidden []float32, masks []int64, dim, seqLen, batchSize int, normalize bool) [][]byte {
	result := make([][]byte, batchSize)
	for b := 0; b < batchSize; b++ {
		mask := masks[b*seqLen : (b+1)*seqLen]
		vec := make([]float32, dim)
		var count int
		for s := 0; s < seqLen; s++ {
			if mask[s] == 0 {
				continue
			}
			count++
			offset := (b*seqLen + s) * dim
			for d := 0; d < dim; d++ {
				vec[d] += hidden[offset+d]
			}
		}
		if count > 0 {
			inv := 1.0 / float32(count)
			for d := 0; d < dim; d++ {
				vec[d] *= inv
			}
		}
		if normalize {
			L2Normalize(vec)
		}
		bytes := make([]byte, dim*4)
		for d := 0; d < dim; d++ {
			binary.LittleEndian.PutUint32(bytes[d*4:], math.Float32bits(vec[d]))
		}
		result[b] = bytes
	}
	return result
}

func L2Normalize(vec []float32) {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	if sumSq == 0 {
		return
	}
	norm := float32(math.Sqrt(sumSq))
	for i := range vec {
		vec[i] /= norm
	}
}
