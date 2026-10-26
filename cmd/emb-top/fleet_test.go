package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

// fakeResolver is a scripted DNS resolver for the fleet's re-resolution.
type fakeResolver struct {
	answers map[string][]string
	errs    map[string]error
	calls   int
}

func (r *fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	r.calls++
	if err := r.errs[host]; err != nil {
		return nil, err
	}
	return r.answers[host], nil
}

// ---- fakes ----

type fakeResult struct {
	res *embtop.PollResult
	err error
}

// fakeNodeClient is a scripted dashboardClient. results are consumed in order;
// the last result repeats. A non-nil gate blocks Poll until it is closed.
type fakeNodeClient struct {
	addr string

	mu        sync.Mutex
	connected bool
	dials     int
	calls     int
	results   []fakeResult
	gate      <-chan struct{}
}

func (c *fakeNodeClient) Addr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.addr
}

func (c *fakeNodeClient) SetAddr(a string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addr = a
}

func (c *fakeNodeClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	return nil
}

func (c *fakeNodeClient) EnsureConn() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connected {
		return false, nil
	}
	c.connected = true
	c.dials++
	return true, nil
}

func (c *fakeNodeClient) Poll(_ []string, _ uint64) (*embtop.PollResult, error) {
	if c.gate != nil {
		<-c.gate
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected {
		return nil, errors.New("not connected")
	}
	i := c.calls
	c.calls++
	if len(c.results) == 0 {
		return fakePoll(10, 100, 0, 0), nil
	}
	if i >= len(c.results) {
		i = len(c.results) - 1
	}
	return c.results[i].res, c.results[i].err
}

func (c *fakeNodeClient) pollCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func rn(addr string) embtop.ResolvedNode {
	return embtop.ResolvedNode{Key: addr, Spec: addr, Addr: addr}
}

func namedRN(name, addr string) embtop.ResolvedNode {
	return embtop.ResolvedNode{Key: name, Spec: name, Name: name, TLSName: name, Addr: addr}
}

func fakeFleet(opts *options, clients map[string]dashboardClient, nodes ...embtop.ResolvedNode) *fleet {
	if opts == nil {
		opts = &options{interval: time.Second, window: 10}
	}
	return newFleetWith(nodes, opts, &fakeResolver{}, func(addr, _ string) dashboardClient {
		return clients[addr]
	})
}

// seedNode fills a node's sampler with rates and marks it reachable.
func seedNode(n *fleetNode, reqs ...int64) {
	t := time.Unix(0, 0)
	n.tui.sampler.SetClock(func() time.Time { return t })
	for _, r := range reqs {
		n.tui.applyResult(fakePoll(r, r*4, 0, 0))
		t = t.Add(time.Second)
	}
	n.reachable = true
	n.tui.connected = true
}

// ---- 1.5: refresh & membership ----

// `-nodes emb.internal:6399` reaches the fleet rows: one per address record,
// dialled and labelled on the specification's port, and a record that appears
// later joins without a restart.
func TestFleetExpandsAPortCarryingName(t *testing.T) {
	specs, err := embtop.BuildSpecs("", nil, "emb.internal:6399")
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{answers: map[string][]string{
		"emb.internal": {"10.0.0.1", "10.0.0.2", "10.0.0.3"},
	}}
	f := newFleetWith(nil, &options{interval: time.Second, window: 10, specs: specs}, res,
		func(addr, _ string) dashboardClient { return &fakeNodeClient{addr: addr} })
	f.refreshSync()

	var addrs []string
	for _, n := range f.nodes {
		addrs = append(addrs, n.addr)
		if n.displayName() != n.addr {
			t.Errorf("row %s is labelled %q, want its address", n.addr, n.displayName())
		}
		if msg := f.pollOnce(n); msg.err != nil {
			t.Errorf("poll %s: %v", n.addr, msg.err)
		}
	}
	if got := strings.Join(addrs, ","); got != "10.0.0.1:6399,10.0.0.2:6399,10.0.0.3:6399" {
		t.Fatalf("rows = %s, want one per record on port 6399", got)
	}

	res.answers["emb.internal"] = []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4"}
	f.refreshSync()
	if len(f.nodes) != 4 || f.nodes[3].addr != "10.0.0.4:6399" {
		t.Fatalf("scaled-out record did not join: %+v", f.nodes)
	}
}

// A name that resolves to one record and later to several must keep one row per
// address: the row keyed by the specification is rekeyed, not left orphaned and
// polled a second time under its address.
func TestFleetRekeysWhenANameStartsExpanding(t *testing.T) {
	specs, err := embtop.BuildSpecs("", nil, "emb.internal:6399")
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{answers: map[string][]string{"emb.internal": {"10.0.0.1"}}}
	built := 0
	f := newFleetWith(nil, &options{interval: time.Second, window: 10, specs: specs}, res,
		func(addr, _ string) dashboardClient {
			built++
			return &fakeNodeClient{addr: addr}
		})
	f.refreshSync()
	if len(f.nodes) != 1 || f.nodes[0].key != "emb.internal:6399" {
		t.Fatalf("single-address name = %+v, want one row keyed by the specification", f.nodes)
	}

	res.answers["emb.internal"] = []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	f.refreshSync()
	var keys []string
	for _, n := range f.nodes {
		keys = append(keys, n.key)
	}
	if got := strings.Join(keys, ","); got != "10.0.0.1:6399,10.0.0.2:6399,10.0.0.3:6399" {
		t.Fatalf("rows = %s, want one per address and no stale specification row", got)
	}
	if built != 3 {
		t.Errorf("built %d clients, want 3: the existing row is rekeyed, not rebuilt and not polled twice", built)
	}
}

// The same identity rule in reverse: a name that collapses to one record keeps
// the surviving node's row (rekeyed to the specification) instead of adding a
// second row for an address it is already polling.
func TestFleetRekeysWhenANameCollapses(t *testing.T) {
	specs, err := embtop.BuildSpecs("", nil, "emb.internal:6399")
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{answers: map[string][]string{
		"emb.internal": {"10.0.0.1", "10.0.0.2", "10.0.0.3"},
	}}
	f := newFleetWith(nil, &options{interval: time.Second, window: 10, specs: specs}, res,
		func(addr, _ string) dashboardClient { return &fakeNodeClient{addr: addr} })
	f.refreshSync()

	res.answers["emb.internal"] = []string{"10.0.0.2"}
	f.refreshSync()
	perAddr := map[string]int{}
	for _, n := range f.nodes {
		perAddr[n.addr]++
		if n.resolved && n.key != "emb.internal:6399" {
			t.Errorf("surviving row %s is keyed %q, want the specification", n.addr, n.key)
		}
	}
	for addr, count := range perAddr {
		if count != 1 {
			t.Errorf("%s has %d rows, want 1", addr, count)
		}
	}
}

func TestFleetAddsResolvedAddressWithoutRestart(t *testing.T) {
	a, b := "10.0.0.1:6379", "10.0.0.2:6379"
	ca := &fakeNodeClient{addr: a}
	cb := &fakeNodeClient{addr: b}
	f := fakeFleet(nil, map[string]dashboardClient{a: ca, b: cb}, rn(a))
	f.applyResolved([]embtop.ResolvedNode{rn(a), rn(b)})
	if len(f.nodes) != 2 {
		t.Fatalf("fleet has %d nodes, want 2", len(f.nodes))
	}
	if ok := f.pollAllSync(); ok != 2 {
		t.Fatalf("pollAllSync sampled %d nodes, want 2", ok)
	}
	if ca.pollCount() == 0 || cb.pollCount() == 0 {
		t.Fatalf("new node was not polled: a=%d b=%d", ca.pollCount(), cb.pollCount())
	}
}

func TestRefreshFailureKeepsKnownFleet(t *testing.T) {
	a := "10.0.0.1:6379"
	f := fakeFleet(nil, map[string]dashboardClient{a: &fakeNodeClient{addr: a}}, rn(a))
	spec, err := embtop.ParseNodeSpec("emb.internal")
	if err != nil {
		t.Fatal(err)
	}
	f.opts.specs = []embtop.NodeSpec{spec}
	f.resolver = &fakeResolver{errs: map[string]error{"emb.internal": errors.New("dns down")}}
	f.refreshSync()
	if len(f.nodes) != 1 || f.refreshErr == nil {
		t.Fatalf("resolver failure changed the fleet: nodes=%d err=%v", len(f.nodes), f.refreshErr)
	}
}

func TestDueRefreshOnIntervalAndUnreachable(t *testing.T) {
	clock := time.Unix(1000, 0)
	a := "10.0.0.1:6379"
	f := fakeFleet(nil, map[string]dashboardClient{a: &fakeNodeClient{addr: a}}, rn(a))
	f.now = func() time.Time { return clock }
	f.resolveEvery = 30 * time.Second
	f.lastRefresh = clock
	if f.dueRefresh() {
		t.Fatal("dueRefresh true right after a refresh")
	}
	clock = clock.Add(31 * time.Second)
	if !f.dueRefresh() {
		t.Fatal("dueRefresh false after the interval")
	}
	f.lastRefresh = clock
	f.applyPoll(nodePollMsg{key: a, err: errors.New("gone")})
	if !f.dueRefresh() {
		t.Fatal("an unreachable node did not request an immediate refresh")
	}
}

func TestUnresolvedNodeStaysUntilUnreachableGrace(t *testing.T) {
	clock := time.Unix(1000, 0)
	a := "10.0.0.1:6379"
	f := fakeFleet(nil, map[string]dashboardClient{a: &fakeNodeClient{addr: a}}, rn(a))
	f.now = func() time.Time { return clock }
	f.grace = time.Minute

	f.applyResolved(nil)
	if f.nodes[0].resolved {
		t.Fatal("node still resolved after the name vanished")
	}
	f.applyPoll(nodePollMsg{key: a, res: fakePoll(10, 100, 0, 0)})
	f.prune()
	if len(f.nodes) != 1 {
		t.Fatal("an orphaned but answering node was dropped")
	}
	f.applyPoll(nodePollMsg{key: a, err: errors.New("down")})
	clock = clock.Add(30 * time.Second)
	f.prune()
	if len(f.nodes) != 1 {
		t.Fatal("node removed before the grace period")
	}
	clock = clock.Add(2 * time.Minute)
	f.prune()
	if len(f.nodes) != 0 {
		t.Fatal("node not removed after being unresolved and unreachable past the grace period")
	}
}

func TestNodeOrderIsFirstSeen(t *testing.T) {
	a, b, c := "a:1", "b:1", "c:1"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b}, c: &fakeNodeClient{addr: c},
	}, rn(a), rn(b))
	f.applyResolved([]embtop.ResolvedNode{rn(c), rn(b), rn(a)})
	got := make([]string, len(f.nodes))
	for i, n := range f.nodes {
		got[i] = n.addr
	}
	if strings.Join(got, ",") != "a:1,b:1,c:1" {
		t.Fatalf("node order follows resolution: %v", got)
	}
}

