package pipeline

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type batchItem struct {
	req    Request
	encs   []Encoding
	tokens int // real tokens in this item's texts
}

// tokResult is what a tokenizer producer hands to the run loop.
type tokResult struct {
	req    Request
	encs   []Encoding
	tokens int
	err    error
}

type Batcher struct {
	reqChan         chan Request
	workChan        chan tokResult
	session         onnx.Session
	tokenizer       tokenizer.Tokenizer
	dim             int
	maxLen          int
	normalize       bool
	pooling         string
	timeout         time.Duration
	maxBatch        int
	maxBatchTokens  int
	tokenizeWorkers int
	requests        atomic.Int64
	totalLat        atomic.Int64
	tokens          atomic.Int64
	errors          atomic.Int64
	realTokens      atomic.Int64
	processedSlots  atomic.Int64
	done            chan struct{}
	once            sync.Once
}

// NewBatcher creates a windowed batch collector. maxBatch bounds the window by
// request count; maxBatchTokens > 0 additionally bounds it by accumulated real
// tokens (0 = count-only behavior). A single request is never split, even when
// its own tokens exceed the budget. tokenizeWorkers > 0 spawns dedicated
// tokenizer goroutines so tokenization overlaps inference.
func NewBatcher(sess onnx.Session, tok tokenizer.Tokenizer, dim, maxLen int, normalize bool, pooling string, timeoutMS, maxBatch, maxBatchTokens, tokenizeWorkers int) *Batcher {
	b := &Batcher{
		reqChan:         make(chan Request, 128),
		workChan:        make(chan tokResult, 256),
		session:         sess,
		tokenizer:       tok,
		dim:             dim,
		maxLen:          maxLen,
		normalize:       normalize,
		pooling:         pooling,
		timeout:         time.Duration(timeoutMS) * time.Millisecond,
		maxBatch:        maxBatch,
		maxBatchTokens:  maxBatchTokens,
		tokenizeWorkers: tokenizeWorkers,
		done:            make(chan struct{}),
	}
	for range tokenizeWorkers {
		go b.producer()
	}
	go b.run()
	return b
}

// producer encodes queued requests and hands encodings to the run loop,
// overlapping tokenization of later requests with inference of earlier batches.
func (b *Batcher) producer() {
	for {
		select {
		case req, ok := <-b.reqChan:
			if !ok {
				return
			}
			encs, toks, err := encodeTexts(b.tokenizer, req.Texts, b.maxLen)
			select {
			case b.workChan <- tokResult{req: req, encs: encs, tokens: toks, err: err}:
			case <-b.done:
				return
			}
		case <-b.done:
			return
		}
	}
}

func (b *Batcher) Embed(texts []string) (Response, error) {
	result := make(chan Response, 1)
	b.reqChan <- Request{Texts: texts, Result: result}
	return <-result, nil
}

func (b *Batcher) run() {
	var batch []batchItem
	budget := 0
	timerRunning := false
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	flush := func() {
		if len(batch) == 0 {
			return
		}
		start := time.Now()

		all := make([]Encoding, 0, len(batch))
		offsets := make([]int, len(batch))
		totalTokens := 0
		for i, it := range batch {
			offsets[i] = len(all)
			all = append(all, it.encs...)
			totalTokens += it.tokens
		}

		resp, seqLen, err := b.process(all)
		if err == nil {
			// Track padding efficiency: real tokens / processed token-slots.
			b.tokens.Add(int64(totalTokens))
			b.realTokens.Add(int64(totalTokens))
			b.processedSlots.Add(int64(len(all)) * int64(seqLen))
		} else {
			b.errors.Add(1)
		}

		for i, it := range batch {
			n := len(it.encs)
			if err != nil {
				it.req.Result <- Response{Err: resp.Err}
			} else {
				it.req.Result <- Response{Embeddings: resp.Embeddings[offsets[i] : offsets[i]+n]}
			}
		}

		b.requests.Add(int64(len(batch)))
		b.totalLat.Add(time.Since(start).Microseconds())

		batch = batch[:0]
		budget = 0
	}

	enqueue := func(it batchItem) {
		batch = append(batch, it)
		budget += it.tokens
		// Budget reached OR count reached — flush as one run.
		if len(batch) >= b.maxBatch || (b.maxBatchTokens > 0 && budget >= b.maxBatchTokens) {
			flush()
			if timerRunning {
				timer.Stop()
				timerRunning = false
			}
		} else if !timerRunning {
			timer.Reset(b.timeout)
			timerRunning = true
		}
	}

	for {
		select {
		case req := <-b.reqChan:
			// Serial (tokenizeWorkers == 0): encode inline like pre-change behavior.
			encs, toks, err := encodeTexts(b.tokenizer, req.Texts, b.maxLen)
			if err != nil {
				b.errors.Add(1)
				req.Result <- Response{Err: err}
				continue
			}
			enqueue(batchItem{req: req, encs: encs, tokens: toks})
		case tr := <-b.workChan:
			// Async: a producer tokenized this request off the run path.
			if tr.err != nil {
				b.errors.Add(1)
				tr.req.Result <- Response{Err: tr.err}
				continue
			}
			enqueue(batchItem{req: tr.req, encs: tr.encs, tokens: tr.tokens})
		case <-timer.C:
			timerRunning = false
			flush()
		case <-b.done:
			flush()
			return
		}
	}
}

func (b *Batcher) process(encs []Encoding) (Response, int, error) {
	embeddings, seqLen, err := runEncodings(b.session, encs, b.dim, b.normalize, b.pooling)
	if err != nil {
		return Response{Err: err}, seqLen, err
	}
	return Response{Embeddings: embeddings}, seqLen, nil
}

func (b *Batcher) paddingEfficiency() float64 {
	slots := b.processedSlots.Load()
	if slots == 0 {
		return 0
	}
	return float64(b.realTokens.Load()) / float64(slots)
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

func (b *Batcher) Tokens() int64 {
	return b.tokens.Load()
}

func (b *Batcher) Errors() int64 {
	return b.errors.Load()
}

func (b *Batcher) Close() error {
	b.once.Do(func() {
		close(b.done)
	})
	return b.session.Close()
}
