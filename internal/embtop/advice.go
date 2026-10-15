package embtop

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Actions name what an operator should change. The resource says what binds;
// the action says which knob to turn, and it is the vocabulary the server's own
// logs use ("lower intra_op_threads to overlap runs", "consider raising
// script_workers or intra_op_threads").
const (
	ActionNone      = "none"
	ActionHostCPU   = "host_cpu"
	ActionCPUBudget = "cpu_budget"
	ActionThreads   = "intra_op_threads"
	ActionWorkers   = "workers"
	ActionBatch     = "batch_window"
	ActionCache     = "cache"
	ActionModel     = "model"
	ActionMemory    = "memory"
	ActionGrowth    = "growth"
	ActionNodes     = "nodes"
)

// Advice is the advisory course of action for one node: the action, the
// counters that decided it, the action the same evidence rejects and the
// counter that ruled it out, plus the tail attribution behind it.
type Advice struct {
	Action string
	// Text is one line: the action, its evidence, and the rejected action.
	Text string
	// Phases is the window's measured split, with the dominant phase behind it.
	Phases PhaseSplit
	Phase  Phase
	Share  float64
	// CachedPct is the share of the window's requests the cache answered.
	CachedPct float64
	// CPUPerRun is the CPU the window spent per uncached run.
	CPUPerRun time.Duration
	// Rejected and Why name the action the same evidence does not support and
	// the counter that ruled it out, so the reader sees why not.
	Rejected string
	Why      string
	// Thin is set when the tail was reported as too small to attribute, so a
	// reader can tell "nothing is wrong" from "nothing can be said".
	Thin bool
}

// Empty reports whether there is nothing to advise.
func (a Advice) Empty() bool { return a.Action == "" || a.Action == ActionNone }

// AdviceInput is the evidence one node's advice is derived from. Every field is
// measured by the node or by the observer; nothing is inferred.
type AdviceInput struct {
	Window []Point                 // the node's series
	Models map[string][]ModelPoint // per-model series
	Split  PhaseSplit              // the window's measured phases
	Paths  PathCounts              // the event window by path
	// Raw is the latest EMB.INFO snapshot per model, for the knobs only the
	// raw reply carries (the batch window).
	Raw map[string]*ModelStats
	// UncachedP95Us is the p95 of the requests that ran, over UncachedN events.
	UncachedP95Us int64
	UncachedN     int
}

// Advice thresholds: fixed built-in defaults, like the health and sizing rules.
const (
	// phaseDominance is the share of the measured time a phase must hold to be
	// named as the tail's cause; below it the window is reported as spread.
	phaseDominance = 0.5
	// tailMinEvents and tailMinRuns are the evidence a latency claim needs
	// before capacity is recommended on it.
	tailMinEvents = 8
	tailMinRuns   = 3
	// cacheEvictPct is the share of a model's window requests whose entries
	// were evicted before the cache reads as undersized for the working set.
	cacheEvictPct = 5.0
	// dispatchFloorUs is the dispatch wait per run below which queueing for a
	// session is not worth acting on.
	dispatchFloorUs = 1000
)