// ---- 3.x: polling isolation ----

func TestStalledNodeDoesNotBlockAnother(t *testing.T) {
	gate := make(chan struct{})
	slow := &fakeNodeClient{addr: "slow:1", gate: gate}
	fast := &fakeNodeClient{addr: "fast:1", results: []fakeResult{{res: fakePoll(10, 100, 0, 0)}}}
	f := fakeFleet(nil, map[string]dashboardClient{"slow:1": slow, "fast:1": fast}, rn("slow:1"), rn("fast:1"))

	slowDone := make(chan struct{})
	go func() { f.pollOnce(f.nodes[0]); close(slowDone) }()
	fastMsg := make(chan nodePollMsg, 1)
	go func() { fastMsg <- f.pollOnce(f.nodes[1]) }()

	select {
	case m := <-fastMsg:
		if m.err != nil {
			t.Fatalf("fast node poll error: %v", m.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fast node blocked behind the stalled node")
	}
	select {
	case <-slowDone:
		t.Fatal("stalled node returned before its gate opened")
	default:
	}
	close(gate)
	select {
	case <-slowDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stalled node never returned after the gate opened")
	}
}

func TestUnreachableNodeKeepsLastValues(t *testing.T) {
	a := "a:1"
	client := &fakeNodeClient{addr: a}
	f := fakeFleet(nil, map[string]dashboardClient{a: client}, rn(a))
	n := f.nodes[0]
	f.applyPoll(nodePollMsg{key: a, res: fakePoll(10, 100, 0, 0)})
	f.applyPoll(nodePollMsg{key: a, err: errors.New("down")})
	if n.reachable {
		t.Fatal("node still marked reachable")
	}
	if n.tui.sampler.Latest.TotalRequests != 10 {
		t.Fatalf("last values lost: %+v", n.tui.sampler.Latest)
	}
	if n.unreachableSince.IsZero() {
		t.Fatal("unreachable since was not recorded")
	}
}

func TestReconnectRebasesRates(t *testing.T) {
	a := "a:1"
	f := fakeFleet(nil, map[string]dashboardClient{a: &fakeNodeClient{addr: a}}, rn(a))
	n := f.nodes[0]
	clock := time.Unix(0, 0)
	n.tui.sampler.SetClock(func() time.Time { return clock })

	f.applyPoll(nodePollMsg{key: a, res: fakePoll(100, 1000, 0, 0), dialed: true})
	clock = clock.Add(time.Second)
	f.applyPoll(nodePollMsg{key: a, res: fakePoll(200, 2000, 0, 0)})
	if got := n.tui.sampler.Latest.ReqRate; got != 100 {
		t.Fatalf("steady rate = %v, want 100", got)
	}
	// The node restarts with reset counters; the fresh connection rebases.
	clock = clock.Add(time.Second)
	f.applyPoll(nodePollMsg{key: a, res: fakePoll(5, 50, 0, 0), dialed: true})
	if got := n.tui.sampler.Latest.ReqRate; got != 0 {
		t.Fatalf("rejoin rate = %v, want 0 (rebased, no spike)", got)
	}
}

// An expanded row is labelled and dialled by its address, but its certificate
// is still verified against the name it was resolved from.
func TestFleetBuildsExpandedRowsWithTheirResolvedName(t *testing.T) {
	var built []string
	nodes := []embtop.ResolvedNode{
		{Key: "10.0.0.1:6399", Spec: "emb.internal:6399", TLSName: "emb.internal", Addr: "10.0.0.1:6399"},
		{Key: "10.0.0.2:6399", Spec: "emb.internal:6399", TLSName: "emb.internal", Addr: "10.0.0.2:6399"},
	}
	f := newFleetWith(nodes, &options{interval: time.Second, window: 10}, &fakeResolver{},
		func(addr, tlsName string) dashboardClient {
			built = append(built, addr+"|"+tlsName)
			return &fakeNodeClient{addr: addr}
		})
	for _, n := range f.nodes {
		if n.name != "" {
			t.Errorf("expanded row %s is labelled %q, want its address", n.addr, n.name)
		}
	}
	want := []string{"10.0.0.1:6399|emb.internal", "10.0.0.2:6399|emb.internal"}
	if strings.Join(built, ",") != strings.Join(want, ",") {
		t.Fatalf("clients built with %v, want %v", built, want)
	}
}

func TestTLSServerNameOverrideWins(t *testing.T) {
	if got := tlsServerName(&options{tlsServerName: "override"}, "emb.internal"); got != "override" {
		t.Errorf("-tls-server-name ignored for a DNS-expanded node: got %q", got)
	}
	if got := tlsServerName(&options{}, "emb.internal"); got != "emb.internal" {
		t.Errorf("node name fallback: got %q", got)
	}
	if got := tlsServerName(&options{}, ""); got != "" {
		t.Errorf("no name: got %q", got)
	}
}

func TestStalledNodeDoesNotBlockTheNextTick(t *testing.T) {
	a, b := "slow:1", "fast:1"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a},
		b: &fakeNodeClient{addr: b},
	}, rn(a), rn(b))

	um, cmd := f.Update(tickMsg{})
	f = um.(*fleet)
	if cmd == nil {
		t.Fatal("tick returned no command")
	}
	if !f.tickScheduled {
		t.Fatal("the next tick was not scheduled while a poll is outstanding")
	}
	// Deliver the fast node's result; the slow node's poll stays outstanding.
	um, _ = f.Update(nodePollMsg{key: b, res: fakePoll(10, 100, 0, 0)})
	f = um.(*fleet)
	um, _ = f.Update(tickMsg{})
	f = um.(*fleet)
	if !f.nodes[1].polling {
		t.Fatal("the fast node was not re-polled while the slow node stalled")
	}
	if !f.nodes[0].polling {
		t.Fatal("the stalled node's outstanding poll was duplicated")
	}
}

