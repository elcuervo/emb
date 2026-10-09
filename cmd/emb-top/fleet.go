package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
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

	// fleetChrome is the fixed number of non-node rows the fleet view spends
	// on the header, membership line, banner, aggregate band and footer.
	fleetChrome = 12
	// fleetAggH is the aggregate stream chart's canvas height.
	fleetAggH = 4
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
	firstSeen        int
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

	aggChart   streamlinechart.Model
	aggHistory []float64

	tickScheduled bool
	inFlight      int
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
			name := tlsName
			if name == "" {
				name = opts.tlsServerName
			}
			if name != "" {
				c.SetTLSServerName(name)
			}
			return c
		}
	}
	f.aggChart = newAggChart(60)
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

// newAggChart builds the aggregate request-rate stream chart at the given
// canvas width.
func newAggChart(w int) streamlinechart.Model {
	return streamlinechart.New(w, fleetAggH,
		streamlinechart.WithStyles(runes.ArcLineStyle, reqLineStyle),
		streamlinechart.WithLineChart(yFmtChart(w, fleetAggH, yFmtRate)))
}

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
		var cmds []tea.Cmd
		if f.dueRefresh() && !f.refreshing {
			f.refreshing = true
			f.lastRefresh = f.now()
			cmds = append(cmds, f.resolveCmd())
		}
		f.inFlight = 0
		for _, n := range f.nodes {
			f.inFlight++
			cmds = append(cmds, f.pollCmd(n))
		}
		if len(cmds) == 0 {
			return f, f.schedule()
		}
		return f, tea.Batch(cmds...)

	case nodePollMsg:
		f.inFlight--
		f.applyPoll(msg)
		if f.inFlight <= 0 {
			return f, f.schedule()
		}
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
		f.aggChart.ClearAllData()
		f.aggChart.Clear()
		f.aggChart.Draw()
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
	pushStream(&f.aggChart, req)
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
	w := f.width/2 - 4
	if w < 20 {
		w = 20
	}
	f.aggChart.Resize(w, fleetAggH)
	f.aggChart.Draw()
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
	b.WriteString(f.membershipView())
	b.WriteString("\n")
	b.WriteString(f.fleetBannerView())
	b.WriteString("\n")
	b.WriteString(f.aggregateView())
	b.WriteString("\n")
	for _, line := range f.nodeRows() {
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

func (f *fleet) aggregateView() string {
	req, tok, errRate, active, conns := f.aggregate()
	line := labelStyle.Render("aggregate") + " " +
		fmtRate(req) + " r/s · " + fmtRate(tok) + " t/s · " +
		fmtRate(errRate) + " err/s · " + fmt.Sprintf("%d active · %d conns", active, conns)
	return lipgloss.NewStyle().Width(widthOr(f.width, 120)).Render(line) + "\n" +
		borderStyle.Render(f.aggChart.View())
}

// pointStrip renders a node's request-rate history as the same heat cells the
// single-node model rows use, scaled to the busiest node in view.
func pointStrip(hist []embtop.Point, w int, maxV float64) string {
	if w < 1 {
		w = 1
	}
	if len(hist) == 0 || maxV <= 0 {
		return dimStyle.Render(strings.Repeat(heatCellBlock, w))
	}
	var b strings.Builder
	for x := 0; x < w; x++ {
		idx := x * len(hist) / w
		if idx >= len(hist) {
			idx = len(hist) - 1
		}
		b.WriteString(heatCellStyle(hist[idx].ReqRate / maxV).Render(heatCellBlock))
	}
	return b.String()
}

func (f *fleet) nodeRows() []string {
	if len(f.nodes) == 0 {
		return []string{dimStyle.Render("  no nodes to monitor")}
	}
	reqTotal, _, _, _, _ := f.aggregate()
	maxV := 0.0
	for _, n := range f.nodes {
		_, window, _ := n.tui.sampler.Snapshot()
		for _, p := range window {
			if p.ReqRate > maxV {
				maxV = p.ReqRate
			}
		}
	}
	vis := f.visibleNodes()
	out := make([]string, 0, vis+1)
	for i := f.scroll; i < f.scroll+vis && i < len(f.nodes); i++ {
		out = append(out, f.nodeRow(f.nodes[i], reqTotal, maxV, i == f.sel))
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

func (f *fleet) nodeRow(n *fleetNode, reqTotal, maxV float64, selected bool) string {
	cursor := "  "
	if selected {
		cursor = labelStyle.Render("▶ ")
	}
	dot := okStyle.Render("●")
	state := ""
	chips := ""
	switch f.nodeStateOf(n) {
	case nodeUnreachable:
		dot = errStyle.Render("✗")
		age := ""
		if !n.unreachableSince.IsZero() {
			age = fmtDuration(int64(f.now().Sub(n.unreachableSince).Seconds())) + " "
		}
		state = errStyle.Render("unreachable " + age)
	case nodeOrphaned:
		dot = warnStyle.Render("◌")
		state = warnStyle.Render("orphaned")
	case nodeIdle:
		dot = dimStyle.Render("○")
		state = dimStyle.Render("idle")
	}
	if state != "" {
		chips = state
	} else {
		st, sigs := health(n.tui.healthState())
		parts := []string{styleFor(st).Render(healthLabel(st))}
		for _, s := range sigs {
			if s.text == "connected" {
				continue
			}
			parts = append(parts, styleFor(s.level).Render(s.text))
		}
		chips = strings.Join(parts, dimStyle.Render(" · "))
	}

	share := "—"
	if reqTotal > 0 {
		p := n.tui.sampler.Latest
		if n.reachable {
			share = fmt.Sprintf("%.0f%% of %d (1/%d)", p.ReqRate/reqTotal*100, len(f.nodes), len(f.nodes))
		}
	}

	ident := n.displayName()
	if ident != n.addr {
		ident += " " + dimStyle.Render(n.addr)
	}
	_, window, _ := n.tui.sampler.Snapshot()
	row := cursor + dot + " " +
		headerStyle.Width(28).Render(trimModelLen(ident, 28)) + " " +
		labelStyle.Width(22).Render(trimModelLen(share, 22)) + " " +
		chips + "  " +
		pointStrip(window, f.nodeStripWidth(), maxV)
	return lipgloss.NewStyle().MaxWidth(widthOr(f.width, 120)).Render(row)
}

// nodeStripWidth gives the strip whatever room is left after the fixed
// columns, with a floor so it always renders.
func (f *fleet) nodeStripWidth() int {
	w := widthOr(f.width, 120) - 2 - 2 - 29 - 23 - 40
	if w < minStrip {
		w = minStrip
	}
	return w
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

// fleetInputs builds the fleet verdict's per-node inputs from the live rows.
func (f *fleet) fleetInputs() []fleetNodeInput {
	ins := make([]fleetNodeInput, 0, len(f.nodes))
	for _, n := range f.nodes {
		p := n.tui.sampler.Latest
		hasCache := false
		for _, ms := range n.tui.sampler.RawModels() {
			if ms.CacheMaxBytes > 0 {
				hasCache = true
				break
			}
		}
		ins = append(ins, fleetNodeInput{
			label:      n.displayName(),
			health:     n.tui.healthState(),
			reqRate:    p.ReqRate,
			idle:       f.nodeIdle(n),
			orphaned:   !n.resolved,
			uptimeSecs: p.UptimeSecs,
			hasCache:   hasCache,
			cachePct:   p.CacheHitRate,
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
