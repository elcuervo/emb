package main

import (
	"strings"
	"testing"
	"time"
)

func TestHeatmapGridRendered(t *testing.T) {
	m := newSizedTUI(120, 30)
	for i := 1; i <= 10; i++ {
		m.applyResult(fakePoll(int64(i)*10, int64(i)*300, 0, 1))
		m.lastGood = time.Now()
		time.Sleep(time.Millisecond)
	}
	m.connected = true
	view := m.View()
	// Heatmap legend + labeled block rows render (colors are applied by
	// lipgloss only in a real terminal; structure is what we assert here).
	if !strings.Contains(view, "req/s · models × recent polls") {
		t.Fatalf("missing heatmap title")
	}
	if !strings.Contains(view, "█") {
		t.Fatalf("missing heatmap block cells:\n%s", view)
	}
	if !strings.Contains(view, "minilm") {
		t.Fatalf("missing heatmap row label")
	}
}
