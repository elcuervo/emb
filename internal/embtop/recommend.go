package embtop

import (
	"fmt"
	"slices"
	"strings"
)

// The sizing recommendation is advisory and separate from the health verdict:
// it has its own fixed thresholds, no severity, and it never raises or lowers
// a status. Its rules are the ones an operator acts on — the node holds its own
// CPU budget, the host is starving it, the cache-restore headroom is spent, or
// memory climbs with nothing to explain it.
const (
	// budgetHeldPct is the share of its budget a node must be holding for the
	// budget itself to read as the limit.
	budgetHeldPct = 95.0
	// headroomSpentPct is the cache-restore headroom below which memory reads
	// as nearly spent.
	headroomSpentPct = 10.0
	// growthPolls is how many polls of the window memory must climb across
	// before the rise is reported.
	growthPolls = 4
	// growthPct is the share of its starting RSS a node must gain across the
	// window for the rise to read as growth rather than allocator noise, and
	// growthMinMB floors it so a small process still needs a real rise.
	growthPct   = 10.0
	growthMinMB = 8
	// flatPct is how much a series may vary across the window and still count
	// as flat.
	flatPct = 10.0
	// fleetHeadroomPct is the fleet-wide CPU headroom below which the answer is
	// capacity rather than a repair.
	fleetHeadroomPct = 20.0
	// peerP95Rise is the tail-latency factor over its peers' median that makes
	// a node the fleet's first suspect.
	peerP95Rise = 2.0
	// peerLoadFloor is the share of its peers' offered load a node must carry
	// to be compared on tail latency at all.
	peerLoadFloor = 0.5
	// hostStarvedPct is the share of the window's CPU capacity the host must
	// have throttled or stolen before it is named as the constraint, and
	// hostBusyPct the CPU a node must have used for that claim when its
	// attribution is unavailable (an older server). A few milliseconds of steal
	// over an idle window is not starvation, and naming it as one is how advice
	// stops being read.
	hostStarvedPct = 1.0
	hostBusyPct    = 5.0
	// hostThrottledPct is the share of the cgroup's throttle periods that must
	// have been held off the CPU, for a host whose throttling is visible as
	// periods rather than as measured microseconds.
	hostThrottledPct = 10.0
)

// Resource names a recommendation can name.
const (
	ResourceCPU    = "cpu"
	ResourceHost   = "host"
	ResourceMemory = "memory"
	ResourceGrowth = "growth"
	ResourceNodes  = "nodes"
)

// Recommendation is the advisory sizing verdict for one node: the resource to
// change and the counters it is derived from. The zero value recommends
// nothing.
type Recommendation struct {
	Kind string // one of the Resource* names, or "" for quiet
	Text string // one line naming the counters, without severity
}

// Empty reports whether there is nothing to recommend.
func (r Recommendation) Empty() bool { return r.Kind == "" }

