package pipeline

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// countingRunSession counts runs so a test can prove abandoned work never
// reached ONNX.
type countingRunSession struct {
	runs *atomic.Int64
	dim  int
}

func (s *countingRunSession) Run(_, _ []int64, batchSize, seqLen, dim int) ([]float32, error) {
	if s.runs != nil {
		s.runs.Add(1)
	}
	return make([]float32, batchSize*seqLen*dim), nil
}

func (s *countingRunSession) Close() error { return nil }

// TestPoolEmbedContextAbandonsTheWait covers the bounded-admission contract on
// the unbatched path: with the only worker busy, a request whose deadline
// expires returns to its caller instead of waiting for the worker to free.
func TestPoolEmbedContextAbandonsTheWait(t *testing.T) {
	sess := &gatedRunSession{entered: make(chan struct{}, 1), release: make(chan struct{})}
	p, err := NewPool(func() (onnx.Session, error) { return sess, nil }, fakeTok{}, 1, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	busy := make(chan struct{})
	go func() {
		_, _ = p.Embed([]string{"aa"})
		close(busy)
	}()
	waitOn(t, sess.entered) // the single worker is inside Run

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := p.EmbedContext(ctx, []string{"bb"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("returned only after %s", waited)
	}
	close(sess.release)
	<-busy
}

// TestBatcherEmbedContextAbandonsTheWait is the same contract for the batched
// path: the run slot is held, so the next request's deadline is what ends it.
func TestBatcherEmbedContextAbandonsTheWait(t *testing.T) {
	sess := &gatedRunSession{entered: make(chan struct{}, 1), release: make(chan struct{})}
	b := NewBatcher(sess, fakeTok{}, 2, 16, false, "mean", 1, 32, 0, 0)
	defer func() { _ = b.Close() }()

	go func() { _, _ = b.Embed([]string{"aa"}) }()
	waitOn(t, sess.entered)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := b.EmbedContext(ctx, []string{"bb"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	close(sess.release)
}

// TestAbandonedRequestNeverRuns covers the abandonment half of the contract:
// a request whose caller has gone away is not carried into inference.
func TestAbandonedRequestNeverRuns(t *testing.T) {
	var runs atomic.Int64
	sess := &countingRunSession{runs: &runs}
	p, err := NewPool(func() (onnx.Session, error) { return sess, nil }, fakeTok{}, 1, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.EmbedContext(ctx, []string{"aa"}); err == nil {
		t.Fatal("a cancelled request was served")
	}
	if got := runs.Load(); got != 0 {
		t.Fatalf("a cancelled request still reached inference: %d runs", got)
	}

	// The same for the batched path, where the request is dropped out of the
	// window before the run is built.
	var batchRuns atomic.Int64
	bsess := &countingRunSession{runs: &batchRuns}
	b := NewBatcher(bsess, fakeTok{}, 2, 16, false, "mean", 1, 32, 0, 0)
	defer func() { _ = b.Close() }()
	bctx, bcancel := context.WithCancel(context.Background())
	bcancel()
	if _, err := b.EmbedContext(bctx, []string{"aa"}); err == nil {
		t.Fatal("a cancelled request was served by the batcher")
	}
	time.Sleep(50 * time.Millisecond) // let a window close if one was armed
	if got := batchRuns.Load(); got != 0 {
		t.Fatalf("a cancelled batched request still reached inference: %d runs", got)
	}
}
