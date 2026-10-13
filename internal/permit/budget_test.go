package permit

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

// waitBudgetQueue waits until exactly n runs are queued. It spins on Gosched
// rather than sleeping: the goroutines under test block on the budget, so the
// queue length is the only observable state that matters.
func waitBudgetQueue(t *testing.T, b *Budget, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if queuedRuns(b) == n {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("queued runs did not reach %d (have %d)", n, queuedRuns(b))
}

func mustAcquire(b *Budget, model string, width int) func() {
	release, err := b.Acquire(context.Background(), model, width)
	if err != nil {
		panic(err)
	}
	return release
}

func mustEnter(s *Shared) func() {
	exit, err := s.Enter(context.Background())
	if err != nil {
		panic(err)
	}
	return exit
}

// acquireErrAsync starts an Acquire under ctx and reports its outcome.
func acquireErrAsync(ctx context.Context, b *Budget, model string, width int) <-chan error {
	out := make(chan error, 1)
	go func() {
		release, err := b.Acquire(ctx, model, width)
		if err == nil {
			release()
		}
		out <- err
	}()
	return out
}

func queuedRuns(b *Budget) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, q := range b.models {
		n += len(q.waiters)
	}
	return n
}

func usedTokens(b *Budget) int {
	active, _ := b.Stats()
	return active
}

// acquireAsync starts an Acquire and reports its release on the returned
// channel once granted.
func acquireAsync(b *Budget, model string, width int) <-chan func() {
	out := make(chan func(), 1)
	go func() { out <- mustAcquire(b, model, width) }()
	return out
}

func TestBudgetDoesNotBargeAroundWideWaiterOfTheSameModel(t *testing.T) {
	b := NewBudget(4, nil)
	first := mustAcquire(b, "m", 4)
	wide := acquireAsync(b, "m", 3)
	waitBudgetQueue(t, b, 1)
	narrow := acquireAsync(b, "m", 2)
	waitBudgetQueue(t, b, 2)
	first()
	w := <-wide
	if queuedRuns(b) != 1 || usedTokens(b) != 3 {
		t.Fatalf("after the wide grant: queued %d used %d, want the narrow run still queued behind it", queuedRuns(b), usedTokens(b))
	}
	w()
	(<-narrow)()
}

func TestBudgetAdmitsOversizedRunAlone(t *testing.T) {
	b := NewBudget(2, nil)
	l := mustAcquire(b, "m", 4)
	small := acquireAsync(b, "m", 1)
	waitBudgetQueue(t, b, 1)
	l()
	(<-small)()
	if active, _ := b.Stats(); active != 0 {
		t.Fatalf("active = %d after release, want 0", active)
	}
}

// An idle model reserves nothing: a busy model may take the whole capacity even
// though a heavier-weighted peer is configured.
func TestBudgetBorrowsIdleCapacity(t *testing.T) {
	b := NewBudget(4, map[string]float64{"busy": 1, "idle": 8})
	var releases []func()
	for range 4 {
		releases = append(releases, mustAcquire(b, "busy", 1))
	}
	if got := usedTokens(b); got != 4 {
		t.Fatalf("busy model holds %d tokens, want the full capacity 4", got)
	}
	for _, r := range releases {
		r()
	}
}

// grantOrder queues runs for each model behind a held token, then releases one
// grant at a time and returns the order models were admitted in.
func grantOrder(t *testing.T, b *Budget, queue []string) []string {
	t.Helper()
	hold := mustAcquire(b, "hold", 1)
	granted := make(chan string, len(queue))
	release := make(chan struct{})
	for i, model := range queue {
		go func() {
			r := mustAcquire(b, model, 1)
			granted <- model
			<-release
			r()
		}()
		waitBudgetQueue(t, b, i+1)
	}
	hold()
	order := make([]string, 0, len(queue))
	for range queue {
		order = append(order, <-granted)
		release <- struct{}{}
	}
	return order
}

func TestBudgetAlternatesEqualWeights(t *testing.T) {
	b := NewBudget(1, nil)
	order := grantOrder(t, b, []string{"a", "a", "a", "b", "b", "b"})
	want := []string{"a", "b", "a", "b", "a", "b"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("grant order = %v, want %v", order, want)
		}
	}
}

func TestBudgetWeightsShareContendedTurns(t *testing.T) {
	b := NewBudget(1, map[string]float64{"a": 2, "b": 1})
	order := grantOrder(t, b, []string{"a", "a", "a", "a", "b", "b"})
	counts := map[string]int{}
	for _, m := range order {
		counts[m]++
	}
	if counts["a"] != 4 || counts["b"] != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("grant order = %v, want a:b at 2:1 starting a,b", order)
	}
}

// Idleness earns no credit: after a model sat idle while the other ran many
// turns, it gets its fair turn but does not monopolize the budget.
func TestBudgetIdleQueueKeepsNoCredit(t *testing.T) {
	b := NewBudget(1, nil)
	for range 5 {
		mustAcquire(b, "busy", 1)()
	}
	order := grantOrder(t, b, []string{"busy", "busy", "idle", "idle"})
	if order[0] == order[1] {
		t.Fatalf("grant order = %v, want the returning model interleaved, not run to exhaustion", order)
	}
}

