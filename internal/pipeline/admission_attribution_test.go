package pipeline

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// admissionUnit is the wait one unit of padded sequence length reports at the
// fake gate below. The unit is large enough that a wait reaching the wrong
// caller is unmistakable, and the fake never consults the clock: attribution is
// asserted exactly, not against a wall-clock bound.
const admissionUnit = 10 * time.Microsecond

// sharedGateSession stands in for a gate-wrapped session that several callers
// share: the pool's workers are those callers, exactly as a scripted session is
// leased to script_callers_per_session evaluations. The wait it reports is
// derived from the run it served (the run's padded sequence length), so a wait
// attributed to the wrong caller is visible.
//
// It also reproduces the pre-fix attribution shape — one shared slot holding the
// wait of whichever run stored last, read back after the request was served — so
// the invariant below can be checked against the tree before the fix: there each
// caller is credited with a single run's wait instead of its own texts' sum.
type sharedGateSession struct {
	slot atomic.Int64
}

func (s *sharedGateSession) result(batchSize, seqLen, dim int) []float32 {
	return make([]float32, batchSize*seqLen*dim)
}

func (s *sharedGateSession) waitFor(seqLen int) time.Duration {
	return time.Duration(seqLen) * admissionUnit
}

func (s *sharedGateSession) Run(ids, mask []int64, batchSize, seqLen, dim int) ([]float32, error) {
	return s.result(batchSize, seqLen, dim), nil
}

func (s *sharedGateSession) RunContext(_ context.Context, _, _ []int64, batchSize, seqLen, dim int) ([]float32, error) {
	s.slot.Store(int64(s.waitFor(seqLen)))
	return s.result(batchSize, seqLen, dim), nil
}

func (s *sharedGateSession) RunContextAdmitted(_ context.Context, _, _ []int64, batchSize, seqLen, dim int) ([]float32, time.Duration, error) {
	return s.result(batchSize, seqLen, dim), s.waitFor(seqLen), nil
}

// TakeAdmissionWait is the pre-fix read path: the last run's wait, shared by
// every caller of the session.
func (s *sharedGateSession) TakeAdmissionWait() time.Duration {
	return time.Duration(s.slot.Swap(0)) * time.Microsecond
}

func (s *sharedGateSession) Close() error { return nil }

// waitForTexts is the wait a request's own texts must be credited with: the sum
// of the waits its runs paid, one run per text.
func waitForTexts(texts []string) time.Duration {
	var total time.Duration
	for _, text := range texts {
		total += time.Duration(len(text)) * admissionUnit
	}
	return total
}

// newAttributionPool builds an unbatched pool whose workers all share one
// session, so concurrent callers contend for the same watch face.
func newAttributionPool(t *testing.T, sess *sharedGateSession, workers int) *Pool {
	t.Helper()
	p, err := NewPool(func() (onnx.Session, error) { return sess, nil }, fakeTok{}, workers, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestAdmissionWaitIsAttributedPerRun covers the attribution contract on the
// shared-session path: the wait a run paid at the gate travels with that run's
// own result, so a multi-text request accounts for every text it ran rather
// than only the last, and a later run never reads a wait it did not pay.
func TestAdmissionWaitIsAttributedPerRun(t *testing.T) {
	sess := &sharedGateSession{}
	p := newAttributionPool(t, sess, 1)

	texts := []string{"aaa", "bbbbb", "ccccccc"}
	resp, err := p.Embed(texts)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if want := waitForTexts(texts); resp.AdmissionWait != want {
		t.Fatalf("request admission wait = %s, want %s (every text's claim, not the last one's)", resp.AdmissionWait, want)
	}
	if got, want := p.Stats().AdmissionWaitUs, wantForTextsUs(texts); got != want {
		t.Fatalf("admission wait total = %d us, want %d us", got, want)
	}

	// The wait was claimed by the run that paid it: the next request reports its
	// own texts' wait, not a value left behind on the session.
	next := []string{"dd"}
	resp, err = p.Embed(next)
	if err != nil {
		t.Fatalf("second embed: %v", err)
	}
	if want := waitForTexts(next); resp.AdmissionWait != want {
		t.Fatalf("second request admission wait = %s, want %s", resp.AdmissionWait, want)
	}
}

// TestConcurrentCallersKeepTheirOwnAdmissionWait drives concurrent callers onto
// one shared session and asserts the invariant the attribution depends on: each
// caller is credited with its own texts' wait — never a value another caller's
// run stored — and the per-model counters never run backwards.
func TestConcurrentCallersKeepTheirOwnAdmissionWait(t *testing.T) {
	// Each caller's texts are long enough that the sum of its waits cannot
	// coincide with any single run's wait, so a truncated or cross-read value is
	// always a mismatch.
	requests := [][]string{
		{"aaaaaaaaaa", "bbbbbbbbbbb", "cccccccccccc"},
		{"dddddddddddd", "eeeeeeeeeeeee", "ffffffffffffff"},
		{"gggggggggggggg", "hhhhhhhhhhhhhhh", "iiiiiiiiiiiiiiii"},
	}
	sess := &sharedGateSession{}
	p := newAttributionPool(t, sess, len(requests))

	// A reader that samples the counters while the callers run: a value that
	// decreased between two reads would mean a wait was subtracted from a run
	// that did not pay it.
	stop := make(chan struct{})
	backwards := make(chan string, 1)
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		var prev Stats
		for {
			st := p.Stats()
			if st.AdmissionWaitUs < prev.AdmissionWaitUs || st.RunUs < prev.RunUs || st.Runs < prev.Runs {
				select {
				case backwards <- fmt.Sprintf("counters went backwards: admission %d->%d, run %d->%d, runs %d->%d",
					prev.AdmissionWaitUs, st.AdmissionWaitUs, prev.RunUs, st.RunUs, prev.Runs, st.Runs):
				default:
				}
				return
			}
			prev = st
			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	var wg sync.WaitGroup
	waits := make([]time.Duration, len(requests))
	errs := make([]error, len(requests))
	for i, texts := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := p.Embed(texts)
			waits[i], errs[i] = resp.AdmissionWait, err
		}()
	}
	wg.Wait()
	close(stop)
	watcher.Wait()

	select {
	case msg := <-backwards:
		t.Fatal(msg)
	default:
	}
	var total int64
	for i, texts := range requests {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if want := waitForTexts(texts); waits[i] != want {
			t.Fatalf("caller %d admission wait = %s, want %s (another caller's run was credited)", i, waits[i], want)
		}
		total += wantForTextsUs(texts)
	}
	// Nothing was lost and nothing was counted twice.
	if got := p.Stats().AdmissionWaitUs; got != total {
		t.Fatalf("admission wait total = %d us, want %d us (the callers' own waits)", got, total)
	}
}

func wantForTextsUs(texts []string) int64 {
	return waitForTexts(texts).Microseconds()
}