// ---- 4.x: rendering ----

func TestTrafficChartDrawsSeriesPerNode(t *testing.T) {
	f := fleetOfThree(t)
	if view := f.traffic.View(); !strings.ContainsAny(view, "─╭╮╰╯") {
		t.Fatalf("traffic chart has no line runes:\n%s", view)
	}
	lines := strings.Split(strings.TrimRight(f.trafficView(), "\n"), "\n")
	if want := trafficH + 3; len(lines) != want {
		t.Fatalf("trafficView is %d lines, want %d (caption + bordered chart)", len(lines), want)
	}
}

func TestSparklineRampScales(t *testing.T) {
	got := sparkline([]float64{0, 1, 2, 3, 4, 5, 6, 7}, 8, okStyle)
	if got != "▁▂▃▄▅▆▇█" {
		t.Fatalf("sparkline = %q, want the full ramp", got)
	}
	if w := lipgloss.Width(sparkline(nil, 5, okStyle)); w != 5 {
		t.Fatalf("empty sparkline width = %d, want 5", w)
	}
}

func TestInboundBarProportion(t *testing.T) {
	if n := filledCells(0.5, 10); n != 5 {
		t.Fatalf("filledCells(0.5, 10) = %d, want 5", n)
	}
	if n := filledCells(2, 10); n != 10 {
		t.Fatalf("filledCells clamps high to %d, want 10", n)
	}
	if n := filledCells(-1, 10); n != 0 {
		t.Fatalf("filledCells clamps low to %d, want 0", n)
	}
}

