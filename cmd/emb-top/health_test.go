package main

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

func TestHealthVerdicts(t *testing.T) {
	cases := []struct {
		name string
		in   healthInput
		want healthStatus
		chip string
	}{
		{"no data", healthInput{connected: true, polls: 1}, healthNoData, ""},
		{"healthy", healthInput{connected: true, polls: 5}, healthHealthy, "err   0.0%"},
		{"errors degrade", healthInput{connected: true, polls: 5, errRatio: 0.02}, healthDegraded, "err   2.0%"},
		{"errors critical", healthInput{connected: true, polls: 5, errRatio: 0.10}, healthCritical, "err  10.0%"},
		{"cpu critical", healthInput{connected: true, polls: 5, cpuPct: 97}, healthCritical, "cpu  97%"},
		{"latency ok", healthInput{connected: true, polls: 5, p95Us: 1500, baselineUs: 1000}, healthHealthy, "p95 1.5ms  "},
		{"latency degrade", healthInput{connected: true, polls: 5, p95Us: 3000, baselineUs: 1000}, healthDegraded, "p95 3.0ms  "},
		{"latency critical", healthInput{connected: true, polls: 5, p95Us: 5000, baselineUs: 1000}, healthCritical, "p95 5.0ms  "},
		{"cache degrade", healthInput{connected: true, polls: 5, hasCache: true, cachePct: 25}, healthDegraded, "cache  25%"},
		{"disconnected", healthInput{connected: false, polls: 5}, healthCritical, "reconnecting"},
	}
	for _, tc := range cases {
		got, sigs := health(tc.in)
		if got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.name, got, tc.want)
		}
		if tc.chip == "" {
			continue
		}
		found := false
		for _, s := range sigs {
			if s.text == tc.chip {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: missing chip %q in %+v", tc.name, tc.chip, sigs)
		}
	}
}

// TestHealthChipsKeepFixedWidth guards the banner against text jumps: a chip's
// width must not change as its value grows, or every chip after it shifts.
func TestHealthChipsKeepFixedWidth(t *testing.T) {
	low := healthInput{connected: true, polls: 5, p95Us: 1200, baselineUs: 1200, hasCache: true, cachePct: 1}
	high := healthInput{connected: true, polls: 5, errRatio: 0.99, p95Us: 120000, baselineUs: 1200, cpuPct: 293, hasCache: true, cachePct: 100}
	_, a := health(low)
	_, b := health(high)
	if len(a) != len(b) {
		t.Fatalf("chip count changed: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if wa, wb := lipgloss.Width(a[i].text), lipgloss.Width(b[i].text); wa != wb {
			t.Errorf("chip %d width changed with its value: %q (%d) vs %q (%d)", i, a[i].text, wa, b[i].text, wb)
		}
	}
}

func TestBannerRenders(t *testing.T) {
	m := newSizedTUI(120, 40)
	m.connected = true
	m.polls = 5
	if v := m.View(); !strings.Contains(v, "HEALTHY") {
		t.Errorf("expected HEALTHY banner:\n%s", v)
	}
	m.polls = 0
	if v := m.View(); !strings.Contains(v, "NO DATA") {
		t.Errorf("expected NO DATA banner:\n%s", v)
	}
	m.polls = 5
	m.connected = false
	if v := m.View(); !strings.Contains(v, "CRITICAL") || !strings.Contains(v, "reconnecting") {
		t.Errorf("expected CRITICAL/reconnecting banner:\n%s", v)
	}
}

func TestHealthStateNormalizesCPUByCores(t *testing.T) {
	// CPUPercent is percent of one core; the verdict must read it as percent
	// of total capacity so a busy multi-core node is not called critical.
	m := newTUI(nil, time.Second, 120)
	m.sampler.Latest.CPUPercent = 100 * float64(runtime.NumCPU())
	if got := m.healthState().cpuPct; got != 100 {
		t.Fatalf("cpuPct = %v, want 100", got)
	}
}

func TestModelHealthStates(t *testing.T) {
	busy := []embtop.ModelPoint{{ReqRate: 5, AvgLatencyUs: 100}, {ReqRate: 6, AvgLatencyUs: 200}}
	idle := []embtop.ModelPoint{{ReqRate: 0, TokRate: 0}, {ReqRate: 0, TokRate: 0}}
	errs := []embtop.ModelPoint{{ReqRate: 5}, {ReqRate: 5, ErrRate: 0.5}}

	if mh := modelHealth(idle); !mh.idle {
		t.Errorf("idle model not marked idle: %+v", mh)
	}
	if mh := modelHealth(nil); !mh.idle {
		t.Errorf("empty history should be idle: %+v", mh)
	}
	if mh := modelHealth(busy); mh.idle || mh.status != healthHealthy || !mh.latRise {
		t.Errorf("busy model health wrong: %+v", mh)
	}
	if mh := modelHealth(errs); !mh.errRise || mh.status < healthDegraded {
		t.Errorf("erroring model not flagged: %+v", mh)
	}
}

func TestModelIndicatorRendered(t *testing.T) {
	m := newSizedTUI(120, 40)
	m.applyResult(fakePoll(0, 0, 0, 0))
	m.applyResult(fakePoll(10, 100, 1, 0)) // errors in the recent window
	m.connected = true
	if v := m.View(); !strings.Contains(v, "↑") {
		t.Errorf("erroring model has no rising indicator:\n%s", v)
	}

	idle := newSizedTUI(120, 40)
	for i := 0; i < 3; i++ {
		idle.applyResult(fakePoll(0, 0, 0, 0))
	}
	idle.connected = true
	if v := idle.View(); !strings.Contains(v, "○") {
		t.Errorf("idle model has no idle indicator:\n%s", v)
	}
}
