package pipeline

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

type Worker struct {
	session   onnx.Session
	tokenizer tokenizer.Tokenizer
	reqChan   chan Request
	dim       int
	maxLen    int
	normalize bool
	pooling   string
	requests  atomic.Int64
	totalLat  atomic.Int64
	tokens    atomic.Int64
	errors    atomic.Int64
}

func NewWorker(sess onnx.Session, tok tokenizer.Tokenizer, dim, maxLen int, normalize bool, pooling string) *Worker {
	w := &Worker{
		session:   sess,
		tokenizer: tok,
		reqChan:   make(chan Request),
		dim:       dim,
		maxLen:    maxLen,
		normalize: normalize,
		pooling:   pooling,
	}
	go w.run()
	return w
}

func (w *Worker) run() {
	for req := range w.reqChan {
		start := time.Now()

		resp := w.process(req.Texts)

		w.requests.Add(1)
		w.totalLat.Add(time.Since(start).Microseconds())

		req.Result <- resp
	}
}

func (w *Worker) process(texts []string) Response {
	embeddings, totalTokens, err := processBatch(w.session, w.tokenizer, texts, w.dim, w.maxLen, w.normalize, w.pooling)
	w.tokens.Add(int64(totalTokens))
	if err != nil {
		w.errors.Add(1)
		return Response{Err: err}
	}
	return Response{Embeddings: embeddings}
}

func (w *Worker) Requests() int64 {
	return w.requests.Load()
}

func (w *Worker) AvgLatency() float64 {
	r := w.requests.Load()
	if r == 0 {
		return 0
	}
	return float64(w.totalLat.Load()) / float64(r)
}

func (w *Worker) Tokens() int64 {
	return w.tokens.Load()
}

func (w *Worker) Errors() int64 {
	return w.errors.Load()
}

func (w *Worker) Close() error {
	return w.session.Close()
}

type Pool struct {
	workers   []*Worker
	batcher   *Batcher
	next      atomic.Uint64
	pooling   string
	normalize bool
	maxLen    int
}

func NewPool(sessionFactory func() (onnx.Session, error), tok tokenizer.Tokenizer, numWorkers, dim, maxLen int, normalize bool, pooling string, timeoutMS, maxBatch int) (*Pool, error) {
	if timeoutMS > 0 {
		sess, err := sessionFactory()
		if err != nil {
			return nil, fmt.Errorf("creating batcher session: %w", err)
		}
		return &Pool{
			batcher:   NewBatcher(sess, tok, dim, maxLen, normalize, pooling, timeoutMS, maxBatch),
			pooling:   pooling,
			normalize: normalize,
			maxLen:    maxLen,
		}, nil
	}

	workers := make([]*Worker, numWorkers)
	for i := range numWorkers {
		sess, err := sessionFactory()
		if err != nil {
			return nil, fmt.Errorf("creating worker %d session: %w", i, err)
		}
		workers[i] = NewWorker(sess, tok, dim, maxLen, normalize, pooling)
	}
	return &Pool{
		workers:   workers,
		pooling:   pooling,
		normalize: normalize,
		maxLen:    maxLen,
	}, nil
}

func (p *Pool) Embed(texts []string) (Response, error) {
	if p.batcher != nil {
		return p.batcher.Embed(texts)
	}
	idx := p.next.Add(1) - 1
	w := p.workers[idx%uint64(len(p.workers))]

	result := make(chan Response, 1)
	w.reqChan <- Request{Texts: texts, Result: result}
	return <-result, nil
}

func (p *Pool) Stats() Stats {
	if p.batcher != nil {
		return Stats{
			Requests:         p.batcher.Requests(),
			AvgLatency:       p.batcher.AvgLatency(),
			NumWorkers:       1,
			Tokens:           p.batcher.Tokens(),
			Errors:           p.batcher.Errors(),
			Pooling:          p.pooling,
			Normalize:        p.normalize,
			MaxLen:           p.maxLen,
			BatchingTimeout:  int(p.batcher.timeout.Milliseconds()),
			BatchingMaxBatch: p.batcher.maxBatch,
		}
	}
	var totalReqs int64
	var totalLat int64
	var totalTokens int64
	var totalErrors int64
	for _, w := range p.workers {
		totalReqs += w.Requests()
		totalLat += w.totalLat.Load()
		totalTokens += w.Tokens()
		totalErrors += w.Errors()
	}
	avg := 0.0
	if totalReqs > 0 {
		avg = float64(totalLat) / float64(totalReqs)
	}
	return Stats{
		Requests:   totalReqs,
		AvgLatency: avg,
		NumWorkers: len(p.workers),
		Tokens:     totalTokens,
		Errors:     totalErrors,
		Pooling:    p.pooling,
		Normalize:  p.normalize,
		MaxLen:     p.maxLen,
	}
}

func (p *Pool) Close() error {
	if p.batcher != nil {
		return p.batcher.Close()
	}
	for _, w := range p.workers {
		w.Close()
	}
	return nil
}
