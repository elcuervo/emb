package pipeline

import (
	"encoding/binary"
	"math"
	"runtime"
	"sync"
	"unsafe"

	"github.com/kelindar/simd"
)

// f32Bytes returns a little-endian byte view of f without copying. The caller
// must not retain the view past the lifetime of f (Go is little-endian on all
// supported platforms; buffers are 4-byte aligned because they are freshly
// allocated float32 slices).
func f32Bytes(f []float32) []byte {
	if len(f) == 0 {
		return nil
	}
	// This view is deliberately non-copying for the marshalling hot path; the
	// backing buffer is always freshly allocated (or an ORT-owned tensor) and
	// the view is only used transiently. gosec flags all unsafe usage.
	//nolint:gosec
	return unsafe.Slice((*byte)(unsafe.Pointer(&f[0])), len(f)*4)
}

// rowViews splits a single batch*dim*4 byte buffer into per-row views.
func rowViews(out []byte, batch, dim int) [][]byte {
	res := make([][]byte, batch)
	for b := range batch {
		res[b] = out[b*dim*4 : (b+1)*dim*4]
	}
	return res
}

// marchalRows copies each float32 row (row-major, batch×dim) of src into the
// single little-endian byte buffer dst without allocating per row.
func marshalRows(dst []byte, src []float32, batch, dim int) {
	if len(src) == 0 || len(dst) == 0 {
		return
	}
	srcBytes := f32Bytes(src)
	for b := range batch {
		off := b * dim
		copy(dst[b*dim*4:], srcBytes[off*4:off*4+dim*4])
	}
}

// MeanPoolAndNormalize mean-pools the per-token hidden states (batch×seq×dim,
// row-major) over the real (masked) tokens and returns one little-endian
// float32 vector per batch row. The accumulator uses SIMD vector adds that
// preserve per-dimension, per-token summation order, so the result is
// bit-identical to the scalar reference (see meanPoolAndNormalizeScalar).
// The result rows share one backing buffer; they are owned by the caller.
func MeanPoolAndNormalize(hidden []float32, masks []int64, dim, seqLen, batchSize int, normalize bool) [][]byte {
	if batchSize == 0 {
		return nil
	}
	out := make([]byte, batchSize*dim*4)
	acc := make([]float32, batchSize*dim)

	poolRow := func(b int) {
		base := b * seqLen * dim
		vec := acc[b*dim : (b+1)*dim]
		var count int
		for s := range seqLen {
			if masks[b*seqLen+s] == 0 {
				continue
			}
			count++
			tok := hidden[base+s*dim : base+(s+1)*dim]
			simd.AddFloat32s(vec, vec, tok)
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
		copy(out[b*dim*4:], f32Bytes(vec))
	}

	parallelRows(batchSize, poolRow)
	return rowViews(out, batchSize, dim)
}

// ExtractPrePooled returns the pre-pooled rows of a 2D output tensor
// (batch×dim, row-major) as little-endian float32 bytes. When normalize is
// false (pooling/normalization baked into the graph, e.g. a custom `e5`
// export) this is a pure buffer export with no arithmetic. Result rows share
// one backing buffer owned by the caller.
func ExtractPrePooled(hidden []float32, batchSize, dim int, normalize bool) [][]byte {
	if batchSize == 0 {
		return nil
	}
	out := make([]byte, batchSize*dim*4)
	if !normalize {
		marshalRows(out, hidden, batchSize, dim)
		return rowViews(out, batchSize, dim)
	}
	scratch := make([]float32, batchSize*dim)
	normRow := func(b int) {
		dst := scratch[b*dim : (b+1)*dim]
		copy(dst, hidden[b*dim:(b+1)*dim])
		L2Normalize(dst)
		copy(out[b*dim*4:], f32Bytes(dst))
	}
	parallelRows(batchSize, normRow)
	return rowViews(out, batchSize, dim)
}

// ExtractCLS returns the first-token (CLS) embedding of each sequence of a 3D
// output tensor (batch×seq×dim) as little-endian float32 bytes.
func ExtractCLS(hidden []float32, batchSize, dim, seqLen int, normalize bool) [][]byte {
	if batchSize == 0 {
		return nil
	}
	out := make([]byte, batchSize*dim*4)
	scratch := make([]float32, batchSize*dim)
	clsRow := func(b int) {
		dst := scratch[b*dim : (b+1)*dim]
		src := hidden[b*seqLen*dim : b*seqLen*dim+dim]
		copy(dst, src)
		if normalize {
			L2Normalize(dst)
		}
		copy(out[b*dim*4:], f32Bytes(dst))
	}
	parallelRows(batchSize, clsRow)
	return rowViews(out, batchSize, dim)
}

// parallelRows fans out per-row work across GOMAXPROCS workers, running
// inline for a single row to avoid goroutine overhead on the dominant
// single-text path.
func parallelRows(batchSize int, work func(b int)) {
	if batchSize <= 1 {
		work(0)
		return
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > batchSize {
		workers = batchSize
	}
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for b := w; b < batchSize; b += workers {
				work(b)
			}
		}()
	}
	wg.Wait()
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

// --- Scalar references (bit-exact baseline for tests and the validation
// harness) ---

func meanPoolAndNormalizeScalar(hidden []float32, masks []int64, dim, seqLen, batchSize int, normalize bool) [][]byte {
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

func extractPrePooledScalar(hidden []float32, batchSize, dim int, normalize bool) [][]byte {
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