// Narrow runs of one model may pass a wide head of another only for a bounded
// round; then the budget drains to the wide run.
func TestBudgetWideHeadProgressesUnderNarrowTraffic(t *testing.T) {
	b := NewBudget(4, nil)
	n1, n2, n3 := mustAcquire(b, "narrow", 1), mustAcquire(b, "narrow", 1), mustAcquire(b, "narrow", 1)
	wide := acquireAsync(b, "wide", 4)
	waitBudgetQueue(t, b, 1)
	// A fitting narrow run may pass the wide head once per active model...
	n4 := mustAcquire(b, "narrow", 1)
	n5 := acquireAsync(b, "narrow", 1)
	waitBudgetQueue(t, b, 2)
	n1()
	r5 := <-n5
	n6 := acquireAsync(b, "narrow", 1)
	waitBudgetQueue(t, b, 2)
	// ...after which free tokens are held back for the wide head.
	n2()
	if got := queuedRuns(b); got != 2 {
		t.Fatalf("queued = %d after the bypass limit, want narrow and wide both held", got)
	}
	n3()
	n4()
	r5()
	w := <-wide
	if got := usedTokens(b); got != 4 {
		t.Fatalf("wide run holds %d tokens, want the full capacity", got)
	}
	w()
	(<-n6)()
	if got := usedTokens(b); got != 0 {
		t.Fatalf("active = %d after every release, want 0", got)
	}
}

func TestBudgetStatsTrackPeak(t *testing.T) {
	b := NewBudget(3, nil)
	r1, r2 := mustAcquire(b, "m", 2), mustAcquire(b, "m", 1)
	r1()
	r2()
	if active, peak := b.Stats(); active != 0 || peak != 3 {
		t.Fatalf("stats = active %d peak %d, want 0 and 3", active, peak)
	}
}

// The first caller holds the session's whole width; each concurrent caller adds
// one token; the width returns when the last caller leaves.
func TestSharedChargesSessionOnceAndEachExtraCaller(t *testing.T) {
	b := NewBudget(8, nil)
	s := b.Shared("m", 4)
	exit1 := mustEnter(s)
	if got := usedTokens(b); got != 4 {
		t.Fatalf("one caller holds %d tokens, want the session width 4", got)
	}
	exits := []func(){exit1}
	for range 3 { // callers up to a cap of 4
		exits = append(exits, mustEnter(s))
	}
	if got := usedTokens(b); got != 7 {
		t.Fatalf("four callers hold %d tokens, want width + 3 = 7", got)
	}
	exits[0]() // the first caller leaves while others run: the width stays held
	if got := usedTokens(b); got != 7 {
		t.Fatalf("after the first caller left: %d tokens, want the width still held with 3 extras = 7", got)
	}
	exits[1]()
	exits[2]()
	exits[3]()
	if got := usedTokens(b); got != 0 {
		t.Fatalf("all callers left: %d tokens, want 0", got)
	}
}

func TestSharedSessionsAreChargedIndependently(t *testing.T) {
	b := NewBudget(8, nil)
	a, c := b.Shared("m", 3), b.Shared("m", 3)
	exitA, exitC := mustEnter(a), mustEnter(c)
	if got := usedTokens(b); got != 6 {
		t.Fatalf("two sessions hold %d tokens, want 3 each", got)
	}
	extra := mustEnter(a)
	if got := usedTokens(b); got != 7 {
		t.Fatalf("a second caller on one session holds %d tokens, want 7", got)
	}
	extra()
	exitA()
	exitC()
	if got := usedTokens(b); got != 0 {
		t.Fatalf("active = %d, want 0", got)
	}
}

// A script session and an embedding run share one budget: the session's width
// waits for the embedding run, and error release frees it.
func TestSharedMixedWithEmbeddingRuns(t *testing.T) {
	b := NewBudget(4, nil)
	embed := mustAcquire(b, "embed", 2)
	s := b.Shared("script", 3)
	entered := make(chan func(), 1)
	go func() { entered <- mustEnter(s) }()
	waitBudgetQueue(t, b, 1)
	embed()
	exit := <-entered
	if got := usedTokens(b); got != 3 {
		t.Fatalf("script session holds %d tokens, want 3", got)
	}
	exit()
	if got := usedTokens(b); got != 0 {
		t.Fatalf("active = %d after the session left, want 0", got)
	}
}

