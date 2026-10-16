package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

// latSample is one poll's latency percentiles (µs); zero when no events.
type latSample struct {
	p50, p95, p99 int64
}

// pushLat appends a percentile sample and bounds the ring to the window.
func (m *tuiModel) pushLat(s latSample) {
	m.latHist = append(m.latHist, s)
	if len(m.latHist) > m.window {
		m.latHist = m.latHist[len(m.latHist)-m.window:]
	}
}

// observeP95 folds a p95 sample into the slow session baseline the health
// verdict compares against, so latency health is relative, not absolute.
func (m *tuiModel) observeP95(us int64) {
	if us <= 0 {
		return
	}
	if m.p95Base == 0 {
		m.p95Base = us
		return
	}
	m.p95Base = (m.p95Base*9 + us) / 10
}

// latBandCell / latP95Cell are the shaded-band and p95-line runes.
const (
	latBandCell = "░"
	latP95Cell  = "█"
)

// latBand renders the p50–p99 latency band over the sampled window: each
// column is a poll, cells between p50 and p99 are shaded, and p95 is a bright
// rune. It always returns h lines (blank grid plus a legend) so the panel
// keeps its height when there is no data yet.
func latBand(hist []latSample, w, h int, tail string) []string {
	body := h - 1
	withLegend := true
	if body < 1 {
		body, withLegend = 1, false
	}
	grid := make([][]string, body)
	for y := range grid {
		grid[y] = make([]string, w)
		for x := range grid[y] {
			grid[y][x] = " "
		}
	}
	if len(hist) > 0 && w > 0 {
		maxV := 0.0
		for _, s := range hist {
			if float64(s.p99) > maxV {
				maxV = float64(s.p99)
			}
		}
		if maxV <= 0 {
			maxV = 1
		}
		yOf := func(us int64) int {
			if us <= 0 {
				return body - 1
			}
			y := body - 1 - int(float64(us)/maxV*float64(body-1))
			if y < 0 {
				y = 0
			}
			if y > body-1 {
				y = body - 1
			}
			return y
		}
		for x := 0; x < w; x++ {
			idx := x * len(hist) / w
			if idx >= len(hist) {
				idx = len(hist) - 1
			}
			s := hist[idx]
			lo, hi := yOf(s.p99), yOf(s.p50)
			if lo > hi {
				lo, hi = hi, lo
			}
			for y := lo; y <= hi; y++ {
				grid[y][x] = latBandCell
			}
			grid[yOf(s.p95)][x] = latP95Cell
		}
	}
	lines := make([]string, 0, h)
	for _, row := range grid {
		var b strings.Builder
		for _, c := range row {
			switch c {
			case latP95Cell:
				b.WriteString(latP95Style.Render(c))
			case latBandCell:
				b.WriteString(latBandStyle.Render(c))
			default:
				b.WriteString(c)
			}
		}
		lines = append(lines, b.String())
	}
	if withLegend {
		legend := dimStyle.Render("p50–p99 ") + latBandStyle.Render(latBandCell) +
			dimStyle.Render("  p95 ") + latP95Style.Render(latP95Cell)
		if tail != "" {
			// The band's own p95 describes every request, cache answers
			// included; the tail names what the requests that ran did.
			legend += dimStyle.Render("  " + tail)
		}
		if w > 0 {
			legend = lipgloss.NewStyle().MaxWidth(w).Render(legend)
		}
		lines = append(lines, legend)
	}
	return lines
}

// latBandView renders the band panel's content; the caller wraps it in the
// shared panel border. The legend carries the uncached tail and the cached
// share, so a window that mixes microsecond cache answers with second-long runs
// says which population the band's p95 came from.
func (m tuiModel) latBandView() string {
	w, h := streamDims(m.width, m.height)
	tail := ""
	counts := m.sampler.PathCounts()
	if _, _, p95, n, ok := m.sampler.UncachedLatency(); ok && counts.Reran > 0 {
		tail = fmt.Sprintf("runs p95 %s · n=%d", embtop.FormatLatency(p95), n)
	} else if counts.Attributed > 0 && counts.Cached == counts.Total {
		tail = fmt.Sprintf("all %d served from cache", counts.Total)
	}
	return strings.Join(latBand(m.latHist, w, h, tail), "\n")
}
