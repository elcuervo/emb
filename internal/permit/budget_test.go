package permit

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestBudgetDoesNotBargeAroundWideWaiter(t *testing.T) {
	b := NewBudget(4)
	first := mustAcquire(t, b, 4)
	wide := make(chan func(), 1)
	narrow := make(chan func(), 1)
	go func() { wide <- mustAcquire(t, b, 3) }()
	waitBudgetQueue(t, b, 1)
	go func() { narrow <- mustAcquire(t, b, 2) }()
	waitBudgetQueue(t, b, 2)
	first()
	w := <-wide
	select {
	case release := <-narrow:
		release()
		t.Fatal("narrow waiter barged ahead of the wide waiter")
	default:
	}
	w()
	(<-narrow)()
}

func TestBudgetAdmitsOversizedRunAlone(t *testing.T) {
	b := NewBudget(2)
	large := make(chan func(), 1)
	small := make(chan func(), 1)
	go func() { large <- mustAcquire(t, b, 4) }()
	l := <-large
	go func() { small <- mustAcquire(t, b, 1) }()
	select {
	case release := <-small:
		release()
		t.Fatal("run admitted alongside an oversized run")
	default:
	}
	l()
	(<-small)()
}

// mustAcquire waits for a permit without a deadline, the way a call site that
// carries no context does.
func mustAcquire(t *testing.T, b *Budget, width int) func() {
	t.Helper()
	release, err := b.AcquireContext(context.Background(), width)
	if err != nil {
		t.Fatalf("AcquireContext(width=%d): %v", width, err)
	}
	return release
}

func waitBudgetQueue(t *testing.T, b *Budget, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		queued := len(b.waiters)
		b.mu.Unlock()
		if queued == n {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("waiters did not reach %d", n)
}

func TestBudgetAcquireRespectsDeadline(t *testing.T) {
	b := NewBudget(2)
	release := mustAcquire(t, b, 2)
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := b.AcquireContext(ctx, 2)
	if err == nil {
		got()
		t.Fatal("acquire succeeded while the budget was exhausted")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if waited := time.Since(start); waited < 50*time.Millisecond {
		t.Fatalf("returned after %s, before the deadline", waited)
	}
	if active, _ := b.Stats(); active != 2 {
		t.Fatalf("deadline path leaked a permit: active=%d", active)
	}
	if b.Pressure().Waiters != 0 {
		t.Fatalf("expired waiter left in the queue: %d", b.Pressure().Waiters)
	}
}

func TestBudgetAcquireContextCancelled(t *testing.T) {
	b := NewBudget(1)
	release := mustAcquire(t, b, 1)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if _, err := b.AcquireContext(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if b.Pressure().Waiters != 0 {
		t.Fatalf("cancelled waiter left in the queue: %d", b.Pressure().Waiters)
	}
}

func TestBudgetQueueBoundRefusesRatherThanQueues(t *testing.T) {
	b := NewBudget(1)
	b.SetQueueBound(1)
	release := mustAcquire(t, b, 1)
	defer release()

	blocked := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := b.AcquireContext(ctx, 1)
		blocked <- err
	}()
	waitBudgetQueue(t, b, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := b.AcquireContext(ctx, 1); !errors.Is(err, ErrBusy) {
		t.Fatalf("want ErrBusy past the bound, got %v", err)
	}
	if refused := b.Pressure().Refused; refused != 1 {
		t.Fatalf("refusal not counted: %d", refused)
	}
	release()
	if err := <-blocked; err != nil {
		t.Fatalf("the queued waiter failed: %v", err)
	}
}

func TestBudgetBorrowsIdleCapacityPastABlockedHead(t *testing.T) {
	b := NewBudget(4)
	held := mustAcquire(t, b, 2)
	defer held()

	wide := make(chan func(), 1)
	go func() { wide <- mustAcquire(t, b, 3) }()
	waitBudgetQueue(t, b, 1)

	narrow := make(chan func(), 1)
	go func() { narrow <- mustAcquire(t, b, 2) }()

	// The narrow waiter fits the idle capacity, so it must be admitted at once
	// while the wide head stays queued (2 held + 3 does not fit).
	select {
	case release := <-narrow:
		release()
	case <-time.After(time.Second):
		t.Fatal("a narrow waiter that fits idle capacity waited behind a blocked head")
	}
	select {
	case release := <-wide:
		release()
		t.Fatal("the wide waiter was admitted while its width did not fit")
	default:
	}
	if queued := b.Pressure().Waiters; queued != 1 {
		t.Fatalf("want the wide waiter still queued, got %d waiters", queued)
	}
	held()
	(<-wide)()
}

func TestBudgetReleaseIsIdempotent(t *testing.T) {
	b := NewBudget(2)
	release := mustAcquire(t, b, 2)
	release()
	release()
	if active, _ := b.Stats(); active != 0 {
		t.Fatalf("double release corrupted the accounting: active=%d", active)
	}
	next := mustAcquire(t, b, 2)
	defer next()
	if active, _ := b.Stats(); active != 2 {
		t.Fatalf("budget unusable after a double release: active=%d", active)
	}
}

func TestBudgetAdmissionNeverLeaksWhenTheDeadlineRacesAdmission(t *testing.T) {
	b := NewBudget(1)
	for i := 0; i < 200; i++ {
		release := mustAcquire(t, b, 1)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%5)*time.Microsecond)
		done := make(chan struct{})
		var got func()
		var err error
		go func() {
			got, err = b.AcquireContext(ctx, 1)
			close(done)
		}()
		release()
		<-done
		cancel()
		if err == nil && got != nil {
			got()
		}
		if active, _ := b.Stats(); active != 0 {
			t.Fatalf("iteration %d leaked width: active=%d", i, active)
		}
		if queued := b.Pressure().Waiters; queued != 0 {
			t.Fatalf("iteration %d left %d waiters queued", i, queued)
		}
	}
}

// TestBudgetDoesNotStarveAWideWaiterBehindNarrowBorrows covers the fairness
// half of the admission contract: idle capacity is borrowable past a blocked
// wide head, but a stream of narrow waiters cannot starve it, so the gate
// eventually drains and lets the wide waiter in.
func TestBudgetDoesNotStarveAWideWaiterBehindNarrowBorrows(t *testing.T) {
	b := NewBudget(4)
	b.borrowBurst = 2
	held := mustAcquire(t, b, 3)

	wide := make(chan func(), 1)
	go func() { wide <- mustAcquire(t, b, 4) }()
	waitBudgetQueue(t, b, 1)

	narrow1 := mustAcquire(t, b, 1) // borrows the idle unit
	narrow2 := make(chan func(), 1)
	go func() { narrow2 <- mustAcquire(t, b, 1) }()
	waitBudgetQueue(t, b, 2)
	narrow1() // frees the unit, which the second narrow waiter then borrows

	var second func()
	select {
	case second = <-narrow2:
	case <-time.After(time.Second):
		t.Fatal("the second narrow waiter never borrowed the idle unit")
	}
	if active, _ := b.Stats(); active > b.Capacity() {
		t.Fatalf("admitted width %d exceeds the budget %d", active, b.Capacity())
	}

	// The borrow burst is spent: a third narrow waiter must wait for the wide
	// head rather than extend its starvation.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := b.AcquireContext(ctx, 1); err == nil {
		t.Fatal("a narrow waiter barged past a wide head after the borrow burst")
	}

	second()
	held()
	select {
	case release := <-wide:
		release()
	case <-time.After(time.Second):
		t.Fatal("the wide waiter was starved by narrow borrows")
	}
}

