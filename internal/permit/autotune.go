package permit

import "time"

// Class is one model path's observed traffic shape over a sampling window.
type Class string

const (
	// ClassIdle means no inference runs completed in the window.
	ClassIdle Class = "idle"
	// ClassLatency means requests arrive with little or no queueing.
	ClassLatency Class = "latency"
	// ClassThroughput means requests queue (concurrency exceeds sessions or
	// dispatch wait is a material fraction of run time) with CPU headroom.
	ClassThroughput Class = "throughput"
	// ClassSaturated means CPU is the bottleneck; more concurrency cannot help.
	ClassSaturated Class = "saturated"
)

// Sample is one window's cumulative counter reading plus live gauges.
type Sample struct {
	Runs           int64
	DispatchWaitUs int64
	RunUs          int64
	InFlight       int64
	Sessions       int64
	CPUUserUsec    uint64
	CPUSysUsec     uint64
}

// Classify returns the class implied by the delta between two samples over
// window at the given CPU capacity. cores below 1 counts as 1.
func Classify(prev, cur Sample, window time.Duration, cores int, cpuHigh, waitRatio float64) Class {
	runs := cur.Runs - prev.Runs
	if runs <= 0 {
		return ClassIdle
	}
	if cores < 1 {
		cores = 1
	}
	if window > 0 {
		cpuUs := float64((cur.CPUUserUsec - prev.CPUUserUsec) + (cur.CPUSysUsec - prev.CPUSysUsec))
		if cpuUs/float64(window.Microseconds()*int64(cores)) >= cpuHigh {
			return ClassSaturated
		}
	}
	waitPerRun := float64(cur.DispatchWaitUs-prev.DispatchWaitUs) / float64(runs)
	runPerRun := float64(cur.RunUs-prev.RunUs) / float64(runs)
	if cur.InFlight > cur.Sessions {
		return ClassThroughput
	}
	if runPerRun > 0 && waitPerRun/runPerRun >= waitRatio {
		return ClassThroughput
	}
	if runPerRun == 0 && waitPerRun > 0 {
		return ClassThroughput
	}
	return ClassLatency
}

// Controller adapts a per-session concurrency allowance within [1, cap] using
// additive increase and multiplicative decrease, with a dwell of consecutive
// agreeing windows before each change.
type Controller struct {
	cap          int
	allowance    int
	growAfter    int
	shrinkAfter  int
	growStreak   int
	shrinkStreak int
	recommended  bool
}

// NewController builds a controller with the given ceiling and starting
// allowance. allowAfter/shrinkAfter below 1 default to 2 and 3.
func NewController(cap, initial, growAfter, shrinkAfter int) *Controller {
	if cap < 1 {
		cap = 1
	}
	if initial < 1 {
		initial = 1
	}
	if initial > cap {
		initial = cap
	}
	if growAfter < 1 {
		growAfter = 2
	}
	if shrinkAfter < 1 {
		shrinkAfter = 3
	}
	return &Controller{cap: cap, allowance: initial, growAfter: growAfter, shrinkAfter: shrinkAfter}
}

// Observe folds one window's class in and reports the resulting allowance,
// whether it changed, and whether a provisioning recommendation should be
// logged. Throughput expands the allowance straight to the cap after its dwell
// (fast expansion keeps a burst from queueing behind a low allowance);
// sustained latency halves it (bounded below by 1). Idle holds: no traffic is
// not evidence of a latency-sensitive workload.
func (c *Controller) Observe(class Class) (allowance int, changed, recommend bool) {
	switch class {
	case ClassThroughput:
		c.shrinkStreak = 0
		c.growStreak++
		if c.growStreak >= c.growAfter && c.allowance < c.cap {
			c.allowance = c.cap
			c.growStreak = 0
			return c.allowance, true, false
		}
	case ClassLatency:
		c.growStreak = 0
		c.shrinkStreak++
		if c.shrinkStreak >= c.shrinkAfter && c.allowance > 1 {
			c.allowance = max(1, c.allowance/2)
			c.shrinkStreak = 0
			return c.allowance, true, false
		}
	case ClassIdle:
		c.growStreak, c.shrinkStreak = 0, 0
	case ClassSaturated:
		c.growStreak, c.shrinkStreak = 0, 0
		if !c.recommended {
			c.recommended = true
			return c.allowance, false, true
		}
	}
	return c.allowance, false, false
}
