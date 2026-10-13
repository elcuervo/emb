package pipeline

import (
	"errors"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
)

// gateSession stands in for a gate-wrapped session: every Run "queues" for
// the whole 2ms it takes (or is refused), and reports that as admission wait.
type gateSession struct {
	err     error
	pending time.Duration
}

func (s *gateSession) Run(_, _ []int64, batchSize, seqLen, dim int) ([]float32, error) {
	start := time.Now()
	for time.Since(start) < 2*time.Millisecond {
	}
	s.pending += time.Since(start)
	if s.err != nil {
		return nil, s.err
	}
	return make([]float32, batchSize*seqLen*dim), nil
}

func (s *gateSession) Close() error { return nil }

func (s *gateSession) TakeAdmissionWait() time.Duration {
	d := s.pending
	s.pending = 0
	return d
}

// A request that only queued at the gate adds admission wait, not run time.
func TestGateWaitIsAdmissionNotRunTime(t *testing.T) {
	for name, timeoutMS := range map[string]int{"unbatched": 0, "batched": 1} {
		t.Run(name, func(t *testing.T) {
			p, err := NewPool(func() (onnx.Session, error) { return &gateSession{}, nil },
				fakeTok{}, 1, 2, 16, false, "mean", timeoutMS, 32, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			if resp, err := p.Embed([]string{"aa"}); err != nil || resp.Err != nil {
				t.Fatalf("Embed = %v / %v", err, resp.Err)
			}
			st := p.Stats()
			if st.AdmissionWaitUs < 2000 {
				t.Fatalf("AdmissionWaitUs = %d, want the 2ms queued at the gate", st.AdmissionWaitUs)
			}
			if st.RunUs >= st.AdmissionWaitUs {
				t.Fatalf("RunUs = %d includes the %dus gate wait", st.RunUs, st.AdmissionWaitUs)
			}
			if st.Runs != 1 {
				t.Fatalf("Runs = %d, want 1", st.Runs)
			}
		})
	}
}

// A refused admission is an error but never a native run.
func TestRefusedAdmissionIsNotARun(t *testing.T) {
	for name, timeoutMS := range map[string]int{"unbatched": 0, "batched": 1} {
		t.Run(name, func(t *testing.T) {
			p, err := NewPool(func() (onnx.Session, error) {
				return &gateSession{err: permit.ErrAdmissionTimeout}, nil
			}, fakeTok{}, 1, 2, 16, false, "mean", timeoutMS, 32, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			resp, err := p.Embed([]string{"aa"})
			if err != nil || !errors.Is(resp.Err, permit.ErrAdmissionTimeout) {
				t.Fatalf("Embed = %v / %v, want the admission timeout in the response", err, resp.Err)
			}
			if st := p.Stats(); st.Runs != 0 || st.Errors != 1 {
				t.Fatalf("Runs = %d Errors = %d, want 0 runs and 1 error", st.Runs, st.Errors)
			}
		})
	}
}
