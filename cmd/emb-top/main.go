// emb-top is a live dashboard for a running emb node: aggregate and
// per-model throughput, latency percentiles, cache/resource gauges, rendered
// as a Bubble Tea TUI with ntcharts. It is a pure RESP2 client — no
// server-side changes required beyond the server's existing commands plus
// MONITOR, and no CGo/onnxruntime (build with CGO_ENABLED=0).
//
// Usage:
//
//	emb-top [-addr host:port] [-interval 1s] [-password p] [-tls]
//	emb-top -once -samples 10 [-interval 1s]   # headless, machine-readable
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

// version is injected at build time via -ldflags "-X main.version=…".
var version = "dev"

// ---- palette & panel styles (eye candy) ----

// heatColors is the heatmap color scale (dark-ish → hot). Cells with the
// lowest values blend toward the terminal background.
var heatColors = []lipgloss.Color{
	lipgloss.Color("#181825"), // base
	lipgloss.Color("#313244"), // mantle
	lipgloss.Color("#45475a"), // surface2
	lipgloss.Color("#89b4fa"), // blue
	lipgloss.Color("#74c7ec"), // sky
	lipgloss.Color("#a6e3a1"), // green
	lipgloss.Color("#f9e2af"), // yellow
	lipgloss.Color("#f38ba8"), // red
}

var (
	reqLineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))  // blue
	p95LineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13")) // magenta
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))  // cyan
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // red
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))           // green
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))            // yellow
	headerStyle  = lipgloss.NewStyle().Bold(true)
	metaStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	borderStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	modelColors  = []string{"4", "10", "5", "11", "6", "13", "2", "12"}
	chartBars    = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	}
)

func main() {
	addr := flag.String("addr", "localhost:6379", "emb node address (host:port)")
	interval := flag.Duration("interval", time.Second, "poll interval")
	password := flag.String("password", os.Getenv("EMB_TOP_PASSWORD"),
		"AUTH password (prefer $EMB_TOP_PASSWORD: command-line values are visible in process listings)")
	useTLS := flag.Bool("tls", false, "connect over TLS")
	once := flag.Bool("once", false, "headless mode: print polling lines and exit")
	samples := flag.Int("samples", 10, "number of polls in -once mode")
	window := flag.Int("window", 120, "history window (number of polls)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// time.NewTicker panics on a non-positive interval; fail clearly instead.
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "emb-top: -interval must be positive")
		os.Exit(2)
	}
	if *once && *samples < 1 {
		fmt.Fprintln(os.Stderr, "emb-top: -samples must be at least 1")
		os.Exit(2)
	}
	if *password != "" && !*useTLS && !isLoopback(*addr) {
		fmt.Fprintln(os.Stderr,
			"emb-top: warning: sending an AUTH password to a non-loopback address without -tls")
	}

	client := embtop.NewClient(*addr, *password, *useTLS)

	if *once {
		if err := embtop.RunOnce(client, *interval, *samples, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "emb-top:", err)
			os.Exit(1)
		}
		return
	}

	m := newTUI(client, *interval, *window)
	prog := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "emb-top:", err)
		os.Exit(1)
	}
}

// isLoopback reports whether addr (host:port) resolves to a loopback host, so
// a plaintext AUTH is only silent for local connections.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ---- messages ----

type tickMsg struct{}

type pollMsg struct {
	res *embtop.PollResult
	err error
}

// ---- TUI model ----

type tuiModel struct {
	client   *embtop.Client
	interval time.Duration
	sampler  *embtop.Sampler

	width, height int

	paused    bool
	showHelp  bool
	connected bool
	lastGood  time.Time
	scroll    int

	// tickScheduled/pollInFlight keep exactly one polling chain alive: pause
	// toggles must not spawn overlapping polls on the shared client conn.
	tickScheduled bool
	pollInFlight  bool

	known      []string // models polled via EMB.INFO
	modelOrder []string // server EMB.MODELS order
	lastSeq    uint64   // last MONITOR event seq seen

	reqChart streamlinechart.Model
	latChart streamlinechart.Model
	sparks   map[string]*sparkline.Model

	cacheBar barchart.Model
	cpuBar   barchart.Model
	memBar   barchart.Model
}

