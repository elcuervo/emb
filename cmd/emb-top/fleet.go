package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/elcuervo/emb/internal/embtop"
)

// Fleet cadence and membership bounds. Re-resolution is timer-based because
// net.Resolver exposes no TTL; the poll interval is far shorter than the
// staleness that matters operationally.
const (
	resolveInterval = 30 * time.Second
	resolveTimeout  = 5 * time.Second
	orphanGrace     = 5 * time.Minute

	// fleetChrome is the fixed number of non-node rows the fleet view spends on
	// the header, banner, membership, traffic totals, load bar, ingress header,
	// node legend, imbalance panel and footer.
	fleetChrome = 13
)

// fleetNode is one monitored node: its identity, its independent poll state
// (reusing the single-node dashboard's model) and its membership bookkeeping.
type fleetNode struct {
	key  string // stable row identity: a per-node name, else the address
	spec string // the specification that produced it
	name string // DNS name this row follows, "" for a literal address
	addr string // host:port dialed

	tui tuiModel // per-node dashboard state, charts and sampler

	resolved         bool
	reachable        bool
	unreachableSince time.Time
	polling          bool // a poll is outstanding; the next tick skips this node
	firstSeen        int
}

// tlsServerName is the name the TLS handshake verifies: the explicit
// -tls-server-name override wins, and a DNS-expanded node's own name is the
// fallback.
func tlsServerName(opts *options, nodeName string) string {
	if opts.tlsServerName != "" {
		return opts.tlsServerName
	}
	return nodeName
}

// hasCache reports whether any model on the node runs a cache, so the row
// only shows a cache column when a hit rate exists.
func (n *fleetNode) hasCache() bool {
	for _, ms := range n.tui.sampler.RawModels() {
		if ms.CacheMaxBytes > 0 {
			return true
		}
	}
	return false
}

// displayName is the row's identity: the name it follows when it has one,
// otherwise the address it dials.
func (n *fleetNode) displayName() string {
	if n.name != "" {
		return n.name
	}
	return n.addr
}

// fleet owns the monitored nodes and drives their independent polls. It is a
// Bubble Tea model for the interactive view and a plain loop driver for the
// headless modes.
type fleet struct {
	opts     *options
	resolver resolver

	nodes []*fleetNode
	byKey map[string]*fleetNode

	width, height int
	paused        bool
	showHelp      bool
	detail        bool
	sel           int
	scroll        int

	aggHistory []float64

	tickScheduled bool
	lastRefresh   time.Time
	refreshNow    bool
	refreshing    bool
	refreshErr    error

	// tunables, overridden by tests
	resolveEvery time.Duration
	grace        time.Duration
	now          func() time.Time
	newClient    func(addr, tlsName string) dashboardClient

	nextSeen int
}

// newFleet builds the monitored fleet from an initial resolution. Each node
// gets its own RESP client and its own single-node dashboard model, so its
// poll cannot touch another's connection or counters.
func newFleet(resolved []resolvedNode, opts *options) *fleet {
	return newFleetWith(resolved, opts, netResolver{}, nil)
}

// newFleetWith is newFleet with an injectable resolver and client factory
// (tests supply fakes). A nil resolver or factory takes the production one.
func newFleetWith(resolved []resolvedNode, opts *options, r resolver, mk func(addr, tlsName string) dashboardClient) *fleet {
	f := &fleet{
		opts:         opts,
		resolver:     netResolver{},
		byKey:        map[string]*fleetNode{},
		resolveEvery: resolveInterval,
		grace:        orphanGrace,
		now:          time.Now,
	}
	if r != nil {
		f.resolver = r
	}
	if mk != nil {
		f.newClient = mk
	} else {
		f.newClient = func(addr, tlsName string) dashboardClient {
			c := embtop.NewClient(addr, opts.password, opts.useTLS)
			if name := tlsServerName(opts, tlsName); name != "" {
				c.SetTLSServerName(name)
			}
			return c
		}
	}
	f.aggHistory = nil
	for _, rn := range resolved {
		f.addNode(rn)
	}
	return f
}

func (f *fleet) addNode(rn resolvedNode) *fleetNode {
	n := &fleetNode{
		key:       rn.key,
		spec:      rn.spec,
		name:      rn.name,
		addr:      rn.addr,
		tui:       newTUI(f.newClient(rn.addr, rn.name), f.opts.interval, f.opts.window),
		resolved:  true,
		firstSeen: f.nextSeen,
	}
	f.nextSeen++
	f.nodes = append(f.nodes, n)
	f.byKey[rn.key] = n
	return n
}

func (f *fleet) close() {
	for _, n := range f.nodes {
		_ = n.tui.client.Close()
	}
}

// model returns the fleet as a Bubble Tea model. The fleet drives polling and
// refresh for every node, including a fleet of one.
func (f *fleet) model() tea.Model { return f }

// ---- resolution & membership ----

type resolveMsg struct {
	nodes []resolvedNode
	err   error
}

func (f *fleet) dueRefresh() bool {
	return f.refreshNow || f.lastRefresh.IsZero() ||
		f.now().Sub(f.lastRefresh) >= f.resolveEvery
}

