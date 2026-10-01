package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestLatHistRingBounds(t *testing.T) {
	m := newTUI(nil, time.Second, 3)
	for i := 0; i < 5; i++ {
		m.pushLat(latSample{p50: int64(i), p95: int64(i), p99: int64(i)})
	}
	if len(m.latHist) != 3 {
		t.Fatalf("latHist len = %d, want 3", len(m.latHist))
	}
	if m.latHist[2].p50 != 4 {
		t.Fatalf("latHist kept wrong samples: %+v", m.latHist)
	}
}

func TestLatBand(t *testing.T) {
	// Empty history still returns a full-height panel with a legend.
	lines := latBand(nil, 20, 4)
	if len(lines) != 4 {
		t.Fatalf("empty band height = %d, want 4", len(lines))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "p95") {
		t.Fatalf("band legend missing")
	}

	// A spread sample shades between p50 and p99 and marks p95.
	hist := []latSample{
		{p50: 100, p95: 500, p99: 1000},
		{p50: 200, p95: 600, p99: 2000},
	}
	lines = latBand(hist, 20, 4)
	if len(lines) != 4 {
		t.Fatalf("band height = %d, want 4", len(lines))
	}
	body := strings.Join(lines[:3], "")
	if !strings.Contains(body, latBandCell) || !strings.Contains(body, latP95Cell) {
		t.Fatalf("band body missing shaded/p95 cells: %q", body)
	}

	// All-zero samples do not panic and still render at full height.
	if got := latBand([]latSample{{}}, 10, 3); len(got) != 3 {
		t.Fatalf("zero band height = %d, want 3", len(got))
	}
}

func TestLatBandViewAdvances(t *testing.T) {
	m := newSizedTUI(120, 40)
	for i := 0; i < 10; i++ {
		m.applyResult(fakePoll(int64(i*10), int64(i*100), 0, 0))
	}
	m.connected = true
	if len(m.latHist) == 0 {
		t.Fatalf("band history did not advance")
	}
	w, _ := streamDims(m.width, m.height)
	for _, line := range strings.Split(m.latBandView(), "\n") {
		if got := lipgloss.Width(line); got > w {
			t.Errorf("band line is %d cols, want <= %d: %q", got, w, line)
		}
	}
}