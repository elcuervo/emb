package main

import (
	"testing"
	"time"
)

// TestRollingWindowRotation covers the rolling counter's normal advance: same
// bucket, one-step rotation, summed buckets, a backwards/no-op advance, and the
// full-window reset.
func TestRollingWindowRotation(t *testing.T) {
	start := time.Unix(0, 0)
	r := newRolling(60*time.Second, 6) // step = 10s

	if got := r.used(start); got != 0 {
		t.Fatalf("fresh window used = %d, want 0", got)
	}
	r.add(start, 5)
	if got := r.used(start.Add(9 * time.Second)); got != 5 {
		t.Fatalf("same-bucket used = %d, want 5", got)
	}
	// One step elapses: the current bucket rotates but the value stays in the
	// window until a full window has passed.
	if got := r.used(start.Add(10 * time.Second)); got != 5 {
		t.Fatalf("after one step used = %d, want 5", got)
	}
	// A second entry lands in the new bucket and sums with the first.
	r.add(start.Add(10*time.Second), 3)
	if got := r.used(start.Add(10 * time.Second)); got != 8 {
		t.Fatalf("summed window = %d, want 8", got)
	}
	// Before `last`, no time has elapsed: advance is a no-op.
	r.advance(start.Add(5 * time.Second))
	if got := r.used(start.Add(10 * time.Second)); got != 8 {
		t.Fatalf("backwards advance changed the window: %d, want 8", got)
	}
	// A full window elapsed: everything resets.
	if got := r.used(start.Add(70 * time.Second)); got != 0 {
		t.Fatalf("after a full window used = %d, want 0", got)
	}
}

// TestRollingFullResetClearsIndex pins the steps >= len(buckets) branch: the
// whole window is cleared and the index returns to zero.
func TestRollingFullResetClearsIndex(t *testing.T) {
	start := time.Unix(0, 0)
	r := newRolling(30*time.Second, 3) // step = 10s
	r.add(start, 2)
	r.add(start.Add(10*time.Second), 4)
	r.add(start.Add(20*time.Second), 6)
	if got := r.used(start.Add(20 * time.Second)); got != 12 {
		t.Fatalf("window total = %d, want 12", got)
	}

	r.advance(start.Add(60 * time.Second))
	if r.total != 0 || r.idx != 0 {
		t.Fatalf("full reset left total=%d idx=%d, want 0/0", r.total, r.idx)
	}
}