func (f *fleet) resolveCmd() tea.Cmd {
	specs := f.opts.specs
	r := f.resolver
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
		defer cancel()
		nodes, err := resolveSpecs(ctx, r, specs)
		return resolveMsg{nodes: nodes, err: err}
	}
}

// refreshSync re-resolves once, synchronously, for the headless modes.
func (f *fleet) refreshSync() {
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	nodes, err := resolveSpecs(ctx, f.resolver, f.opts.specs)
	if err != nil {
		f.refreshErr = err
		return
	}
	f.refreshErr = nil
	f.applyResolved(nodes)
}

// applyResolved folds a resolution result into the fleet: known rows update in
// place (following a name whose address moved), new addresses are added, and
// addresses that vanished are marked unresolved rather than dropped.
func (f *fleet) applyResolved(resolved []resolvedNode) {
	f.lastRefresh = f.now()
	f.refreshNow = false
	seen := make(map[string]bool, len(resolved))
	for _, rn := range resolved {
		seen[rn.key] = true
		if n := f.byKey[rn.key]; n != nil {
			if n.addr != rn.addr { // a name's record moved: follow it
				n.tui.client.SetAddr(rn.addr)
				n.addr = rn.addr
				n.tui.reset()
			}
			n.name = rn.name
			n.resolved = true
			continue
		}
		f.addNode(rn)
	}
	for _, n := range f.nodes {
		if !seen[n.key] {
			n.resolved = false
		}
	}
}

// prune removes a node only once it is both unresolved and unreachable beyond
// the grace period, so a node that outlives its DNS record keeps its row while
// it still answers.
func (f *fleet) prune() {
	now := f.now()
	kept := f.nodes[:0]
	for _, n := range f.nodes {
		remove := !n.resolved && !n.reachable &&
			!n.unreachableSince.IsZero() && now.Sub(n.unreachableSince) > f.grace
		if remove {
			_ = n.tui.client.Close()
			delete(f.byKey, n.key)
			continue
		}
		kept = append(kept, n)
	}
	f.nodes = kept
	f.clampSel()
	f.clampFleetScroll()
}

// ---- polling ----

type nodePollMsg struct {
	key    string
	res    *embtop.PollResult
	dialed bool
	err    error
}

// pollOnce runs one node's pipelined poll with reconnect, resetting the event
// cursor when a fresh connection was established.
func (f *fleet) pollOnce(n *fleetNode) nodePollMsg {
	client := n.tui.client
	afterSeq := n.tui.lastSeq
	dialed, err := client.EnsureConn()
	if err != nil {
		return nodePollMsg{key: n.key, err: err}
	}
	if dialed {
		afterSeq = 0 // a fresh (or restarted) node numbers events from the start
	}
	res, err := client.Poll(append([]string(nil), n.tui.known...), afterSeq)
	if err != nil {
		return nodePollMsg{key: n.key, err: err}
	}
	return nodePollMsg{key: n.key, res: res, dialed: dialed}
}

func (f *fleet) pollCmd(n *fleetNode) tea.Cmd {
	return func() tea.Msg { return f.pollOnce(n) }
}

// pollAllSync polls every node concurrently and applies the results in order.
func (f *fleet) pollAllSync() int {
	msgs := make([]nodePollMsg, len(f.nodes))
	var wg sync.WaitGroup
	for i, n := range f.nodes {
		wg.Add(1)
		go func(i int, n *fleetNode) {
			defer wg.Done()
			msgs[i] = f.pollOnce(n)
		}(i, n)
	}
	wg.Wait()
	ok := 0
	for _, m := range msgs {
		if m.err == nil {
			ok++
		}
		f.applyPoll(m)
	}
	return ok
}

// applyPoll folds one node's poll outcome into its row. A fresh connection
// rebases that node's sampler so the aggregate cannot spike against counters
// from before the gap.
func (f *fleet) applyPoll(m nodePollMsg) {
	n := f.byKey[m.key]
	if n == nil {
		return
	}
	n.polling = false
	if m.err != nil {
		n.reachable = false
		if n.unreachableSince.IsZero() {
			n.unreachableSince = f.now()
		}
		n.tui.connected = false
		f.refreshNow = true // a pod that moved is most likely to be seen now
		return
	}
	if m.dialed {
		n.tui.reset()
	}
	n.reachable = true
	n.unreachableSince = time.Time{}
	n.tui.connected = true
	n.tui.lastGood = f.now()
	n.tui.applyResult(m.res)
}

// ---- bubbletea model ----