func TestStackedShareBarWidthAndProportion(t *testing.T) {
	cells := shareCells([]float64{3, 1}, 20)
	if cells[0] != 15 || cells[1] != 5 {
		t.Fatalf("shareCells(3:1, 20) = %v, want [15 5]", cells)
	}
	// An idle node must not absorb the rounding remainder.
	cells = shareCells([]float64{3, 0}, 20)
	if cells[0] != 20 || cells[1] != 0 {
		t.Fatalf("shareCells(3:0, 20) = %v, want [20 0]", cells)
	}
	if w := lipgloss.Width(stackedShareBar([]float64{3, 1}, nil, 20)); w != 20 {
		t.Fatalf("stacked bar width = %d, want 20", w)
	}
}

func TestImbalanceInboundNamesOffender(t *testing.T) {
	checks := imbalanceChecks([]embtop.FleetNodeInput{
		{Label: "a", Health: embtop.HealthInput{Connected: true, Polls: 5}, ReqRate: 90},
		{Label: "b", Health: embtop.HealthInput{Connected: true, Polls: 5}, ReqRate: 10},
	})
	if checks[0].name != "inbound" || checks[0].ok {
		t.Fatalf("inbound check = %+v, want a failure", checks[0])
	}
	if !strings.Contains(checks[0].detail, "a") {
		t.Fatalf("inbound detail does not name the offender: %q", checks[0].detail)
	}
}

