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
		{"low cache is not a verdict driver", healthInput{connected: true, polls: 5, hasCache: true, cachePct: 2}, healthHealthy, "cache   2%"},
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

func TestCPUUsageUsesNodeOwnParallelism(t *testing.T) {
	// A node reports its own gomaxprocs through INFO cpu; CPU is measured
	// against that, not against the machine running emb-top.
	m := newTUI(nil, time.Second, 120)
	m.sampler.Latest.CPUPercent = 400
	m.sampler.Latest.GoMaxProcs = 4
	if got := m.healthState().cpuPct; got != 100 {
		t.Fatalf("cpuPct = %v, want 100 (400%% of 4 cores)", got)
	}
}

func TestFleetHealthVerdicts(t *testing.T) {
	node := func(label string, rate float64) fleetNodeInput {
		return fleetNodeInput{
			label:   label,
			health:  healthInput{connected: true, polls: 5},
			reqRate: rate,
		}
	}
	withP95 := func(n fleetNodeInput, p95 int64) fleetNodeInput { n.health.p95Us = p95; return n }

	cases := []struct {
		name   string
		nodes  []fleetNodeInput
		want   healthStatus
		reason string
	}{
		{"no data", []fleetNodeInput{{label: "a", health: healthInput{connected: true, polls: 1}}}, healthNoData, ""},
		{"healthy", []fleetNodeInput{node("a", 10), node("b", 10)}, healthHealthy, ""},
		{"unreachable is critical", []fleetNodeInput{
			node("a", 10),
			{label: "b", health: healthInput{connected: false, polls: 5}},
		}, healthCritical, ""},
		{"skew degrades", []fleetNodeInput{node("a", 90), node("b", 5), node("c", 5)}, healthDegraded, "skew"},
		{"cache spread is not a fault", []fleetNodeInput{
			func() fleetNodeInput { n := node("a", 10); n.hasCache, n.cachePct = true, 5; return n }(),
			func() fleetNodeInput { n := node("b", 10); n.hasCache, n.cachePct = true, 95; return n }(),
		}, healthHealthy, ""},
		{"cold cache after restart degrades", []fleetNodeInput{
			func() fleetNodeInput {
				n := node("a", 10)
				n.hasCache, n.cachePct, n.uptimeSecs = true, 5, 10
				return n
			}(),
			func() fleetNodeInput {
				n := node("b", 10)
				n.hasCache, n.cachePct, n.uptimeSecs = true, 80, 3600
				return n
			}(),
		}, healthDegraded, "cold cache"},
		{"slow peer degrades", []fleetNodeInput{
			withP95(node("a", 10), 100),
			withP95(node("b", 10), 100),
			withP95(node("c", 10), 1000),
		}, healthDegraded, "slow"},
	}
	for _, tc := range cases {
		got, sigs := fleetHealth(tc.nodes)
		if got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.name, got, tc.want)
		}
		if tc.reason != "" {
			found := false
			for _, s := range sigs {
				if strings.Contains(s.text, tc.reason) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: no chip containing %q in %+v", tc.name, tc.reason, sigs)
			}
		}
	}
}

func TestFleetBannerNamesOffendingNode(t *testing.T) {
	a, b, c := "10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b}, c: &fakeNodeClient{addr: c},
	}, rn(a), rn(b), rn(c))
	f.width = 160
	seedNode(f.nodes[0], 0, 900)
	seedNode(f.nodes[1], 0, 50)
	seedNode(f.nodes[2], 0, 50)
	banner := f.fleetBannerView()
	if !strings.Contains(banner, "skew") {
		t.Fatalf("banner has no skew reason: %q", banner)
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
