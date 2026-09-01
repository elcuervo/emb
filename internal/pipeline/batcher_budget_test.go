package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/tokenizer"
)

// fakeTok returns one token per rune, truncated to maxLen.
type fakeTok struct{}

func (fakeTok) Encode(text string, maxLength int) ([]int64, []int64, error) {
	n := len(text)
	if n > maxLength {
		n = maxLength
	}
	ids := make([]int64, n)
	mask := make([]int64, n)
	for i := 0; i < n; i++ {
		ids[i] = int64(i + 1)
		mask[i] = 1
	}
	return ids, mask, nil
}

func (fakeTok) Close() error { return nil }

// recordingSession records the (batchSize, seqLen, dim) of each run and returns
// a fixed hidden buffer of the right size for mean pooling.
type recordingSession struct {
	calls      []runCall
	dim        int
	sleep      time.Duration
	lastHidden []float32
}

type runCall struct {
	batchSize int
	seqLen    int
	dim       int
}

func (s *recordingSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	if s.sleep > 0 {
		time.Sleep(s.sleep)
	}
	s.calls = append(s.calls, runCall{batchSize, seqLen, dim})
	hidden := make([]float32, batchSize*seqLen*dim)
	for b := 0; b < batchSize; b++ {
		for t := 0; t < seqLen; t++ {
			for d := 0; d < dim; d++ {
				hidden[b*seqLen*dim+t*dim+d] = float32(b*1000 + t*10 + d)
			}
		}
	}
	s.lastHidden = hidden
	return hidden, nil
}

func (s *recordingSession) Close() error { return nil }

var _ onnx.Session = (*recordingSession)(nil)
var _ tokenizer.Tokenizer = fakeTok{}

// embedAsync drives Embed from goroutines so the synchronous result wait
// doesn't deadlock the single batcher goroutine.
func embedAsync(b *Batcher, texts []string, out chan<- Response) {
	resp, err := b.Embed(texts)
	if err != nil {
		out <- Response{Err: err}
		return
	}
	out <- resp
}

func TestBatcherFushesOnTokenBudget(t *testing.T) {
	sess := &recordingSession{dim: 2}
	tok := &gatedTok{started: make(chan struct{}, 8), allowed: make(chan struct{}, 8)}
	b := NewBatcher(sess, tok, 2, 16, false, "mean", 100000, 100, 7, 0)
	defer b.Close()

	out := make(chan Response, 3)
	// Tokens {1,1,5} sum to exactly the budget; no prefix reaches 7. The first
	// request parks the serial run loop inside Encode; the other two queue while
	// it is blocked, so the idle-flush drain (which encodes inline in serial
	// mode) is guaranteed to see all three and flush one padded run.
	go embedAsync(b, []string{"a"}, out)
	waitOn(t, tok.started)
	go embedAsync(b, []string{"b"}, out)
	go embedAsync(b, []string{"efghi"}, out)
	waitQueued(t, b, 2)
	tok.allowed <- struct{}{}
	tok.allowed <- struct{}{}
	tok.allowed <- struct{}{}
	for range 3 {
		r := <-out
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
	}

	// All three requests were queued before the drain ran, so they must flush
	// as one run: budget 7 admits {1,1,5} and the padded run is 3×5 slots.
	if len(sess.calls) != 1 {
		t.Fatalf("expected one fully-coalesced run, got %d: %v", len(sess.calls), sess.calls)
	}
	if sess.calls[0].batchSize != 3 {
		t.Fatalf("expected batchSize 3, got %d: %v", sess.calls[0].batchSize, sess.calls)
	}
	if sess.calls[0].seqLen != 5 {
		t.Fatalf("expected seqLen 5 (padded to longest), got %d", sess.calls[0].seqLen)
	}
	if got := b.Tokens(); got != 7 {
		t.Fatalf("expected 7 total tokens, got %d", got)
	}
	if eff := b.paddingEfficiency(); eff != 7.0/15.0 {
		t.Fatalf("expected padding efficiency 7/15, got %f", eff)
	}
}