func TestImbalanceSlowPeerNamesOffender(t *testing.T) {
	checks := imbalanceChecks([]embtop.FleetNodeInput{
		{Label: "fast", Health: embtop.HealthInput{Connected: true, Polls: 5, P95Us: 100}, ReqRate: 10},
		{Label: "slow", Health: embtop.HealthInput{Connected: true, Polls: 5, P95Us: 1000}, ReqRate: 10},
	})
	if checks[1].name != "latency" || checks[1].ok {
		t.Fatalf("latency check = %+v, want a failure", checks[1])
	}
	if !strings.Contains(checks[1].detail, "slow") {
		t.Fatalf("latency detail does not name the offender: %q", checks[1].detail)
	}
}

// TestFleetRowsFitWidth guards every fleet line against overflowing the
// terminal, which would wrap mid-row and corrupt the layout.
func TestFleetRowsFitWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		f := fleetOfThree(t)
		f.width, f.height = w, 40
		f.layout()
		for _, line := range strings.Split(strings.TrimRight(f.fleetView(), "\n"), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d: line is %d cols wide:\n%s", w, got, line)
			}
		}
	}
}

// TestIngressColumnsDoNotShift guards the fixed columns: a rate growing by
// orders of magnitude must not move the latency column after it.
func TestIngressColumnsDoNotShift(t *testing.T) {
	f := fleetOfThree(t)
	f.width, f.height = 140, 40
	col := func() int {
		for _, line := range f.ingressRows() {
			if strings.Contains(line, "10.0.0.1:6379") {
				if i := strings.Index(line, "p95"); i >= 0 {
					return lipgloss.Width(line[:i])
				}
			}
		}
		return -1
	}
	before := col()
	seedNode(f.nodes[0], 10, 900000)
	after := col()
	if before < 0 || after < 0 || before != after {
		t.Fatalf("p95 column shifted: %d -> %d", before, after)
	}
}

