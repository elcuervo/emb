package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	// New model discovered after start appears in order and is polled next round.
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
	if !m.present["bge"] || m.known[len(m.known)-1] != "bge" {
		t.Fatalf("new model not tracked/polled: present=%v known=%v", m.present, m.known)
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

func TestModelRowsStayInServerOrder(t *testing.T) {
	m := newSizedTUI(120, 40)
	pollAB := func(aReqs, bReqs int64) *embtop.PollResult {
		res := fakePoll(aReqs+bReqs, aReqs+bReqs, 0, 0)
		res.Models = []embtop.ModelListEntry{
			{Name: "alpha", Dim: 1, Status: "ready"},
			{Name: "beta", Dim: 1, Status: "ready"},
		}
		res.PerModel = map[string]*embtop.ModelStats{
			"alpha": {Requests: aReqs, Tokens: aReqs, Pooling: "mean"},
			"beta":  {Requests: bReqs, Tokens: bReqs, Pooling: "mean"},
		}
		return res
	}
	m.applyResult(pollAB(0, 0))
	time.Sleep(time.Millisecond)
	m.applyResult(pollAB(1, 100)) // beta busiest: a rate sort would flip the rows
	m.connected = true
	view := m.View()
	ai, bi := strings.Index(view, "alpha"), strings.Index(view, "beta")
	if ai < 0 || bi < 0 || ai > bi {
		t.Fatalf("rows not in stable server order (alpha=%d beta=%d):\n%s", ai, bi, view)
	}
}

func TestModelRowsStayStableAcrossServerReorders(t *testing.T) {
	m := newSizedTUI(120, 40)
	poll := func(names ...string) *embtop.PollResult {
		res := fakePoll(10, 100, 0, 0)
		res.Models = nil
		res.PerModel = map[string]*embtop.ModelStats{}
		for _, n := range names {
			res.Models = append(res.Models, embtop.ModelListEntry{Name: n, Dim: 1, Status: "ready"})
			res.PerModel[n] = &embtop.ModelStats{Dim: 1, Requests: 10, Tokens: 100, Pooling: "mean"}
		}
		return res
	}
	m.applyResult(poll("minilm", "bge", "e5"))
	first := strings.Join(m.modelOrder, ",")
	// The server's EMB.MODELS order is unspecified; the client must not follow it.
	for _, order := range [][]string{
		{"e5", "minilm", "bge"},
		{"bge", "e5", "minilm"},
		{"minilm", "bge", "e5"},
	} {
		m.applyResult(poll(order...))
		if got := strings.Join(m.modelOrder, ","); got != first {
			t.Fatalf("rows moved after server reorder %v: %q, want %q", order, got, first)
		}
	}
	// A model discovered later appends; existing rows stay put.
	m.applyResult(poll("jina", "minilm", "bge", "e5"))
	if got, want := strings.Join(m.modelOrder, ","), first+",jina"; got != want {
		t.Fatalf("new model order = %q, want %q", got, want)
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

// ---- stable single model list ----

// multiPoll builds a PollResult for the named models, each with its own
// cumulative request counter so rates differ across rows.
func multiPoll(names []string, reqs map[string]int64) *embtop.PollResult {
	res := fakePoll(0, 0, 0, 0)
	res.Models = nil
	res.PerModel = map[string]*embtop.ModelStats{}
	var total int64
	for _, n := range names {
		r := reqs[n]
		total += r
		res.Models = append(res.Models, embtop.ModelListEntry{Name: n, Dim: 8, Status: "ready"})
		res.PerModel[n] = &embtop.ModelStats{
			Dim: 8, Requests: r, Tokens: r * 4, AvgLatencyUs: 900,
			Pooling: "mean", Quantization: "int8",
		}
	}
	res.TotalRequests = total
	res.TotalTokens = total * 4
	res.ModelsLoaded = len(names)
	return res
}

func reqMap(names []string) map[string]int64 {
	reqs := map[string]int64{}
	for i, n := range names {
		reqs[n] = int64(i+1) * 10
	}
	return reqs
}

func testModelNames() []string {
	return []string{"alpha", "bravo", "charlie", "delta", "echo",
		"foxtrot", "golf", "hotel", "india", "juliet"}
}

// rowIndexOf returns the line index of the first line mentioning name.
func rowIndexOf(lines []string, name string) int {
	for i, l := range lines {
		if strings.Contains(l, name) {
			return i
		}
	}
	return -1
}

// TestModelRowsAreSingleLine guards constant row height: a model whose
// metadata is present and one whose EMB.INFO reply is missing must each
// occupy exactly one line, so no row can shift the ones below it.
func TestModelRowsAreSingleLine(t *testing.T) {
	m := newSizedTUI(120, 30)
	res := multiPoll([]string{"alpha", "bravo"}, map[string]int64{"alpha": 10, "bravo": 5})
	delete(res.PerModel, "bravo") // no metadata for bravo this poll
	m.applyResult(res)
	lines := m.modelsView()
	if len(lines) != 3 { // two model rows + the range/legend line
		t.Fatalf("modelsView returned %d lines, want 3 (constant row height):\n%s",
			len(lines), strings.Join(lines, "\n"))
	}
}

// TestActivityStripAlwaysRendersWithinWidth guards that the in-row strip
// survives narrow terminals by dropping numeric segments, never wrapping.
func TestActivityStripAlwaysRendersWithinWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		m := newSizedTUI(w, 30)
		m.applyResult(multiPoll([]string{"alpha", "bravo"}, map[string]int64{"alpha": 100, "bravo": 1}))
		row := ""
		for _, l := range m.modelsView() {
			if strings.Contains(l, "alpha") {
				row = l
			}
		}
		if !strings.Contains(row, heatCellBlock) {
			t.Errorf("width %d: activity strip missing from the model row: %q", w, row)
		}
		if got := lipgloss.Width(row); got > w {
			t.Errorf("width %d: model row is %d cols: %q", w, got, row)
		}
	}
}

// TestViewFitsHeight guards the model area against the fixed chrome budget.
func TestViewFitsHeight(t *testing.T) {
	for _, h := range []int{24, 30, 40, 50} {
		m := newSizedTUI(120, h)
		m.applyResult(multiPoll(testModelNames(), reqMap(testModelNames())))
		lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
		if len(lines) > h {
			t.Errorf("height %d: view is %d lines", h, len(lines))
		}
	}
}

// TestReturningModelKeepsPosition guards that a model dropping out of
// EMB.MODELS and returning reappears where it was, not at the end.
func TestReturningModelKeepsPosition(t *testing.T) {
	m := newSizedTUI(120, 40)
	reqs := map[string]int64{"alpha": 1, "bravo": 1, "charlie": 1}
	m.applyResult(multiPoll([]string{"alpha", "bravo", "charlie"}, reqs))
	m.applyResult(multiPoll([]string{"alpha", "charlie"}, reqs)) // bravo drops out
	if got := strings.Join(m.presentNames(), ","); got != "alpha,charlie" {
		t.Fatalf("present after drop = %q, want alpha,charlie", got)
	}
	m.applyResult(multiPoll([]string{"alpha", "bravo", "charlie"}, reqs)) // bravo returns
	if got := strings.Join(m.presentNames(), ","); got != "alpha,bravo,charlie" {
		t.Fatalf("returning model position = %q, want alpha,bravo,charlie", got)
	}
}

// TestMissedPollKeepsRowPositionAndHeight guards that one missing EMB.INFO
// reply neither moves nor resizes any row.
func TestMissedPollKeepsRowPositionAndHeight(t *testing.T) {
	m := newSizedTUI(120, 30)
	names := []string{"alpha", "bravo", "charlie"}
	m.applyResult(multiPoll(names, reqMap(names)))
	before := m.modelsView()

	res := multiPoll(names, map[string]int64{"alpha": 20, "bravo": 20, "charlie": 30})
	delete(res.PerModel, "bravo") // EMB.INFO reply missed
	m.applyResult(res)
	after := m.modelsView()

	if len(before) != len(after) {
		t.Fatalf("row count changed after a missed poll: %d -> %d", len(before), len(after))
	}
	for _, name := range names {
		if rowIndexOf(before, name) != rowIndexOf(after, name) {
			t.Errorf("%s moved after a missed poll: %d -> %d", name, rowIndexOf(before, name), rowIndexOf(after, name))
		}
	}
	if !m.sampler.LatestModels["bravo"].Stale {
		t.Error("missed model not marked stale")
	}
}

// TestScrollClampsAndNamesVisibleRange guards the single scroll offset and the
// range indicator.
func TestScrollClampsAndNamesVisibleRange(t *testing.T) {
	m := newSizedTUI(120, 24) // visibleRows = height - modelChrome
	names := testModelNames()
	m.applyResult(multiPoll(names, reqMap(names)))

	wantMax := len(names) - m.visibleRows()
	for i := 0; i < 50; i++ {
		um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = um.(tuiModel)
	}
	if m.scroll != wantMax {
		t.Fatalf("scroll after j = %d, want %d", m.scroll, wantMax)
	}
	lines := m.modelsView()
	legend := lines[len(lines)-1]
	if want := fmt.Sprintf("rows %d–%d of %d", wantMax+1, len(names), len(names)); !strings.Contains(legend, want) {
		t.Errorf("legend %q does not name the visible range %q", legend, want)
	}
	for i := 0; i < 50; i++ {
		um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = um.(tuiModel)
	}
	if m.scroll != 0 {
		t.Fatalf("scroll after k = %d, want 0", m.scroll)
	}
}

// TestStableModelListReachesEveryModel guards that scrolling reaches every
// model, and that a model appears at exactly one position in a frame (visible)
// or nowhere (off-screen).
func TestStableModelListReachesEveryModel(t *testing.T) {
	m := newSizedTUI(120, 24)
	names := testModelNames()
	m.applyResult(multiPoll(names, reqMap(names)))

	vis := m.visibleRows()
	if vis >= len(names) {
		t.Fatalf("test needs more models than fit: vis=%d n=%d", vis, len(names))
	}
	seen := map[string]bool{}
	for s := 0; s <= len(names)-vis; s++ {
		m.scroll = s
		view := m.View()
		visible := map[string]bool{}
		for _, n := range m.presentNames()[s : s+vis] {
			visible[n] = true
			seen[n] = true
		}
		for _, n := range names {
			c := strings.Count(view, n)
			if visible[n] && c != 1 {
				t.Errorf("scroll %d: visible model %q appears %d times, want 1", s, n, c)
			}
			if !visible[n] && c != 0 {
				t.Errorf("scroll %d: off-screen model %q appears %d times, want 0", s, n, c)
			}
		}
	}
	if len(seen) != len(names) {
		t.Fatalf("scrolling reached %d of %d models: %v", len(seen), len(names), seen)
	}
}
