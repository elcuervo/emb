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
	requests  atomic.Int64
	totalLat  atomic.Int64
}

func NewWorker(sess onnx.Session, tok tokenizer.Tokenizer, dim, maxLen int, normalize bool) *Worker {
	w := &Worker{
		session:   sess,
		tokenizer: tok,
		reqChan:   make(chan Request),
		dim:       dim,
		maxLen:    maxLen,
		normalize: normalize,
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
	encs := make([]Encoding, len(texts))
	for i, text := range texts {
		ids, mask, err := w.tokenizer.Encode(text, w.maxLen)
		if err != nil {
			return Response{Err: fmt.Errorf("tokenizing text %d: %w", i, err)}
		}
		encs[i] = Encoding{InputIDs: ids, AttentionMask: mask}
	}

	inputIDs, attnMask, seqLen := PadEncodings(encs)
	batchSize := len(texts)

	hidden, err := w.session.Run(inputIDs, attnMask, batchSize, seqLen, w.dim)
	if err != nil {
		return Response{Err: fmt.Errorf("inference: %w", err)}
	}

	embeddings := MeanPoolAndNormalize(hidden, attnMask, w.dim, seqLen, batchSize, w.normalize)

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

func (w *Worker) Close() error {
	return w.session.Close()
}

type Pool struct {
	workers []*Worker
	next    atomic.Uint64
}

func NewPool(sessionFactory func() (onnx.Session, error), tok tokenizer.Tokenizer, numWorkers, dim, maxLen int, normalize bool) (*Pool, error) {
	workers := make([]*Worker, numWorkers)
	for i := range numWorkers {
		sess, err := sessionFactory()
		if err != nil {
			return nil, fmt.Errorf("creating worker %d session: %w", i, err)
		}
		workers[i] = NewWorker(sess, tok, dim, maxLen, normalize)
	}
	return &Pool{workers: workers}, nil
}

func (p *Pool) Embed(texts []string) (Response, error) {
	idx := p.next.Add(1) - 1
	w := p.workers[idx%uint64(len(p.workers))]

	result := make(chan Response, 1)
	w.reqChan <- Request{Texts: texts, Result: result}
	return <-result, nil
}

func (p *Pool) Stats() Stats {
	var totalReqs int64
	var totalLat int64
	for _, w := range p.workers {
		totalReqs += w.Requests()
		totalLat += w.totalLat.Load()
	}
	avg := 0.0
	if totalReqs > 0 {
		avg = float64(totalLat) / float64(totalReqs)
	}
	return Stats{Requests: totalReqs, AvgLatency: avg, NumWorkers: len(p.workers)}
}

func (p *Pool) Close() error {
	for _, w := range p.workers {
		w.Close()
	}
	return nil
}