// RecommendNode derives the advisory sizing recommendation for one node from
// its Point series. It reports the first rule that fires, in the order of what
// the operator can act on: a starving host is not fixed by a wider budget, and
// a spent cache is bought away while unexplained growth is debugged.
func RecommendNode(hist []Point) Recommendation {
	if len(hist) < 2 {
		return Recommendation{}
	}
	first, cur := hist[0], hist[len(hist)-1]

	throttled := delta(first.CPUThrottlePeriods, cur.CPUThrottlePeriods)
	throttledPeriods := delta(first.CPUThrottledPeriods, cur.CPUThrottledPeriods)
	throttleUs := delta(first.CPUThrottleUsec, cur.CPUThrottleUsec)
	stealUs := delta(first.CPUStealUsec, cur.CPUStealUsec)
	budgetHeld := cur.CPUBudget > 0 &&
		float64(cur.CPUActiveTokens) >= float64(cur.CPUBudget)*budgetHeldPct/100

	// The kernel refused CPU the process wanted. Widening the node's own budget
	// cannot help: the host is the constraint. The loss has to matter against
	// the capacity the node had over the window, and the node has to have been
	// doing work: an idle node with a few milliseconds of steal is not starved.
	starvedPct := hostStarvedShare(hist, throttleUs+stealUs)
	throttledPct := hostThrottledShare(throttledPeriods, throttled)
	starved := starvedPct >= hostStarvedPct || throttledPct >= hostThrottledPct
	if !budgetHeld && starved && hostCarriedWork(hist) {
		text := fmt.Sprintf("host is the constraint: %d/%d periods throttled · %s throttled · %s stolen (%.1f%% of the window's cpu)",
			throttledPeriods, throttled, FormatLatency(throttleUs), FormatLatency(stealUs), starvedPct)
		return Recommendation{Kind: ResourceHost, Text: text}
	}

	// The node holds its budget and callers still leave without a grant, so the
	// budget is what binds.
	if budgetHeld {
		if timeouts := delta(first.CPUAdmissionTimeouts, cur.CPUAdmissionTimeouts); timeouts > 0 {
			return Recommendation{Kind: ResourceCPU, Text: fmt.Sprintf(
				"cpu budget is the limit: tokens %d/%d held · %d admits timed out",
				cur.CPUActiveTokens, cur.CPUBudget, timeouts)}
		}
	}

	// The cache-restore limit is nearly spent and entries are already being
	// dropped for want of memory.
	if cur.CacheRestoreLimitBytes > 0 {
		headroomPct := float64(cur.CacheRestoreHeadroomBytes) / float64(cur.CacheRestoreLimitBytes) * 100
		if skipped := delta(first.CacheRestoreSkippedMemory, cur.CacheRestoreSkippedMemory); headroomPct < headroomSpentPct && skipped > 0 {
			return Recommendation{Kind: ResourceMemory, Text: fmt.Sprintf(
				"memory: cache-restore headroom %s of %s (%.0f%%) · %d entries skipped",
				FormatBytes(cur.CacheRestoreHeadroomBytes), FormatBytes(cur.CacheRestoreLimitBytes), headroomPct, skipped)}
		}
	}

	// Memory climbs while the load offered and the cache held stay put: there
	// is nothing to buy here, there is something to fix. The rise has to clear
	// a magnitude floor, or ordinary allocator creep under a steady load would
	// be reported as a leak on every run.
	riseMB := halfRise(hist, func(p Point) float64 { return float64(p.MemMB) })
	if riseMB >= float64(first.MemMB)*growthPct/100 && riseMB >= growthMinMB &&
		flatSeries(hist, func(p Point) float64 { return p.ReqRate }) &&
		flatSeries(hist, func(p Point) float64 { return float64(p.CacheBytes) }) {
		return Recommendation{Kind: ResourceGrowth, Text: fmt.Sprintf(
			"memory rose %.0f MB over the window while load held at %.0f req/s and cache at %s · unexplained growth, not a purchase",
			riseMB, cur.ReqRate, FormatBytes(cur.CacheBytes))}
	}

	return Recommendation{}
}

// hostStarvedShare is the lost CPU as a share of the capacity the node had over
// the window (its own parallelism times the window's duration): the same
// denominator the CPU readout uses, so the two agree.
func hostStarvedShare(hist []Point, lostUs int64) float64 {
	if lostUs <= 0 || len(hist) < 2 {
		return 0
	}
	wall := hist[len(hist)-1].At.Sub(hist[0].At).Microseconds()
	if wall <= 0 {
		return 0
	}
	capacity := wall * int64(NodeParallelism(hist[len(hist)-1].GoMaxProcs))
	if capacity <= 0 {
		return 0
	}
	return float64(lostUs) / float64(capacity) * 100
}

// hostThrottledShare is the share of the window's throttle periods the node was
// held off the CPU in, 0 when the platform reports none.
func hostThrottledShare(throttled, periods int64) float64 {
	if throttled <= 0 || periods <= 0 {
		return 0
	}
	return float64(throttled) / float64(periods) * 100
}

// hostCarriedWork reports whether the window saw inference work at all: tokens
// held, CPU used, or requests the cache did not answer. A host claim about an
// idle node is a claim about nothing.
func hostCarriedWork(hist []Point) bool {
	if len(hist) < 2 {
		return false
	}
	first, cur := hist[0], hist[len(hist)-1]
	if cur.CPUActiveTokens > 0 || cur.CPUPercent >= hostBusyPct {
		return true
	}
	requests := delta(first.TotalRequests, cur.TotalRequests)
	if requests <= 0 {
		return false
	}
	return delta(first.CacheHits, cur.CacheHits) < requests
}

// FleetNodeRecInput is one node's contribution to the fleet recommendation: its
// display label, its Point series, and the tail latency the client measured.
type FleetNodeRecInput struct {
	Label string
	Hist  []Point
	P95Us int64
	// Advice is the node's own advice evidence, when the caller has it. The
	// fleet line uses it to name the action and the phase behind the first
	// node's tail, instead of only naming the node.
	Advice *AdviceInput
}