func TestBatcherNeverSplitsOversizedRequest(t *testing.T) {
	sess := &recordingSession{dim: 2}
	b := NewBatcher(sess, fakeTok{}, 2, 16, false, "mean", 100000, 100, 3, 0)
	defer b.Close()

	out := make(chan Response, 1)
	go embedAsync(b, []string{"vwxyz"}, out) // 5 tokens > budget 3
	r := <-out
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Embeddings) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(r.Embeddings))
	}
	if len(sess.calls) != 1 || sess.calls[0].batchSize != 1 {
		t.Fatalf("expected a single unsplit run, got %+v", sess.calls)
	}
}

func TestBatcherCountOnlyMode(t *testing.T) {
	sess := &recordingSession{dim: 2}
	tok := &gatedTok{started: make(chan struct{}, 8), allowed: make(chan struct{}, 8)}
	// maxBatchTokens = 0 -> count-only flush at maxBatch=2. The first request
	// parks the run loop in Encode; the other two queue while it is blocked, so
	// the idle-flush drain deterministically pairs two requests and the third
	// flushes alone.
	b := NewBatcher(sess, tok, 2, 16, false, "mean", 100000, 2, 0, 0)
	defer b.Close()

	out := make(chan Response, 3)
	go embedAsync(b, []string{"a"}, out)
	waitOn(t, tok.started)
	go embedAsync(b, []string{"b"}, out)
	go embedAsync(b, []string{"c"}, out)
	waitQueued(t, b, 2)
	tok.allowed <- struct{}{}
	tok.allowed <- struct{}{}
	tok.allowed <- struct{}{}
	for range 3 {
		r := <-out
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		if len(r.Embeddings) != 1 {
			t.Fatalf("request was split: expected 1 embedding, got %d", len(r.Embeddings))
		}
	}
	// Count-only mode truncates the first drain at maxBatch 2; the third queued
	// request flushes on its own. Two runs of sizes {2,1}, nothing oversized,
	// no request split.
	if len(sess.calls) != 2 {
		t.Fatalf("expected 2 runs (2+1), got %d: %v", len(sess.calls), sess.calls)
	}
	if sess.calls[0].batchSize != 2 || sess.calls[1].batchSize != 1 {
		t.Fatalf("unexpected batch sizes: %+v", sess.calls)
	}
}

func TestBatcherDistributesEmbeddingsToRightRequests(t *testing.T) {
	sess := &recordingSession{dim: 2}
	// Budget 6 binds only once all three 2-token texts are in the window, so all
	// arrival orders batch together (2+2+2=6).
	b := NewBatcher(sess, fakeTok{}, 2, 16, true, "mean", 100000, 100, 6, 0)
	defer b.Close()

	out := make(chan Response, 2)
	go embedAsync(b, []string{"ab", "cd"}, out) // 2 texts
	go embedAsync(b, []string{"ef"}, out)       // 1 text
	for range 2 {
		r := <-out
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		// mean pooling normalizes; embeddings are 2 floats each (dim=2)
		if len(r.Embeddings) > 2 {
			t.Fatalf("expected <=2 embeddings, got %d", len(r.Embeddings))
		}
		for _, e := range r.Embeddings {
			if len(e) != 8 { // 2 float32 bytes x 2 dims
				t.Fatalf("expected 8 bytes per embedding, got %d", len(e))
			}
		}
	}
	// Distributions must be correct regardless of run boundaries; idle-flush
	// means near-simultaneous arrivals may coalesce into one run or split into
	// two, but a single request is never split and nothing runs oversized.
	if len(sess.calls) > 2 {
		t.Fatalf("expected ≤2 runs (idle-flush), got %d", len(sess.calls))
	}
	totalBatch := 0
	for _, c := range sess.calls {
		totalBatch += c.batchSize
		if c.batchSize > 3 {
			t.Fatalf("run exceeded 3 texts: %+v", sess.calls)
		}
	}
	if totalBatch != 3 {
		t.Fatalf("expected 3 texts across runs, got %d: %v", totalBatch, sess.calls)
	}
}

