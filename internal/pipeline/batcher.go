package pipeline

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type Batcher struct {
	reqChan   chan Request
	session   onnx.Session
	tokenizer tokenizer.Tokenizer
	dim       int
	maxLen    int
	normalize bool
	pooling   string
	timeout   time.Duration
	maxBatch  int
	requests  atomic.Int64
	totalLat  atomic.Int64
	done      chan struct{}
	once      sync.Once
}

func NewBatcher(sess onnx.Session, tok tokenizer.Tokenizer, dim, maxLen int, normalize bool, pooling string, timeoutMS, maxBatch int) *Batcher {
	b := &Batcher{
		reqChan:   make(chan Request, 128),
		session:   sess,
		tokenizer: tok,
		dim:       dim,
		maxLen:    maxLen,
		normalize: normalize,
		pooling:   pooling,
		timeout:   time.Duration(timeoutMS) * time.Millisecond,
		maxBatch:  maxBatch,
		done:      make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *Batcher) Embed(texts []string) (Response, error) {
	result := make(chan Response, 1)
	b.reqChan <- Request{Texts: texts, Result: result}
	return <-result, nil
}

func (b *Batcher) run() {
	var batch []Request
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	flush := func() {
		if len(batch) == 0 {
			return
		}
		start := time.Now()

		allTexts := make([]string, 0, len(batch))
		for _, req := range batch {
			allTexts = append(allTexts, req.Texts...)
		}

		resp := b.process(allTexts)

		idx := 0
		for _, req := range batch {
			n := len(req.Texts)
			if resp.Err != nil {
				req.Result <- Response{Err: resp.Err}
			} else {
				req.Result <- Response{Embeddings: resp.Embeddings[idx : idx+n]}
				idx += n
			}
		}

		b.requests.Add(int64(len(batch)))
		b.totalLat.Add(time.Since(start).Microseconds())

		batch = batch[:0]
	}

	timerRunning := false
	for {
		select {
		case req := <-b.reqChan:
			batch = append(batch, req)
			if len(batch) >= b.maxBatch {
				flush()
				if timerRunning {
					timer.Stop()
					timerRunning = false
				}
			} else if !timerRunning {
				timer.Reset(b.timeout)
				timerRunning = true
			}
		case <-timer.C:
			timerRunning = false
			flush()
		case <-b.done:
			flush()
			return
		}
	}
}

func (b *Batcher) process(texts []string) Response {
	encs := make([]Encoding, len(texts))
	for i, text := range texts {
		ids, mask, err := b.tokenizer.Encode(text, b.maxLen)
		if err != nil {
			return Response{Err: fmt.Errorf("tokenizing text %d: %w", i, err)}
		}
		encs[i] = Encoding{InputIDs: ids, AttentionMask: mask}
	}

	inputIDs, attnMask, seqLen := PadEncodings(encs)
	batchSize := len(texts)

	hidden, err := b.session.Run(inputIDs, attnMask, batchSize, seqLen, b.dim)
	if err != nil {
		return Response{Err: fmt.Errorf("inference: %w", err)}
	}

	var embeddings [][]byte
	switch b.pooling {
	case "none":
		embeddings = ExtractPrePooled(hidden, batchSize, b.dim, b.normalize)
	default:
		embeddings = MeanPoolAndNormalize(hidden, attnMask, b.dim, seqLen, batchSize, b.normalize)
	}

	return Response{Embeddings: embeddings}
}

func (b *Batcher) Requests() int64 {
	return b.requests.Load()
}

func (b *Batcher) AvgLatency() float64 {
	r := b.requests.Load()
	if r == 0 {
		return 0
	}
	return float64(b.totalLat.Load()) / float64(r)
}

func (b *Batcher) Close() error {
	b.once.Do(func() {
		close(b.done)
	})
	return b.session.Close()
}