// Advise derives the node's course of action: one action, its evidence, and the
// action the same evidence rejects. It is advisory like the sizing
// recommendation: it never moves the health status.
func Advise(in AdviceInput) Advice {
	if len(in.Window) < 2 {
		return Advice{}
	}
	adv := Advice{Phases: in.Split, CachedPct: in.Paths.Share(), CPUPerRun: CPUUsPerRun(in.Window, in.Split.Runs)}
	adv.Phase, adv.Share = in.Split.Dominant()
	adv.CachedPct = cachedShareOfWindow(in)

	diagnosis := RecommendNode(in.Window)

	// The tail this window can support. A thin one is stated, never acted on.
	thin := in.Paths.Attributed > 0 && (in.Paths.Total < tailMinEvents || (in.Paths.Reran > 0 && in.Paths.Reran < tailMinRuns))

	switch diagnosis.Kind {
	case ResourceHost:
		adv.Action = ActionHostCPU
		adv.Text = diagnosis.Text
		adv.Rejected, adv.Why = "workers", "the kernel never gave the CPU over"
		return adv
	case ResourceCPU:
		adv.Action = ActionCPUBudget
		adv.Text = diagnosis.Text
		adv.Rejected, adv.Why = "workers", "callers are refused at the gate"
		return adv
	case ResourceMemory:
		adv.Action = ActionMemory
		adv.Text = diagnosis.Text
		return adv
	case ResourceGrowth:
		adv.Action = ActionGrowth
		adv.Text = diagnosis.Text
		return adv
	}

	// No resource binds. The tail decides, when there is one worth deciding on.
	if thin {
		adv.Thin = true
		adv.Text = fmt.Sprintf("no attribution: the p95 rests on %d events, %d of them runs — too thin to act on",
			in.Paths.Total, in.Paths.Reran)
		return adv
	}

	// Entries the cache dropped are work the node did not have to do. A
	// reservable fix comes before the layout and the model, and before any
	// capacity that would be bought for traffic the cache can answer.
	if pct, model := cacheUndersized(in.Models, in.Window); pct >= cacheEvictPct {
		adv.Action = ActionCache
		adv.Text = fmt.Sprintf("%s: the cache evicted %.0f%% of its lookups, %.0f%% served from cache",
			model, pct, adv.CachedPct)
		adv.Rejected, adv.Why = ActionCPUBudget, "capacity would buy work the cache can answer"
		return adv
	}

	if adv.Phase == PhaseRun && adv.Share >= phaseDominance {
		// The tail is inference itself. Name the model holding it when one
		// does; the action is the same either way.
		subject := "runs"
		if dominated, model := modelTailingOn(in, PhaseRun, phaseDominance); dominated {
			subject = model + ": runs"
		}
		adv.Action = ActionModel
		adv.Text = fmt.Sprintf("%s hold %.0f%% of the uncached tail (p95 %s)",
			subject, adv.Share*100, FormatLatency(in.UncachedP95Us))
		adv.Rejected, adv.Why = ActionCPUBudget, "the time is inference, not queueing"
		return adv
	}

	if adv.Phase == PhaseAdmission && adv.Share >= phaseDominance && adv.Share > 0 {
		// The gate holds the tail while the budget is not refused: the layout,
		// not the capacity, is what serializes.
		adv.Action = ActionThreads
		adv.Text = fmt.Sprintf("the cpu gate holds %.0f%% of the uncached tail (%s per run)",
			adv.Share*100, FormatLatency(adv.Phases.MeanUs(PhaseAdmission).Microseconds()))
		adv.Rejected, adv.Why = ActionCPUBudget, "the layout serializes, not the allowance"
		return adv
	}

	if adv.Phase == PhaseDispatch && adv.Share >= phaseDominance {
		mean := adv.Phases.MeanUs(PhaseDispatch).Microseconds()
		if mean >= dispatchFloorUs {
			if batchedModels(in.Raw) {
				adv.Action = ActionBatch
				adv.Text = fmt.Sprintf("session dispatch holds %.0f%% of the uncached tail (%s per run)",
					adv.Share*100, FormatLatency(mean))
			} else {
				adv.Action = ActionWorkers
				adv.Text = fmt.Sprintf("session dispatch holds %.0f%% of the uncached tail (%s per run)",
					adv.Share*100, FormatLatency(mean))
			}
			adv.Rejected, adv.Why = ActionCPUBudget,
				fmt.Sprintf("queueing dwarfs the gate wait (%.0f%% of the tail) and nobody was refused", shareOf(in.Split.AdmissionWaitUs, in.Split.TotalUs()))
			return adv
		}
	}

	adv.Action = ActionNone
	return adv
}

// modelTailingOn reports whether one model's runs dominate the node's measured
// run time, and which model that is.
func modelTailingOn(in AdviceInput, phase Phase, minShare float64) (bool, string) {
	if in.Split.TotalUs() <= 0 || minShare <= 0 {
		return false, ""
	}
	type row struct {
		name string
		us   int64
	}
	rows := make([]row, 0, len(in.Models))
	var total int64
	for name, hist := range in.Models {
		split := ModelPhaseSplit(hist)
		var us int64
		switch phase {
		case PhaseRun:
			us = split.RunUs
		case PhaseDispatch:
			us = split.DispatchWaitUs
		case PhaseAdmission:
			us = split.AdmissionWaitUs
		}
		if us <= 0 {
			continue
		}
		rows = append(rows, row{name, us})
		total += us
	}
	if total <= 0 {
		return false, ""
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].us > rows[j].us })
	if share := float64(rows[0].us) / float64(total); share >= minShare {
		return true, rows[0].name
	}
	return false, ""
}

