package main

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

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
	if got := m.healthState().CPUPct; got != 100 {
		t.Fatalf("cpuPct = %v, want 100", got)
	}
}

func TestCPUUsageUsesNodeOwnParallelism(t *testing.T) {
	// A node reports its own gomaxprocs through INFO cpu; CPU is measured
	// against that, not against the machine running emb-top.
	m := newTUI(nil, time.Second, 120)
	m.sampler.Latest.CPUPercent = 400
	m.sampler.Latest.GoMaxProcs = 4
	if got := m.healthState().CPUPct; got != 100 {
		t.Fatalf("cpuPct = %v, want 100 (400%% of 4 cores)", got)
	}
}

func TestFleetBannerNamesDegradedNode(t *testing.T) {
	a, b := "10.0.0.1:6379", "10.0.0.2:6379"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b},
	}, rn(a), rn(b))
	f.width = 200
	for _, n := range f.nodes {
		n.tui.connected = true
		n.tui.polls = 5
		n.tui.sampler.Latest.ReqRate = 100
	}
	f.nodes[1].tui.sampler.Latest.ErrRate = 5 // 5% error ratio: degraded
	banner := f.fleetBannerView()
	if !strings.Contains(banner, "10.0.0.2:6379") || !strings.Contains(banner, "err") {
		t.Fatalf("banner does not name the degraded node: %q", banner)
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
	if mh := modelHealth(busy); mh.idle || mh.status != embtop.HealthHealthy || !mh.latRise {
		t.Errorf("busy model health wrong: %+v", mh)
	}
	if mh := modelHealth(errs); !mh.errRise || mh.status < embtop.HealthDegraded {
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
