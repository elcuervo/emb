package embtop

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
)

// HealthStatus is the synthesized dashboard verdict, ordered by severity so
// the worst signal wins by simple comparison.
type HealthStatus int

const (
	HealthNoData HealthStatus = iota
	HealthHealthy
	HealthDegraded
	HealthCritical
)

// String names the status for display.
func (s HealthStatus) String() string {
	switch s {
	case HealthNoData:
		return "NO DATA"
	case HealthDegraded:
		return "DEGRADED"
	case HealthCritical:
		return "CRITICAL"
	default:
		return "HEALTHY"
	}
}

// Fixed health thresholds. Defaults on purpose: no new flags, one place to
// tune.
const (
	ErrRatioDegraded = 0.01 // 1% of requests erroring
	errRatioCritical = 0.05 // 5%
	p95RiseDegraded  = 2.0  // × session baseline
	p95RiseCritical  = 4.0
	CPUDegradedPct   = 85.0
	cPUCriticalPct   = 95.0
)

// Signal is one health chip: its value text and its own severity. Only
// signals marked driving can raise the overall verdict; informational chips
// are shown but never influence it.
type Signal struct {
	Text    string
	Level   HealthStatus
	Driving bool
}

// HealthInput is everything the verdict is derived from, so Health stays a
// pure function.
type HealthInput struct {
	Connected  bool
	Polls      int
	ErrRatio   float64 // err/s ÷ req/s; 0 when unknown
	P95Us      int64   // current p95; 0 when unknown
	BaselineUs int64   // session baseline p95; 0 when unknown
	CPUPct     float64
	HasCache   bool
	CachePct   float64
}

// Health synthesizes the overall verdict and the per-signal chips. The overall
// status is the worst signal level, except that fewer than two polls reads as
// no-data rather than a verdict.
func Health(in HealthInput) (HealthStatus, []Signal) {
	conn := Signal{Text: "connected", Level: HealthHealthy, Driving: true}
	if !in.Connected {
		conn = Signal{Text: "reconnecting", Level: HealthCritical, Driving: true}
	}
	sigs := []Signal{conn}
	// No verdict until two polls of data, except that disconnection after any
	// successful sample is its own verdict.
	if in.Polls < 2 {
		if !in.Connected && in.Polls >= 1 {
			return HealthCritical, sigs
		}
		return HealthNoData, sigs
	}

	errSig := Signal{Text: fmt.Sprintf("err %5.1f%%", in.ErrRatio*100), Level: HealthHealthy, Driving: true}
	switch {
	case in.ErrRatio > errRatioCritical:
		errSig.Level = HealthCritical
	case in.ErrRatio > ErrRatioDegraded:
		errSig.Level = HealthDegraded
	}
	sigs = append(sigs, errSig)

	if in.P95Us > 0 {
		latSig := Signal{Text: fmt.Sprintf("p95 %-7s", FormatLatency(in.P95Us)), Level: HealthHealthy, Driving: true}
		if in.BaselineUs > 0 {
			ratio := float64(in.P95Us) / float64(in.BaselineUs)
			switch {
			case ratio > p95RiseCritical:
				latSig.Level = HealthCritical
			case ratio > p95RiseDegraded:
				latSig.Level = HealthDegraded
			}
		}
		sigs = append(sigs, latSig)
	}

	cpuSig := Signal{Text: fmt.Sprintf("cpu %3.0f%%", in.CPUPct), Level: HealthHealthy, Driving: true}
	switch {
	case in.CPUPct > cPUCriticalPct:
		cpuSig.Level = HealthCritical
	case in.CPUPct > CPUDegradedPct:
		cpuSig.Level = HealthDegraded
	}
	sigs = append(sigs, cpuSig)

	// Cache hit rate is a workload property, not node health: it stays a
	// visible chip but never drives the verdict.
	if in.HasCache {
		sigs = append(sigs, Signal{Text: fmt.Sprintf("cache %3.0f%%", in.CachePct), Level: HealthHealthy})
	}

	worst := HealthHealthy
	for _, s := range sigs {
		if s.Driving && s.Level > worst {
			worst = s.Level
		}
	}
	return worst, sigs
}

// FormatLatency renders a microsecond latency compactly (µs / ms / s), the
// shape both the verdict chips and the dashboard's metric columns use.
func FormatLatency(us int64) string {
	switch {
	case us >= 1_000_000:
		return fmt.Sprintf("%.2fs", float64(us)/1e6)
	case us >= 1000:
		return fmt.Sprintf("%.1fms", float64(us)/1e3)
	default:
		return fmt.Sprintf("%dµs", us)
	}
}

// FormatBytes renders a byte count compactly (KiB / MiB / GiB), the shape the
// sizing recommendation quotes its headroom in.
func FormatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// NodeParallelism is the denominator for a node's CPU usage: the node's own
// reported processor count when it has one, else the local machine's.
func NodeParallelism(reported int) int {
	if reported > 0 {
		return reported
	}
	return runtime.NumCPU()
}

// FleetNodeInput is one node's contribution to the fleet verdict.
type FleetNodeInput struct {
	Label       string
	Health      HealthInput
	ReqRate     float64
	Idle        bool
	Orphaned    bool
	Unreachable bool // a poll has actually failed (not merely not-yet-polled)
	UptimeSecs  int64
	HasCache    bool
	CachePct    float64
}

// Fixed fleet thresholds, built-in defaults like the node ones: no new flags.
const (
	FleetSkewDegraded = 2.5 // node rate × the mean of its peers' rates
	FleetPeerP95Rise  = 2.0 // node p95 vs its peers' median
	fleetColdCachePct = 0.5 // node hit rate vs the peers' median
	fleetRestartSecs  = 120 // uptime under this reads as recently restarted
)

