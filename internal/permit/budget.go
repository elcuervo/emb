package permit

import "sync"

// Budget admits runs by their configured intra-op width. Waiters are FIFO;
// an oversized run is admitted alone.
type Budget struct {
	mu       sync.Mutex
	cond     *sync.Cond
	capacity int
	used     int
	peak     int
	waiters  []*budgetWaiter
}

type budgetWaiter struct {
	width int
}

func NewBudget(capacity int) *Budget {
	if capacity < 1 {
		capacity = 1
	}
	b := &Budget{capacity: capacity}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *Budget) Acquire(width int) func() {
	if width < 1 {
		width = 1
	}
	w := &budgetWaiter{width: width}
	b.mu.Lock()
	b.waiters = append(b.waiters, w)
	for b.waiters[0] != w || !b.canAdmit(width) {
		b.cond.Wait()
	}
	b.waiters = b.waiters[1:]
	b.used += width
	if b.used > b.peak {
		b.peak = b.used
	}
	b.cond.Broadcast()
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		b.used -= width
		b.cond.Broadcast()
		b.mu.Unlock()
	}
}

func (b *Budget) canAdmit(width int) bool {
	if width > b.capacity {
		return b.used == 0
	}
	return b.used+width <= b.capacity
}

func (b *Budget) Capacity() int { return b.capacity }

func (b *Budget) Stats() (active, peak int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.peak
}