func (f *fleet) Init() tea.Cmd {
	f.tickScheduled = true
	return tea.Tick(f.opts.interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (f *fleet) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		f.layout()
		f.clampFleetScroll()
		return f, nil

	case tea.KeyMsg:
		return f.key(msg)

	case tickMsg:
		f.tickScheduled = false
		if f.paused {
			return f, nil
		}
		f.pushAggregate()
		f.prune()
		// Always keep the poll cadence: the next tick is scheduled before this
		// round's polls finish, so a stalled node cannot hold back the others.
		next := f.schedule()
		var cmds []tea.Cmd
		if f.dueRefresh() && !f.refreshing {
			f.refreshing = true
			f.lastRefresh = f.now()
			cmds = append(cmds, f.resolveCmd())
		}
		for _, n := range f.nodes {
			if n.polling {
				continue // its previous poll is still outstanding; don't overlap it
			}
			n.polling = true
			cmds = append(cmds, f.pollCmd(n))
		}
		if len(cmds) == 0 {
			return f, next
		}
		return f, tea.Batch(append(cmds, next)...)

	case nodePollMsg:
		f.applyPoll(msg)
		return f, nil

	case resolveMsg:
		f.refreshing = false
		if msg.err != nil {
			f.refreshErr = msg.err // keep the known fleet on resolver failure
		} else {
			f.refreshErr = nil
			f.applyResolved(msg.nodes)
		}
		return f, nil
	}
	return f, nil
}

func (f *fleet) schedule() tea.Cmd {
	if f.tickScheduled || f.paused {
		return nil
	}
	f.tickScheduled = true
	return tea.Tick(f.opts.interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (f *fleet) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return f, tea.Quit
	case "?":
		f.showHelp = !f.showHelp
		return f, nil
	case "p", " ":
		f.paused = !f.paused
		for _, n := range f.nodes {
			n.tui.paused = f.paused
		}
		if !f.paused {
			return f, f.schedule()
		}
		return f, nil
	case "r":
		for _, n := range f.nodes {
			n.tui.reset()
		}
		f.aggHistory = nil
		return f, nil
	case "esc":
		f.detail = false
		return f, nil
	case "enter":
		if len(f.nodes) > 1 && !f.detail {
			f.detail = true
		}
		return f, nil
	case "j", "down":
		if f.detail || len(f.nodes) == 1 {
			return f.delegate(msg)
		}
		f.sel++
		f.clampSel()
		return f, nil
	case "k", "up":
		if f.detail || len(f.nodes) == 1 {
			return f.delegate(msg)
		}
		f.sel--
		f.clampSel()
		return f, nil
	}
	if f.detail || len(f.nodes) == 1 {
		return f.delegate(msg)
	}
	return f, nil
}

// delegate forwards a key to the selected node's single-node model, dropping
// its own scheduling command because the fleet owns the poll chain.
func (f *fleet) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	n := f.selected()
	if n == nil {
		return f, nil
	}
	um, _ := n.tui.Update(msg)
	n.tui = um.(tuiModel)
	n.tui.tickScheduled = false
	n.tui.pollInFlight = false
	return f, nil
}

func (f *fleet) selected() *fleetNode {
	if len(f.nodes) == 0 {
		return nil
	}
	f.clampSel()
	return f.nodes[f.sel]
}

func (f *fleet) clampSel() {
	if f.sel >= len(f.nodes) {
		f.sel = len(f.nodes) - 1
	}
	if f.sel < 0 {
		f.sel = 0
	}
}

// ---- aggregate ----

// aggregate sums the reachable nodes' latest rates and gauges. Summing each
// node's own rate keeps a rejoining node from spiking the total.
func (f *fleet) aggregate() (req, tok, errRate float64, active, conns int64) {
	for _, n := range f.nodes {
		if !n.reachable {
			continue
		}
		p := n.tui.sampler.Latest
		req += p.ReqRate
		tok += p.TokRate
		errRate += p.ErrRate
		active += p.Active
		conns += p.Conns
	}
	return
}

func (f *fleet) pushAggregate() {
	req, _, _, _, _ := f.aggregate()
	f.aggHistory = append(f.aggHistory, req)
	if len(f.aggHistory) > f.opts.window {
		f.aggHistory = f.aggHistory[len(f.aggHistory)-f.opts.window:]
	}
}

// ---- layout ----

func (f *fleet) layout() {
	for _, n := range f.nodes {
		n.tui.width, n.tui.height = f.width, f.height
		n.tui.layoutCharts()
	}
}

func (f *fleet) visibleNodes() int {
	h := f.height - fleetChrome
	if h < 1 {
		h = 1
	}
	return h
}

func (f *fleet) clampFleetScroll() {
	max := len(f.nodes) - f.visibleNodes()
	if max < 0 {
		max = 0
	}
	if f.scroll > max {
		f.scroll = max
	}
	if f.scroll < 0 {
		f.scroll = 0
	}
}

// ---- rendering ----

func (f *fleet) View() string {
	if f.showHelp {
		return f.helpView()
	}
	if len(f.nodes) == 1 {
		return f.nodes[0].tui.View()
	}
	if f.detail {
		if n := f.selected(); n != nil {
			return footerStyle.Render("◂ esc back to fleet") + "\n" + n.tui.View()
		}
		f.detail = false
	}
	return f.fleetView()
}

