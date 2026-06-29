package pipeline

type Encoding struct {
	InputIDs      []int64
	AttentionMask []int64
}

func PadEncodings(encs []Encoding) (inputIDs []int64, attnMask []int64, seqLen int) {
	maxLen := 0
	for _, enc := range encs {
		if len(enc.InputIDs) > maxLen {
			maxLen = len(enc.InputIDs)
		}
	}

	batchSize := len(encs)
	inputIDs = make([]int64, batchSize*maxLen)
	attnMask = make([]int64, batchSize*maxLen)

	for i, enc := range encs {
		offset := i * maxLen
		origLen := len(enc.InputIDs)
		copy(inputIDs[offset:], enc.InputIDs)
		for j := 0; j < origLen; j++ {
			attnMask[offset+j] = 1
		}
	}

	return inputIDs, attnMask, maxLen
}