// batchedModels reports whether the node's models queue in a batch window
// rather than for a free worker, so the advice names the knob the server
// actually has. A model with a batch timeout is served by the batcher.
func batchedModels(raw map[string]*ModelStats) bool {
	for _, ms := range raw {
		if ms.BatchingTimeout > 0 {
			return true
		}
	}
	return false
}

// cacheUndersized reports the model whose entries the window dropped, as a
// share of that model's own lookups, and its name. It is the signal that the
// working set, not the CPU, is what the window was short of: entries are being
// evicted, so the traffic would have been cache-served had they fit.
func cacheUndersized(models map[string][]ModelPoint, window []Point) (float64, string) {
	worstPct, worstModel := 0.0, ""
	for name, hist := range models {
		if len(hist) < 2 {
			continue
		}
		first, last := hist[0], hist[len(hist)-1]
		// A hit is a request too, so the requests served are the lookups: adding
		// the hits again would halve the share the rule is graded on.
		lookups := delta(first.Requests, last.Requests)
		if lookups <= 0 {
			continue
		}
		pct := float64(delta(first.CacheEvictions, last.CacheEvictions)) / float64(lookups) * 100
		if pct > worstPct {
			worstPct, worstModel = pct, name
		}
	}
	return worstPct, worstModel
}

// shareOf is part as a percentage of whole, 0 when whole is not positive.
func shareOf(part, whole int64) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

// cachedShareOfWindow is the share of the window's cache lookups the cache
// answered, from the node's own cumulative counters.
func cachedShareOfWindow(in AdviceInput) float64 {
	if len(in.Window) < 2 {
		return in.Paths.Share()
	}
	pct, _, requests := CachedShare(in.Window)
	if requests <= 0 {
		return in.Paths.Share()
	}
	return pct
}

// AdviceText renders one line for the node's advice: the action and its
// evidence, then the action the same evidence rejects and the counter that
// ruled it out. A renderer with room for two lines uses AdviceHead and
// AdviceRejected instead.
func AdviceText(a Advice) string {
	line := AdviceHead(a)
	if rejected := AdviceRejected(a); rejected != "" {
		if line != "" {
			line += " · " + rejected
		} else {
			line = rejected
		}
	}
	return strings.TrimSpace(line)
}

// AdviceHead is the action and the counters that decided it.
func AdviceHead(a Advice) string {
	if a.Empty() {
		return a.Text
	}
	return strings.TrimSpace(actionVerb(a.Action) + a.Text)
}

// AdviceRejected is the action the same evidence rejects and the counter that
// ruled it out, or "" when the evidence supports no rival.
func AdviceRejected(a Advice) string {
	if a.Empty() || a.Rejected == "" {
		return ""
	}
	return "not " + ActionName(a.Rejected) + ": " + a.Why
}

// ActionName is the operator-facing short name of an action, for the sentence
// that rules it out.
func ActionName(action string) string {
	switch action {
	case ActionCPUBudget:
		return "more cpu"
	case ActionWorkers:
		return "more workers"
	case ActionBatch:
		return "a wider batch window"
	case ActionThreads:
		return "a wider layout"
	case ActionCache:
		return "a bigger cache"
	case ActionModel:
		return "a different model"
	case ActionHostCPU:
		return "a host change"
	case ActionNodes:
		return "more nodes"
	case ActionMemory:
		return "more memory"
	default:
		return action
	}
}

// isCapacity reports whether an action buys capacity rather than repairing a
// setting: a repair outranks capacity, while two capacity answers are named
// together.
func isCapacity(action string) bool {
	return action == ActionCPUBudget || action == ActionNodes
}

// actionVerb prefixes an advice line with what to do about it.
func actionVerb(action string) string {
	switch action {
	case ActionCPUBudget:
		return "raise cpu_budget — "
	case ActionHostCPU:
		return "raise the node's CPU entitlement — "
	case ActionThreads:
		return "lower intra_op_threads — "
	case ActionWorkers:
		return "raise workers — "
	case ActionBatch:
		return "widen the batch window — "
	case ActionCache:
		return "grow the cache — "
	case ActionModel:
		return "take work off the model — "
	case ActionMemory:
		return "raise memory — "
	case ActionGrowth:
		return "debug the growth — "
	default:
		return ""
	}
}
