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
	b := NewBatcher(sess, fakeTok{}, 2, 16, false, "mean", 100000, 100, 7, 0)
	defer b.Close()

	out := make(chan Response, 3)
	// Tokens {1,1,5} sum to exactly the budget; no prefix reaches 7, so every
	// arrival order flushes all three in one run.
	go embedAsync(b, []string{"a"}, out)
	go embedAsync(b, []string{"b"}, out)
	go embedAsync(b, []string{"efghi"}, out)
	for range 3 {
		r := <-out
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
	}

	// Idle-flush (batcher) serves the first arrival immediately, so a strict
	// "every arrival order ends in one run" is no longer guaranteed — the drain
	// coalesces what is already queued, and near-simultaneous goroutine starts
	// may land either side of the drain. Contract: no request is ever split, all
	// tokens count, and at most one split run beyond the fully-coalesced case is
	// possible.
	if len(sess.calls) > 2 {
		t.Fatalf("expected ≤2 runs (idle-flush), got %d: %v", len(sess.calls), sess.calls)
	}
	totalBatch := 0
	for _, c := range sess.calls {
		totalBatch += c.batchSize
	}
	if totalBatch != 3 {
		t.Fatalf("expected 3 texts across runs, got %d: %v", totalBatch, sess.calls)
	}
	if got := b.Tokens(); got != 7 {
		t.Fatalf("expected 7 total tokens, got %d", got)
	}
	if len(sess.calls) == 1 {
		// Fully coalesced: verify padding accounting and longest-seq padding.
		if sess.calls[0].batchSize != 3 {
			t.Fatalf("expected batchSize 3, got %d", sess.calls[0].batchSize)
		}
		if sess.calls[0].seqLen != 5 {
			t.Fatalf("expected seqLen 5 (padded to longest), got %d", sess.calls[0].seqLen)
		}
		if eff := b.paddingEfficiency(); eff != 7.0/15.0 {
			t.Fatalf("expected padding efficiency 7/15, got %f", eff)
		}
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
	// maxBatchTokens = 0 -> count-only flush at maxBatch=2. Short timeout so the
	// leftover third request flushes via the window instead of awaiting Close.
	b := NewBatcher(sess, fakeTok{}, 2, 16, false, "mean", 50, 2, 0, 0)
	defer b.Close()

	out := make(chan Response, 3)
	go embedAsync(b, []string{"a"}, out)
	go embedAsync(b, []string{"b"}, out)
	go embedAsync(b, []string{"c"}, out)
	for range 3 {
		r := <-out
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
	}
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

// TestAsyncTokenizerOverlapsWork proves dedicated tokenizer workers hide
// tokenization behind inference: with a slow tokenizer and a slow session,
// 3 concurrent 1-token requests should complete in less than the fully
// serialized encode+run time (3×(15ms+20ms) = 105ms).
func TestAsyncTokenizerOverlapsWork(t *testing.T) {
	sess := &recordingSession{dim: 2, sleep: 20 * time.Millisecond}
	b := NewBatcher(sess, slowTok{delay: 15 * time.Millisecond}, 2, 16, false, "mean", 100000, 100, 1, 1)
	defer b.Close()

	out := make(chan Response, 3)
	start := time.Now()
	go embedAsync(b, []string{"a"}, out)
	go embedAsync(b, []string{"b"}, out)
	go embedAsync(b, []string{"c"}, out)
	for range 3 {
		r := <-out
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
	}
	wall := time.Since(start)

	if len(sess.calls) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(sess.calls))
	}
	if wall > 80*time.Millisecond {
		t.Fatalf("expected tokenization to overlap inference, wall=%v (fully serialized ~105ms)", wall)
	}
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
