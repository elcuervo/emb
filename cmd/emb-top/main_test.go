package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elcuervo/emb/internal/embtop"
)

// fakePoll builds a PollResult with one model and given cumulative counters.
func fakePoll(reqs, toks, errs int64, active int64) *embtop.PollResult {
	return &embtop.PollResult{
		UptimeSecs:     42,
		TotalRequests:  reqs,
		TotalTokens:    toks,
		TotalErrors:    errs,
		ActiveRequests: active,
		Connections:    3,
		MemMB:          512,
		CPUUserUsec:    reqs * 1000,
		CPUSysUsec:     reqs * 200,
		Goroutines:     14,
		CacheHits:      reqs,
		CacheMisses:    1,
		CacheEvictions: 0,
		ModelsLoaded:   1,
		Models:         []embtop.ModelListEntry{{Name: "minilm", Dim: 384, Status: "ready"}},
		PerModel: map[string]*embtop.ModelStats{
			"minilm": {
				Dim: 384, Requests: reqs, Tokens: toks, Errors: errs,
				AvgLatencyUs: 450, Pooling: "mean", Normalize: true,
				Quantization: "int8", CacheHits: reqs, CacheMisses: 1,
			},
		},
	}
}

func newSizedTUI(w, h int) tuiModel {
	m := newTUI(embtop.NewClient("localhost:6379", "", false), time.Second, 120)
	// Simulate the initial window size message.
	um, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return um.(tuiModel)
}

func TestViewRendersWithTraffic(t *testing.T) {
	m := newSizedTUI(100, 30)
	for i := 1; i <= 30; i++ {
		m.applyResult(fakePoll(int64(i*10), int64(i*400), 0, int64(i%3)))
		time.Sleep(time.Millisecond) // let the fake clock drift so dt > 0
	}
	// Seed sampler prev so rates are computed on the next push.
	m.paused = false

	view := m.View()
	for _, want := range []string{
		"emb-top", "localhost:6379", "minilm", "r/s", "t/s", "avg", "err",
		"cache", "cpu", "mem", "conns", "active", "q quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestViewIdleZeroesNoCrash(t *testing.T) {
	// All-zero counters: the chart autoscale guard must not panic and the
	// view must still render.
	m := newSizedTUI(80, 24)
	for i := 0; i < 10; i++ {
		m.applyResult(fakePoll(0, 0, 0, 0))
	}
	if view := m.View(); !strings.Contains(view, "minilm") {
		t.Errorf("idle view missing model:\n%s", view)
	}
	if view := m.View(); !strings.Contains(view, "emb-top") {
		t.Errorf("idle view missing header:\n%s", view)
	}
}

func TestViewErrorsHighlighted(t *testing.T) {
	m := newSizedTUI(100, 30)
	for i := 1; i <= 3; i++ {
		m.applyResult(fakePoll(int64(i*10), int64(i*400), int64(i), 0))
		time.Sleep(time.Millisecond)
	}
	view := m.View()
	if !strings.Contains(view, "err") {
		t.Errorf("errors not rendered:\n%s", view)
	}
}

func TestHelpAndPause(t *testing.T) {
	m := newSizedTUI(100, 30)
	// Toggle help.
	um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = um.(tuiModel)
	if !m.showHelp || !strings.Contains(m.View(), "emb-top — live emb node dashboard") {
		t.Errorf("help not shown")
	}
	um, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = um.(tuiModel)
	if m.showHelp {
		t.Errorf("help should have toggled off")
	}
	// Pause.
	um, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = um.(tuiModel)
	if !m.paused {
		t.Errorf("expected paused")
	}
	if !strings.Contains(m.View(), "paused") {
		t.Errorf("paused state not shown in view")
	}
}

func TestModelAppearingMidRun(t *testing.T) {
	// New model discovered after start gets a sparkline and appears in order.
	m := newSizedTUI(100, 30)
	m.applyResult(fakePoll(10, 400, 0, 0))
	if len(m.modelOrder) != 1 || m.modelOrder[0] != "minilm" {
		t.Fatalf("unexpected modelOrder: %v", m.modelOrder)
	}
	second := fakePoll(20, 800, 0, 0)
	second.Models = append(second.Models, embtop.ModelListEntry{Name: "bge", Dim: 1024, Status: "ready"})
	second.PerModel["bge"] = &embtop.ModelStats{Dim: 1024, Requests: 0, Tokens: 0, Errors: 0, Pooling: "mean", Quantization: "fp32"}
	m.applyResult(second)
	if len(m.modelOrder) != 2 {
		t.Fatalf("expected 2 models, got %v", m.modelOrder)
	}
	if _, ok := m.sparks["bge"]; !ok {
		t.Fatalf("no sparkline created for new model")
	}
}

func TestResetClearsCharts(t *testing.T) {
	m := newSizedTUI(100, 30)
	for i := 1; i <= 5; i++ {
		m.applyResult(fakePoll(int64(i*10), int64(i*400), 0, 0))
	}
	m.applyResult(fakePoll(50, 2000, 0, 0))
	m.reset()
	p, _, _ := m.sampler.Snapshot()
	if len(m.sampler.Window) != 0 || p.ReqRate != 0 {
		t.Fatalf("reset did not clear sampler (window=%d)", len(m.sampler.Window))
	}
}

func TestEmitFrameIsOneJSONLine(t *testing.T) {
	var buf bytes.Buffer
	frame := "line one\x1b[31m red\x1b[0m\nline two"
	if err := emitFrame(&buf, frame); err != nil {
		t.Fatalf("emitFrame: %v", err)
	}
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Fatalf("want one newline (one line per frame), got %d: %q", got, buf.String())
	}
	var msg frameMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &msg); err != nil {
		t.Fatalf("frame is not valid JSON: %v", err)
	}
	if msg.ANSI != frame {
		t.Fatalf("frame round-trip mismatch:\n got %q\nwant %q", msg.ANSI, frame)
	}
}

