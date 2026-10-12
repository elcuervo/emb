package permit

import (
	"runtime"
	"testing"
	"time"
)

func TestBudgetDoesNotBargeAroundWideWaiter(t *testing.T) {
	b := NewBudget(4)
	first := b.Acquire(4)
	wide := make(chan func(), 1)
	narrow := make(chan func(), 1)
	go func() { wide <- b.Acquire(3) }()
	waitBudgetQueue(t, b, 1)
	go func() { narrow <- b.Acquire(2) }()
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
	go func() { large <- b.Acquire(4) }()
	l := <-large
	go func() { small <- b.Acquire(1) }()
	select {
	case release := <-small:
		release()
		t.Fatal("run admitted alongside an oversized run")
	default:
	}
	l()
	(<-small)()
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
