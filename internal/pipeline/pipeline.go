package pipeline

import (
	"fmt"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type Request struct {
	Texts  []string
	Result chan<- Response
}

type Response struct {
	Embeddings [][]byte
	Err        error
}

type Stats struct {
	Requests          int64
	AvgLatency        float64
	NumWorkers        int
	Tokens            int64
	Errors            int64
	MemoryMB          int64
	Pooling           string
	Normalize         bool
	MaxLen            int
	BatchingTimeout   int
	BatchingMaxBatch  int
	BatchingMaxTokens int
	PaddingEfficiency float64
}

// encodeTexts tokenizes texts into encodings and returns the real token total.
func encodeTexts(tok tokenizer.Tokenizer, texts []string, maxLen int) ([]Encoding, int, error) {
	encs := make([]Encoding, len(texts))
	var totalTokens int
	for i, text := range texts {
		ids, mask, err := tok.Encode(text, maxLen)
		if err != nil {
			return nil, 0, fmt.Errorf("tokenizing text %d: %w", i, err)
		}
		totalTokens += len(ids)
		encs[i] = Encoding{InputIDs: ids, AttentionMask: mask}
	}
	return encs, totalTokens, nil
}

// runEncodings pads the encodings, runs one batched inference, pools/normalizes,
// and returns the embeddings plus the padded sequence length used (for padding
// efficiency measurement).
func runEncodings(sess onnx.Session, encs []Encoding, dim int, normalize bool, pooling string) ([][]byte, int, error) {
	inputIDs, attnMask, seqLen := PadEncodings(encs)
	batchSize := len(encs)

	hidden, err := sess.Run(inputIDs, attnMask, batchSize, seqLen, dim)
	if err != nil {
		return nil, seqLen, fmt.Errorf("inference: %w", err)
	}

	var embeddings [][]byte
	switch pooling {
	case "none":
		embeddings = ExtractPrePooled(hidden, batchSize, dim, normalize)
	default:
		embeddings = MeanPoolAndNormalize(hidden, attnMask, dim, seqLen, batchSize, normalize)
	}

	return embeddings, seqLen, nil
}

func processBatch(sess onnx.Session, tok tokenizer.Tokenizer, texts []string, dim, maxLen int, normalize bool, pooling string) ([][]byte, int, error) {
	encs, totalTokens, err := encodeTexts(tok, texts, maxLen)
	if err != nil {
		return nil, 0, err
	}
	embeddings, _, err := runEncodings(sess, encs, dim, normalize, pooling)
	if err != nil {
		return nil, totalTokens, err
	}
	return embeddings, totalTokens, nil
}
