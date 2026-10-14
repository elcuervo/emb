package embtop

import (
	"strings"
	"testing"
)

func TestHealthVerdicts(t *testing.T) {
	cases := []struct {
		name string
		in   HealthInput
		want HealthStatus
		chip string
	}{
		{"no data", HealthInput{Connected: true, Polls: 1}, HealthNoData, ""},
		{"disconnected before two polls", HealthInput{Connected: false, Polls: 1}, HealthCritical, "reconnecting"},
		{"never polled is no data", HealthInput{Connected: false, Polls: 0}, HealthNoData, ""},
		{"healthy", HealthInput{Connected: true, Polls: 5}, HealthHealthy, "err   0.0%"},
		{"errors degrade", HealthInput{Connected: true, Polls: 5, ErrRatio: 0.02}, HealthDegraded, "err   2.0%"},
		{"errors critical", HealthInput{Connected: true, Polls: 5, ErrRatio: 0.10}, HealthCritical, "err  10.0%"},
		{"cpu critical", HealthInput{Connected: true, Polls: 5, CPUPct: 97}, HealthCritical, "cpu  97%"},
		{"latency ok", HealthInput{Connected: true, Polls: 5, P95Us: 1500, BaselineUs: 1000}, HealthHealthy, "p95 1.5ms  "},
		{"latency degrade", HealthInput{Connected: true, Polls: 5, P95Us: 3000, BaselineUs: 1000}, HealthDegraded, "p95 3.0ms  "},
		{"latency critical", HealthInput{Connected: true, Polls: 5, P95Us: 5000, BaselineUs: 1000}, HealthCritical, "p95 5.0ms  "},
		{"low cache is not a verdict driver", HealthInput{Connected: true, Polls: 5, HasCache: true, CachePct: 2}, HealthHealthy, "cache   2%"},
		{"disconnected", HealthInput{Connected: false, Polls: 5}, HealthCritical, "reconnecting"},
	}
	for _, tc := range cases {
		got, sigs := Health(tc.in)
		if got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.name, got, tc.want)
		}
		if tc.chip == "" {
			continue
		}
		found := false
		for _, s := range sigs {
			if s.Text == tc.chip {
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
	low := HealthInput{Connected: true, Polls: 5, P95Us: 1200, BaselineUs: 1200, HasCache: true, CachePct: 1}
	high := HealthInput{Connected: true, Polls: 5, ErrRatio: 0.99, P95Us: 120000, BaselineUs: 1200, CPUPct: 293, HasCache: true, CachePct: 100}
	_, a := Health(low)
	_, b := Health(high)
	if len(a) != len(b) {
		t.Fatalf("chip count changed: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if wa, wb := len(a[i].Text), len(b[i].Text); wa != wb {
			t.Errorf("chip %d width changed with its value: %q (%d) vs %q (%d)", i, a[i].Text, wa, b[i].Text, wb)
		}
	}
}

func TestFleetHealthVerdicts(t *testing.T) {
	node := func(label string, rate float64) FleetNodeInput {
		return FleetNodeInput{
			Label:   label,
			Health:  HealthInput{Connected: true, Polls: 5},
			ReqRate: rate,
		}
	}
	withP95 := func(n FleetNodeInput, p95 int64) FleetNodeInput { n.Health.P95Us = p95; return n }

	cases := []struct {
		name   string
		nodes  []FleetNodeInput
		want   HealthStatus
		reason string
	}{
		{"no data", []FleetNodeInput{{Label: "a", Health: HealthInput{Connected: true, Polls: 1}}}, HealthNoData, ""},
		{"healthy", []FleetNodeInput{node("a", 10), node("b", 10)}, HealthHealthy, ""},
		{"unreachable is critical", []FleetNodeInput{
			node("a", 10),
			{Label: "b", Health: HealthInput{Connected: false, Polls: 5}},
		}, HealthCritical, ""},
		{"skew degrades", []FleetNodeInput{node("a", 90), node("b", 5), node("c", 5)}, HealthDegraded, "skew"},
		{"two-node skew degrades", []FleetNodeInput{node("a", 90), node("b", 10)}, HealthDegraded, "skew"},
		{"never-reached node is critical", []FleetNodeInput{
			node("a", 10),
			{Label: "b", Health: HealthInput{Connected: false, Polls: 0}, Unreachable: true},
		}, HealthCritical, ""},
		{"cache spread is not a fault", []FleetNodeInput{
			func() FleetNodeInput { n := node("a", 10); n.HasCache, n.CachePct = true, 5; return n }(),
			func() FleetNodeInput { n := node("b", 10); n.HasCache, n.CachePct = true, 95; return n }(),
		}, HealthHealthy, ""},
		{"cold cache after restart degrades", []FleetNodeInput{
			func() FleetNodeInput {
				n := node("a", 10)
				n.HasCache, n.CachePct, n.UptimeSecs = true, 5, 10
				return n
			}(),
			func() FleetNodeInput {
				n := node("b", 10)
				n.HasCache, n.CachePct, n.UptimeSecs = true, 80, 3600
				return n
			}(),
		}, HealthDegraded, "cold cache"},
		{"slow peer degrades", []FleetNodeInput{
			withP95(node("a", 10), 100),
			withP95(node("b", 10), 100),
			withP95(node("c", 10), 1000),
		}, HealthDegraded, "slow"},
		{"two-node slow peer degrades", []FleetNodeInput{
			withP95(node("a", 10), 100),
			withP95(node("b", 10), 1000),
		}, HealthDegraded, "slow"},
	}
	for _, tc := range cases {
		got, sigs := FleetHealth(tc.nodes)
		if got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.name, got, tc.want)
		}
		if tc.reason != "" {
			found := false
			for _, s := range sigs {
				if strings.Contains(s.Text, tc.reason) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: no chip containing %q in %+v", tc.name, tc.reason, sigs)
			}
		}
	}
}
