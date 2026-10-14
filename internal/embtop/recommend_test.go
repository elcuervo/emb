package embtop

import (
	"strings"
	"testing"
	"time"
)

// recPoint builds a Point at a fixed cadence, so a series' shape is what the
// rules see rather than its timestamps.
func recPoint(i int, mutate func(*Point)) Point {
	p := Point{
		At:        time.Unix(int64(i), 0),
		CPUBudget: 8, CPUActiveTokens: 2, CPUPlanKnown: true,
		MemMB: 100, ReqRate: 100, CacheBytes: 1 << 20,
	}
	if mutate != nil {
		mutate(&p)
	}
	return p
}

func recSeries(n int, mutate func(i int, p *Point)) []Point {
	out := make([]Point, 0, n)
	for i := range n {
		out = append(out, recPoint(i, func(p *Point) {
			if mutate != nil {
				mutate(i, p)
			}
		}))
	}
	return out
}

func TestRecommendNodeRules(t *testing.T) {
	cases := []struct {
		name string
		hist []Point
		want string // expected Kind, "" for quiet
		text []string
	}{
		{
			name: "quiet when nothing is near a threshold",
			hist: recSeries(6, nil),
			want: "",
		},
		{
			name: "too short a window says nothing",
			hist: recSeries(1, nil),
			want: "",
		},
		{
			name: "a held budget with timed-out waits is named",
			hist: recSeries(6, func(i int, p *Point) {
				p.CPUActiveTokens = 8
				p.CPUAdmissionTimeouts = int64(i * 4)
			}),
			want: ResourceCPU,
			text: []string{"cpu budget is the limit", "tokens 8/8", "20 admits timed out"},
		},
		{
			name: "a held budget without timeouts is not a recommendation",
			hist: recSeries(6, func(_ int, p *Point) { p.CPUActiveTokens = 8 }),
			want: "",
		},
		{
			name: "a starving host is told apart from a binding budget",
			hist: recSeries(6, func(i int, p *Point) {
				p.CPUActiveTokens = 2 // the budget is not held
				p.CPUThrottlePeriods = int64(i * 100)
				p.CPUThrottledPeriods = int64(i * 30)
				p.CPUThrottleUsec = int64(i) * 5_000_000
			}),
			want: ResourceHost,
			text: []string{"host is the constraint", "150/500 periods throttled", "25.00s throttled"},
		},
		{
			name: "a held budget is not blamed on the host",
			hist: recSeries(6, func(i int, p *Point) {
				p.CPUActiveTokens = 8
				p.CPUAdmissionTimeouts = int64(i)
				p.CPUThrottledPeriods = int64(i * 30)
			}),
			want: ResourceCPU,
			text: []string{"cpu budget is the limit"},
		},
		{
			name: "stolen time alone starves the host",
			hist: recSeries(6, func(i int, p *Point) {
				p.CPUStealUsec = int64(i) * 7_000_000
			}),
			want: ResourceHost,
			text: []string{"host is the constraint", "35.00s stolen"},
		},
		{
			name: "spent cache headroom names memory",
			hist: recSeries(6, func(i int, p *Point) {
				p.CacheRestoreLimitBytes = 1 << 30
				p.CacheRestoreHeadroomBytes = 1 << 26 // 6%: nearly spent
				p.CacheRestoreSkippedMemory = int64(i * 3)
			}),
			want: ResourceMemory,
			text: []string{"memory: cache-restore headroom", "64.0MiB of 1.0GiB", "15 entries skipped"},
		},
		{
			name: "spent headroom with nothing skipped yet is quiet",
			hist: recSeries(6, func(_ int, p *Point) {
				p.CacheRestoreLimitBytes = 1 << 30
				p.CacheRestoreHeadroomBytes = 1 << 26
			}),
			want: "",
		},
		{
			name: "healthy headroom is quiet however many entries were skipped",
			hist: recSeries(6, func(i int, p *Point) {
				p.CacheRestoreLimitBytes = 1 << 30
				p.CacheRestoreHeadroomBytes = 1 << 29 // 50%
				p.CacheRestoreSkippedMemory = int64(i * 3)
			}),
			want: "",
		},
		{
			name: "growth at flat load is reported, not bought",
			hist: recSeries(6, func(i int, p *Point) { p.MemMB = int64(100 + i*5) }),
			want: ResourceGrowth,
			text: []string{"memory rose 15 MB over the window", "unexplained growth, not a purchase"},
		},
		{
			name: "growth while the cache fills is not unexplained",
			hist: recSeries(6, func(i int, p *Point) {
				p.MemMB = int64(100 + i*5)
				p.CacheBytes = int64(i) << 20
			}),
			want: "",
		},
		{
			name: "growth while the load climbs is not unexplained",
			hist: recSeries(6, func(i int, p *Point) {
				p.MemMB = int64(100 + i*5)
				p.ReqRate = 100 + float64(i)*50
			}),
			want: "",
		},
		{
			name: "a memory dip ends the growth suspicion",
			hist: func() []Point {
				h := recSeries(6, func(i int, p *Point) { p.MemMB = int64(100 + i*5) })
				h[3].MemMB = 90
				return h
			}(),
			want: "",
		},
		{
			name: "too short a window cannot show growth",
			hist: recSeries(3, func(i int, p *Point) { p.MemMB = int64(100 + i*10) }),
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RecommendNode(tc.hist)
			if got.Kind != tc.want {
				t.Fatalf("Kind = %q (%s), want %q", got.Kind, got.Text, tc.want)
			}
			if tc.want == "" && got.Text != "" {
				t.Fatalf("quiet recommendation carried text %q", got.Text)
			}
			for _, want := range tc.text {
				if !strings.Contains(got.Text, want) {
					t.Errorf("text %q does not state %q", got.Text, want)
				}
			}
		})
	}
}

