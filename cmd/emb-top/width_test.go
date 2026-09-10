package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

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
