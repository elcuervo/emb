package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// ErrClosed is returned when inference is submitted after a pool or batcher
// has begun closing.
var ErrClosed = errors.New("embedding pipeline is closed")

type Request struct {
	Texts  []string
	Result chan<- Response
	// Ctx bounds how long this request may wait for admission and lets a
	// cancelled request be dropped instead of run. Nil means unbounded.
	Ctx context.Context
	// Deadline is Ctx's deadline, pre-computed so a batcher can pick the
	// longest-deadline context for a coalesced batch without unwrapping each
	// request's context. Zero means no deadline.
	Deadline time.Time
}

// DeadlineOf reports a context's deadline as a zero time when it has none.
func DeadlineOf(ctx context.Context) time.Time {
	if ctx == nil {
		return time.Time{}
	}
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Time{}
}

// DeadlineAfter returns the context with the latest deadline and a cancel
// function; the whole caller's work may be abandoned only once every request it
// carries has expired.
func DeadlineAfter(deads ...time.Time) (context.Context, context.CancelFunc) {
	var max time.Time
	for _, d := range deads {
		if d.After(max) {
			max = d
		}
	}
	if max.IsZero() {
		return context.Background(), func() {}
	}
	return context.WithDeadline(context.Background(), max)
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
	Pooling           string
	Normalize         bool
	MaxLen            int
	BatchingTimeout   int
	BatchingMaxBatch  int
	BatchingMaxTokens int
	PaddingEfficiency float64
	// Dispatch/run observability (EMB.STATS/EMB.INFO): cumulative time waiting
	// for a free worker or queue slot, cumulative inference time, the number of
	// inference runs, and the live busy/total session gauges.
	DispatchWaitUs int64
	RunUs          int64
	Runs           int64
	SessionsBusy   int64
	SessionsTotal  int64
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
func runEncodings(ctx context.Context, sess onnx.Session, encs []Encoding, dim int, normalize bool, pooling string) ([][]byte, int, error) {
	inputIDs, attnMask, seqLen := PadEncodings(encs)
	batchSize := len(encs)

	hidden, err := onnx.RunContext(ctx, sess, inputIDs, attnMask, batchSize, seqLen, dim)
	if err != nil {
		return nil, seqLen, fmt.Errorf("inference: %w", err)
	}

	var embeddings [][]byte
	switch pooling {
	case "none":
		embeddings = ExtractPrePooled(hidden, batchSize, dim, normalize)
	case "cls":
		embeddings = ExtractCLS(hidden, batchSize, dim, seqLen, normalize)
	default:
		embeddings = MeanPoolAndNormalize(hidden, attnMask, dim, seqLen, batchSize, normalize)
	}

	return embeddings, seqLen, nil
}

func processBatch(ctx context.Context, sess onnx.Session, tok tokenizer.Tokenizer, texts []string, dim, maxLen int, normalize bool, pooling string) ([][]byte, int, error) {
	encs, totalTokens, err := encodeTexts(tok, texts, maxLen)
	if err != nil {
		return nil, 0, err
	}
	embeddings, _, err := runEncodings(ctx, sess, encs, dim, normalize, pooling)
	if err != nil {
		return nil, totalTokens, err
	}
	return embeddings, totalTokens, nil
}