// TestBudgetPressureSeparatesWaitFromAdmission covers the attribution contract:
// a caller that had to queue reports the wait, a caller admitting immediately
// reports none, and a wait that ends without a grant is counted as one.
func TestBudgetPressureSeparatesWaitFromAdmission(t *testing.T) {
	b := NewBudget(1)
	release := mustAcquire(t, b, 1)
	if p := b.Pressure(); p.WaitUs != 0 || p.Waiters != 0 || p.Timeouts != 0 {
		t.Fatalf("an uncontended gate reported pressure %+v", p)
	}

	// A waiter that is admitted after queueing reports the wait and clears the
	// queue depth.
	admitted := make(chan func(), 1)
	go func() { admitted <- mustAcquire(t, b, 1) }()
	waitBudgetQueue(t, b, 1)
	if p := b.Pressure(); p.Waiters != 1 {
		t.Fatalf("queue depth not reported while a caller waited: %+v", p)
	}
	time.Sleep(30 * time.Millisecond)
	release()
	second := <-admitted
	p := b.Pressure()
	if p.Waiters != 0 {
		t.Fatalf("queue depth not cleared after the wait ended: %+v", p)
	}
	if p.WaitUs < 20_000 {
		t.Fatalf("a wait of ~30ms was not reported: %d us", p.WaitUs)
	}
	if p.Timeouts != 0 {
		t.Fatalf("an admitted wait was counted as a timeout: %+v", p)
	}

	// A wait that expires is counted, and does not become a run.
	expired := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := b.AcquireContext(ctx, 1)
		expired <- err
	}()
	waitBudgetQueue(t, b, 1)
	if err := <-expired; err == nil {
		t.Fatal("the second waiter was admitted while the first held the gate")
	}
	p = b.Pressure()
	if p.Timeouts != 1 {
		t.Fatalf("expired wait not counted: %+v", p)
	}
	if p.Waiters != 0 {
		t.Fatalf("expired waiter left queued: %+v", p)
	}
	if p.WaitUs < 70_000 {
		t.Fatalf("the expired wait was not added to the reported wait: %d us", p.WaitUs)
	}
	if !IsAdmissionError(context.DeadlineExceeded) || IsAdmissionError(errors.New("inference: shape mismatch")) {
		t.Fatal("IsAdmissionError misclassifies a refusal")
	}
	second()
}
