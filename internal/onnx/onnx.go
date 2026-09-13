package onnx

type Session interface {
	Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error)
	Close() error
}

type OutputInfo struct {
	Name string
	Rank int
	Dim  int64
}

// InputInfo describes one graph input tensor. Dimensions may contain -1 for a
// dynamic axis.
type InputInfo struct {
	Name       string
	Rank       int
	Dimensions []int64
}