func TestBatcherTokenizeErrorIsolation(t *testing.T) {
	sess := &recordingSession{dim: 2}
	b := NewBatcher(sess, errTok{}, 2, 16, false, "mean", 100000, 100, 5, 0)
	defer b.Close()

	out := make(chan Response, 1)
	go embedAsync(b, []string{"boom"}, out)
	r := <-out
	if r.Err == nil {
		t.Fatal("expected tokenize error")
	}
	if len(sess.calls) != 0 {
		t.Fatalf("expected no session run on tokenize error, got %d", len(sess.calls))
	}
}

type errTok struct{ fakeTok }

func (errTok) Encode(text string, maxLength int) ([]int64, []int64, error) {
	return nil, nil, fmt.Errorf("boom")
}

type slowTok struct {
	fakeTok
	delay time.Duration
}

func (t slowTok) Encode(text string, maxLength int) ([]int64, []int64, error) {
	time.Sleep(t.delay)
	return t.fakeTok.Encode(text, maxLength)
}

// gatedTok blocks each Encode until the test releases it, letting the test
// control exactly when tokenized results become available to the batcher.
type gatedTok struct {
	fakeTok
	started chan struct{}
	allowed chan struct{}
}

func (t *gatedTok) Encode(text string, maxLength int) ([]int64, []int64, error) {
	t.started <- struct{}{}
	<-t.allowed
	return t.fakeTok.Encode(text, maxLength)
}

// gatedSession blocks each Run until released, letting the test hold an
// inference in flight while inspecting whether tokenization is progressing.
type gatedSession struct {
	recordingSession
	runStarted chan struct{}
	runRelease chan struct{}
}

func (s *gatedSession) Run(inputIDs, attnMask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	s.runStarted <- struct{}{}
	<-s.runRelease
	return s.recordingSession.Run(inputIDs, attnMask, batchSize, seqLen, dim)
}

var _ onnx.Session = (*gatedSession)(nil)
var _ tokenizer.Tokenizer = (*gatedTok)(nil)

// waitOn waits for a gated signal with a generous timeout so a regression
// fails the test (via t.Fatal) instead of hanging the suite.
func waitOn(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for gated signal")
	}
}

// waitQueued waits until n requests sit in the batcher's pending queue. Used
// with the serial (tokenizeWorkers=0) path, where the run loop encodes from
// reqChan inline: guaranteeing the queue is full before releasing the gate
// makes the idle-flush drain's coalescing deterministic.
func waitQueued(t *testing.T, b *Batcher, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(b.reqChan) < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d queued requests, have %d", n, len(b.reqChan))
		}
		time.Sleep(time.Millisecond)
	}
}

