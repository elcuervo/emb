package permit

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrAdmissionTimeout is returned when a wait at the gate outlives the
	// configured admission timeout. The text is the wire reply after "ERR ".
	ErrAdmissionTimeout = errors.New("busy: CPU admission timeout")
	// ErrAdmissionClosed is returned to waiters (and later callers) once the
	// budget is closed for shutdown.
	ErrAdmissionClosed = errors.New("CPU admission closed")
)

// Budget admits runs by their intra-op width against one process capacity.
//
// Each model has its own FIFO queue. Across models, admission is work
// conserving: a free token is never held back for an idle model, and the next
// run is the head whose model has the lowest weighted pass (stride scheduling,
// one turn costs 1/weight). A model that was idle rejoins at the current
// frontier, so idleness earns no credit. A head that does not fit is skipped
// for fitting heads of other models, but once it has been skipped for a full
// round of the active models it is served next and everyone waits for the
// budget to drain to it, so a wide run is never starved. An oversized run
// (width > capacity) is admitted alone.
//
// A wait ends with the caller's context, the optional timeout or Close; the
// waiter then leaves its queue under the lock and wakes successors. A grant
// and a cancellation that race have one owner: whoever finds the waiter's
// ready channel closed under the lock (the grant) wins and the cancellation is
// ignored, so no token leaks.
type Budget struct {
	mu       sync.Mutex
	capacity int
	used     int
	peak     int
	waiting  int
	closed   bool
	timeout  time.Duration
	waitUs   atomic.Int64
	timeouts atomic.Int64
	weights  map[string]float64
	models   map[string]*modelQueue
}

type modelQueue struct {
	name    string
	weight  float64
	pass    float64
	waiters []*budgetWaiter
}

type budgetWaiter struct {
	width    int
	bypassed int
	ready    chan struct{}
	// err is set before ready closes when the wait was ended by Close.
	err error
}

// NewBudget builds a budget of the given capacity (at least 1). weights maps a
// model name to its fair-share weight; unknown or non-positive weights are 1.
func NewBudget(capacity int, weights map[string]float64) *Budget {
	if capacity < 1 {
		capacity = 1
	}
	return &Budget{capacity: capacity, weights: weights, models: map[string]*modelQueue{}}
}

// IsAdmissionError reports that err is a refused or expired CPU admission, so
// no native call ran.
func IsAdmissionError(err error) bool {
	return errors.Is(err, ErrAdmissionTimeout) || errors.Is(err, ErrAdmissionClosed)
}

// SetTimeout bounds every wait at the gate; zero is unlimited. Call it before
// the budget is shared.
func (b *Budget) SetTimeout(d time.Duration) { b.timeout = d }

// bound derives the context a wait runs under: the caller's, limited by the
// admission timeout. Its cause is ErrAdmissionTimeout on expiry.
func (b *Budget) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	if b.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeoutCause(ctx, b.timeout, ErrAdmissionTimeout)
}

// abort reports why a bounded wait ended and counts a timeout.
func (b *Budget) abort(ctx context.Context) error {
	err := context.Cause(ctx)
	if errors.Is(err, ErrAdmissionTimeout) {
		b.timeouts.Add(1)
	}
	return err
}

// Acquire blocks until width tokens are granted to model and returns the
// release. width below 1 counts as 1. It fails, holding nothing, when ctx ends,
// the admission timeout expires or the budget is closed.
func (b *Budget) Acquire(ctx context.Context, model string, width int) (func(), error) {
	if width < 1 {
		width = 1
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrAdmissionClosed
	}
	q := b.queue(model)
	if len(q.waiters) == 0 && b.idle() && b.fits(width) {
		// Nothing is waiting anywhere, so admitting now cannot reorder anyone.
		b.grantLocked(q, width)
		b.mu.Unlock()
		return b.releaseFunc(width), nil
	}
	w := &budgetWaiter{width: width, ready: make(chan struct{})}
	if len(q.waiters) == 0 {
		q.pass = max(q.pass, b.frontier())
	}
	q.waiters = append(q.waiters, w)
	b.waiting++
	b.dispatchLocked()
	b.mu.Unlock()

	start := time.Now()
	ctx, cancel := b.bound(ctx)
	defer cancel()
	select {
	case <-w.ready:
	case <-ctx.Done():
		b.mu.Lock()
		select {
		case <-w.ready: // granted (or closed) first: that outcome stands
		default:
			b.removeLocked(q, w)
			b.dispatchLocked()
			b.mu.Unlock()
			b.waitUs.Add(time.Since(start).Microseconds())
			return nil, b.abort(ctx)
		}
		b.mu.Unlock()
	}
	b.waitUs.Add(time.Since(start).Microseconds())
	if w.err != nil {
		return nil, w.err
	}
	return b.releaseFunc(width), nil
}

// removeLocked takes an ungranted waiter out of its queue.
func (b *Budget) removeLocked(q *modelQueue, w *budgetWaiter) {
	for i, x := range q.waiters {
		if x == w {
			q.waiters = append(q.waiters[:i], q.waiters[i+1:]...)
			b.waiting--
			return
		}
	}
}

// Close fails every waiter and every later Acquire. Runs that already hold
// tokens are untouched and release normally.
func (b *Budget) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, q := range b.models {
		for _, w := range q.waiters {
			w.err = ErrAdmissionClosed
			close(w.ready)
		}
		b.waiting -= len(q.waiters)
		q.waiters = nil
	}
}

