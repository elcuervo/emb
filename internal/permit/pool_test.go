package permit

import (
	"testing"
	"time"
)

func TestPoolAcquireRelease(t *testing.T) {
	p := New([]int{1, 2}, 2, 4)
	a, b := p.Acquire(), p.Acquire()
	if p.InFlight() != 2 {
		t.Fatalf("in flight = %d, want 2", p.InFlight())
	}
	p.Release(a)
	p.Release(b)
	if p.InFlight() != 0 {
		t.Fatalf("in flight = %d, want 0", p.InFlight())
	}
	if p.Available() != 2 {
		t.Fatalf("available = %d, want 2", p.Available())
	}
}

func TestPoolCapacityClamps(t *testing.T) {
	p := New([]int{1}, 4, 2)
	if p.Capacity() != 2 {
		t.Fatalf("initial capacity = %d, want 2 (clamped to max)", p.Capacity())
	}
	p.SetCapacity(10)
	if p.Capacity() != 2 {
		t.Fatalf("over-max capacity = %d, want 2", p.Capacity())
	}
	p.SetCapacity(0)
	if p.Capacity() != 1 {
		t.Fatalf("under-min capacity = %d, want 1", p.Capacity())
	}
}

// TestPoolShrinkNeverRevokesInFlight proves a shrink only lowers the ceiling:
// held leases complete, new ones wait, and the free-list invariant holds.
func TestPoolShrinkNeverRevokesInFlight(t *testing.T) {
	p := New([]int{1}, 3, 3)
	leases := []int{p.Acquire(), p.Acquire(), p.Acquire()}
	p.SetCapacity(1)
	if p.Capacity() != 1 || p.InFlight() != 3 {
		t.Fatalf("after shrink: capacity=%d in-flight=%d, want 1/3", p.Capacity(), p.InFlight())
	}

	got := make(chan int, 1)
	go func() { got <- p.Acquire() }()
	select {
	case <-got:
		t.Fatal("acquired while over capacity")
	case <-time.After(50 * time.Millisecond):
	}

	for _, it := range leases {
		p.Release(it)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("acquire did not unblock after releases")
	}
	if p.InFlight() != 1 || p.Available() != 0 {
		t.Fatalf("after re-acquire: in-flight=%d available=%d, want 1/0", p.InFlight(), p.Available())
	}
}

func TestPoolMultiLeaseSharesItems(t *testing.T) {
	p := New([]string{"only"}, 4, 4)
	seen := map[string]int{}
	for range 4 {
		seen[p.Acquire()]++
	}
	if seen["only"] != 4 {
		t.Fatalf("leases per item = %v, want 4", seen)
	}
}
