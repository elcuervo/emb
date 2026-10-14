package embtop

import "testing"

// TestHealthStatusNames covers the display mapping both tools render.
func TestHealthStatusNames(t *testing.T) {
	cases := map[HealthStatus]string{
		HealthNoData:   "NO DATA",
		HealthHealthy:  "HEALTHY",
		HealthDegraded: "DEGRADED",
		HealthCritical: "CRITICAL",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("HealthStatus(%d).String() = %q, want %q", st, got, want)
		}
	}
	if got := HealthStatus(99).String(); got != "HEALTHY" {
		t.Errorf("an unknown status rendered as %q", got)
	}
}

// TestNodeParallelismFallsBackLocally covers the CPU denominator: the node's
// own reported parallelism when it has one, the local machine's otherwise.
func TestNodeParallelismFallsBackLocally(t *testing.T) {
	if got := NodeParallelism(4); got != 4 {
		t.Errorf("NodeParallelism(4) = %d, want the reported value", got)
	}
	if got := NodeParallelism(0); got < 1 {
		t.Errorf("NodeParallelism(0) = %d, want the local processor count", got)
	}
	if got := NodeParallelism(-1); got < 1 {
		t.Errorf("NodeParallelism(-1) = %d, want the local processor count", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		512:       "512B",
		2048:      "2.0KiB",
		1 << 20:   "1.0MiB",
		1 << 30:   "1.0GiB",
		3 << 29:   "1.5GiB",
		1<<26 + 1: "64.0MiB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestDeltaFloorsAtZero guards the counter-reset invariant: a server that
// restarted reports lower cumulative counters, and that must not read as a
// negative burst.
func TestDeltaFloorsAtZero(t *testing.T) {
	if got := delta(10, 25); got != 15 {
		t.Errorf("delta(10, 25) = %d, want 15", got)
	}
	if got := delta(25, 10); got != 0 {
		t.Errorf("delta(25, 10) = %d, want 0 after a counter reset", got)
	}
	if got := delta(7, 7); got != 0 {
		t.Errorf("delta(7, 7) = %d, want 0", got)
	}
}

func TestCurrentRateAndFlatSeriesEdges(t *testing.T) {
	if got := currentRate(nil); got != 0 {
		t.Errorf("currentRate(nil) = %v, want 0", got)
	}
	if got := currentRate([]Point{{ReqRate: 12}}); got != 12 {
		t.Errorf("currentRate = %v, want the last point's rate", got)
	}
	// An all-zero series is flat; one non-zero sample in an otherwise idle
	// series is not.
	if !flatSeries([]Point{{}, {}, {}}, func(p Point) float64 { return p.ReqRate }) {
		t.Error("an all-zero series did not read as flat")
	}
	if flatSeries([]Point{{}, {ReqRate: 5}, {}}, func(p Point) float64 { return p.ReqRate }) {
		t.Error("a series with a spike read as flat")
	}
	if !flatSeries(nil, func(p Point) float64 { return p.ReqRate }) {
		t.Error("an empty series did not read as flat")
	}
}

// TestNodeReasonNamesTheUnreachableNode covers the banner chip a fleet verdict
// carries for a node that stopped answering.
func TestNodeReasonNamesTheUnreachableNode(t *testing.T) {
	n := FleetNodeInput{Label: "a", Unreachable: true}
	if got := nodeReason(n, HealthCritical, nil); got != "a unreachable" {
		t.Errorf("nodeReason = %q, want the node named as unreachable", got)
	}
	n = FleetNodeInput{Label: "b", Health: HealthInput{Connected: false, Polls: 3}}
	if got := nodeReason(n, HealthCritical, nil); got != "b unreachable" {
		t.Errorf("nodeReason = %q, want the disconnected node named", got)
	}
	// A degraded node with no matching driving signal still names itself and
	// its status, so a fleet verdict is never a bare colour.
	n = FleetNodeInput{Label: "c", Health: HealthInput{Connected: true, Polls: 3}}
	if got := nodeReason(n, HealthDegraded, nil); got != "c degraded" {
		t.Errorf("nodeReason = %q, want the node and its status", got)
	}
}
