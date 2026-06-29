package pipeline

import (
	"encoding/binary"
	"math"
)

func MeanPoolAndNormalize(hidden []float32, masks []int64, dim, seqLen, batchSize int, normalize bool) [][]byte {
	result := make([][]byte, batchSize)
	for b := range batchSize {
		mask := masks[b*seqLen : (b+1)*seqLen]
		vec := make([]float32, dim)
		var count int
		for s := range seqLen {
			if mask[s] == 0 {
				continue
			}
			count++
			offset := (b*seqLen + s) * dim
			for d := range dim {
				vec[d] += hidden[offset+d]
			}
		}
		if count > 0 {
			inv := 1.0 / float32(count)
			for d := range dim {
				vec[d] *= inv
			}
		}
		if normalize {
			L2Normalize(vec)
		}
		bytes := make([]byte, dim*4)
		for d := range dim {
			binary.LittleEndian.PutUint32(bytes[d*4:], math.Float32bits(vec[d]))
		}
		result[b] = bytes
	}
	return result
}

func ExtractPrePooled(hidden []float32, batchSize, dim int, normalize bool) [][]byte {
	result := make([][]byte, batchSize)
	for b := range batchSize {
		offset := b * dim
		vec := hidden[offset : offset+dim]
		if normalize {
			cp := make([]float32, dim)
			copy(cp, vec)
			L2Normalize(cp)
			vec = cp
		}
		bytes := make([]byte, dim*4)
		for d := range dim {
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
