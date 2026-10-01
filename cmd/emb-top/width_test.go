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