func newTUI(client *embtop.Client, interval time.Duration, window int) tuiModel {
	m := tuiModel{
		client:   client,
		interval: interval,
		sampler:  embtop.NewSampler(window),
		sparks:   map[string]*sparkline.Model{},
	}
	m.initCharts(80, 24)
	// Init returns the first tick, so record it as scheduled.
	m.tickScheduled = true
	return m
}

func (m *tuiModel) initCharts(w, h int) {
	chartW, chartH := streamDims(w, h)
	m.reqChart = streamlinechart.New(chartW, chartH,
		streamlinechart.WithStyles(runes.ArcLineStyle, reqLineStyle),
		streamlinechart.WithLineChart(yFmtChart(chartW, chartH, yFmtRate)))
	m.latChart = streamlinechart.New(chartW, chartH,
		streamlinechart.WithStyles(runes.ArcLineStyle, p95LineStyle),
		streamlinechart.WithLineChart(yFmtChart(chartW, chartH, yFmtLatency)))
	m.cacheBar = bar(100)
	m.cpuBar = bar(100)
	m.memBar = bar(0) // 0: autoscale
	for _, sp := range m.sparks {
		sp.Resize(m.sparkW(), 1)
	}
}

// yFmtChart builds a linechart like streamlinechart.New's default but with a
// compact Y-axis label formatter (12.3k, 4.5M, …) instead of raw integers.
func yFmtChart(w, h int, fmtV func(float64) string) *linechart.Model {
	lc := linechart.New(w, h, 0, 1, 0, 1,
		linechart.WithXYSteps(0, 2),
		linechart.WithAutoYRange(),
		linechart.WithUpdateHandler(linechart.YAxisUpdateHandler(1)),
		linechart.WithYLabelFormatter(func(_ int, v float64) string { return fmtV(v) }))
	return &lc
}

// yFmtRate / yFmtLatency are the Y-axis label formatters for the req/s and
// latency (µs-valued) stream charts.
func yFmtRate(v float64) string { return fmtRate(v) }

func yFmtLatency(v float64) string { return fmtLatency(int64(v)) }

// bar builds a horizontal gauge barchart; max 0 means autoscale.
func bar(max float64) barchart.Model {
	opts := []barchart.Option{barchart.WithHorizontalBars(), barchart.WithNoAxis()}
	if max > 0 {
		opts = append(opts, barchart.WithMaxValue(max))
	}
	return barchart.New(20, 2, opts...)
}

// streamDims derives the two stream-chart canvas sizes from the terminal.
// Each chart is wrapped in a border (2 cols) plus horizontal padding (2 cols),
// so the usable canvas is width/2 minus those decorations and the mid-gap.
func streamDims(width, height int) (int, int) {
	if width <= 0 {
		width = 80
	}
	w := (width-8)/2 - 2
	if w < 10 {
		w = 10
	}
	return w, 6
}

// hotW derives the heatmap grid width from the terminal.
func hotW(width int) int {
	if width <= 0 {
		width = 80
	}
	if w := width - 26; w > 10 {
		return w
	}
	return 10
}

func (m *tuiModel) sparkW() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	sw := w/4 - 4
	if sw < 6 {
		sw = 6
	}
	if sw > 34 {
		sw = 34
	}
	return sw
}

// ---- bubbletea plumbing ----

func (m tuiModel) Init() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layoutCharts()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "p", " ":
			m.paused = !m.paused
			if m.paused {
				return m, nil
			}
			return m, m.schedule()
		case "r":
			m.reset()
			return m, nil
		case "?", "h":
			m.showHelp = !m.showHelp
			return m, nil
		case "j", "down":
			m.scroll++
			return m, nil
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
			return m, nil
		}
		return m, nil

	case tickMsg:
		m.tickScheduled = false
		if m.paused || m.pollInFlight {
			return m, nil
		}
		m.pollInFlight = true
		return m, m.pollCmd()

	case pollMsg:
		m.pollInFlight = false
		if msg.err != nil {
			m.connected = false
		} else {
			m.connected = true
			m.lastGood = time.Now()
			m.applyResult(msg.res)
		}
		return m, m.schedule()
	}
	return m, nil
}

// schedule starts the next poll tick unless one is already pending, keeping a
// single polling chain so pause toggles cannot run overlapping polls on the
// shared client connection.
func (m *tuiModel) schedule() tea.Cmd {
	if m.tickScheduled || m.paused {
		return nil
	}
	m.tickScheduled = true
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{} })
}