// fakePoller drives runFrames without a node: it dials on calls 1 and 3,
// loses the connection on poll 2, and returns a model on every other poll.
type fakePoller struct {
	mu    sync.Mutex
	calls int
	seqs  []uint64
}

func (f *fakePoller) Addr() string { return "fake:6379" }
func (f *fakePoller) Close() error { return nil }

func (f *fakePoller) EnsureConn() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// A dial on the first poll and again after the drop on the second.
	return f.calls == 0 || f.calls == 2, nil
}

func (f *fakePoller) Poll(_ []string, afterSeq uint64) (*embtop.PollResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seqs = append(f.seqs, afterSeq)
	n := f.calls
	f.calls++
	if n == 1 {
		return nil, errors.New("connection lost")
	}
	res := fakePoll(int64(100*(n+1)), int64(400*(n+1)), 0, 1)
	res.NextSeq = uint64(10 * (n + 1))
	return res, nil
}

func (f *fakePoller) observedSeqs() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64(nil), f.seqs...)
}

// cancelWriter cancels the run after max writes and keeps every frame.
type cancelWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	n, max int
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	w.n++
	if w.n >= w.max {
		w.cancel()
	}
	return len(p), nil
}

func (w *cancelWriter) frames(t *testing.T) []string {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(w.buf.String()), "\n") {
		var msg frameMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("frame %q is not valid JSON: %v", line, err)
		}
		out = append(out, msg.ANSI)
	}
	return out
}

func TestRunFramesEmitsColoredFramesAndReconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{max: 4, cancel: cancel}
	poll := &fakePoller{}

	done := make(chan error, 1)
	go func() { done <- runFrames(ctx, poll, time.Millisecond, 300, w) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runFrames: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runFrames did not stop after cancellation")
	}

	frames := w.frames(t)
	if len(frames) < 4 {
		t.Fatalf("want at least 4 frames, got %d", len(frames))
	}
	frames = frames[:4] // the run may emit one more before observing the cancel
	for i, f := range frames {
		if !strings.Contains(f, "\x1b[") {
			t.Errorf("frame %d carries no colour: %q", i, f)
		}
		if !strings.Contains(f, "emb-top") {
			t.Errorf("frame %d is not the dashboard: %q", i, f)
		}
	}
	if !strings.Contains(frames[1], "reconnecting") {
		t.Errorf("frame after the drop does not report reconnecting: %q", frames[1])
	}

	seqs := poll.observedSeqs()
	if len(seqs) < 4 {
		t.Fatalf("want at least 4 polls, got %d", len(seqs))
	}
	if seqs[2] != 0 {
		t.Errorf("event cursor was not reset after reconnect: seqs=%v", seqs)
	}
}