func fleetOfThree(t *testing.T) *fleet {
	t.Helper()
	a, b, c := "10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b}, c: &fakeNodeClient{addr: c},
	}, rn(a), rn(b), rn(c))
	f.width, f.height = 120, 40
	seedNode(f.nodes[0], 10, 20)
	seedNode(f.nodes[1], 5, 10)
	seedNode(f.nodes[2], 0, 0)
	f.layout()
	return f
}

// seedNodePlan fills a node's sampler with rates plus a process CPU plan, so
// the fleet's summed plan line can be exercised.
func seedNodePlan(n *fleetNode, reqs, budget, active, peak int64, allowance float64) {
	t := time.Unix(0, 0)
	n.tui.sampler.SetClock(func() time.Time { return t })
	for _, r := range []int64{0, reqs} {
		res := fakePoll(r, r*4, 0, 0)
		res.CPUPlanKnown = true
		res.CPUBudgetEnabled = true
		res.CPUBudget = budget
		res.CPUActiveTokens = active
		res.CPUPeakTokens = peak
		res.CPUAllowance = allowance
		res.CPUAllowanceKnown = true
		res.CPUAllowanceSrc = "scheduler/affinity"
		n.tui.applyResult(res)
		t = t.Add(time.Second)
	}
	n.reachable = true
	n.tui.connected = true
}