func TestRecommendFleetScenarios(t *testing.T) {
	cpuBound := func(i int, p *Point) {
		p.CPUActiveTokens = 8
		p.CPUAdmissionTimeouts = int64(i * 2)
	}
	memBound := func(i int, p *Point) {
		p.CacheRestoreLimitBytes = 1 << 30
		p.CacheRestoreHeadroomBytes = 1 << 26
		p.CacheRestoreSkippedMemory = int64(i * 3)
	}
	quiet := recSeries(6, nil)

	t.Run("different nodes bind on different resources", func(t *testing.T) {
		got := RecommendFleet([]FleetNodeRecInput{
			{Label: "a", Hist: recSeries(6, cpuBound)},
			{Label: "b", Hist: recSeries(6, cpuBound)},
			{Label: "c", Hist: recSeries(6, memBound)},
		})
		if strings.Join(got.Kinds, ",") != ResourceCPU+","+ResourceMemory {
			t.Fatalf("Kinds = %v, want cpu and memory reported separately", got.Kinds)
		}
		if got.First != "c" {
			t.Errorf("First = %q, want the memory-limited node c", got.First)
		}
		if !strings.Contains(got.Text, "memory") || !strings.Contains(got.Text, "cpu") {
			t.Errorf("text does not name both resources: %q", got.Text)
		}
	})

	t.Run("a fallen-behind peer is named first", func(t *testing.T) {
		got := RecommendFleet([]FleetNodeRecInput{
			{Label: "a", Hist: quiet, P95Us: 1000},
			{Label: "b", Hist: quiet, P95Us: 9000},
			{Label: "c", Hist: quiet, P95Us: 1000},
		})
		if got.First != "b" {
			t.Fatalf("First = %q, want the slow peer b", got.First)
		}
		if !strings.Contains(got.Text, "inspect b first") {
			t.Errorf("text does not name b ahead of the rest: %q", got.Text)
		}
	})

	t.Run("a slow node carrying no load is not the first suspect", func(t *testing.T) {
		low := func(_ int, p *Point) { p.ReqRate = 1 }
		got := RecommendFleet([]FleetNodeRecInput{
			{Label: "a", Hist: quiet, P95Us: 1000},
			{Label: "b", Hist: recSeries(6, low), P95Us: 9000},
			{Label: "c", Hist: quiet, P95Us: 1000},
		})
		if got.First == "b" {
			t.Fatalf("a node carrying no load was blamed for its latency: %+v", got)
		}
	})

	t.Run("uniform headroom recommends nodes", func(t *testing.T) {
		held := func(_ int, p *Point) { p.CPUActiveTokens = 8 } // 0% headroom, no timeouts
		got := RecommendFleet([]FleetNodeRecInput{
			{Label: "a", Hist: recSeries(6, held)},
			{Label: "b", Hist: recSeries(6, held)},
		})
		if len(got.Kinds) != 1 || got.Kinds[0] != ResourceNodes {
			t.Fatalf("Kinds = %v, want nodes", got.Kinds)
		}
		if !strings.Contains(got.Text, "add nodes") || !strings.Contains(got.Text, "headroom 0%") {
			t.Errorf("text does not name adding nodes and the headroom: %q", got.Text)
		}
	})

	t.Run("uniform headroom is also named beside a shared binding", func(t *testing.T) {
		got := RecommendFleet([]FleetNodeRecInput{
			{Label: "a", Hist: recSeries(6, cpuBound)},
			{Label: "b", Hist: recSeries(6, cpuBound)},
		})
		if !strings.Contains(got.Text, "add nodes") {
			t.Errorf("a uniformly cpu-limited fleet did not name capacity: %q", got.Text)
		}
	})

	t.Run("a quiet fleet recommends nothing", func(t *testing.T) {
		got := RecommendFleet([]FleetNodeRecInput{
			{Label: "a", Hist: quiet},
			{Label: "b", Hist: quiet},
		})
		if !got.Empty() {
			t.Fatalf("quiet fleet recommended %+v", got)
		}
	})

	t.Run("an empty fleet recommends nothing", func(t *testing.T) {
		if got := RecommendFleet(nil); !got.Empty() {
			t.Fatalf("empty fleet recommended %+v", got)
		}
	})
}