// A waiter whose context ends leaves the queue and the wide head it was
// blocking no longer holds back the run behind it.
func TestBudgetCancelledWaiterLeavesQueueAndWakesSuccessors(t *testing.T) {
	b := NewBudget(4, nil)
	hold := mustAcquire(b, "m", 3)
	ctx, cancel := context.WithCancel(context.Background())
	wide := acquireErrAsync(ctx, b, "m", 4)
	waitBudgetQueue(t, b, 1)
	narrow := acquireAsync(b, "m", 1)
	waitBudgetQueue(t, b, 2)
	cancel()
	if err := <-wide; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error = %v, want context.Canceled", err)
	}
	(<-narrow)()
	hold()
	if used := usedTokens(b); used != 0 {
		t.Fatalf("tokens leaked: %d in use", used)
	}
	if _, waiters, timeouts := b.Pressure(); waiters != 0 || timeouts != 0 {
		t.Fatalf("pressure waiters=%d timeouts=%d, want 0/0 for a cancellation", waiters, timeouts)
	}
}

func TestBudgetTimeoutExpiresWaiterOnce(t *testing.T) {
	b := NewBudget(1, nil)
	b.SetTimeout(time.Millisecond)
	hold := mustAcquire(b, "m", 1)
	err := <-acquireErrAsync(context.Background(), b, "m", 1)
	if !errors.Is(err, ErrAdmissionTimeout) {
		t.Fatalf("error = %v, want ErrAdmissionTimeout", err)
	}
	if _, waiters, timeouts := b.Pressure(); waiters != 0 || timeouts != 1 {
		t.Fatalf("waiters=%d timeouts=%d, want 0/1", waiters, timeouts)
	}
	hold()
	if used := usedTokens(b); used != 0 {
		t.Fatalf("tokens leaked: %d in use", used)
	}
	// The gate still admits after an expiry.
	mustAcquire(b, "m", 1)()
}

// A grant that wins the race against a cancellation stays granted and must be
// released exactly once; a cancellation that wins consumes nothing.
func TestBudgetGrantCancelRaceHasOneOwner(t *testing.T) {
	for range 200 {
		b := NewBudget(1, nil)
		hold := mustAcquire(b, "m", 1)
		ctx, cancel := context.WithCancel(context.Background())
		res := acquireErrAsync(ctx, b, "m", 1)
		waitBudgetQueue(t, b, 1)
		go hold()
		cancel()
		<-res
		// Whichever side won, the budget must be fully free again.
		mustAcquire(b, "m", 1)()
		if used := usedTokens(b); used != 0 {
			t.Fatalf("tokens leaked after grant/cancel race: %d", used)
		}
	}
}

// A run that fails still returns its tokens (the release is the caller's defer).
func TestBudgetErrorPathReleaseFreesTokens(t *testing.T) {
	b := NewBudget(1, nil)
	func() {
		defer mustAcquire(b, "m", 1)()
	}()
	if used := usedTokens(b); used != 0 {
		t.Fatalf("tokens leaked: %d", used)
	}
}

// Close fails queued waiters and later callers; a run that already holds
// tokens keeps them until it releases.
func TestBudgetCloseCancelsQueuedButNotRunning(t *testing.T) {
	b := NewBudget(1, nil)
	running := mustAcquire(b, "m", 1)
	queued := acquireErrAsync(context.Background(), b, "m", 1)
	waitBudgetQueue(t, b, 1)
	b.Close()
	if err := <-queued; !errors.Is(err, ErrAdmissionClosed) {
		t.Fatalf("queued error = %v, want ErrAdmissionClosed", err)
	}
	if used := usedTokens(b); used != 1 {
		t.Fatalf("running call holds %d tokens after Close, want 1", used)
	}
	if _, err := b.Acquire(context.Background(), "m", 1); !errors.Is(err, ErrAdmissionClosed) {
		t.Fatalf("Acquire after Close error = %v", err)
	}
	running()
	if used := usedTokens(b); used != 0 {
		t.Fatalf("tokens leaked after release: %d", used)
	}
}

// A caller waiting for a session's width can time out or cancel without ever
// holding the session's lock, and the next caller can still activate it.
func TestSharedEnterCancelsWhileWaitingForWidth(t *testing.T) {
	b := NewBudget(2, nil)
	hold := mustAcquire(b, "embed", 2)
	s := b.Shared("m", 2)
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		exit, err := s.Enter(ctx)
		if err == nil {
			exit()
		}
		first <- err
	}()
	waitBudgetQueue(t, b, 1)
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("Enter error = %v, want context.Canceled", err)
	}
	hold()
	mustEnter(s)()
	if used := usedTokens(b); used != 0 {
		t.Fatalf("tokens leaked: %d", used)
	}
}

func TestPressureCountsWaitAndWaiters(t *testing.T) {
	b := NewBudget(1, nil)
	hold := mustAcquire(b, "m", 1)
	next := acquireAsync(b, "m", 1)
	waitBudgetQueue(t, b, 1)
	if _, waiters, _ := b.Pressure(); waiters != 1 {
		t.Fatalf("waiters = %d, want 1", waiters)
	}
	hold()
	(<-next)()
	waitUs, waiters, _ := b.Pressure()
	if waiters != 0 || waitUs < 0 {
		t.Fatalf("waitUs=%d waiters=%d", waitUs, waiters)
	}
}
