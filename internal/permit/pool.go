// Package permit provides a resizable concurrency pool over a fixed set of
// resources, plus the traffic classifier and controller that drive it. It has
// no dependency on the rest of emb so both the scripted path and tests can use
// it.
package permit

import "sync"

// Pool hands out leases over a fixed set of items. Capacity is the maximum
// number of concurrent leases and can grow or shrink at runtime; the item set
// never changes. An item may be leased more than once when capacity exceeds
// the item count, which is how several runs share one concurrency-safe session.
//
// Invariant: len(available) == capacity - leased. SetCapacity preserves it by
// only adding or removing free permits, so an in-flight lease is never revoked.
type Pool[T any] struct {
	mu          sync.Mutex
	cond        *sync.Cond
	items       []T
	available   []T
	capacity    int
	leased      int
	maxCapacity int
	next        int // round-robin cursor over items for added permits
}

// New builds a pool over items with the given initial and maximum capacity.
// Both are clamped into [1, maxCapacity], and maxCapacity into [1, len(items)]
// only when it is below that.
func New[T any](items []T, capacity, maxCapacity int) *Pool[T] {
	if len(items) == 0 {
		panic("permit: pool needs at least one item")
	}
	if maxCapacity < 1 {
		maxCapacity = 1
	}
	if capacity < 1 {
		capacity = 1
	}
	if capacity > maxCapacity {
		capacity = maxCapacity
	}
	p := &Pool[T]{items: items, maxCapacity: maxCapacity}
	p.cond = sync.NewCond(&p.mu)
	p.capacity = capacity
	p.fillLocked(capacity)
	return p
}

// fillLocked appends n item references to the free list, round-robin.
func (p *Pool[T]) fillLocked(n int) {
	for i := 0; i < n; i++ {
		p.available = append(p.available, p.items[p.next%len(p.items)])
		p.next++
	}
}

// Acquire blocks until a lease is available and returns the leased item.
func (p *Pool[T]) Acquire() T {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.leased >= p.capacity || len(p.available) == 0 {
		p.cond.Wait()
	}
	item := p.available[len(p.available)-1]
	p.available = p.available[:len(p.available)-1]
	p.leased++
	return item
}

// Release returns a leased item to the pool. If the pool shrank while the
// lease was in flight, the returned permit may be surplus and is dropped.
func (p *Pool[T]) Release(item T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.leased--
	if want := p.capacity - p.leased; len(p.available) < want {
		p.available = append(p.available, item)
	}
	p.cond.Broadcast()
}

// SetCapacity changes the maximum concurrent leases, clamped to [1, max]. A
// shrink takes effect as leases return: it removes only free permits.
func (p *Pool[T]) SetCapacity(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n < 1 {
		n = 1
	}
	if n > p.maxCapacity {
		n = p.maxCapacity
	}
	if n == p.capacity {
		return
	}
	if n > p.capacity {
		p.fillLocked(n - p.capacity)
	} else {
		drop := p.capacity - n
		if drop > len(p.available) {
			drop = len(p.available)
		}
		p.available = p.available[:len(p.available)-drop]
	}
	p.capacity = n
	p.cond.Broadcast()
}

// Capacity returns the current maximum concurrent leases.
func (p *Pool[T]) Capacity() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.capacity
}

// MaxCapacity returns the configured ceiling.
func (p *Pool[T]) MaxCapacity() int { return p.maxCapacity }

// InFlight returns the number of leases currently held.
func (p *Pool[T]) InFlight() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.leased
}

// Available returns the number of free leases.
func (p *Pool[T]) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.available)
}
