package permit

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	const window = time.Second
	const cores = 10

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