// Pressure reports cumulative wait at the gate, the current number of waiters
// and cumulative timeouts.
func (b *Budget) Pressure() (waitUs int64, waiters int, timeouts int64) {
	b.mu.Lock()
	waiters = b.waiting
	b.mu.Unlock()
	return b.waitUs.Load(), waiters, b.timeouts.Load()
}

func (b *Budget) releaseFunc(width int) func() {
	return func() {
		b.mu.Lock()
		b.used -= width
		b.dispatchLocked()
		b.mu.Unlock()
	}
}

func (b *Budget) queue(model string) *modelQueue {
	q, ok := b.models[model]
	if !ok {
		weight := b.weights[model]
		if weight <= 0 {
			weight = 1
		}
		q = &modelQueue{name: model, weight: weight}
		b.models[model] = q
	}
	return q
}

// idle reports that no model has a waiting run.
func (b *Budget) idle() bool {
	for _, q := range b.models {
		if len(q.waiters) > 0 {
			return false
		}
	}
	return true
}

// frontier is the smallest pass among models with waiting runs: where a model
// that just became busy joins.
func (b *Budget) frontier() float64 {
	first, f := true, 0.0
	for _, q := range b.models {
		if len(q.waiters) > 0 && (first || q.pass < f) {
			first, f = false, q.pass
		}
	}
	return f
}

func (b *Budget) fits(width int) bool {
	if width > b.capacity {
		return b.used == 0
	}
	return b.used+width <= b.capacity
}

func (b *Budget) grantLocked(q *modelQueue, width int) {
	b.used += width
	if b.used > b.peak {
		b.peak = b.used
	}
	q.pass += 1 / q.weight
}

// dispatchLocked grants every run that may start now, in scheduling order.
func (b *Budget) dispatchLocked() {
	for {
		q := b.nextLocked()
		if q == nil {
			return
		}
		w := q.waiters[0]
		q.waiters = q.waiters[1:]
		b.waiting--
		b.grantLocked(q, w.width)
		b.bypassLocked(q)
		close(w.ready)
	}
}

// nextLocked picks the model whose head runs next, or nil when nothing may
// start. A starved head that does not fit yet blocks every other grant.
func (b *Budget) nextLocked() *modelQueue {
	active := b.activeLocked()
	var pick *modelQueue
	for _, q := range active {
		if q.waiters[0].bypassed >= len(active) && (pick == nil || q.pass < pick.pass) {
			pick = q
		}
	}
	if pick != nil {
		if b.fits(pick.waiters[0].width) {
			return pick
		}
		return nil
	}
	for _, q := range active {
		if b.fits(q.waiters[0].width) && (pick == nil || q.pass < pick.pass) {
			pick = q
		}
	}
	return pick
}

// bypassLocked records that granted's turn went past every other head that did
// not fit.
func (b *Budget) bypassLocked(granted *modelQueue) {
	for _, q := range b.models {
		if q != granted && len(q.waiters) > 0 && !b.fits(q.waiters[0].width) {
			q.waiters[0].bypassed++
		}
	}
}

// activeLocked returns the models with waiting runs in name order, so ties
// between equal passes resolve the same way every time.
func (b *Budget) activeLocked() []*modelQueue {
	var active []*modelQueue
	for _, q := range b.models {
		if len(q.waiters) > 0 {
			active = append(active, q)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].name < active[j].name })
	return active
}

func (b *Budget) Capacity() int { return b.capacity }

func (b *Budget) Stats() (active, peak int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.peak
}

// Shared accounts the callers of one shared native session. The session's
// intra-op worker pool is active whenever any caller is, so the first caller
// holds its full width; every further concurrent caller adds one token for the
// thread it runs on. The width is released when the last caller leaves.
type Shared struct {
	gate       *Budget
	model      string
	width      int
	mu         sync.Mutex
	active     int
	release    func()
	activating chan struct{} // non-nil while a caller waits for the width
}

// Shared returns the accounting for one session of model with the given
// intra-op width.
func (b *Budget) Shared(model string, width int) *Shared {
	return &Shared{gate: b, model: model, width: width}
}

// Enter admits one caller and returns its exit. It fails, holding nothing,
// when ctx ends, the admission timeout expires or the budget is closed.
func (s *Shared) Enter(ctx context.Context) (func(), error) {
	s.mu.Lock()
	for s.active == 0 {
		if s.activating == nil {
			// One caller waits for the session's width; the others wait on it,
			// not holding the lock, so each can still time out or cancel.
			done := make(chan struct{})
			s.activating = done
			s.mu.Unlock()
			release, err := s.gate.Acquire(ctx, s.model, s.width)
			s.mu.Lock()
			s.activating = nil
			close(done)
			if err != nil {
				s.mu.Unlock()
				return nil, err
			}
			s.release, s.active = release, 1
			s.mu.Unlock()
			return s.exit, nil
		}
		done := s.activating
		s.mu.Unlock()
		bound, cancel := s.gate.bound(ctx)
		select {
		case <-done:
			cancel()
		case <-bound.Done():
			err := s.gate.abort(bound)
			cancel()
			return nil, err
		}
		s.mu.Lock()
	}
	s.active++
	s.mu.Unlock()
	extra, err := s.gate.Acquire(ctx, s.model, 1)
	if err != nil {
		s.exit()
		return nil, err
	}
	return func() {
		extra()
		s.exit()
	}, nil
}

func (s *Shared) exit() {
	s.mu.Lock()
	s.active--
	if s.active == 0 {
		s.release()
		s.release = nil
	}
	s.mu.Unlock()
}
