package embtop

import "testing"

// verdictInput derives the dashboard's verdict input from a Point series, the
// way the TUI does.
func verdictInput(hist []Point) HealthInput {
	cur := hist[len(hist)-1]
	in := HealthInput{
		Connected: true,
		Polls:     len(hist),
		CPUPct:    cur.CPUPercent / float64(NodeParallelism(cur.GoMaxProcs)),
	}
	if cur.ReqRate > 0 {
		in.ErrRatio = cur.ErrRate / cur.ReqRate
	}
	return in
}

// TestRecommendationLeavesTheVerdictAlone guards the advisory layer's boundary:
// a node can be perfectly healthy and still need a resource changed. Two series
// that differ only in the signals a recommendation reads — the same node with
// the recommendation-bearing counters on and off — must produce the identical
// verdict, and the verdict must stay healthy.
func TestRecommendationLeavesTheVerdictAlone(t *testing.T) {
	// The node with nothing to recommend: its verdict is the baseline.
	quiet := recSeries(6, nil)
	quietStatus, quietSigs := Health(verdictInput(quiet))

	cases := []struct {
		name string
		hist []Point
	}{
		{"cpu", recSeries(6, func(i int, p *Point) {
			p.CPUActiveTokens = 8
			p.CPUAdmissionTimeouts = int64(i * 4)
		})},
		{"host", recSeries(6, func(i int, p *Point) {
			// Throttled for a material share of the window: the host is the
			// constraint, not the node's own budget.
			p.GoMaxProcs = 8
			p.CPUThrottlePeriods = int64(i * 100)
			p.CPUThrottledPeriods = int64(i * 40)
			p.CPUThrottleUsec = int64(i) * 1_000_000
		})},
		{"memory", recSeries(6, func(i int, p *Point) {
			p.CacheRestoreLimitBytes = 1 << 30
			p.CacheRestoreHeadroomBytes = 1 << 26
			p.CacheRestoreSkippedMemory = int64(i * 3)
		})},
		{"growth", recSeries(6, func(i int, p *Point) {
			p.MemMB = int64(100 + i*5)
		})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := RecommendNode(tc.hist); rec.Empty() {
				t.Fatalf("no %s recommendation to test the boundary with", tc.name)
			}
			st, sigs := Health(verdictInput(tc.hist))
			if st != quietStatus {
				t.Fatalf("status moved to %v when the %s signals appeared (baseline %v)", st, tc.name, quietStatus)
			}
			if st != HealthHealthy {
				t.Fatalf("a recommendation-bearing but healthy node read as %v", st)
			}
			if len(sigs) != len(quietSigs) {
				t.Fatalf("chip count moved from %d to %d", len(quietSigs), len(sigs))
			}
			for i := range sigs {
				if sigs[i] != quietSigs[i] {
					t.Fatalf("chip %d moved from %+v to %+v", i, quietSigs[i], sigs[i])
				}
			}
		})
	}
}

// TestFleetRecommendationLeavesTheVerdictAlone is the fleet half of the same
// boundary: a healthy fleet can still need capacity, and the fleet verdict does
// not move when its nodes start reporting the signals a recommendation reads.
func TestFleetRecommendationLeavesTheVerdictAlone(t *testing.T) {
	quiet := recSeries(6, nil)
	busy := recSeries(6, func(_ int, p *Point) { p.CPUActiveTokens = 8 })

	inputs := func(hist []Point) []FleetNodeInput {
		return []FleetNodeInput{
			{Label: "a", Health: verdictInput(hist), ReqRate: 100},
			{Label: "b", Health: verdictInput(hist), ReqRate: 100},
		}
	}
	if got := RecommendFleet([]FleetNodeRecInput{
		{Label: "a", Hist: busy}, {Label: "b", Hist: busy},
	}); got.Empty() {
		t.Fatal("no fleet recommendation to test the boundary with")
	}

	before, beforeSigs := FleetHealth(inputs(quiet))
	after, afterSigs := FleetHealth(inputs(busy))
	if before != after || before != HealthHealthy {
		t.Fatalf("fleet verdict moved from %v to %v when the signals appeared", before, after)
	}
	if len(beforeSigs) != len(afterSigs) {
		t.Fatalf("fleet chip count moved from %d to %d", len(beforeSigs), len(afterSigs))
	}
	for i := range beforeSigs {
		if beforeSigs[i] != afterSigs[i] {
			t.Fatalf("fleet chip %d moved from %+v to %+v", i, beforeSigs[i], afterSigs[i])
		}
	}
}