// The fleet total is the sum of independent per-node plans: each node resolves
// its own allowance and budget, so the fleet line shows aggregate tokens in use
// against aggregate admitted capacity.
func TestFleetViewShowsAggregateCPUPlan(t *testing.T) {
	a, b := "10.0.0.1:6379", "10.0.0.2:6379"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b},
	}, rn(a), rn(b))
	f.width, f.height = 140, 40
	seedNodePlan(f.nodes[0], 10, 8, 3, 6, 10)
	seedNodePlan(f.nodes[1], 8, 4, 2, 4, 4)

	view := f.View()
	for _, want := range []string{"cpu budget 12", "tokens 5/12", "peak 10", "14.00 cores"} {
		if !strings.Contains(view, want) {
			t.Errorf("fleet view missing %q:\n%s", want, view)
		}
	}
}

func TestFleetViewOmitsUnknownCPUPlan(t *testing.T) {
	f := fleetOfThree(t)
	if view := f.View(); strings.Contains(view, "cpu budget") {
		t.Errorf("fleet view invented a CPU plan:\n%s", view)
	}
}

func TestFleetViewRendersTrafficAndRows(t *testing.T) {
	f := fleetOfThree(t)
	view := f.View()
	for _, want := range []string{
		"TRAFFIC", "INGRESS", "LOAD", "IMBALANCE", "req/s", "trend",
		"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("fleet view missing %q:\n%s", want, view)
		}
	}
}

func TestFleetRowStatesAreDistinct(t *testing.T) {
	a, b, c := "Idle:1", "orphan:1", "down:1"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b}, c: &fakeNodeClient{addr: c},
	}, rn(a), rn(b), rn(c))
	f.width, f.height = 140, 40
	seedNode(f.nodes[0], 0, 0)
	seedNode(f.nodes[1], 5, 10)
	f.nodes[1].resolved = false // orphaned
	seedNode(f.nodes[2], 5, 10)
	f.nodes[2].reachable = false // unreachable
	view := f.View()
	for _, want := range []string{"idle", "orphaned", "unreachable"} {
		if !strings.Contains(view, want) {
			t.Errorf("fleet view missing state %q:\n%s", want, view)
		}
	}
}

func TestMembershipCountsAreSeparate(t *testing.T) {
	a, b, c := "Idle:1", "orphan:1", "down:1"
	f := fakeFleet(nil, map[string]dashboardClient{
		a: &fakeNodeClient{addr: a}, b: &fakeNodeClient{addr: b}, c: &fakeNodeClient{addr: c},
	}, rn(a), rn(b), rn(c))
	seedNode(f.nodes[0], 0, 0)
	seedNode(f.nodes[1], 5, 10)
	f.nodes[1].resolved = false
	seedNode(f.nodes[2], 5, 10)
	f.nodes[2].reachable = false
	line := f.membershipView()
	for _, want := range []string{"discovered 3", "receiving traffic 1", "idle 1", "orphaned 1", "unreachable 1"} {
		if !strings.Contains(line, want) {
			t.Errorf("membership line %q missing %q", line, want)
		}
	}
}

func TestFleetScrollReachesEveryNode(t *testing.T) {
	var clients = map[string]dashboardClient{}
	var nodes []embtop.ResolvedNode
	for i := 0; i < 20; i++ {
		addr := string(rune('a'+i%26)) + ":1"
		if i >= 26 {
			addr = "z" + string(rune('a'+i-26)) + ":1"
		}
		clients[addr] = &fakeNodeClient{addr: addr}
		nodes = append(nodes, rn(addr))
	}
	f := fakeFleet(nil, clients, nodes...)
	f.width, f.height = 120, 24
	vis := f.visibleNodes()
	if vis >= len(f.nodes) {
		t.Fatalf("test needs more nodes than fit: vis=%d n=%d", vis, len(f.nodes))
	}
	seen := map[string]bool{}
	for s := 0; s <= len(f.nodes)-vis; s++ {
		f.scroll = s
		rows := f.ingressRows()
		for _, n := range f.nodes[s : s+vis] {
			seen[n.addr] = true
		}
		if !strings.Contains(strings.Join(rows, "\n"), "INGRESS") {
			t.Fatal("the ingress panel disappeared while scrolling")
		}
		if !strings.Contains(f.fleetView(), "IMBALANCE") {
			t.Fatal("the imbalance panel disappeared while scrolling")
		}
	}
	if len(seen) != len(f.nodes) {
		t.Fatalf("scrolling reached %d of %d nodes", len(seen), len(f.nodes))
	}
}