// FleetRecommendation is the advisory fleet-level recommendation: every
// resource a node binds on, the fleet's remaining headroom, and the node to
// look at first. The zero value recommends nothing.
type FleetRecommendation struct {
	Kinds []string // one entry per binding resource, first-seen order
	Text  string   // one line naming the resources, the headroom and the first node
	First string   // node to inspect or replace first, "" when none
	// Action is what to change across the fleet: the first node's own action
	// when it needs a repair, otherwise additions. Capacity is named only when
	// no node's own evidence points at a cheaper fix.
	Action string
	// Phase and Share are the first node's attributed tail, so the fleet line
	// points at a phase rather than only at a hostname.
	Phase Phase
	Share float64
}

// Empty reports whether there is nothing to recommend.
func (f FleetRecommendation) Empty() bool { return len(f.Kinds) == 0 && f.First == "" }

// RecommendFleet derives the fleet-level advisory recommendation. It reports
// each node's binding resource separately rather than averaging one node's
// limit into the fleet's, so a memory-limited node in a CPU-limited fleet is
// reported as both.
func RecommendFleet(nodes []FleetNodeRecInput) FleetRecommendation {
	if len(nodes) == 0 {
		return FleetRecommendation{}
	}

	var kinds []string
	byKind := map[string][]string{}
	advice := map[string]Advice{}
	for _, n := range nodes {
		r := RecommendNode(n.Hist)
		if r.Empty() {
			continue
		}
		if _, seen := byKind[r.Kind]; !seen {
			kinds = append(kinds, r.Kind)
		}
		byKind[r.Kind] = append(byKind[r.Kind], n.Label)
	}
	// Every node's own course of action, when the caller supplied its evidence.
	for _, n := range nodes {
		if n.Advice == nil {
			continue
		}
		if a := Advise(*n.Advice); !a.Empty() {
			advice[n.Label] = a
		}
	}

	first := minorityBoundNode(kinds, byKind)
	if first == "" {
		first = slowPeerNode(nodes)
	}
	headroom, uniform, headroomKnown := fleetCPUHeadroom(nodes)

	if len(kinds) == 0 && !uniform && first == "" {
		return FleetRecommendation{}
	}

	var parts []string
	if first != "" {
		// Named ahead of any node-count advice: a node that does not look like
		// its peers is inspected before the fleet is grown.
		parts = append(parts, "inspect "+first+" first")
	}
	// A node's own repair outranks capacity: entries the cache dropped, a
	// layout that serializes or a model whose runs hold the tail are all
	// cheaper than another node, and adding nodes would buy capacity for work
	// that should not be done at all.
	repairLabel, repairAction := "", ""
	for _, n := range nodes {
		a, ok := advice[n.Label]
		if !ok || a.Action == ActionNone || isCapacity(a.Action) {
			// Capacity answers — a wider budget, another node — do not stand in
			// for each other: they are named together below.
			continue
		}
		if repairLabel == "" || (first != "" && n.Label == first) {
			repairLabel, repairAction = n.Label, a.Action
			if first != "" && n.Label == first {
				break
			}
		}
	}

	// A node-count recommendation is capacity, not a repair, and it is only
	// justified when the headroom is uniformly small and no node's own
	// evidence points at a repair: one node binding on its own is named as that
	// node's resource instead.
	if uniform && repairAction == "" {
		if len(kinds) == 0 {
			kinds = []string{ResourceNodes}
		} else if !slices.Contains(kinds, ResourceNodes) {
			kinds = append(kinds, ResourceNodes)
		}
		parts = append(parts, fmt.Sprintf("add nodes: every node's cpu headroom is small (fleet headroom %.0f%%)", headroom))
	}
	for _, k := range kinds {
		if k == ResourceNodes {
			continue // the capacity clause above already states the headroom
		}
		parts = append(parts, k+" bound on "+strings.Join(byKind[k], ", "))
	}
	if !uniform && headroomKnown && len(byKind) > 0 {
		parts = append(parts, fmt.Sprintf("fleet cpu headroom %.0f%%", headroom))
	}

	rec := FleetRecommendation{Kinds: kinds, Text: strings.Join(parts, " · "), First: first}
	if repairAction != "" {
		// The named node's action, with the phase behind its tail.
		a := advice[repairLabel]
		rec.Action = repairAction
		rec.Phase, rec.Share = a.Phase, a.Share
		clause := repairLabel + ": " + a.Text
		if a.Phase != PhaseNone && a.Share > 0 {
			clause += fmt.Sprintf(" (its tail is %.0f%% %s wait)", a.Share*100, a.Phase)
		}
		rec.Text = clause + " · " + rec.Text
	} else if uniform {
		rec.Action = ActionNodes
	}
	return rec
}

