package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TestModelColumnsDoNotShift guards the fixed-width per-model columns: a
// value growing (9.9 -> 900.0k) must not move the latency/error columns.
func TestModelColumnsDoNotShift(t *testing.T) {
	m := newSizedTUI(140, 40)
	marker := func() int {
		for _, line := range m.modelsView() {
			if i := strings.Index(line, "avg"); i >= 0 {
				return lipgloss.Width(line[:i])
			}
		}
		return -1
	}
	m.applyResult(fakePoll(0, 0, 0, 0))
	time.Sleep(time.Millisecond)
	m.applyResult(fakePoll(5, 100, 0, 0))
	a := marker()
	time.Sleep(time.Millisecond)
	m.applyResult(fakePoll(900000, 9000000, 0, 0))
	b := marker()
	if a < 0 || b < 0 {
		t.Fatalf("latency column not found: %d %d", a, b)
	}
	if a != b {
		t.Fatalf("latency column shifted: %d -> %d", a, b)
	}
}

// TestGaugesAlignToModelRowGrid guards the shared column grid: each gauge bar
// starts at the same column as the model-row zone it sits under.
func TestGaugesAlignToModelRowGrid(t *testing.T) {
	col := func(line, tok string) int {
		i := strings.Index(line, tok)
		if i < 0 {
			return -1
		}
		return lipgloss.Width(line[:i])
	}
	for _, w := range []int{100, 120, 160} {
		m := newSizedTUI(w, 40)
		m.applyResult(multiPoll([]string{"alpha", "bravo"}, reqMap([]string{"alpha", "bravo"})))
		idW, numW, _ := newRowLayout(w).gaugeBands()
		captionLine := strings.Split(m.gaugesView()[0], "\n")[0]
		for tok, want := range map[string]int{
			"cache": 0,
			"cpu":   idW + 1,
			"mem":   idW + numW + 3,
		} {
			if got := col(captionLine, tok); got != want {
				t.Errorf("width %d: gauge %q starts at column %d, want %d:\n%s", w, tok, got, want, captionLine)
			}
		}
	}
}

// TestStripStartsOnTheGridWithoutEvents guards that a row falling back to its
// average latency keeps its strip on the grid's strip column instead of
// reclaiming the reserved latency slot.
func TestStripStartsOnTheGridWithoutEvents(t *testing.T) {
	for _, w := range []int{100, 120, 160} {
		m := newSizedTUI(w, 40)
		m.applyResult(multiPoll([]string{"alpha"}, reqMap([]string{"alpha"})))
		l := newRowLayout(w)
		row := ""
		for _, line := range m.modelsView() {
			if strings.Contains(line, "alpha") {
				row = line
			}
		}
		i := strings.Index(row, heatCellBlock)
		if i < 0 {
			t.Fatalf("width %d: no strip in row %q", w, row)
		}
		if got, want := lipgloss.Width(row[:i]), l.idW+l.numW+2; got != want {
			t.Errorf("width %d: strip starts at column %d, want %d:\n%s", w, got, want, row)
		}
	}
}

// TestGaugeValueColumnStaysPut guards the gauge caption: the number and its
// unit keep their column as the rate grows.
func TestGaugeValueColumnStaysPut(t *testing.T) {
	col := func(s string) int {
		i := strings.Index(s, "%")
		if i < 0 {
			return -1
		}
		return lipgloss.Width(s[:i])
	}
	a := caption("cache", "bar", 0.0, "%", 32)
	b := caption("cache", "bar", 63.9, "%", 32)
	if col(a) < 0 || col(a) != col(b) {
		t.Errorf("gauge value column moved: %d -> %d\n%q\n%q", col(a), col(b), a, b)
	}
}

// TestViewFitsWidth guards against rows/panels overflowing the terminal width
// (which wraps mid-row and corrupts the layout).
func TestViewFitsWidth(t *testing.T) {
	for _, w := range []int{100, 120, 160} {
		m := newSizedTUI(w, 40)
		for i := 0; i < 20; i++ {
			m.applyResult(fakePoll(int64(i*10), int64(i*300), int64(i%3), 1))
			m.lastGood = time.Now()
		}
		m.connected = true
		for _, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d: line is %d cols wide:\n%s", w, got, line)
			}
		}
	}
}
