package registry

import (
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/onnx"
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

func idleScriptResources(sessions ...onnx.NamedSession) *ScriptResources {
	res := &ScriptResources{
		sessions: sessions,
		idle:     make(chan onnx.NamedSession, len(sessions)),
	}
	for _, s := range sessions {
		res.idle <- s
	}
	return res
}

// TestScriptIdleDispatchUsesFreeSession proves a call runs on the idle session
// while another session is held busy: under the old round-robin pick the
// second call would have queued behind the first.
func TestScriptIdleDispatchUsesFreeSession(t *testing.T) {
	busy := newGatedNamedSession()
	free := newGatedNamedSession()
	res := idleScriptResources(busy, free)

	first := make(chan struct{})
	go func() {
		_, _ = res.RunNamed(nil)
		close(first)
	}()
	<-busy.entered // first call is parked in the busy session

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
	close(free.release)
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Fatal("second call did not finish on the idle session")
	}
	close(busy.release)
	<-first
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

// TestIdleSessionsSeedsEachCallerSlot verifies each session enters the idle
// channel once per configured caller, so that many evaluations may share it.
func TestIdleSessionsSeedsEachCallerSlot(t *testing.T) {
	a := &fakeNamedSession{}
	b := &fakeNamedSession{}
	idle := idleSessions([]onnx.NamedSession{a, b}, 3)
	if got := len(idle); got != 6 {
		t.Fatalf("idle slots = %d, want 6", got)
	}
	counts := map[onnx.NamedSession]int{}
	for i := 0; i < 6; i++ {
		counts[<-idle]++
	}
	if counts[a] != 3 || counts[b] != 3 {
		t.Fatalf("session slots = %v, want 3 each", counts)
	}
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