// A slow peer with healthy headroom is still worth naming: the recommendation
// must not require the fleet to be out of capacity.
func TestRecommendFleetSlowPeerWithHealthyHeadroom(t *testing.T) {
	got := RecommendFleet([]FleetNodeRecInput{
		{Label: "a", Hist: recSeries(6, nil), P95Us: 1000},
		{Label: "b", Hist: recSeries(6, nil), P95Us: 9000},
	})
	if got.First != "b" {
		t.Fatalf("First = %q, want b", got.First)
	}
	if !strings.HasPrefix(got.Text, "inspect b first") {
		t.Fatalf("the slow peer is not named ahead of everything else: %q", got.Text)
	}
	if strings.Contains(got.Text, "add nodes") {
		t.Fatalf("a fleet with headroom was told to add nodes: %q", got.Text)
	}
	if len(got.Kinds) != 0 {
		t.Fatalf("Kinds = %v, want none: nothing is binding", got.Kinds)
	}
}

// A fleet that recommends capacity must not also claim the headroom is fine.
func TestRecommendFleetHeadroomClaimIsConsistent(t *testing.T) {
	// Every node holds its budget: the headroom really is spent.
	held := func(_ int, p *Point) { p.CPUActiveTokens = 8 }
	spent := RecommendFleet([]FleetNodeRecInput{
		{Label: "a", Hist: recSeries(6, held)},
		{Label: "b", Hist: recSeries(6, held)},
	})
	if !strings.Contains(spent.Text, "add nodes") || !strings.Contains(spent.Text, "headroom 0%") {
		t.Fatalf("spent headroom was not reported as capacity: %q", spent.Text)
	}

	// The same fleet idle: the headroom is untouched, so no capacity advice.
	idle := RecommendFleet([]FleetNodeRecInput{
		{Label: "a", Hist: recSeries(6, nil)},
		{Label: "b", Hist: recSeries(6, nil)},
	})
	if !idle.Empty() {
		t.Fatalf("an idle fleet was told to add nodes: %+v", idle)
	}
}

// TestRecommendNodeIgnoresAllocatorCreep guards the growth rule's magnitude
// floor: a steady load that drifts a little must not be reported as a leak on
// every run, or the advice becomes noise.
func TestRecommendNodeIgnoresAllocatorCreep(t *testing.T) {
	creep := recSeries(12, func(i int, p *Point) { p.MemMB = int64(100 + i/4) }) // +2MB, 2%
	if got := RecommendNode(creep); got.Kind != "" {
		t.Fatalf("a 2MB drift was reported as growth: %+v", got)
	}
	// The same drift on a tiny process is still under the absolute floor.
	tiny := recSeries(12, func(i int, p *Point) { p.MemMB = int64(20 + i/3) }) // +3MB
	if got := RecommendNode(tiny); got.Kind != "" {
		t.Fatalf("a 3MB drift was reported as growth: %+v", got)
	}
	// A rise that clears both floors is reported.
	real := recSeries(12, func(i int, p *Point) { p.MemMB = int64(100 + i*3) }) // +33MB, 33%
	if got := RecommendNode(real); got.Kind != ResourceGrowth {
		t.Fatalf("a 33MB rise was not reported: %+v", got)
	}
}

// TestRecommendNodeSeesALeakThroughASawtooth guards the half-means comparison:
// RSS dips as the allocator returns pages, so a leak that goes up and down must
// still be reported. A rule that required every step to rise would miss it.
func TestRecommendNodeSeesALeakThroughASawtooth(t *testing.T) {
	// +10 MB per pair of polls with a 4 MB giveback in between: the series is
	// never monotonic, but the second half sits well above the first.
	sawtooth := recSeries(12, func(i int, p *Point) {
		p.MemMB = int64(100 + i*5 - (i%2)*4)
	})
	if got := RecommendNode(sawtooth); got.Kind != ResourceGrowth {
		t.Fatalf("a sawtoothing leak was not reported: %+v", got)
	}
}