// pollCmd captures a snapshot of the known-model list and does a single
// pipelined poll (with reconnect when the transport dropped).
func (m tuiModel) pollCmd() tea.Cmd {
	known := append([]string(nil), m.known...)
	afterSeq := m.lastSeq
	return func() tea.Msg {
		dialed, err := m.client.EnsureConn()
		if err != nil {
			return pollMsg{err: err}
		}
		if dialed {
			// A fresh (or restarted) node numbers events from the start;
			// a stale cursor would hide every new event.
			afterSeq = 0
		}
		res, err := m.client.Poll(known, afterSeq)
		if err != nil {
			return pollMsg{err: err}
		}
		return pollMsg{res: res}
	}
}

func (m *tuiModel) applyResult(res *embtop.PollResult) {
	m.lastSeq = res.NextSeq
	m.sampler.PushEvents(res.Events)
	p := m.sampler.Push(res)

	// Reconcile known models (EMB.MODELS each tick; EMB.INFO for the new
	// names starts next round).
	m.known = m.known[:0]
	m.modelOrder = m.modelOrder[:0]
	for _, mod := range res.Models {
		if _, ok := m.sparks[mod.Name]; !ok {
			sp := sparkline.New(m.sparkW(), 1,
				sparkline.WithStyle(modelStyle(len(m.modelOrder))))
			m.sparks[mod.Name] = &sp
		}
		m.known = append(m.known, mod.Name)
		m.modelOrder = append(m.modelOrder, mod.Name)
	}
	for name := range m.sparks {
		if !contains(m.modelOrder, name) {
			delete(m.sparks, name)
		}
	}

	// Stream charts: aggregate req/s and p95 latency.
	pushStream(&m.reqChart, p.ReqRate)
	if _, _, p95, ok := m.sampler.Latency(); ok {
		pushStream(&m.latChart, float64(p95))
	} else {
		pushStream(&m.latChart, 0)
	}

	// Per-model sparklines (req/s).
	for _, name := range m.modelOrder {
		if sp, ok := m.sparks[name]; ok {
			if hist := m.sampler.ModHist[name]; len(hist) > 0 {
				sp.Push(hist[len(hist)-1].ReqRate)
				sp.Draw()
			}
		}
	}

	// Gauges (clear-then-push: Push appends to the data set).
	m.cacheBar.Clear()
	m.cpuBar.Clear()
	m.memBar.Clear()
	m.cacheBar.Push(barchart.BarData{Label: "cache", Values: []barchart.BarValue{
		{Name: "hit", Value: p.CacheHitRate, Style: chartBars[0]}}})
	m.cpuBar.Push(barchart.BarData{Label: "cpu", Values: []barchart.BarValue{
		{Name: "cpu", Value: p.CPUPercent, Style: chartBars[1]}}})
	m.memBar.Push(barchart.BarData{Label: "mem", Values: []barchart.BarValue{
		{Name: "mem", Value: float64(p.MemMB), Style: chartBars[2]}}})
	m.cacheBar.Draw()
	m.cpuBar.Draw()
	m.memBar.Draw()
}

func latestRate(hist map[string][]embtop.ModelPoint, name string) float64 {
	pts := hist[name]
	if len(pts) == 0 {
		return 0
	}
	return pts[len(pts)-1].ReqRate
}

// pushStream pushes a value into a stream chart and guards against a
// degenerate all-zero Y range (idle node).
func pushStream(ch *streamlinechart.Model, v float64) {
	ch.Push(v)
	if ch.ViewMaxY() <= ch.ViewMinY() {
		ch.SetYRange(0, 1)
		ch.SetViewYRange(0, 1)
	}
	ch.Draw()
}

func (m *tuiModel) reset() {
	m.sampler.Reset()
	m.reqChart.ClearAllData()
	m.reqChart.Clear()
	m.reqChart.Draw()
	m.latChart.ClearAllData()
	m.latChart.Clear()
	m.latChart.Draw()
	for _, sp := range m.sparks {
		sp.Clear()
		sp.Draw()
	}
	m.cacheBar = bar(100)
	m.cpuBar = bar(100)
	m.memBar = bar(0)
	m.layoutCharts()
}

