package pipeline

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

type closeCountingSession struct {
	recordingSession
	closes   atomic.Int32
	closeErr error
}

func (s *closeCountingSession) Close() error {
	s.closes.Add(1)
	return s.closeErr
}

func TestNewPoolRollsBackSessionsAfterFactoryFailure(t *testing.T) {
	first := &closeCountingSession{}
	calls := 0
	_, err := NewPool(func() (onnx.Session, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("injected factory failure")
		}
		return first, nil
	}, fakeTok{}, 2, 2, 16, false, "mean", 0, 32, 0, 0)
	if err == nil {
		t.Fatal("expected factory failure")
	}
	if got := first.closes.Load(); got != 1 {
		t.Fatalf("first session closed %d times, want 1", got)
	}
}

func TestPoolCloseIsConcurrentSafeAndIdempotent(t *testing.T) {
	sessions := make([]*closeCountingSession, 2)
	next := 0
	p, err := NewPool(func() (onnx.Session, error) {
		s := &closeCountingSession{}
		sessions[next] = s
		next++
		return s, nil
	}, fakeTok{}, 2, 2, 16, false, "mean", 0, 32, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := p.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
		})
	}
	wg.Wait()
	if _, err := p.Embed([]string{"after close"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Embed after close error = %v, want ErrClosed", err)
	}
	for i, s := range sessions {
		if got := s.closes.Load(); got != 1 {
			t.Fatalf("session %d closed %d times, want 1", i, got)
		}
	}
}

func TestBatcherCloseDrainsActiveInference(t *testing.T) {
	sess := &gatedSession{
		recordingSession: recordingSession{dim: 2},
		runStarted:       make(chan struct{}, 1),
		runRelease:       make(chan struct{}, 1),
	}
	b := NewBatcher(sess, fakeTok{}, 2, 16, false, "mean", 1, 32, 0, 0)
	result := make(chan Response, 1)
	go embedAsync(b, []string{"active"}, result)
	waitOn(t, sess.runStarted)

	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before inference completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	sess.runRelease <- struct{}{}
	if resp := <-result; resp.Err != nil {
		t.Fatalf("accepted request failed: %v", resp.Err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if _, err := b.Embed([]string{"after close"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Embed after close error = %v, want ErrClosed", err)
	}
}

func TestBatcherCloseWaitsForTokenizerHandoff(t *testing.T) {
	sess := &closeCountingSession{recordingSession: recordingSession{dim: 2}}
	tok := &gatedTok{started: make(chan struct{}, 1), allowed: make(chan struct{}, 1)}
	b := NewBatcher(sess, tok, 2, 16, false, "mean", 1, 32, 0, 1)
	result := make(chan Response, 1)
	go embedAsync(b, []string{"active"}, result)
	waitOn(t, tok.started)

	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before tokenization completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	tok.allowed <- struct{}{}
	if resp := <-result; resp.Err != nil {
		t.Fatalf("accepted request failed: %v", resp.Err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if got := sess.closes.Load(); got != 1 {
		t.Fatalf("session closed %d times, want 1", got)
	}
}