// TestAsyncTokenizerOverlapsWork proves dedicated tokenizer workers hide
// tokenization behind inference. Instead of wall-clock timing (which is
// machine- and load-dependent), it uses gates: with tokenizeWorkers >= 1 the
// producer keeps tokenizing the next queued request while the batcher's Run is
// blocked mid-inference; with tokenizeWorkers == 0 the single run loop cannot
// encode while it is inside Run.
func TestAsyncTokenizerOverlapsWork(t *testing.T) {
	t.Run("async tokenization overlaps inference", func(t *testing.T) {
		sess := &gatedSession{runStarted: make(chan struct{}, 8), runRelease: make(chan struct{}, 8)}
		sess.dim = 2
		tok := &gatedTok{started: make(chan struct{}, 8), allowed: make(chan struct{}, 8)}
		b := NewBatcher(sess, tok, 2, 16, false, "mean", 100000, 100, 1, 1)
		defer b.Close()

		out := make(chan Response, 3)
		go embedAsync(b, []string{"a"}, out)
		go embedAsync(b, []string{"b"}, out)
		go embedAsync(b, []string{"c"}, out)

		// req1 is tokenized, then handed to the run loop which blocks in Run.
		waitOn(t, tok.started)
		tok.allowed <- struct{}{}
		waitOn(t, sess.runStarted)

		// While inference of req1 is held in flight, the producer must already
		// be tokenizing req2 — the definitive overlap signal.
		waitOn(t, tok.started)

		// Drain the remainder one result at a time: R1, E2, R2, E3, R3.
		sess.runRelease <- struct{}{}
		tok.allowed <- struct{}{}
		waitOn(t, sess.runStarted)
		sess.runRelease <- struct{}{}
		waitOn(t, tok.started)
		tok.allowed <- struct{}{}
		waitOn(t, sess.runStarted)
		sess.runRelease <- struct{}{}

		for range 3 {
			r := <-out
			if r.Err != nil {
				t.Fatalf("unexpected error: %v", r.Err)
			}
		}
		if len(sess.calls) != 3 {
			t.Fatalf("expected 3 runs (budget 1), got %d: %v", len(sess.calls), sess.calls)
		}
	})

	t.Run("serial path cannot overlap", func(t *testing.T) {
		sess := &gatedSession{runStarted: make(chan struct{}, 8), runRelease: make(chan struct{}, 8)}
		sess.dim = 2
		tok := &gatedTok{started: make(chan struct{}, 8), allowed: make(chan struct{}, 8)}
		b := NewBatcher(sess, tok, 2, 16, false, "mean", 100000, 100, 1, 0)
		defer b.Close()

		out := make(chan Response, 3)

		// The run loop tokenizes req1 inline, then blocks inside Run.
		go embedAsync(b, []string{"a"}, out)
		waitOn(t, tok.started)
		tok.allowed <- struct{}{}
		waitOn(t, sess.runStarted)

		// Send the next request only while inference is blocked: in serial mode
		// the single run loop cannot tokenize it, so a short probe must see
		// nothing start.
		go embedAsync(b, []string{"b"}, out)
		select {
		case <-tok.started:
			t.Fatal("serial path tokenized a later request while running")
		case <-time.After(50 * time.Millisecond):
		}

		sess.runRelease <- struct{}{}
		waitOn(t, tok.started)
		tok.allowed <- struct{}{}
		waitOn(t, sess.runStarted)

		go embedAsync(b, []string{"c"}, out)
		select {
		case <-tok.started:
			t.Fatal("serial path tokenized a later request while running")
		case <-time.After(50 * time.Millisecond):
		}

		sess.runRelease <- struct{}{}
		waitOn(t, tok.started)
		tok.allowed <- struct{}{}
		waitOn(t, sess.runStarted)
		sess.runRelease <- struct{}{}

		for range 3 {
			r := <-out
			if r.Err != nil {
				t.Fatalf("unexpected error: %v", r.Err)
			}
		}
		if len(sess.calls) != 3 {
			t.Fatalf("expected 3 runs (budget 1), got %d: %v", len(sess.calls), sess.calls)
		}
	})
}

// TestSerialTokenizerIsDesiredOverlap shows tokenize_workers=0 keeps the
// serial behavior (mostly a guard against regressing the fast path).
func TestSerialTokenizerSerialWall(t *testing.T) {
	sess := &recordingSession{dim: 2, sleep: 10 * time.Millisecond}
	b := NewBatcher(sess, slowTok{delay: 10 * time.Millisecond}, 2, 16, false, "mean", 100000, 100, 1, 0)
	defer b.Close()

	out := make(chan Response, 3)
	start := time.Now()
	go embedAsync(b, []string{"a"}, out)
	go embedAsync(b, []string{"b"}, out)
	go embedAsync(b, []string{"c"}, out)
	for range 3 {
		<-out
	}
	wall := time.Since(start)
	// Serialized encode+run per request: 3×(10+10) = 60ms lower bound.
	if wall < 48*time.Millisecond {
		t.Fatalf("serial path finished suspiciously fast (no per-request encode expected), wall=%v", wall)
	}
}