// nodeReason is one node's banner chip: its worst driving signal, or the
// disconnection/unreachable cause.
func nodeReason(n FleetNodeInput, st HealthStatus, sigs []Signal) string {
	if n.Unreachable || !n.Health.Connected {
		return n.Label + " unreachable"
	}
	for _, s := range sigs {
		if s.Driving && s.Level == st {
			return n.Label + " " + strings.TrimSpace(s.Text)
		}
	}
	return n.Label + " " + strings.ToLower(st.String())
}

// FleetHealth synthesizes the fleet verdict. It reuses the per-node rules and
// thresholds, then layers only actionable cross-node signals: a node carrying
// far more than its 1/N share, a node whose tail latency is far above its
// peers', and a node whose cache is cold after a restart. An unreachable or
// auth-refusing node is already critical through its own verdict; cache spread
// alone never drives the fleet.
func FleetHealth(nodes []FleetNodeInput) (HealthStatus, []Signal) {
	if len(nodes) == 0 {
		return HealthNoData, nil
	}
	anyData := false
	worst := HealthHealthy
	var sigs []Signal
	const maxNodeReasons = 3
	for _, n := range nodes {
		st, nsigs := Health(n.Health)
		if n.Unreachable {
			// A poll has failed, so this node is down even if it never reached
			// two polls; never let it hide behind another node's data.
			st = HealthCritical
		}
		if st != HealthNoData {
			anyData = true
		}
		if st > worst {
			worst = st
		}
		// Name the node and its worst driving signal, so a fleet verdict driven
		// by one node's health is never a bare colour.
		if st >= HealthDegraded && len(sigs) < maxNodeReasons {
			sigs = append(sigs, Signal{Text: nodeReason(n, st, nsigs), Level: st, Driving: true})
		}
	}
	if !anyData {
		return HealthNoData, nil
	}

	raise := func(st HealthStatus, text string) {
		if st > worst {
			worst = st
		}
		sigs = append(sigs, Signal{Text: text, Level: st, Driving: true})
	}

	// Load skew is judged only when every node is reachable, has data, and
	// none is idle or orphaned: an idle node means the clients may simply not
	// know it, so 0% is not a fault to blame on anyone.
	var total float64
	allSettled := len(nodes) >= 2
	for _, n := range nodes {
		if n.Idle || n.Orphaned || n.Health.Polls < 2 || !n.Health.Connected {
			allSettled = false
		}
		total += n.ReqRate
	}
	if allSettled && total > 0 {
		expected := 1 / float64(len(nodes))
		// Compare each node with the mean of its peers, so a two-node fleet
		// can flag a split a 1/N-of-fleet share threshold never would.
		for i, n := range nodes {
			var peersSum float64
			for j, o := range nodes {
				if j != i {
					peersSum += o.ReqRate
				}
			}
			peersMean := peersSum / float64(len(nodes)-1)
			if peersMean > 0 && n.ReqRate > peersMean*FleetSkewDegraded {
				raise(HealthDegraded, fmt.Sprintf("skew %s %.0f%% vs %.0f%%", n.Label, n.ReqRate/total*100, expected*100))
			}
		}
	}

	raiseSlowPeer(nodes, raise)
	raiseColdCache(nodes, raise)
	return worst, sigs
}

// raiseSlowPeer flags a node whose p95 is far above its peers' median. The
// candidate is excluded from its own baseline, so a two-node fleet compares
// one node against the other rather than a median that includes the straggler.
func raiseSlowPeer(nodes []FleetNodeInput, raise func(HealthStatus, string)) {
	for i, n := range nodes {
		if !n.Health.Connected || n.Health.P95Us <= 0 {
			continue
		}
		var peers []int64
		for j, o := range nodes {
			if j != i && o.Health.Connected && o.Health.P95Us > 0 {
				peers = append(peers, o.Health.P95Us)
			}
		}
		if len(peers) == 0 {
			continue
		}
		median := MedianInt64(peers)
		if median > 0 && float64(n.Health.P95Us) > float64(median)*FleetPeerP95Rise {
			raise(HealthDegraded, fmt.Sprintf("slow %s p95 %s vs median %s",
				n.Label, FormatLatency(n.Health.P95Us), FormatLatency(median)))
		}
	}
}

// raiseColdCache flags a recently restarted node whose cache is far colder
// than its established peers'.
func raiseColdCache(nodes []FleetNodeInput, raise func(HealthStatus, string)) {
	var peers, restarted []FleetNodeInput
	for _, n := range nodes {
		if !n.Health.Connected || !n.HasCache {
			continue
		}
		if n.UptimeSecs > 0 && n.UptimeSecs < fleetRestartSecs {
			restarted = append(restarted, n)
			continue
		}
		peers = append(peers, n)
	}
	if len(restarted) == 0 || len(peers) == 0 {
		return
	}
	medians := make([]float64, 0, len(peers))
	for _, p := range peers {
		medians = append(medians, p.CachePct)
	}
	median := medianFloat64(medians)
	for _, n := range restarted {
		if n.CachePct < median*fleetColdCachePct {
			raise(HealthDegraded, fmt.Sprintf("cold cache %s %.0f%% vs %.0f%%", n.Label, n.CachePct, median))
		}
	}
}

// MedianInt64 returns the median of vs, or 0 when vs is empty.
func MedianInt64(vs []int64) int64 {
	if len(vs) == 0 {
		return 0
	}
	sorted := append([]int64(nil), vs...)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}

func medianFloat64(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), vs...)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}
