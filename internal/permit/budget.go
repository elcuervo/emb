package permit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrBusy is returned when the admission queue is at its bound: the caller is
// refused rather than queued, so overload becomes a bounded error.
var ErrBusy = errors.New("permit: admission queue is full")

// IsAdmissionError reports that err is a refusal or an abandoned wait at this
// gate, so no native call ran for it. Callers keep such work out of their run
// counters and run-time totals.
func IsAdmissionError(err error) bool {
	return errors.Is(err, ErrBusy) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

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
	waitUs      int64 // cumulative time spent queueing, whether then admitted or abandoned
	timeouts    int64 // cumulative waits that ended without a grant
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

// AcquireContext blocks until a permit of the given width is available, the
// context is done, or the queue bound refuses the caller. On success it returns
// the release function, which must be called exactly once and is safe to call
// more than once.
func (b *Budget) AcquireContext(ctx context.Context, width int) (func(), error) {
	release, _, err := b.AcquireContextWait(ctx, width)
	return release, err
}

// AcquireContextWait is AcquireContext and also reports how long the caller
// queued: zero when it was admitted on the spot, so a run that never waited can
// report no wait rather than the cost of asking. The time is accumulated for the
// stats surface whether or not the caller was admitted.
func (b *Budget) AcquireContextWait(ctx context.Context, width int) (func(), time.Duration, error) {
	if width < 1 {
		width = 1
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	// start is read only on the parked branch (see below).
	w := &budgetWaiter{width: width, ready: make(chan struct{}, 1)}

	b.mu.Lock()
	if b.maxWaiters > 0 && len(b.waiters) >= b.maxWaiters {
		b.mu.Unlock()
		atomic.AddInt64(&b.refused, 1)
		return nil, 0, ErrBusy
	}
	b.waiters = append(b.waiters, w)
	b.dispatchLocked()
	// Released under the lock without queueing: nothing waited, so nothing is
	// recorded and the context is not consulted again.
	queued := !w.admitted
	b.mu.Unlock()
	if !queued {
		return b.releaseFunc(width), 0, nil
	}
	// Only a caller that actually parks pays for the clock, and it is read after
	// the lock is released: the fast path stays clock-free and a queued caller
	// does not lengthen the critical section everyone else waits on.
	start := time.Now()

	select {
	case <-w.ready:
		waited := b.recordWait(start, false)
		return b.releaseFunc(width), waited, nil
	case <-ctx.Done():
		// The context may have expired in the same instant the waiter was
		// admitted; whichever wins, exactly one of the two paths owns it.
		b.mu.Lock()
		if w.admitted {
			b.mu.Unlock()
			waited := b.recordWait(start, false)
			return b.releaseFunc(width), waited, nil
		}
		b.removeLocked(w)
		b.mu.Unlock()
		waited := b.recordWait(start, true)
		return nil, waited, ctx.Err()
	}
}

// recordWait accumulates a wait that has ended — expired marks a wait that ended
// without a grant — and returns it.
func (b *Budget) recordWait(start time.Time, expired bool) time.Duration {
	waited := time.Since(start)
	atomic.AddInt64(&b.waitUs, waited.Microseconds())
	if expired {
		atomic.AddInt64(&b.timeouts, 1)
	}
	return waited
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

// Pressure is the gate's own view of load, for the stats surface: how long
// callers queued, how many are queueing now, and how many waits ended without a
// grant (refused at the bound or abandoned by their caller).
type Pressure struct {
	WaitUs   int64
	Waiters  int
	Timeouts int
	Refused  int64
}

// Pressure reports the gate's cumulative queueing time, its current queue depth
// and the waits that ended without a grant: how loaded admission was, separate
// from how long the runs it admitted took.
func (b *Budget) Pressure() Pressure {
	b.mu.Lock()
	waiters := len(b.waiters)
	b.mu.Unlock()
	return Pressure{
		WaitUs:   atomic.LoadInt64(&b.waitUs),
		Waiters:  waiters,
		Timeouts: int(atomic.LoadInt64(&b.timeouts)),
		Refused:  atomic.LoadInt64(&b.refused),
	}
}
