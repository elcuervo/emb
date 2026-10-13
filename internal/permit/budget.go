package permit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrBusy is returned when the admission queue is at its bound: the caller is
// refused rather than queued, so overload becomes a bounded error.
var ErrBusy = errors.New("permit: admission queue is full")

// Budget admits runs by their configured intra-op width against one
// process-wide capacity. Waiters are served in arrival order, and idle
// capacity is borrowable: when the head waiter cannot be admitted but a later
// one fits, the later one proceeds. An oversized run is admitted alone.
//
// Admission is bounded: a caller supplies a context, so a wait ends with the
// deadline or the cancellation instead of blocking forever.
type Budget struct {
	mu          sync.Mutex
	capacity    int
	used        int
	peak        int
	waiters     []*budgetWaiter
	maxWaiters  int   // 0 = unbounded
	borrowBurst int   // followers admitted past a blocked head before the gate drains
	borrowed    int   // borrowed admissions since the head last changed
	refused     int64 // cumulative ErrBusy outcomes
	admitted    int64 // cumulative admissions
}

type budgetWaiter struct {
	width    int
	ready    chan struct{} // buffered 1; closed is never used so release is explicit
	admitted bool          // set under Budget.mu before ready is signalled
}

func NewBudget(capacity int) *Budget {
	if capacity < 1 {
		capacity = 1
	}
	return &Budget{capacity: capacity, borrowBurst: max(2, 2*capacity)}
}

// SetQueueBound limits how many callers may wait for admission at once. Zero or
// less leaves the queue unbounded. Callers past the bound are refused with
// ErrBusy.
func (b *Budget) SetQueueBound(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxWaiters = n
}

// Acquire waits for a permit without a deadline. It is the compatibility entry
// point for call sites that cannot carry a context yet: it blocks until
// admitted and ignores the queue bound.
func (b *Budget) Acquire(width int) func() {
	if width < 1 {
		width = 1
	}
	w := &budgetWaiter{width: width, ready: make(chan struct{}, 1)}
	b.mu.Lock()
	b.waiters = append(b.waiters, w)
	b.dispatchLocked()
	b.mu.Unlock()
	<-w.ready
	return b.releaseFunc(width)
}

// AcquireContext blocks until a permit of the given width is available, the
// context is done, or the queue bound refuses the caller. On success it returns
// the release function, which must be called exactly once and is safe to call
// more than once.
func (b *Budget) AcquireContext(ctx context.Context, width int) (func(), error) {
	if width < 1 {
		width = 1
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w := &budgetWaiter{width: width, ready: make(chan struct{}, 1)}

	b.mu.Lock()
	if b.maxWaiters > 0 && len(b.waiters) >= b.maxWaiters {
		b.mu.Unlock()
		atomic.AddInt64(&b.refused, 1)
		return nil, ErrBusy
	}
	b.waiters = append(b.waiters, w)
	b.dispatchLocked()
	b.mu.Unlock()

	select {
	case <-w.ready:
		return b.releaseFunc(width), nil
	case <-ctx.Done():
		// The context may have expired in the same instant the waiter was
		// admitted; whichever wins, exactly one of the two paths owns it.
		b.mu.Lock()
		if w.admitted {
			b.mu.Unlock()
			return b.releaseFunc(width), nil
		}
		b.removeLocked(w)
		b.mu.Unlock()
		return nil, ctx.Err()
	}
}

// dispatchLocked admits waiters while the capacity allows: the head first, then
// any waiter that fits in the idle capacity it left behind.
func (b *Budget) dispatchLocked() {
	for {
		idx := b.nextAdmissibleLocked()
		if idx < 0 {
			return
		}
		w := b.waiters[idx]
		b.waiters = append(b.waiters[:idx], b.waiters[idx+1:]...)
		b.used += w.width
		if b.used > b.peak {
			b.peak = b.used
		}
		atomic.AddInt64(&b.admitted, 1)
		w.admitted = true
		w.ready <- struct{}{}
	}
}

// nextAdmissibleLocked returns the index of the waiter to admit next: the head
// when it fits, otherwise the earliest waiter that fits the idle capacity. A
// blocked head gives followers at most borrowBurst admissions before the gate
// drains for it, so idle capacity stays usable without starving a wide waiter
// behind a stream of narrow ones.
func (b *Budget) nextAdmissibleLocked() int {
	if len(b.waiters) == 0 {
		return -1
	}
	if b.canAdmit(b.waiters[0].width) {
		b.borrowed = 0
		return 0
	}
	if b.borrowed >= b.borrowBurst {
		return -1
	}
	for i := 1; i < len(b.waiters); i++ {
		if b.canAdmit(b.waiters[i].width) {
			b.borrowed++
			return i
		}
	}
	return -1
}

func (b *Budget) removeLocked(w *budgetWaiter) {
	for i, candidate := range b.waiters {
		if candidate == w {
			b.waiters = append(b.waiters[:i], b.waiters[i+1:]...)
			// The head changed, so the borrow budget starts over.
			b.borrowed = 0
			return
		}
	}
}

func (b *Budget) releaseFunc(width int) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.used -= width
			if b.used < 0 {
				b.used = 0
			}
			b.dispatchLocked()
			b.mu.Unlock()
		})
	}
}

func (b *Budget) canAdmit(width int) bool {
	if width > b.capacity {
		return b.used == 0
	}
	return b.used+width <= b.capacity
}

func (b *Budget) Capacity() int { return b.capacity }

// Stats reports the admitted width and its high-water mark.
func (b *Budget) Stats() (active, peak int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.peak
}

// Counters reports the cumulative admissions and refusals and the current queue
// depth: the gate's own view of load, for the stats surface and its tests.
func (b *Budget) Counters() (admitted, refused int64, queued int) {
	b.mu.Lock()
	queued = len(b.waiters)
	b.mu.Unlock()
	return atomic.LoadInt64(&b.admitted), atomic.LoadInt64(&b.refused), queued
}

// Waiters reports the current queue depth.
func (b *Budget) Waiters() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters)
}