func TestSingleNodeFleetRendersDashboardUnchanged(t *testing.T) {
	a := "only:1"
	f := fakeFleet(nil, map[string]dashboardClient{a: &fakeNodeClient{addr: a}}, rn(a))
	f.width, f.height = 120, 40
	seedNode(f.nodes[0], 10, 20)
	want := f.nodes[0].tui.View()
	if got := f.View(); got != want {
		t.Fatalf("single-node fleet changed the dashboard:\n got %q\nwant %q", got, want)
	}
}

// ---- 6.x: interaction ----

func TestFleetSelectEnterEscape(t *testing.T) {
	f := fleetOfThree(t)
	f.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	um, _ := f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	f = um.(*fleet)
	if f.sel != 1 {
		t.Fatalf("selection = %d, want 1", f.sel)
	}
	um, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f = um.(*fleet)
	if !f.detail {
		t.Fatal("enter did not open the drill-down")
	}
	um, _ = f.Update(tea.KeyMsg{Type: tea.KeyEscape})
	f = um.(*fleet)
	if f.detail {
		t.Fatal("escape did not return to the fleet view")
	}
	if f.nodes[0].displayName() != "10.0.0.1:6379" {
		t.Fatal("row order changed")
	}
}

func TestFleetHelpFitsFrameWidth(t *testing.T) {
	f := fleetOfThree(t)
	for _, line := range strings.Split(f.helpView(), "\n") {
		if lipgloss.Width(line) > frameWidth {
			t.Errorf("help line exceeds the frame width: %q", line)
		}
	}
}

func TestFleetKeysPauseResumeReset(t *testing.T) {
	f := fleetOfThree(t)
	um, _ := f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	f = um.(*fleet)
	if !f.paused {
		t.Fatal("p did not pause")
	}
	um, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	f = um.(*fleet)
	if f.paused {
		t.Fatal("p did not resume")
	}
	um, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	f = um.(*fleet)
	if len(f.nodes[0].tui.sampler.Window) != 0 {
		t.Fatal("r did not reset the samplers")
	}
}

// ---- 7.3: fleet frames ----

func TestFleetFramesShowEveryNodeAndSurviveDrop(t *testing.T) {
	a, b := "a:1", "b:1"
	ca := &fakeNodeClient{addr: a, results: []fakeResult{
		{res: fakePoll(10, 100, 0, 0)},
		{err: errors.New("down")},
		{err: errors.New("down")},
	}}
	cb := &fakeNodeClient{addr: b}
	f := fakeFleet(&options{interval: time.Millisecond, window: 10}, map[string]dashboardClient{a: ca, b: cb}, rn(a), rn(b))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{max: 4, cancel: cancel}
	done := make(chan error, 1)
	go func() { done <- f.runFrames(ctx, time.Millisecond, 10, w) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runFrames: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runFrames did not stop after cancellation")
	}

	frames := w.frames(t)
	if len(frames) < 4 {
		t.Fatalf("want at least 4 frames, got %d", len(frames))
	}
	frames = frames[:4]
	for i, fr := range frames {
		if !strings.Contains(fr, a) || !strings.Contains(fr, b) {
			t.Errorf("frame %d does not contain every node: %q", i, fr)
		}
	}
}
