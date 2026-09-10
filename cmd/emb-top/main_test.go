package main

import (
	"strings"
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
