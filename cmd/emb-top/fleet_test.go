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

func rn(addr string) resolvedNode {
	return resolvedNode{key: addr, spec: addr, addr: addr}
}

func namedRN(name, addr string) resolvedNode {
	return resolvedNode{key: name, spec: name, name: name, addr: addr}
}

func fakeFleet(opts *options, clients map[string]dashboardClient, nodes ...resolvedNode) *fleet {
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

func TestFleetAddsResolvedAddressWithoutRestart(t *testing.T) {
	a, b := "10.0.0.1:6379", "10.0.0.2:6379"
	ca := &fakeNodeClient{addr: a}
	cb := &fakeNodeClient{addr: b}
	f := fakeFleet(nil, map[string]dashboardClient{a: ca, b: cb}, rn(a))
	f.applyResolved([]resolvedNode{rn(a), rn(b)})
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
	f.opts.specs = []nodeSpec{{raw: "emb.internal", host: "emb.internal", port: "6379", isName: true}}
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
	f.applyResolved([]resolvedNode{rn(c), rn(b), rn(a)})
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
	if w := lipgloss.Width(stackedShareBar([]float64{3, 1}, 20)); w != 20 {
		t.Fatalf("stacked bar width = %d, want 20", w)
	}
}

func TestImbalanceInboundNamesOffender(t *testing.T) {
	checks := imbalanceChecks([]fleetNodeInput{
		{label: "a", health: healthInput{connected: true, polls: 5}, reqRate: 90},
		{label: "b", health: healthInput{connected: true, polls: 5}, reqRate: 10},
	})
	if checks[0].name != "inbound" || checks[0].ok {
		t.Fatalf("inbound check = %+v, want a failure", checks[0])
	}
	if !strings.Contains(checks[0].detail, "a") {
		t.Fatalf("inbound detail does not name the offender: %q", checks[0].detail)
	}
}

func TestImbalanceSlowPeerNamesOffender(t *testing.T) {
	checks := imbalanceChecks([]fleetNodeInput{
		{label: "fast", health: healthInput{connected: true, polls: 5, p95Us: 100}, reqRate: 10},
		{label: "slow", health: healthInput{connected: true, polls: 5, p95Us: 1000}, reqRate: 10},
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
	return f
}

func TestFleetViewRendersTrafficAndRows(t *testing.T) {
	f := fleetOfThree(t)
	view := f.View()
	for _, want := range []string{
		"INGRESS", "LOAD", "IMBALANCE", "req/s", "trend",
		"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("fleet view missing %q:\n%s", want, view)
		}
	}
}

func TestFleetRowStatesAreDistinct(t *testing.T) {
	a, b, c := "idle:1", "orphan:1", "down:1"
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
	a, b, c := "idle:1", "orphan:1", "down:1"
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
	var nodes []resolvedNode
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
