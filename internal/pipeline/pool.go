package pipeline

import (
	"errors"
	"fmt"
	"sync"
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
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
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
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *Worker) run() {
	defer close(w.done)
	for {
		var req Request
		select {
		case req = <-w.reqChan:
		case <-w.stop:
			return
		}
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
	w.closeOnce.Do(func() {
		close(w.stop)
		<-w.done
		w.closeErr = w.session.Close()
	})
	return w.closeErr
}

type Pool struct {
	workers   []*Worker
	batcher   *Batcher
	next      atomic.Uint64
	pooling   string
	normalize bool
	maxLen    int
	// tok is the tokenizer shared by every worker/batcher in the pool. It is
	// retained so the scripted path can reuse it instead of loading a second
	// tokenizer for the same model (see registry.openScriptResources).
	tok tokenizer.Tokenizer

	lifecycleMu sync.Mutex
	accepting   bool
	active      sync.WaitGroup
	closeOnce   sync.Once
	closeErr    error
}

// Tokenizer returns the tokenizer shared by the pool, or nil when the pool was
// constructed without one. Callers must not close it: the registry owns its
// lifetime.
func (p *Pool) Tokenizer() tokenizer.Tokenizer { return p.tok }

func NewPool(sessionFactory func() (onnx.Session, error), tok tokenizer.Tokenizer, numWorkers, dim, maxLen int, normalize bool, pooling string, timeoutMS, maxBatch, maxBatchTokens, tokenizeWorkers int) (*Pool, error) {
	if timeoutMS > 0 {
		sess, err := sessionFactory()
		if err != nil {
			return nil, fmt.Errorf("creating batcher session: %w", err)
		}
		return &Pool{
			batcher:   NewBatcher(sess, tok, dim, maxLen, normalize, pooling, timeoutMS, maxBatch, maxBatchTokens, tokenizeWorkers),
			pooling:   pooling,
			normalize: normalize,
			maxLen:    maxLen,
			tok:       tok,
			accepting: true,
		}, nil
	}

	// Construct every native session before starting any worker. If a later
	// factory call fails, roll back the sessions already opened so a failed
	// pool is never partially published and leaves no goroutine behind.
	sessions := make([]onnx.Session, 0, numWorkers)
	for i := range numWorkers {
		sess, err := sessionFactory()
		if err != nil {
			var closeErrs []error
			for _, opened := range sessions {
				if closeErr := opened.Close(); closeErr != nil {
					closeErrs = append(closeErrs, closeErr)
				}
			}
			if closeErr := errors.Join(closeErrs...); closeErr != nil {
				return nil, fmt.Errorf("creating worker %d session: %w (rollback: %v)", i, err, closeErr)
			}
			return nil, fmt.Errorf("creating worker %d session: %w", i, err)
		}
		sessions = append(sessions, sess)
	}
	workers := make([]*Worker, len(sessions))
	for i, sess := range sessions {
		workers[i] = NewWorker(sess, tok, dim, maxLen, normalize, pooling)
	}
	return &Pool{
		workers:   workers,
		pooling:   pooling,
		normalize: normalize,
		maxLen:    maxLen,
		tok:       tok,
		accepting: true,
	}, nil
}

func (p *Pool) Embed(texts []string) (Response, error) {
	p.lifecycleMu.Lock()
	if !p.accepting {
		p.lifecycleMu.Unlock()
		return Response{}, ErrClosed
	}
	p.active.Add(1)
	p.lifecycleMu.Unlock()
	defer p.active.Done()

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
			Requests:          p.batcher.Requests(),
			AvgLatency:        p.batcher.AvgLatency(),
			NumWorkers:        1,
			Tokens:            p.batcher.Tokens(),
			Errors:            p.batcher.Errors(),
			Pooling:           p.pooling,
			Normalize:         p.normalize,
			MaxLen:            p.maxLen,
			BatchingTimeout:   int(p.batcher.timeout.Milliseconds()),
			BatchingMaxBatch:  p.batcher.maxBatch,
			BatchingMaxTokens: p.batcher.maxBatchTokens,
			PaddingEfficiency: p.batcher.paddingEfficiency(),
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
	p.closeOnce.Do(func() {
		p.lifecycleMu.Lock()
		p.accepting = false
		p.lifecycleMu.Unlock()
		p.active.Wait()

		var closeErrs []error
		if p.batcher != nil {
			closeErrs = append(closeErrs, p.batcher.Close())
		} else {
			for _, w := range p.workers {
				closeErrs = append(closeErrs, w.Close())
			}
		}
		p.closeErr = errors.Join(closeErrs...)
	})
	return p.closeErr
}
