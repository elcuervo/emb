package pipeline

import (
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

// gatedRunSession blocks in Run until release is closed, so a test can hold one
// worker busy while another is free.
type gatedRunSession struct {
	entered chan struct{}
	release chan struct{}
}

func (s *gatedRunSession) Run(_, _ []int64, batchSize, seqLen, dim int) ([]float32, error) {
	s.entered <- struct{}{}
	<-s.release
	return make([]float32, batchSize*seqLen*dim), nil
}

func (s *gatedRunSession) Close() error { return nil }

// TestUnbatchedPoolServesFreeWorker covers the inference-performance spec: a
// request runs on the free worker without waiting for a busy one. The pool's
// workers share one request channel, so a request is handed to whichever worker
// is ready rather than a fixed round-robin index.
func TestUnbatchedPoolServesFreeWorker(t *testing.T) {
	made := make([]*gatedRunSession, 0, 2)
	p, err := NewPool(func() (onnx.Session, error) {
		s := &gatedRunSession{entered: make(chan struct{}, 1), release: make(chan struct{})}
		made = append(made, s)
		return s, nil
	}, fakeTok{}, 2, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	firstDone := make(chan struct{})
	go func() {
		_, _ = p.Embed([]string{"aa"})
		close(firstDone)
	}()

	// Identify which worker took the first request.
	first := -1
	select {
	case <-made[0].entered:
		first = 0
	case <-made[1].entered:
		first = 1
	case <-time.After(2 * time.Second):
		t.Fatal("first request never started")
	}
	other := 1 - first

	// A second request must reach the free worker while the first is blocked.
	secondDone := make(chan struct{})
	go func() {
		_, _ = p.Embed([]string{"bb"})
		close(secondDone)
	}()
	select {
	case <-made[other].entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("second request did not reach the free worker (busy=%d)", first)
	}

	close(made[0].release)
	close(made[1].release)
	for name, done := range map[string]chan struct{}{"first": firstDone, "second": secondDone} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s request did not finish", name)
		}
	}
}