func (f *fleet) fleetView() string {
	var b strings.Builder
	b.WriteString(f.headerView())
	b.WriteString("\n")
	b.WriteString(f.fleetBannerView())
	b.WriteString("\n")
	b.WriteString(f.membershipView())
	b.WriteString("\n")
	b.WriteString(f.totalsView())
	b.WriteString("\n")
	b.WriteString(f.loadView())
	b.WriteString("\n")
	for _, line := range f.ingressRows() {
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, line := range f.imbalanceView() {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(f.footerView())
	return b.String()
}

func (f *fleet) headerView() string {
	line := headerStyle.Render("emb-top v"+version) +
		" · " + labelStyle.Render(fmt.Sprintf("%d nodes", len(f.nodes))) +
		" · poll " + f.opts.interval.String()
	if f.refreshErr != nil {
		line += " " + warnStyle.Render("(resolve failed)")
	}
	if f.paused {
		line += " " + warnStyle.Render("(paused)")
	}
	return lipgloss.NewStyle().Width(f.width).Render(line)
}

// membershipView reports membership and traffic separately, so the DNS-set
// versus client-set gap is visible instead of misread as under-utilization.
func (f *fleet) membershipView() string {
	discovered, traffic, idle, orphaned, unreachable := 0, 0, 0, 0, 0
	for _, n := range f.nodes {
		discovered++
		if !n.resolved {
			orphaned++
		}
		if !n.reachable {
			unreachable++
			continue
		}
		if f.nodeIdle(n) {
			idle++
		} else {
			traffic++
		}
	}
	line := dimStyle.Render(fmt.Sprintf("  discovered %d · receiving traffic %d · idle %d · orphaned %d",
		discovered, traffic, idle, orphaned))
	if unreachable > 0 {
		line += " · " + errStyle.Render(fmt.Sprintf("unreachable %d", unreachable))
	}
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(line)
}

func widthOr(w, def int) int {
	if w <= 0 {
		return def
	}
	return w
}

func (f *fleet) fleetBannerView() string {
	st, sigs := fleetHealth(f.fleetInputs())
	dot := okStyle.Render("●")
	switch st {
	case healthNoData:
		dot = warnStyle.Render("○")
	case healthDegraded:
		dot = warnStyle.Render("●")
	case healthCritical:
		dot = errStyle.Render("●")
	}
	chips := make([]string, 0, len(sigs))
	for _, s := range sigs {
		chips = append(chips, styleFor(s.level).Render(s.text))
	}
	line := dot + " " + headerStyle.Render(fmt.Sprintf("%-8s", healthLabel(st)))
	if len(chips) > 0 {
		line += "  " + strings.Join(chips, dimStyle.Render(" │ "))
	}
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(line)
}

// totalsView is the fleet's inbound/outbound totals with a sparkline of the
// aggregate request rate over the window.
func (f *fleet) totalsView() string {
	req, tok, errRate, active, conns := f.aggregate()
	errPct := 0.0
	if req > 0 {
		errPct = errRate / req * 100
	}
	var p95s []int64
	for _, n := range f.nodes {
		if !n.reachable {
			continue
		}
		if _, _, p95, ok := n.tui.sampler.Latency(); ok {
			p95s = append(p95s, p95)
		}
	}
	line := labelStyle.Render("IN  ") + fmtRate(req) + " req/s" +
		dimStyle.Render(" · ") + labelStyle.Render("OUT ") + fmtRate(tok) + " tok/s" +
		dimStyle.Render(" · ") + fmt.Sprintf("%d active · %d conns", active, conns) +
		dimStyle.Render(" · ") + fmt.Sprintf("err %.1f%%", errPct) +
		dimStyle.Render(" · ") + "p95 " + fmtLatency(medianInt64(p95s)) +
		dimStyle.Render("   trend ") + sparkline(f.aggHistory, 24, reqLineStyle)
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(line)
}

// loadView is the fleet's load distribution: one segment per node, its width
// its share of inbound requests, coloured to match the node's row.
func (f *fleet) loadView() string {
	reqTotal, _, _, _, _ := f.aggregate()
	rates := make([]float64, len(f.nodes))
	for i, n := range f.nodes {
		if n.reachable {
			rates[i] = n.tui.sampler.Latest.ReqRate
		}
	}
	detail := "no traffic yet"
	if reqTotal > 0 {
		busiest, top := 0, -1.0
		for i, r := range rates {
			if r > top {
				top, busiest = r, i
			}
		}
		detail = fmt.Sprintf("busiest %s %.0f%% · expected %.0f%% each",
			f.nodes[busiest].displayName(), top/reqTotal*100, 100/float64(len(f.nodes)))
	}
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(
		labelStyle.Render("LOAD ") + stackedShareBar(rates, 36) + "  " + dimStyle.Render(detail))
}

// sparkRamp is the block-character ramp for trend sparklines (low → high).
var sparkRamp = []rune("▁▂▃▄▅▆▇█")

// sparkline resamples vals to w columns and draws them with the block ramp,
// scaled between the series' own min and max so motion stays visible. An empty
// series renders a low flat line.
func sparkline(vals []float64, w int, style lipgloss.Style) string {
	if w < 1 {
		w = 1
	}
	if len(vals) == 0 {
		return style.Render(strings.Repeat("▁", w))
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	span := hi - lo
	var b strings.Builder
	for x := 0; x < w; x++ {
		idx := x * len(vals) / w
		if idx >= len(vals) {
			idx = len(vals) - 1
		}
		level := 0
		if span > 0 {
			level = int((vals[idx]-lo)/span*float64(len(sparkRamp)-1) + 0.5)
		}
		if level < 0 {
			level = 0
		}
		if level >= len(sparkRamp) {
			level = len(sparkRamp) - 1
		}
		b.WriteRune(sparkRamp[level])
	}
	return style.Render(b.String())
}

// filledCells is the number of filled cells for frac in a w-cell bar.
func filledCells(frac float64, w int) int {
	if w < 1 {
		w = 1
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac*float64(w) + 0.5)
	if n > w {
		n = w
	}
	return n
}

// inboundBar draws a proportional bar: the filled part in the node's colour,
// the remainder dim, so every node's inbound rate is comparable at a glance.
func inboundBar(frac float64, w int, fill lipgloss.Style) string {
	n := filledCells(frac, w)
	return fill.Render(strings.Repeat("█", n)) + dimStyle.Render(strings.Repeat("░", w-n))
}

// shareCells splits w cells across rates in proportion, assigning the rounding
// remainder to the largest segment so an idle node never gains cells.
func shareCells(rates []float64, w int) []int {
	cells := make([]int, len(rates))
	if w < 1 || len(rates) == 0 {
		return cells
	}
	total, bi := 0.0, 0
	for i, r := range rates {
		if r > 0 {
			total += r
		}
		if rates[i] > rates[bi] {
			bi = i
		}
	}
	if total <= 0 {
		return cells
	}
	sum := 0
	for i, r := range rates {
		cells[i] = int(r/total*float64(w) + 0.5)
		sum += cells[i]
	}
	cells[bi] += w - sum
	if cells[bi] < 0 {
		cells[bi] = 0
	}
	return cells
}

// stackedShareBar draws one segment per rate, its width its share of the total,
// coloured by node identity, so the fleet's whole load is one bar.
func stackedShareBar(rates []float64, w int) string {
	if w < 1 {
		w = 1
	}
	cells := shareCells(rates, w)
	var b strings.Builder
	used := 0
	for i, n := range cells {
		if n > 0 {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(modelColors[i%len(modelColors)]))
			b.WriteString(style.Render(strings.Repeat("█", n)))
		}
		used += n
	}
	if used < w {
		b.WriteString(dimStyle.Render(strings.Repeat("░", w-used)))
	}
	return b.String()
}

// ratesOf extracts the request-rate series from a node's window.
func ratesOf(hist []embtop.Point) []float64 {
	out := make([]float64, len(hist))
	for i, p := range hist {
		out[i] = p.ReqRate
	}
	return out
}

// padTo pads s on the right to w columns (or clips it), so a state label can
// stand in for the activity zone without moving the columns after it.
func padTo(s string, w int) string {
	n := w - lipgloss.Width(s)
	if n > 0 {
		return s + strings.Repeat(" ", n)
	}
	if n < 0 {
		return trimModelLen(s, w)
	}
	return s
}

// ingressRows renders the per-node inbound-traffic table: one row per node
// with its identity, inbound req/s, share of the fleet, a proportional bar
// scaled to the busiest node and a trend sparkline. Optional signal columns
// (p95, cpu, cache, err) fit when the terminal is wide enough.
func (f *fleet) ingressRows() []string {
	if len(f.nodes) == 0 {
		return []string{dimStyle.Render("  no nodes to monitor")}
	}
	reqTotal, _, _, _, _ := f.aggregate()
	maxV := 0.0
	for _, n := range f.nodes {
		if r := n.tui.sampler.Latest.ReqRate; n.reachable && r > maxV {
			maxV = r
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	l := newFleetRowLayout(widthOr(f.width, 120))
	out := []string{dimStyle.Render("INGRESS  req/s into each node · bar scaled to the busiest · trend = recent polls")}
	vis := f.visibleNodes()
	for i := f.scroll; i < f.scroll+vis && i < len(f.nodes); i++ {
		out = append(out, f.ingressRow(f.nodes[i], reqTotal, maxV, i == f.sel, l))
	}
	out = append(out, f.nodeLegend(len(f.nodes)))
	return out
}

func (f *fleet) nodeIdle(n *fleetNode) bool {
	if !n.reachable {
		return false
	}
	p := n.tui.sampler.Latest
	return n.tui.polls >= 2 && p.ReqRate <= 0 && p.TokRate <= 0
}

// nodeState is the distinct state a node row can be in.
type nodeState int

const (
	nodeOK nodeState = iota
	nodeIdle
	nodeOrphaned
	nodeUnreachable
)

func (f *fleet) nodeStateOf(n *fleetNode) nodeState {
	switch {
	case !n.reachable:
		return nodeUnreachable
	case !n.resolved:
		return nodeOrphaned
	case f.nodeIdle(n):
		return nodeIdle
	default:
		return nodeOK
	}
}

// nodeHealth is the per-node verdict the row's trend colour follows.
func (f *fleet) nodeHealth(n *fleetNode) healthStatus {
	switch f.nodeStateOf(n) {
	case nodeUnreachable:
		return healthCritical
	case nodeOrphaned:
		return healthDegraded
	}
	st, _ := health(n.tui.healthState())
	return st
}

// ingressRow is one node's inbound-traffic line. Columns are fixed by the
// layout so a value growing never shifts the ones after it; a node that is not
// OK replaces the activity zone with its state, keeping the signal columns
// aligned with the healthy rows.
func (f *fleet) ingressRow(n *fleetNode, reqTotal, maxV float64, selected bool, l fleetRowLayout) string {
	cursor := "  "
	if selected {
		cursor = labelStyle.Render("▶ ")
	}
	dot := okStyle.Render("●")
	state, stateStyle := "", okStyle
	dim := false
	switch f.nodeStateOf(n) {
	case nodeUnreachable:
		dot, dim = errStyle.Render("✗"), true
		since := ""
		if !n.unreachableSince.IsZero() {
			since = " " + fmtDuration(int64(f.now().Sub(n.unreachableSince).Seconds()))
		}
		state, stateStyle = "unreachable"+since, errStyle
	case nodeOrphaned:
		dot, state, stateStyle = warnStyle.Render("◌"), "orphaned", warnStyle
	case nodeIdle:
		dot, state, stateStyle = dimStyle.Render("○"), "idle", dimStyle
	}

	p := n.tui.sampler.Latest
	rate := p.ReqRate
	if !n.reachable {
		dim = true
	}
	share := 0.0
	if reqTotal > 0 {
		share = rate / reqTotal
	}
	ident := n.displayName()
	if ident != n.addr {
		ident += " " + n.addr
	}
	num := func(s string) string {
		if dim {
			return dimStyle.Render(s)
		}
		return labelStyle.Render(s)
	}

	row := cursor + dot + " " + headerStyle.Width(l.identW).Render(trimModelLen(ident, l.identW))
	if l.showIn {
		row += " " + num(colRight(fmtRate(rate)+" r/s", l.inW))
	}
	if l.showShare {
		row += " " + num(colRight(fmt.Sprintf("%.0f%%", share*100), l.shareW))
	}

	zoneW := l.barW
	if l.showSpark {
		zoneW += 1 + l.sparkW
	}
	if state != "" {
		row += " " + stateStyle.Render(padTo(state, zoneW))
	} else {
		fill := lipgloss.NewStyle().Foreground(lipgloss.Color(modelColors[n.firstSeen%len(modelColors)]))
		row += " " + inboundBar(rate/maxV, l.barW, fill)
		if l.showSpark {
			_, window, _ := n.tui.sampler.Snapshot()
			row += " " + sparkline(ratesOf(window), l.sparkW, styleFor(f.nodeHealth(n)))
		}
	}
	if l.showP95 {
		txt := "—"
		if _, _, p95, ok := n.tui.sampler.Latency(); ok && n.reachable {
			txt = fmtLatency(p95)
		}
		row += " " + dimStyle.Render(colRight("p95 "+txt, l.latW))
	}
	if l.showCPU {
		cpu := p.CPUPercent / float64(nodeParallelism(p.GoMaxProcs))
		row += " " + num(colRight(fmt.Sprintf("cpu %.0f%%", cpu), l.cpuW))
	}
	if l.showCache {
		cache := "—"
		if n.hasCache() {
			cache = fmt.Sprintf("%.0f%%", p.CacheHitRate)
		}
		row += " " + num(colRight("cache "+cache, l.cacheW))
	}
	if l.showErr {
		errPct := 0.0
		if rate > 0 {
			errPct = p.ErrRate / rate * 100
		}
		row += " " + num(colRight(fmt.Sprintf("err %.1f%%", errPct), l.errW))
	}
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(row)
}

// colRight right-aligns s in a w-column field, clipping it first so a value
// that outgrows its column cannot push the ones after it.
func colRight(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = trimModelLen(s, w)
	}
	return fmt.Sprintf("%*s", w, s)
}

// fleetRowLayout is the per-node ingress row's column grid: identity, inbound
// rate, share, the activity bar and trend, and the signal columns that fit.
// It is a pure function of the terminal width, so a value changing never moves
// a column.
type fleetRowLayout struct {
	identW, inW, shareW, barW, sparkW, latW, cpuW, cacheW, errW        int
	showIn, showShare, showP95, showCPU, showCache, showErr, showSpark bool
}

func newFleetRowLayout(width int) fleetRowLayout {
	if width <= 0 {
		width = 120
	}
	l := fleetRowLayout{
		identW: 24, inW: 10, shareW: 5, sparkW: 14, latW: 12,
		cpuW: 8, cacheW: 10, errW: 8,
		showIn: true, showShare: true, showP95: true, showCPU: true,
		showCache: true, showErr: true, showSpark: true,
	}
	// Everything except the bar and its leading space.
	fixed := func() int {
		w := 4 + l.identW + 1 + l.inW + 1 + l.shareW
		if l.showSpark {
			w += 1 + l.sparkW
		}
		if l.showP95 {
			w += 1 + l.latW
		}
		if l.showCPU {
			w += 1 + l.cpuW
		}
		if l.showCache {
			w += 1 + l.cacheW
		}
		if l.showErr {
			w += 1 + l.errW
		}
		return w
	}
	const minBar = 10
	for _, drop := range []*bool{&l.showErr, &l.showCache, &l.showCPU, &l.showP95, &l.showSpark} {
		if fixed()+1+minBar <= width {
			break
		}
		*drop = false
	}
	l.barW = width - fixed() - 1
	if l.barW < minBar {
		l.barW = minBar
	}
	return l
}

func (f *fleet) nodeLegend(n int) string {
	vis := f.visibleNodes()
	first, last := f.scroll+1, f.scroll+vis
	if last > n {
		last = n
	}
	if last < first {
		last = first
	}
	legend := "  " + dimStyle.Render(fmt.Sprintf("nodes %d–%d of %d", first, last, n)) +
		dimStyle.Render(" · share of fleet req/s · j/k select · enter detail")
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(legend)
}

func (f *fleet) footerView() string {
	return footerStyle.Render("q quit · p pause · r reset · j/k select · enter detail · esc back · ? help") + "\n"
}

// ---- cross-node imbalance panel ----

// imbalanceCheck is one cross-node check for the panel: its name, whether it
// passed, and a one-line detail naming the range or the offending node.
type imbalanceCheck struct {
	name   string
	ok     bool
	level  healthStatus
	detail string
}

// imbalanceChecks runs the actionable cross-node checks: inbound spread,
// latency, errors and CPU. Cache spread is informational and deliberately not
// one of them.
func imbalanceChecks(nodes []fleetNodeInput) []imbalanceCheck {
	return []imbalanceCheck{
		inboundSpreadCheck(nodes),
		latencySpreadCheck(nodes),
		errorCheck(nodes),
		cpuCheck(nodes),
	}
}

func inboundSpreadCheck(nodes []fleetNodeInput) imbalanceCheck {
	type sample struct {
		label string
		rate  float64
	}
	var s []sample
	for _, n := range nodes {
		if n.health.connected && !n.unreachable && !n.idle {
			s = append(s, sample{n.label, n.reqRate})
		}
	}
	if len(s) < 2 {
		return imbalanceCheck{name: "inbound", ok: true, detail: "fewer than two busy nodes"}
	}
	lo, hi, top, total := s[0].rate, s[0].rate, 0, 0.0
	for i, x := range s {
		total += x.rate
		if x.rate < lo {
			lo = x.rate
		}
		if x.rate > hi {
			hi, top = x.rate, i
		}
	}
	// Same peers-mean rule the verdict uses, so the panel and the banner agree.
	peersMean := (total - hi) / float64(len(s)-1)
	ok := peersMean <= 0 || hi <= peersMean*fleetSkewDegraded
	detail := fmt.Sprintf("range %s–%s req/s", fmtRate(lo), fmtRate(hi))
	if !ok {
		detail = fmt.Sprintf("%s is %.1f× its peers' mean (%.1f req/s) — %s",
			s[top].label, hi/peersMean, peersMean, detail)
	}
	return imbalanceCheck{name: "inbound", ok: ok, level: healthDegraded, detail: detail}
}

func latencySpreadCheck(nodes []fleetNodeInput) imbalanceCheck {
	worst, worstRatio := fleetNodeInput{}, 0.0
	found := false
	for i, n := range nodes {
		if !n.health.connected || n.health.p95Us <= 0 {
			continue
		}
		var peers []int64
		for j, o := range nodes {
			if j != i && o.health.connected && o.health.p95Us > 0 {
				peers = append(peers, o.health.p95Us)
			}
		}
		if len(peers) == 0 {
			continue
		}
		med := medianInt64(peers)
		if med <= 0 {
			continue
		}
		ratio := float64(n.health.p95Us) / float64(med)
		if !found || ratio > worstRatio {
			found, worst, worstRatio = true, n, ratio
		}
	}
	if !found {
		return imbalanceCheck{name: "latency", ok: true, detail: "no latency samples"}
	}
	return imbalanceCheck{
		name:  "latency",
		ok:    worstRatio <= fleetPeerP95Rise,
		level: healthDegraded,
		detail: fmt.Sprintf("worst %s p95 %s (%.1f× peers' median)",
			worst.label, fmtLatency(worst.health.p95Us), worstRatio),
	}
}

func errorCheck(nodes []fleetNodeInput) imbalanceCheck {
	worst, label := 0.0, ""
	for _, n := range nodes {
		if n.health.connected && n.health.errRatio > worst {
			worst, label = n.health.errRatio, n.label
		}
	}
	ok := worst <= errRatioDegraded
	detail := fmt.Sprintf("max %.1f%%", worst*100)
	if !ok {
		detail = fmt.Sprintf("%s %.1f%% — %s", label, worst*100, detail)
	}
	return imbalanceCheck{name: "errors", ok: ok, level: healthDegraded, detail: detail}
}

func cpuCheck(nodes []fleetNodeInput) imbalanceCheck {
	worst, label := 0.0, ""
	for _, n := range nodes {
		if n.health.connected && n.health.cpuPct > worst {
			worst, label = n.health.cpuPct, n.label
		}
	}
	ok := worst <= cpuDegradedPct
	detail := fmt.Sprintf("max %.0f%%", worst)
	if !ok {
		detail = fmt.Sprintf("%s %.0f%% — %s", label, worst, detail)
	}
	return imbalanceCheck{name: "cpu", ok: ok, level: healthDegraded, detail: detail}
}

// imbalanceView renders the cross-node checks as a compact pass/fail panel,
// with the failing check naming the node to look at.
func (f *fleet) imbalanceView() []string {
	checks := imbalanceChecks(f.fleetInputs())
	failed := 0
	for _, c := range checks {
		if !c.ok {
			failed++
		}
	}
	badge := okStyle.Render(fmt.Sprintf("✓ all %d pass", len(checks)))
	if failed > 0 {
		badge = warnStyle.Render(fmt.Sprintf("⚠ %d/%d failed", failed, len(checks)))
	}
	out := []string{labelStyle.Render("IMBALANCE") + "  " + badge}
	for _, c := range checks {
		mark, style := okStyle.Render("✓"), dimStyle
		if !c.ok {
			mark, style = warnStyle.Render("⚠"), warnStyle
		}
		out = append(out, lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(
			"  "+mark+" "+headerStyle.Width(8).Render(c.name)+" "+style.Render(c.detail)))
	}
	return out
}

// fleetInputs builds the fleet verdict's per-node inputs from the live rows.
func (f *fleet) fleetInputs() []fleetNodeInput {
	ins := make([]fleetNodeInput, 0, len(f.nodes))
	for _, n := range f.nodes {
		p := n.tui.sampler.Latest
		ins = append(ins, fleetNodeInput{
			label:       n.displayName(),
			health:      n.tui.healthState(),
			reqRate:     p.ReqRate,
			idle:        f.nodeIdle(n),
			orphaned:    !n.resolved,
			unreachable: !n.reachable && !n.unreachableSince.IsZero(),
			uptimeSecs:  p.UptimeSecs,
			hasCache:    n.hasCache(),
			cachePct:    p.CacheHitRate,
		})
	}
	return ins
}

func (f *fleet) helpView() string {
	lines := []string{
		headerStyle.Render("emb-top — fleet dashboard"),
		"",
		"  q / ctrl+c   quit",
		"  p / space    pause / resume polling",
		"  r            reset visible windows",
		"  j / k        select the node row (scrolls the list)",
		"  enter        open the selected node's per-model dashboard",
		"  esc          return to the fleet view",
		"  ? / h        toggle this help",
		"",
		"Flags:",
		"  -node host:port   node to monitor; repeat for several",
		"  -nodes a,b        comma-separated nodes",
		"  -addr host:port   single-node compatibility alias",
		"  -tls-server-name  TLS name to verify when it differs from the address",
		"  -interval 1s      poll interval · -window N history in polls",
		"  -password P       AUTH password · -tls connect over TLS",
		"  -once -samples N  headless line mode · -frames streamed frames",
		"",
		"A name is re-resolved every 30s and whenever a node goes unreachable;",
		"a node that stops resolving is marked orphaned while it still answers.",
		"",
		"The cluster view shows the fleet banner, inbound/outbound totals with a",
		"trend, a load-distribution bar, one ingress row per node (in req/s,",
		"share, a proportional bar and its trend) and a cross-node imbalance",
		"panel; enter a row for that node's per-model dashboard.",
	}
	return strings.Join(lines, "\n")
}

// ---- headless modes ----

// runOnce prints the fleet aggregate plus one section per node. A fleet of one
// keeps the single-node line format verbatim.
func (f *fleet) runOnce(ctx context.Context, interval time.Duration, samples int, out io.Writer) error {
	_ = ctx
	if len(f.nodes) == 1 {
		if c, ok := f.nodes[0].tui.client.(*embtop.Client); ok {
			return embtop.RunOnce(c, interval, samples, out)
		}
	}
	clients := make([]*embtop.Client, len(f.nodes))
	labels := make([]string, len(f.nodes))
	for i, n := range f.nodes {
		c, ok := n.tui.client.(*embtop.Client)
		if !ok {
			return fmt.Errorf("internal: -once needs embtop clients")
		}
		clients[i] = c
		labels[i] = n.displayName()
	}
	return embtop.RunOnceFleet(clients, labels, interval, samples, out)
}

// runFrames streams the fleet's complete frames. A fleet of one delegates to
// the single-node frame loop, so the website's live plate is byte-for-byte the
// dashboard it always was.
func (f *fleet) runFrames(ctx context.Context, interval time.Duration, window int, out io.Writer) error {
	if len(f.nodes) == 1 {
		if c, ok := f.nodes[0].tui.client.(*embtop.Client); ok {
			return runFrames(ctx, c, interval, window, out)
		}
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	f.width, f.height = frameWidth, frameHeight
	f.layout()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if f.dueRefresh() {
			f.refreshSync()
		}
		f.pushAggregate()
		f.pollAllSync()
		if err := emitFrame(out, f.View()); err != nil {
			return err
		}
	}
}
