package registry

import (
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
	"github.com/elcuervo/emb/internal/permit"
)

// gatedNamedSession parks in RunNamed until release is closed, so a test can
// hold one session busy while another sits idle. entered signals that a run
// has started.
type gatedNamedSession struct {
	entered chan struct{}
	release chan struct{}
}

func newGatedNamedSession() *gatedNamedSession {
	return &gatedNamedSession{entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (g *gatedNamedSession) RunNamed([]onnx.NamedTensor) (map[string]onnx.NamedTensor, error) {
	g.entered <- struct{}{}
	<-g.release
	return nil, nil
}

func (g *gatedNamedSession) Close() error { return nil }

// idleScriptResources builds a ScriptResources over the given sessions with one
// permit per session (the controller grows the pool at runtime in production).
func idleScriptResources(sessions ...onnx.NamedSession) *ScriptResources {
	res := &ScriptResources{
		sessions: sessions,
		pool:     permit.New(sessions, len(sessions), len(sessions)),
		stop:     make(chan struct{}),
	}
	res.allowance.Store(1)
	res.class.Store(permit.ClassIdle)
	return res
}

// TestScriptIdleDispatchUsesFreeSession proves a call runs on the idle session
// while another session is held busy: under the old round-robin pick the
// second call could queue behind the first.
func TestScriptIdleDispatchUsesFreeSession(t *testing.T) {
	a := newGatedNamedSession()
	b := newGatedNamedSession()
	res := idleScriptResources(a, b)

	first := make(chan struct{})
	go func() {
		_, _ = res.RunNamed(nil)
		close(first)
	}()

	var busy *gatedNamedSession
	select {
	case <-a.entered:
		busy = a
	case <-b.entered:
		busy = b
	case <-time.After(2 * time.Second):
		t.Fatal("first call never started")
	}
	free := a
	if busy == a {
		free = b
	}

	second := make(chan struct{})
	go func() {
		_, _ = res.RunNamed(nil)
		close(second)
	}()

	select {
	case <-free.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("second call did not reach the idle session")
	}
	close(a.release)
	close(b.release)
	<-first
	<-second
}

// TestScriptDispatchCountersSerialAndContended checks the wait counter is near
// zero for serial traffic and grows when concurrency exceeds the session count.
func TestScriptDispatchCountersSerialAndContended(t *testing.T) {
	serial := idleScriptResources(&fakeNamedSession{})
	for i := 0; i < 5; i++ {
		if _, err := serial.RunNamed(nil); err != nil {
			t.Fatal(err)
		}
	}
	wait, _, runs, _ := serial.DispatchCounters()
	if runs != 5 {
		t.Fatalf("runs = %d, want 5", runs)
	}
	if wait > 1000 {
		t.Fatalf("serial dispatch wait = %dus, want near zero", wait)
	}

	gated := newGatedNamedSession()
	contended := idleScriptResources(gated)
	first := make(chan struct{})
	go func() {
		_, _ = contended.RunNamed(nil)
		close(first)
	}()
	<-gated.entered
	second := make(chan struct{})
	go func() {
		_, _ = contended.RunNamed(nil)
		close(second)
	}()
	time.Sleep(20 * time.Millisecond) // the second call waits for the only session
	close(gated.release)
	<-first
	<-second

	wait, _, runs, _ = contended.DispatchCounters()
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
	if wait < 5000 {
		t.Fatalf("contended dispatch wait = %dus, want >= 5ms", wait)
	}
}

// TestSamplerStops verifies the sampler exits promptly and stopSampler is
// idempotent, so Close never hangs or leaks the goroutine.
func TestSamplerStops(t *testing.T) {
	res := idleScriptResources(newGatedNamedSession())
	res.cap = 4
	res.allowance.Store(4)
	res.startSampler(2)

	done := make(chan struct{})
	go func() {
		res.stopSampler()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stopSampler did not return")
	}
	res.stopSampler() // idempotent
}

// TestScriptIntraOpThreadsDefaultDividesBudget covers the unset default: the
// cores−2 budget is split across the script sessions, and an explicit value is
// never changed.
func TestScriptIntraOpThreadsDefaultDividesBudget(t *testing.T) {
	if got := scriptIntraOpThreads(0, 4, 10); got != 2 {
		t.Fatalf("10 cores / 4 sessions = %d, want 2", got)
	}
	if got := scriptIntraOpThreads(0, 1, 10); got != 8 {
		t.Fatalf("single session = %d, want 8", got)
	}
	if got := scriptIntraOpThreads(8, 4, 10); got != 8 {
		t.Fatalf("explicit threads = %d, want 8", got)
	}
	if got := scriptIntraOpThreads(0, 4, 2); got != 1 {
		t.Fatalf("2 cores floors at 1, got %d", got)
	}
}

// TestOversubscribedThreadsWarns covers the boot warning decision: a model
// whose sessions × threads exceed the core count is named with its counts.
func TestOversubscribedThreadsWarns(t *testing.T) {
	e := &ModelEntry{Name: "gliner2", cfg: config.ModelConfig{
		Dim: 512, Pooling: "mean",
		ScriptWorkers: 4, IntraOpThreads: 8,
	}}
	overs := OversubscribedThreads([]*ModelEntry{e}, 10)
	if len(overs) == 0 {
		t.Fatal("expected an oversubscription warning")
	}
	found := false
	for _, o := range overs {
		if o.Model == "gliner2" && o.Path == "script" && o.Sessions == 4 && o.Threads == 8 {
			found = true
		}
	}
	if !found {
		t.Fatalf("warning must name gliner2 script 4x8, got %+v", overs)
	}

	within := &ModelEntry{Name: "minilm", cfg: config.ModelConfig{Dim: 384, Pooling: "mean"}}
	if got := OversubscribedThreads([]*ModelEntry{within}, 10); got != nil {
		t.Fatalf("within-budget config warned: %+v", got)
	}
}
