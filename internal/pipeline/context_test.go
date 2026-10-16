package pipeline

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
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
//
// A cancelled caller is answered on whichever path it takes: the dispatch
// select losing to ctx.Done() (an error), or the worker receiving the request
// and dropping it on its own context check (the response carries the error and
// the call returns nil — the shape the server checks). Both are "reported
// cancelled, never run"; asserting on one of them alone is a coin flip, since
// an idle worker parks in its receive and leaves both select cases ready.
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
	resp, err := p.EmbedContext(ctx, []string{"aa"})
	if err == nil {
		err = resp.Err
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled request was served: err=%v, resp.Err=%v", err, resp.Err)
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
	bresp, err := b.EmbedContext(bctx, []string{"aa"})
	if err == nil {
		err = bresp.Err
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled request was served by the batcher: err=%v, resp.Err=%v", err, bresp.Err)
	}
	time.Sleep(50 * time.Millisecond) // let a window close if one was armed
	if got := batchRuns.Load(); got != 0 {
		t.Fatalf("a cancelled batched request still reached inference: %d runs", got)
	}
}

// admittingSession stands in for the CPU-gate wrapper: the wait it reports
// travels back with the run's own result, and it optionally fails the run with
// err so a refused admission can be exercised.
type admittingSession struct {
	wait time.Duration
	err  error
}

func (s *admittingSession) Run(_, _ []int64, batchSize, seqLen, dim int) ([]float32, error) {
	return s.result(batchSize, seqLen, dim)
}

func (s *admittingSession) RunContext(_ context.Context, _, _ []int64, batchSize, seqLen, dim int) ([]float32, error) {
	return s.result(batchSize, seqLen, dim)
}

func (s *admittingSession) RunContextAdmitted(_ context.Context, _, _ []int64, batchSize, seqLen, dim int) ([]float32, time.Duration, error) {
	out, err := s.result(batchSize, seqLen, dim)
	return out, s.wait, err
}

func (s *admittingSession) result(batchSize, seqLen, dim int) ([]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return make([]float32, batchSize*seqLen*dim), nil
}

func (s *admittingSession) Close() error { return nil }

// TestRunTimeExcludesAdmissionWait covers the attribution contract on both
// inference paths: the time a run spent queueing at the CPU gate is reported on
// its own, and the reported run time is not inflated by it.
func TestRunTimeExcludesAdmissionWait(t *testing.T) {
	sess := &admittingSession{wait: 40 * time.Millisecond}
	p, err := NewPool(func() (onnx.Session, error) { return sess, nil }, fakeTok{}, 1, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Embed([]string{"aa"}); err != nil {
		t.Fatal(err)
	}
	st := p.Stats()
	if st.AdmissionWaitUs < 40_000 {
		t.Fatalf("admission wait not reported: %d us", st.AdmissionWaitUs)
	}
	if st.RunUs >= 40_000 {
		t.Fatalf("run time still includes the admission wait: %d us", st.RunUs)
	}
	if st.Runs != 1 {
		t.Fatalf("runs = %d, want 1", st.Runs)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	bsess := &admittingSession{wait: 40 * time.Millisecond}
	bp, err := NewPool(func() (onnx.Session, error) { return bsess, nil }, fakeTok{}, 1, 2, 16, false, "mean", 1, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bp.Embed([]string{"aa"}); err != nil {
		t.Fatal(err)
	}
	bst := bp.Stats()
	if bst.AdmissionWaitUs < 40_000 {
		t.Fatalf("batched admission wait not reported: %d us", bst.AdmissionWaitUs)
	}
	if bst.RunUs >= 40_000 {
		t.Fatalf("batched run time still includes the admission wait: %d us", bst.RunUs)
	}
	_ = bp.Close()
}

// TestAdmissionRefusalIsNotARun covers the other half: work the gate refused
// never ran, so it is an error and not a run.
func TestAdmissionRefusalIsNotARun(t *testing.T) {
	sess := &admittingSession{err: permit.ErrBusy}
	p, err := NewPool(func() (onnx.Session, error) { return sess, nil }, fakeTok{}, 1, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	resp, err := p.Embed([]string{"aa"})
	if err != nil {
		t.Fatalf("pool embed: %v", err)
	}
	if !errors.Is(resp.Err, permit.ErrBusy) {
		t.Fatalf("want the gate's refusal, got %v", resp.Err)
	}
	st := p.Stats()
	if st.Runs != 0 {
		t.Fatalf("a refused run was counted: %d runs", st.Runs)
	}
	if st.Errors != 1 {
		t.Fatalf("the refusal was not counted as an error: %d", st.Errors)
	}

	bsess := &admittingSession{err: permit.ErrBusy}
	bp, err := NewPool(func() (onnx.Session, error) { return bsess, nil }, fakeTok{}, 1, 2, 16, false, "mean", 1, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bp.Close() }()
	bresp, err := bp.Embed([]string{"aa"})
	if err != nil {
		t.Fatalf("batched embed: %v", err)
	}
	if !errors.Is(bresp.Err, permit.ErrBusy) {
		t.Fatalf("want the gate's refusal from the batcher, got %v", bresp.Err)
	}
	if bst := bp.Stats(); bst.Runs != 0 {
		t.Fatalf("a refused batch was counted as a run: %d", bst.Runs)
	}
}
