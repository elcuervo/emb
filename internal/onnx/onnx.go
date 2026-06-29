package onnx

type Session interface {
	Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error)
	Close() error
}
