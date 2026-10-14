package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

// cpuBoundPoll is one poll of a node that holds its whole CPU budget while
// admission waits time out: the sizing rule the node panel names.
func cpuBoundPoll(mutate func(*embtop.PollResult)) *embtop.PollResult {
	res := fakePoll(100, 400, 0, 1)
	res.CPUPlanKnown = true
	res.CPUBudgetEnabled = true
	res.CPUBudget = 8
	res.CPUActiveTokens = 8
	res.CPUAdmissionTimeouts = 3
	if mutate != nil {
		mutate(res)
	}
	return res
}

// TestNodeSizingPanelRenders guards the node recommendation panel: a node that
// holds its budget is told so, and the banner above it does not move.
func TestNodeSizingPanelRenders(t *testing.T) {
	m := newSizedTUI(160, 30)
	for i := range 6 {
		m.applyResult(cpuBoundPoll(func(r *embtop.PollResult) {
			r.CPUAdmissionTimeouts = int64(i * 3)
		}))
	}
	m.connected = true

	view := m.View()
	if !strings.Contains(view, "SIZING") || !strings.Contains(view, "cpu budget is the limit") {
		t.Fatalf("sizing panel missing from the view:\n%s", view)
	}
	if !strings.Contains(view, "tokens 8/8") {
		t.Fatalf("sizing panel does not name the budget in use:\n%s", view)
	}
	// The panel is advisory: the banner verdict is untouched by it.
	if !strings.Contains(m.bannerView(), "HEALTHY") {
		t.Fatalf("banner verdict moved: %q", m.bannerView())
	}
}

// TestNodeSizingPanelQuietRendersNothing guards the quiet case: no signal near
// a threshold renders no recommendation, and the banner line is byte-for-byte
// what it is with a recommendation present — the chips keep their columns.
func TestNodeSizingPanelQuietRendersNothing(t *testing.T) {
	m := newSizedTUI(160, 30)
	for range 6 {
		m.applyResult(fakePoll(100, 400, 0, 1))
	}
	m.connected = true

	if strings.Contains(m.View(), "SIZING") {
		t.Fatalf("a quiet node rendered a sizing panel:\n%s", m.View())
	}

	// Same node, now holding its budget: the panel appears, the banner line
	// does not change.
	quietBanner := m.bannerView()
	before := strings.Split(m.View(), "\n")
	for i := range 6 {
		m.applyResult(cpuBoundPoll(func(r *embtop.PollResult) {
			r.CPUAdmissionTimeouts = int64(i * 3)
		}))
	}
	after := strings.Split(m.View(), "\n")
	if quietBanner != m.bannerView() {
		t.Fatalf("banner moved when the recommendation appeared:\n%q\n%q", quietBanner, m.bannerView())
	}
	if strings.Contains(strings.Join(after, "\n"), "SIZING") == false {
		t.Fatal("the panel did not appear for a CPU-bound node")
	}
	// The banner stays on its own row, at the same row index.
	if before[1] != after[1] {
		t.Fatalf("banner row moved:\n%q\n%q", before[1], after[1])
	}
}

// TestNodeRecommendationStaysWithinWidth guards the panel against a narrow
// terminal: it clips rather than wrapping the fixed grid.
func TestNodeRecommendationStaysWithinWidth(t *testing.T) {
	m := newSizedTUI(80, 30)
	for i := range 6 {
		m.applyResult(cpuBoundPoll(func(r *embtop.PollResult) {
			r.CPUAdmissionTimeouts = int64(i * 3)
		}))
	}
	m.connected = true
	for _, line := range strings.Split(m.View(), "\n") {
		if !strings.Contains(line, "SIZING") {
			continue
		}
		if got := lipgloss.Width(line); got > 80 {
			t.Errorf("sizing panel is %d cols wide at width 80: %q", got, line)
		}
	}
}

// TestFleetSizingPanelRenders guards the cluster view's advisory line and the
// node it names first.
func TestFleetSizingPanelRenders(t *testing.T) {
	a, b, c := "10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b}, c: &fakeNodeClient{addr: c},
	}, rn(a), rn(b), rn(c))
	f.width, f.height = 200, 60

	// Two nodes hold their CPU budget; the third is out of cache-restore room.
	for _, n := range f.nodes[:2] {
		seedSizing(n, func(i int, r *embtop.PollResult) {
			r.CPUPlanKnown = true
			r.CPUBudget = 8
			r.CPUActiveTokens = 8
			r.CPUAdmissionTimeouts = int64(i * 3)
		})
	}
	seedSizing(f.nodes[2], func(i int, r *embtop.PollResult) {
		r.CacheRestoreLimitBytes = 1 << 30
		r.CacheRestoreHeadroomBytes = 1 << 26
		r.CacheRestoreSkippedMemory = int64(i * 2)
	})

	line := f.fleetRecommendView()
	if !strings.Contains(line, "SIZING") {
		t.Fatalf("no fleet sizing panel: %q", line)
	}
	for _, want := range []string{"cpu", "memory", c} {
		if !strings.Contains(line, want) {
			t.Errorf("fleet sizing panel does not name %q: %q", want, line)
		}
	}
	if !strings.Contains(line, "inspect "+c+" first") {
		t.Errorf("fleet sizing panel does not name the memory node first: %q", line)
	}
	// Advisory only: the fleet verdict does not move.
	if st, _ := embtop.FleetHealth(f.fleetInputs()); st != embtop.HealthHealthy {
		t.Fatalf("fleet verdict moved: %v", st)
	}
}

// TestFleetSizingPanelQuietRendersNothing guards the quiet fleet.
func TestFleetSizingPanelQuietRendersNothing(t *testing.T) {
	a, b := "10.0.0.1:6379", "10.0.0.2:6379"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b},
	}, rn(a), rn(b))
	f.width, f.height = 160, 60
	for _, n := range f.nodes {
		seedSizing(n, func(_ int, r *embtop.PollResult) { r.CPUPlanKnown = true })
	}
	if got := f.fleetRecommendView(); got != "" {
		t.Fatalf("a quiet fleet rendered %q", got)
	}
	if strings.Contains(f.fleetView(), "SIZING") {
		t.Fatalf("a quiet fleet view rendered a sizing panel:\n%s", f.fleetView())
	}
}

// seedSizing pushes a window of polls into one node's sampler through the
// public apply path, so the derived series is exactly what a live run builds.
func seedSizing(n *fleetNode, mutate func(int, *embtop.PollResult)) {
	// A fixed clock, so every node's rates are the same and a test about the
	// sizing line is not also a test about skew.
	at := time.Unix(0, 0)
	n.tui.sampler.SetClock(func() time.Time { return at })
	for i := range 6 {
		res := multiPoll([]string{"alpha"}, map[string]int64{"alpha": int64(100 + i)})
		mutate(i, res)
		n.tui.applyResult(res)
		at = at.Add(time.Second)
	}
	n.tui.connected = true
	n.reachable = true
}
