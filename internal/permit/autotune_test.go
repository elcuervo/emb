package permit

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	const window = time.Second
	const cores = 10.0

	base := Sample{Runs: 100, DispatchWaitUs: 1000, RunUs: 10000, InFlight: 1, Sessions: 2, CPUUserUsec: 100, CPUSysUsec: 0}

	cases := []struct {
		name string
		cur  Sample
		want Class
	}{
		{
			name: "idle when no runs completed",
			cur:  base,
			want: ClassIdle,
		},
		{
			name: "latency under light load",
			cur:  Sample{Runs: 101, DispatchWaitUs: 1010, RunUs: 10100, InFlight: 1, Sessions: 2, CPUUserUsec: 200},
			want: ClassLatency,
		},
		{
			name: "missing CPU accounting is unmeasured, never headroom",
			// Both CPU readings are unchanged (accounting unavailable): in-flight
			// exceeds sessions, which would be throughput and grow concurrency,
			// but without measured CPU there is no headroom to claim.
			cur:  Sample{Runs: 101, DispatchWaitUs: 1010, RunUs: 10100, InFlight: 4, Sessions: 2, CPUUserUsec: 100},
			want: ClassUnmeasured,
		},
		{
			name: "throughput when in-flight exceeds sessions",
			cur:  Sample{Runs: 101, DispatchWaitUs: 1010, RunUs: 10100, InFlight: 4, Sessions: 2, CPUUserUsec: 200},
			want: ClassThroughput,
		},
		{
			name: "throughput when wait is a material share of run time",
			cur:  Sample{Runs: 101, DispatchWaitUs: 1040, RunUs: 10100, InFlight: 2, Sessions: 2, CPUUserUsec: 200},
			want: ClassThroughput, // waitPerRun 40 / runPerRun 100 = 0.4
		},
		{
			name: "saturated when CPU is the bottleneck",
			cur:  Sample{Runs: 101, DispatchWaitUs: 1010, RunUs: 10100, InFlight: 4, Sessions: 2, CPUUserUsec: uint64(100 + window.Microseconds()*cores)},
			want: ClassSaturated,
		},
	}
	for _, c := range cases {
		if got := Classify(base, c.cur, window, cores, 0.9, 0.25); got != c.want {
			t.Fatalf("%s: class = %s, want %s", c.name, got, c.want)
		}
	}
}

// A fully busy half-CPU quota consumes 0.5 CPU-seconds per second: that is full
// utilization of the bandwidth, not 50% of a rounded-up CPU.
func TestClassifyNormalizesByFractionalBandwidth(t *testing.T) {
	base := Sample{Runs: 100, CPUUserUsec: 1000}
	cur := Sample{Runs: 200, CPUUserUsec: 1000 + 500_000, InFlight: 4, Sessions: 1}
	if got := Classify(base, cur, time.Second, 0.5, 0.9, 0.25); got != ClassSaturated {
		t.Fatalf("saturated half-CPU window = %s, want %s", got, ClassSaturated)
	}
	// The same CPU time against a whole CPU is genuinely half idle.
	if got := Classify(base, cur, time.Second, 1, 0.9, 0.25); got == ClassSaturated {
		t.Fatalf("0.5 CPU-seconds of 1 CPU = %s, want not saturated", got)
	}
}

// A sample delayed to two elapsed seconds is normalized by those two seconds,
// not by the nominal one-second ticker.
func TestClassifyUsesActualElapsedTime(t *testing.T) {
	base := Sample{Runs: 100, CPUUserUsec: 1000}
	cur := Sample{Runs: 200, CPUUserUsec: 1000 + 1_000_000, InFlight: 4, Sessions: 1}
	if got := Classify(base, cur, time.Second, 1, 0.9, 0.25); got != ClassSaturated {
		t.Fatalf("one-second window = %s, want %s", got, ClassSaturated)
	}
	if got := Classify(base, cur, 2*time.Second, 1, 0.9, 0.25); got == ClassSaturated {
		t.Fatalf("same CPU over two elapsed seconds = %s, want not saturated", got)
	}
	if got := Classify(base, cur, 0, 1, 0.9, 0.25); got != ClassUnmeasured {
		t.Fatalf("zero elapsed = %s, want %s", got, ClassUnmeasured)
	}
}

func TestControllerHoldsWhenUnmeasured(t *testing.T) {
	c := NewController(4, 2, 1, 1)
	for range 3 {
		if a, changed, recommend := c.Observe(ClassUnmeasured); changed || recommend || a != 2 {
			t.Fatalf("unmeasured window: allowance=%d changed=%v recommend=%v, want a held 2", a, changed, recommend)
		}
	}
}

func TestControllerGrowsUnderBurst(t *testing.T) {
	c := NewController(4, 1, 2, 10)
	if a, changed, _ := c.Observe(ClassThroughput); changed || a != 1 {
		t.Fatalf("first window changed=%v allowance=%d, want no change at 1", changed, a)
	}
	a, changed, _ := c.Observe(ClassThroughput)
	if !changed || a != 4 {
		t.Fatalf("second window changed=%v allowance=%d, want change to the cap 4", changed, a)
	}
}

func TestControllerShrinksWhenQuiet(t *testing.T) {
	c := NewController(4, 4, 2, 3)
	for i := 0; i < 2; i++ {
		if _, changed, _ := c.Observe(ClassLatency); changed {
			t.Fatalf("changed on window %d, want dwell", i)
		}
	}
	a, changed, _ := c.Observe(ClassLatency)
	if !changed || a != 2 {
		t.Fatalf("third quiet window changed=%v allowance=%d, want change to 2", changed, a)
	}
}

func TestControllerIdleHolds(t *testing.T) {
	c := NewController(4, 4, 2, 3)
	last := 4
	for i := 0; i < 6; i++ {
		a, changed, _ := c.Observe(ClassIdle)
		if changed {
			t.Fatalf("idle changed the allowance on window %d", i)
		}
		last = a
	}
	if last != 4 {
		t.Fatalf("idle moved allowance to %d, want 4 held", last)
	}
}

func TestControllerJumpsToCap(t *testing.T) {
	c := NewController(8, 1, 1, 10)
	if a, changed, _ := c.Observe(ClassThroughput); !changed || a != 8 {
		t.Fatalf("first throughput window = %d/%v, want 8/true", a, changed)
	}
	if a, changed, _ := c.Observe(ClassThroughput); changed || a != 8 {
		t.Fatalf("allowance past cap: %d/%v, want 8 held", a, changed)
	}
}

func TestControllerRespectsCap(t *testing.T) {
	c := NewController(2, 2, 2, 3)
	for i := 0; i < 4; i++ {
		if a, changed, _ := c.Observe(ClassThroughput); changed || a != 2 {
			t.Fatalf("allowance moved past cap: changed=%v allowance=%d", changed, a)
		}
	}
}

func TestControllerSaturationRecommendsOnce(t *testing.T) {
	c := NewController(4, 4, 2, 3)
	if _, changed, rec := c.Observe(ClassSaturated); !rec || changed {
		t.Fatalf("first saturated: changed=%v recommend=%v, want recommend once", changed, rec)
	}
	if _, _, rec := c.Observe(ClassSaturated); rec {
		t.Fatal("saturated recommended twice")
	}
}

func TestControllerDampsAlternatingLoad(t *testing.T) {
	c := NewController(4, 1, 2, 10)
	last := 1
	for i := 0; i < 6; i++ {
		c.Observe(ClassThroughput)
		last, _, _ = c.Observe(ClassLatency)
	}
	if last != 1 {
		t.Fatalf("alternating load moved allowance to %d, want 1", last)
	}
}