// minorityBoundNode picks the node bound on a resource its peers are not: the
// node that does not look like the rest of the fleet is the one to look at.
func minorityBoundNode(kinds []string, byKind map[string][]string) string {
	for _, k := range kinds {
		if k == ResourceCPU {
			continue // the resource a fleet is usually limited by is not an anomaly
		}
		if len(byKind[k]) <= 1 || len(kinds) > 1 {
			return byKind[k][0]
		}
	}
	return ""
}

// slowPeerNode names a node whose tail latency is far above its peers' while it
// carries comparable load. Load is what rules out "it is slow because it is
// busier", which is a skew, not a fault.
func slowPeerNode(nodes []FleetNodeRecInput) string {
	best, bestRatio := "", 0.0
	for i, n := range nodes {
		if n.P95Us <= 0 {
			continue
		}
		var peersP95, peersLoad []int64
		load := currentRate(n.Hist)
		for j, o := range nodes {
			if j == i {
				continue
			}
			if o.P95Us > 0 {
				peersP95 = append(peersP95, o.P95Us)
			}
			peersLoad = append(peersLoad, int64(currentRate(o.Hist)))
		}
		if len(peersP95) == 0 {
			continue
		}
		median := MedianInt64(peersP95)
		if median <= 0 {
			continue
		}
		peersMean := MedianInt64(peersLoad)
		if peersMean > 0 && load < float64(peersMean)*peerLoadFloor {
			continue // too little load to blame the node for its latency
		}
		ratio := float64(n.P95Us) / float64(median)
		if ratio > peerP95Rise && ratio > bestRatio {
			best, bestRatio = n.Label, ratio
		}
	}
	return best
}

// fleetCPUHeadroom is the fleet's remaining CPU budget across the nodes that
// report a plan, as a percentage, plus whether every one of those nodes is
// nearly out. known is false when no node reports a plan.
func fleetCPUHeadroom(nodes []FleetNodeRecInput) (pct float64, uniform, known bool) {
	var budget, active int64
	uniform = true
	for _, n := range nodes {
		if len(n.Hist) == 0 {
			continue
		}
		p := n.Hist[len(n.Hist)-1]
		if !p.CPUPlanKnown || p.CPUBudget <= 0 {
			continue
		}
		known = true
		budget += p.CPUBudget
		active += p.CPUActiveTokens
		if node := (1 - float64(p.CPUActiveTokens)/float64(p.CPUBudget)) * 100; node >= fleetHeadroomPct {
			uniform = false
		}
	}
	if !known || budget == 0 {
		return 0, false, false
	}
	return (1 - float64(active)/float64(budget)) * 100, uniform, true
}

// currentRate is a node's most recent offered rate.
func currentRate(hist []Point) float64 {
	if len(hist) == 0 {
		return 0
	}
	return hist[len(hist)-1].ReqRate
}

// halfRise is how far the window's second half sits above its first. Comparing
// halves rather than endpoints is what lets a leak be seen at all: RSS goes up
// and down as the allocator returns pages, so a test that every step rises
// misses the sawtooth a real leak draws.
func halfRise(hist []Point, value func(Point) float64) float64 {
	if len(hist) < growthPolls {
		return 0
	}
	half := len(hist) / 2
	return meanOf(hist[half:], value) - meanOf(hist[:half], value)
}

// meanOf averages value over pts, or 0 when pts is empty.
func meanOf(pts []Point, value func(Point) float64) float64 {
	if len(pts) == 0 {
		return 0
	}
	var sum float64
	for _, p := range pts {
		sum += value(p)
	}
	return sum / float64(len(pts))
}

// flatSeries reports whether the series stays within flatPct of its own mean
// across the window.
func flatSeries(hist []Point, value func(Point) float64) bool {
	if len(hist) == 0 {
		return true
	}
	var sum float64
	for _, p := range hist {
		sum += value(p)
	}
	mean := sum / float64(len(hist))
	if mean <= 0 {
		// Nothing to be flat about: an idle series is flat by definition.
		for _, p := range hist {
			if value(p) != 0 {
				return false
			}
		}
		return true
	}
	limit := mean * flatPct / 100
	for _, p := range hist {
		if v := value(p); v > mean+limit || v < mean-limit {
			return false
		}
	}
	return true
}

// delta is cur-prev, floored at zero: a counter that reset (the server
// restarted) must not read as a negative burst.
func delta(prev, cur int64) int64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}