func (m *tuiModel) layoutCharts() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	chartW, chartH := streamDims(m.width, m.height)
	m.reqChart.Resize(chartW, chartH)
	m.latChart.Resize(chartW, chartH)
	gw := (m.width - 8) / 3
	if gw < 10 {
		gw = 10
	}
	m.cacheBar.Resize(gw, 2)
	m.cpuBar.Resize(gw, 2)
	m.memBar.Resize(gw, 2)
	for _, sp := range m.sparks {
		sp.Resize(m.sparkW(), 1)
	}
	m.reqChart.Draw()
	m.latChart.Draw()
	m.cacheBar.Draw()
	m.cpuBar.Draw()
	m.memBar.Draw()
}

// m.hotW is a convenience accessor for the heatmap grid width.
func (m *tuiModel) hotW() int { return hotW(m.width) }

// ---- rendering ----

func (m tuiModel) View() string {
	if m.showHelp {
		return m.helpView()
	}
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n")

	if len(m.modelOrder) > 0 {
		b.WriteString(borderStyle.Render(m.heatmapView()))
		b.WriteString("\n")
	}

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		borderStyle.Render(m.reqChart.View()),
		borderStyle.Render(m.latChart.View()),
	))
	b.WriteString("\n")

	for _, line := range m.modelsView() {
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, line := range m.gaugesView() {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(m.tickerView())
	b.WriteString(m.footerView())
	return b.String()
}

func (m tuiModel) headerView() string {
	p := m.sampler.Latest
	uptime := ""
	if p.UptimeSecs > 0 {
		uptime = fmt.Sprintf("uptime %s", fmtDuration(p.UptimeSecs))
	}
	status := warnStyle.Render("○ connecting")
	if m.connected {
		status = okStyle.Render("● connected")
	} else if m.lastGood.Unix() > 0 {
		status = errStyle.Render("● reconnecting")
	}
	line := headerStyle.Render("emb-top v"+version) +
		" · " + labelStyle.Render(m.client.Addr()) +
		" · " + uptime +
		" · " + fmt.Sprint(p.RegisteredModels) + " models" +
		" · poll " + m.interval.String()
	if p.ReqRate > 0 {
		line += " · " + labelStyle.Render(fmtRate(p.ReqRate)+" r/s")
	}
	if _, _, p95, ok := m.sampler.Latency(); ok {
		line += " · p95 " + dimStyle.Render(fmtLatency(p95))
	}
	line += " · " + status
	if m.paused {
		line += " " + warnStyle.Render("(paused)")
	}
	return lipgloss.NewStyle().Width(m.width).Render(line)
}

// heatmapView renders a hand-rolled model-activity heatmap: rows = models
// (busiest on top), columns = recent polls, cell = colored block intensity by
// req/s. Built by hand because lipgloss v1 does not style whitespace-only
// cells (the ntcharts heatmap widget colors space cells, which render as
// unstyled), so each cell is a colored non-space rune.
func (m tuiModel) heatmapView() string {
	if len(m.modelOrder) == 0 {
		return dimStyle.Render("no models loaded on node")
	}
	_, _, hist := m.sampler.Snapshot()
	rows := m.hotRows()
	if len(rows) == 0 {
		return ""
	}
	w := m.hotW()
	maxV := 0.0
	for _, name := range rows {
		for _, mp := range hist[name] {
			if mp.ReqRate > maxV {
				maxV = mp.ReqRate
			}
		}
	}
	lines := make([]string, 0, len(rows))
	for _, name := range rows {
		pts := hist[name]
		cell := heatCellBlock
		line := lipgloss.NewStyle().Foreground(lipgloss.Color(modelColorIdx(name, m.modelOrder))).
			Width(13).Render(name) + " "
		if len(pts) == 0 || maxV <= 0 {
			line += dimStyle.Render(strings.Repeat(cell, w))
			lines = append(lines, line)
			continue
		}
		for x := 0; x < w; x++ {
			idx := x * len(pts) / w
			if idx >= len(pts) {
				idx = len(pts) - 1
			}
			frac := pts[idx].ReqRate / maxV
			line += heatCellStyle(frac).Render(cell)
		}
		lines = append(lines, line)
	}
	legend := " " + labelStyle.Render("req/s · models × recent polls") + "  "
	for _, c := range heatColors {
		legend += lipgloss.NewStyle().Background(c).Foreground(c).Render(heatCellBlock)
	}
	return legend + "\n" + strings.Join(lines, "\n")
}

// heatCellBlock is the rune used per heatmap cell.
const heatCellBlock = "█"

// heatCellStyle maps a 0..1 intensity to a background color style.
func heatCellStyle(frac float64) lipgloss.Style {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	i := int(frac * float64(len(heatColors)-1))
	if i < 0 {
		i = 0
	}
	if i >= len(heatColors) {
		i = len(heatColors) - 1
	}
	return lipgloss.NewStyle().Background(heatColors[i]).Foreground(heatColors[i])
}

// hotRows returns model names in heatmap row order (busiest first), capped.
func (m tuiModel) hotRows() []string {
	rows := append([]string(nil), m.modelOrder...)
	sort.SliceStable(rows, func(i, j int) bool {
		return latestRate(m.sampler.ModHist, rows[i]) > latestRate(m.sampler.ModHist, rows[j])
	})
	if len(rows) > 8 {
		rows = rows[:8]
	}
	return rows
}

func modelColorIdx(name string, order []string) string {
	for i, n := range order {
		if n == name {
			return modelColors[i%len(modelColors)]
		}
	}
	return modelColors[0]
}

func (m tuiModel) modelsView() []string {
	if len(m.modelOrder) == 0 {
		return []string{dimStyle.Render("  no models loaded on node")}
	}
	// Reserve vertical space for header + heatmap + charts + gauges + ticker.
	maxRows := m.height - 20
	if maxRows < 1 {
		maxRows = 1
	}
	maxRows /= 2 // each model takes two lines (stats + meta)
	if maxRows > len(m.modelOrder) {
		maxRows = len(m.modelOrder)
	}
	if m.scroll > len(m.modelOrder)-maxRows {
		m.scroll = len(m.modelOrder) - maxRows
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	// Display sorted by req/s (busiest first).
	rows := append([]string(nil), m.modelOrder...)
	sort.SliceStable(rows, func(i, j int) bool {
		return latestRate(m.sampler.ModHist, rows[i]) > latestRate(m.sampler.ModHist, rows[j])
	})
	var out []string
	for i := m.scroll; i < m.scroll+maxRows && i < len(rows); i++ {
		name := rows[i]
		mp := m.sampler.LatestModels[name]
		if mp.At.IsZero() {
			mp.At = time.Now()
		}
		sp := m.sparks[name]

		row := lipgloss.NewStyle().Foreground(lipgloss.Color(modelColorIdx(name, m.modelOrder))).Bold(true).
			Width(13).Render(trimModel(name))
		row += " " + sp.View()

		// Fit the row to the terminal: append segments while they fit so the
		// numbers never wrap mid-row.
		budget := m.width
		if budget <= 0 {
			budget = 120
		}
		appendSeg := func(seg string, plain int) {
			if lipgloss.Width(row)+1+plain > budget {
				return
			}
			row += " " + seg
		}
		appendSeg(labelStyle.Render(fmtRate(mp.ReqRate)+" r/s"), len(fmtRate(mp.ReqRate))+4)
		appendSeg(labelStyle.Render(fmtRate(mp.TokRate)+" t/s"), len(fmtRate(mp.TokRate))+4)
		if p50, _, p95, ok := m.sampler.ModelLatency(name); ok {
			appendSeg(dimStyle.Render("p50 "+fmtLatency(p50)), 4+len(fmtLatency(p50)))
			appendSeg(labelStyle.Render("p95 "+fmtLatency(p95)), 4+len(fmtLatency(p95)))
		} else {
			appendSeg(dimStyle.Render("avg "+fmtLatency(mp.AvgLatencyUs)), 4+len(fmtLatency(mp.AvgLatencyUs)))
		}
		errTxt := fmt.Sprintf("err %d", mp.Errors)
		if mp.ErrRate > 0 {
			appendSeg(errStyle.Render(errTxt+" ↑"), len(errTxt)+2)
		} else {
			appendSeg(dimStyle.Render(errTxt), len(errTxt))
		}
		out = append(out, row)

		meta := m.modelMeta(name)
		if meta != "" {
			out = append(out, "   "+meta)
		}
	}
	if len(rows) > maxRows {
		out = append(out, dimStyle.Render(fmt.Sprintf("   %d of %d models (j/k to scroll)", maxRows, len(rows))))
	}
	return out
}

func (m tuiModel) modelMeta(name string) string {
	ms, ok := m.sampler.RawModels()[name]
	if !ok {
		return ""
	}
	meta := []string{
		fmt.Sprintf("dim %d", ms.Dim),
		ms.Pooling,
		ms.Quantization,
	}
	if ms.BatchingMaxBatch > 0 {
		meta = append(meta, fmt.Sprintf("batch %d/%d workers %d", ms.BatchingMaxBatch, ms.BatchingMaxToks, ms.Workers))
	}
	return metaStyle.Render(strings.Join(meta, " · "))
}

func (m tuiModel) gaugesView() []string {
	p := m.sampler.Latest
	barLine := lipgloss.JoinHorizontal(lipgloss.Top,
		caption("cache", m.cacheBar.View(), p.CacheHitRate, "%"),
		caption("cpu", m.cpuBar.View(), p.CPUPercent, "%"),
		caption("mem", m.memBar.View(), float64(p.MemMB), "MB"),
	)
	texts := fmt.Sprintf("conns %d · active %d · goroutines %d · truncated %d/%d",
		p.Conns, p.Active, p.Goroutines, p.TruncatedTexts, p.TruncatedPairs)
	return []string{barLine, dimStyle.Render("  " + texts)}
}

func caption(title, barView string, val float64, unit string) string {
	head := labelStyle.Render(fmt.Sprintf("%-7s %s%s", title, fmtRate(val), unit))
	// Pad on the right so adjacent gauges read as separate bars.
	return lipgloss.NewStyle().PaddingRight(2).Render(head + "\n" + barView)
}

func (m tuiModel) tickerView() string {
	ev, ok := m.sampler.LastEvent()
	if !ok {
		return ""
	}
	mark := okStyle.Render("✓")
	lat := fmtLatency(ev.LatencyUs)
	if ev.Err {
		mark = errStyle.Render("✗")
	}
	line := "  " + dimStyle.Render("event") + " " +
		labelStyle.Render(trimModelLen(ev.Model, 14)) +
		" · " + fmt.Sprintf("%d texts", ev.Texts) +
		" · " + lat + " " + mark
	if !m.connected {
		line = errStyle.Render("✗ connection lost — retrying") + "  " + line
	}
	return footerStyle.Render(line) + "\n"
}

func (m tuiModel) footerView() string {
	keys := "q quit · p pause · r reset · j/k scroll · ? help"
	return footerStyle.Render(keys) + "\n"
}

func (m tuiModel) helpView() string {
	lines := []string{
		headerStyle.Render("emb-top — live emb node dashboard"),
		"",
		"  q / ctrl+c   quit",
		"  p / space    pause / resume polling",
		"  r            reset visible window",
		"  j / k        scroll per-model panel",
		"  ? / h        toggle this help",
		"",
		"Flags:",
		"  -addr host:port   node address (default localhost:6379)",
		"  -interval 1s      poll interval",
		"  -password P       AUTH password",
		"  -tls              connect over TLS",
		"  -once -samples N  headless mode: print N machine-readable lines",
		"  -window N         history window in polls (default 120)",
		"",
		"Metrics: EMB.MODELS / EMB.INFO / EMB.STATS polling plus MONITOR",
		"events for latency percentiles (p50/p95/p99) and the activity heatmap.",
	}
	return strings.Join(lines, "\n")
}

// ---- formatting helpers ----

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func trimModel(s string) string { return trimModelLen(s, 13) }

func trimModelLen(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func fmtRate(v float64) string {
	if v < 0 {
		v = 0
	}
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case v >= 1000:
		return fmt.Sprintf("%.1fk", v/1000)
	case v >= 100:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.1f", v)
	}
}

func fmtLatency(us int64) string {
	switch {
	case us >= 1_000_000:
		return fmt.Sprintf("%.2fs", float64(us)/1e6)
	case us >= 1000:
		return fmt.Sprintf("%.1fms", float64(us)/1e3)
	default:
		return fmt.Sprintf("%dµs", us)
	}
}

func fmtDuration(secs int64) string {
	d := time.Duration(secs) * time.Second
	h := d / time.Hour
	d -= h * time.Hour
	mnt := d / time.Minute
	d -= mnt * time.Minute
	s := d / time.Second
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, mnt)
	case mnt > 0:
		return fmt.Sprintf("%dm%02ds", mnt, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func modelStyle(i int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(modelColors[i%len(modelColors)]))
}
