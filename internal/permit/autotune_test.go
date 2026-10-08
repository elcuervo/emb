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
	c := NewController(4, 1, 2, 3)
	if a, changed, _ := c.Observe(ClassThroughput); changed || a != 1 {
		t.Fatalf("first window changed=%v allowance=%d, want no change at 1", changed, a)
	}
	a, changed, _ := c.Observe(ClassThroughput)
	if !changed || a != 2 {
		t.Fatalf("second window changed=%v allowance=%d, want change to 2", changed, a)
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
	for i := 0; i < 6; i++ {
		if _, changed, _ := c.Observe(ClassIdle); changed {
			t.Fatalf("idle changed the allowance on window %d", i)
		}
	}
	if a := c.Allowance(); a != 4 {
		t.Fatalf("idle moved allowance to %d, want 4 held", a)
	}
}

func TestControllerDoublesToCap(t *testing.T) {
	c := NewController(8, 1, 1, 3)
	if a, changed, _ := c.Observe(ClassThroughput); !changed || a != 2 {
		t.Fatalf("first throughput window = %d/%v, want 2/true", a, changed)
	}
	if a, _, _ := c.Observe(ClassThroughput); a != 4 {
		t.Fatalf("second throughput window = %d, want 4", a)
	}
	if a, _, _ := c.Observe(ClassThroughput); a != 8 {
		t.Fatalf("third throughput window = %d, want 8", a)
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
	c := NewController(4, 1, 2, 3)
	for i := 0; i < 6; i++ {
		c.Observe(ClassThroughput)
		c.Observe(ClassLatency)
	}
	if a := c.Allowance(); a != 1 {
		t.Fatalf("alternating load moved allowance to %d, want 1", a)
	}
}
