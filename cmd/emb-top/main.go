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
//	emb-top -frames [-interval 1s]             # headless, streamed frames
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
	latBandStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))  // magenta band
	latP95Style  = lipgloss.NewStyle().Foreground(lipgloss.Color("13")) // bright p95 line
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
	frames := flag.Bool("frames", false, "headless mode: stream the dashboard's rendered frames as JSON lines")
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

	if *frames {
		if err := runFrames(context.Background(), client, *interval, *window, os.Stdout); err != nil {
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

// dashboardClient is the slice of the emb-top client the dashboard uses: the
// address its header names and a polled snapshot. It is an interface so the
// headless frame mode can be driven by a fake in tests.
type dashboardClient interface {
	Addr() string
	EnsureConn() (bool, error)
	Poll(known []string, afterSeq uint64) (*embtop.PollResult, error)
	Close() error
}

// frameWidth and frameHeight are the fixed grid the frame mode renders. The
// live view is a fixed block of monospace text, not a reflowing terminal, so
// the size is a constant rather than a terminal query.
const (
	frameWidth  = 120
	frameHeight = 40
)

// frameMessage is one newline-delimited frame: the dashboard's complete
// rendered output, cursor-free and self-contained, so a consumer needs no
// terminal emulation and can display only the newest frame it received.
type frameMessage struct {
	ANSI string `json:"ansi"`
}

// runFrames streams the dashboard's complete frames as JSON lines until the
// process is stopped. It is the headless sibling of the TUI: same model, same
// renderer, no terminal and no input. It keeps polling across a lost
// connection instead of exiting, so a live view recovers on its own.
func runFrames(ctx context.Context, c dashboardClient, interval time.Duration, window int, out io.Writer) error {
	// There is no terminal to ask, and the profile must not be inferred from
	// the pipe, or the frames would arrive without their colour.
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	m := newTUI(c, interval, window)
	m.width, m.height = frameWidth, frameHeight
	m.layoutCharts()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if dialed, err := c.EnsureConn(); err != nil {
			m.connected = false
		} else {
			if dialed {
				// A fresh (or restarted) node numbers events from the start.
				m.lastSeq = 0
			}
			res, err := c.Poll(m.known, m.lastSeq)
			if err != nil {
				m.connected = false
				_ = c.Close()
			} else {
				m.connected = true
				m.lastGood = time.Now()
				m.applyResult(res)
			}
		}
		if err := emitFrame(out, m.View()); err != nil {
			return err
		}
	}
}

// emitFrame writes one JSON-encoded frame and its newline in a single write,
// so a reader never sees a partial line.
func emitFrame(out io.Writer, frame string) error {
	b, err := json.Marshal(frameMessage{ANSI: frame})
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = out.Write(b)
	return err
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
	client   dashboardClient
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

	known      []string        // present models polled via EMB.INFO, list order
	modelOrder []string        // ever-seen models, first-seen order (rows never move)
	present    map[string]bool // models announced by the latest EMB.MODELS
	lastSeq    uint64          // last MONITOR event seq seen

	reqChart streamlinechart.Model

	window  int
	polls   int
	latHist []latSample
	p95Base int64 // slow EMA of observed p95, for relative latency health

	cacheBar barchart.Model
	cpuBar   barchart.Model
	memBar   barchart.Model
}

func newTUI(client dashboardClient, interval time.Duration, window int) tuiModel {
	m := tuiModel{
		client:   client,
		interval: interval,
		sampler:  embtop.NewSampler(window),
		window:   window,
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
	m.cacheBar = bar(100)
	m.cpuBar = bar(100)
	m.memBar = bar(0) // 0: autoscale
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

// yFmtRate is the Y-axis label formatter for the req/s stream chart.
func yFmtRate(v float64) string { return fmtRate(v) }

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

// minStrip is the narrowest an activity strip may render; each row reserves
// that much before laying out its numeric segments, so the strip always shows.
const minStrip = 8

// modelChrome is the number of non-model rows the dashboard always spends:
// header, banner, the two stream charts, gauges, ticker, footer and the
// model-list range line.
const modelChrome = 17

// visibleRows is how many single-line model rows fit below the chrome.
func (m tuiModel) visibleRows() int {
	h := m.height - modelChrome
	if h < 1 {
		h = 1
	}
	return h
}

// ---- bubbletea plumbing ----

func (m tuiModel) Init() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
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
			m.clampScroll()
			return m, nil
		case "k", "up":
			m.scroll--
			m.clampScroll()
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

	// Reconcile the model list. modelOrder is append-only and first-seen: a
	// model keeps its row slot for the life of the process, so a model that
	// leaves and returns reappears where it was and traffic can never move a
	// row. The present set drives which rows render and which are polled.
	announced := make(map[string]bool, len(res.Models))
	for _, mod := range res.Models {
		announced[mod.Name] = true
	}
	for _, mod := range res.Models {
		if !slices.Contains(m.modelOrder, mod.Name) {
			m.modelOrder = append(m.modelOrder, mod.Name)
		}
	}
	m.present = announced
	m.known = m.known[:0]
	for _, name := range m.modelOrder {
		if announced[name] {
			m.known = append(m.known, name)
		}
	}
	m.clampScroll()

	// Stream chart: aggregate req/s.
	pushStream(&m.reqChart, p.ReqRate)

	// Latency band: one percentile sample per poll; a zero sample keeps the
	// columns aligned with polls when no events have arrived yet.
	if p50, p95, p99, ok := m.sampler.Latency(); ok {
		m.pushLat(latSample{p50: p50, p95: p95, p99: p99})
		m.observeP95(p95)
	} else {
		m.pushLat(latSample{})
	}
	m.polls++

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
	m.latHist = nil
	m.p95Base = 0
	m.polls = 0
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
	idW, numW, stripW := newRowLayout(m.width).gaugeBands()
	m.cacheBar.Resize(idW, 2)
	m.cpuBar.Resize(numW, 2)
	m.memBar.Resize(stripW, 2)
	m.reqChart.Draw()
	m.cacheBar.Draw()
	m.cpuBar.Draw()
	m.memBar.Draw()
}

// ---- rendering ----

func (m tuiModel) View() string {
	if m.showHelp {
		return m.helpView()
	}
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n")
	b.WriteString(m.bannerView())
	b.WriteString("\n")

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		borderStyle.Render(m.reqChart.View()),
		borderStyle.Render(m.latBandView()),
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
		uptime = fmt.Sprintf("uptime %-6s", fmtDuration(p.UptimeSecs))
	}
	// Health (connection, rates, latency) lives in the banner immediately
	// below; the header is identity only, so it does not repeat it. The uptime
	// and model count are padded so a growing value cannot shift the line.
	line := headerStyle.Render("emb-top v"+version) +
		" · " + labelStyle.Render(m.client.Addr()) +
		" · " + uptime +
		" · " + fmt.Sprintf("%2d models", p.RegisteredModels) +
		" · poll " + m.interval.String()
	if m.paused {
		line += " " + warnStyle.Render("(paused)")
	}
	return lipgloss.NewStyle().Width(m.width).Render(line)
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

// stripView renders one model's req/s history as colored cells: columns are
// recent polls, intensity is req/s scaled to the busiest model in view, so
// strips stay comparable across rows. Built by hand because lipgloss v1 does
// not style whitespace-only cells, so each cell is a colored non-space rune.
func stripView(pts []embtop.ModelPoint, w int, maxV float64) string {
	if w < 1 {
		w = 1
	}
	if len(pts) == 0 || maxV <= 0 {
		return dimStyle.Render(strings.Repeat(heatCellBlock, w))
	}
	var b strings.Builder
	for x := 0; x < w; x++ {
		idx := x * len(pts) / w
		if idx >= len(pts) {
			idx = len(pts) - 1
		}
		b.WriteString(heatCellStyle(pts[idx].ReqRate / maxV).Render(heatCellBlock))
	}
	return b.String()
}

// maxReqRate is the busiest single req/s sample across the given models: the
// one shared scale for every strip in the list.
func maxReqRate(rows []string, hist map[string][]embtop.ModelPoint) float64 {
	maxV := 0.0
	for _, name := range rows {
		for _, p := range hist[name] {
			if p.ReqRate > maxV {
				maxV = p.ReqRate
			}
		}
	}
	return maxV
}

func modelColorIdx(name string, order []string) string {
	for i, n := range order {
		if n == name {
			return modelColors[i%len(modelColors)]
		}
	}
	return modelColors[0]
}

// Fixed column widths (plain-text) for per-model metric segments, so a value
// changing width cannot shift the columns after it.
const (
	colName = 13
	colRate = 12
	colLat  = 12
	colErr  = 10
	colMeta = 16
)

// rowLayout is the dashboard's single column grid: the identity block, the
// metric slots that fit, and the activity strip. It is a pure function of the
// terminal width, so every model row and the gauge row below them share it and
// a value changing between polls can never move a column.
type rowLayout struct {
	idW, numW, stripW         int
	rate, tok, lat, lat2, err bool
}

func newRowLayout(width int) rowLayout {
	if width <= 0 {
		width = 120
	}
	l := rowLayout{idW: 2 + colName + 1 + colMeta}
	row := l.idW
	budget := width - minStrip - 2
	add := func(w int) bool {
		if row+1+w > budget {
			return false
		}
		row += 1 + w
		return true
	}
	l.rate = add(colRate)
	l.tok = add(colRate)
	l.lat = add(colLat)
	l.lat2 = add(colLat)
	l.err = add(colErr)
	l.numW = row - l.idW
	l.stripW = width - row - 2
	if l.stripW < minStrip {
		l.stripW = minStrip
	}
	return l
}

// gaugeBands is the grid the gauges sit on: the identity block, the metric
// block (without its leading separator) and the strip.
func (l rowLayout) gaugeBands() (int, int, int) {
	num := l.numW - 1
	if num < 1 {
		num = 1
	}
	return l.idW, num, l.stripW
}

// fixedCol renders plain text right-aligned in a constant-width column and
// returns the styled segment plus the column width it occupies.
func fixedCol(style lipgloss.Style, plain string, width int) (string, int) {
	if n := width - lipgloss.Width(plain); n > 0 {
		plain = strings.Repeat(" ", n) + plain
	} else if n < 0 {
		plain = string([]rune(plain)[:width])
	}
	return style.Render(plain), width
}

// presentNames returns the currently announced models in stable first-seen
// order. A model that dropped out keeps its slot in modelOrder but
// contributes no row, so the rows around it never move and it reappears in
// place when announced again.
func (m tuiModel) presentNames() []string {
	rows := make([]string, 0, len(m.modelOrder))
	for _, name := range m.modelOrder {
		if m.present[name] {
			rows = append(rows, name)
		}
	}
	return rows
}

// clampScroll keeps the single scroll offset within the present rows.
func (m *tuiModel) clampScroll() {
	maxScroll := len(m.presentNames()) - m.visibleRows()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m tuiModel) modelsView() []string {
	rows := m.presentNames()
	if len(rows) == 0 {
		return []string{dimStyle.Render("  no models loaded on node")}
	}
	vis := m.visibleRows()
	_, _, hist := m.sampler.Snapshot()
	maxV := maxReqRate(rows, hist)
	l := newRowLayout(m.width)

	out := make([]string, 0, vis+1)
	for i := m.scroll; i < m.scroll+vis && i < len(rows); i++ {
		out = append(out, m.modelRow(rows[i], hist[rows[i]], maxV, l))
	}
	out = append(out, m.listLegend(len(rows)))
	return out
}

// modelRow renders one single-line model row: status dot, name, identity
// metadata, current metrics and the activity strip. The strip always renders
// (the numeric segments yield first), and the row's height never varies, so a
// missing EMB.INFO reply cannot shift the rows below it.
func (m tuiModel) modelRow(name string, pts []embtop.ModelPoint, maxV float64, l rowLayout) string {
	mp := m.sampler.LatestModels[name]
	mh := modelHealth(pts)

	dot := healthDot(mh)
	if mp.Stale {
		dot = dimStyle.Render("◌")
	}
	row := dot + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(modelColorIdx(name, m.modelOrder))).Bold(true).
			Width(colName).Render(trimModel(name))

	meta := compactMeta(m.sampler.RawModels()[name])
	if len(meta) > colMeta {
		meta = string([]rune(meta)[:colMeta])
	}
	row += " " + metaStyle.Width(colMeta).Render(meta)

	// Segments sit on the shared layout, which reserved its slots from the
	// width alone: a missing latency reply blanks a slot instead of moving the
	// strip, so no column ever shifts between rows or polls.
	add := func(seg string) { row += " " + seg }
	if l.rate {
		seg, _ := fixedCol(labelStyle, fmtRate(mp.ReqRate)+" r/s", colRate)
		add(seg)
	}
	if l.tok {
		seg, _ := fixedCol(labelStyle, fmtRate(mp.TokRate)+" t/s", colRate)
		add(seg)
	}
	if l.lat {
		latArrow := " "
		if mh.latRise {
			latArrow = "↑"
		}
		if p50, _, p95, ok := m.sampler.ModelLatency(name); ok {
			seg, _ := fixedCol(dimStyle, "p50 "+fmtLatency(p50), colLat)
			add(seg)
			if l.lat2 {
				seg2, _ := fixedCol(labelStyle, "p95 "+fmtLatency(p95)+latArrow, colLat)
				add(seg2)
			}
		} else {
			seg, _ := fixedCol(dimStyle, "avg "+fmtLatency(mp.AvgLatencyUs)+latArrow, colLat)
			add(seg)
			if l.lat2 {
				add(strings.Repeat(" ", colLat))
			}
		}
	}
	if l.err {
		errArrow := " "
		if mh.errRise {
			errArrow = "↑"
		}
		errText := fmt.Sprintf("err %d%s", mp.Errors, errArrow)
		style := dimStyle
		if mh.errRise || mh.status >= healthDegraded {
			style = errStyle
		}
		seg, _ := fixedCol(style, errText, colErr)
		add(seg)
	}

	return row + "  " + stripView(pts, l.stripW, maxV)
}

// listLegend is the list's single trailing line: the visible row range and the
// strip's scale and time direction.
func (m tuiModel) listLegend(n int) string {
	vis := m.visibleRows()
	first, last := m.scroll+1, m.scroll+vis
	if last > n {
		last = n
	}
	if last < first {
		last = first
	}
	ramp := ""
	for _, c := range heatColors {
		ramp += lipgloss.NewStyle().Background(c).Foreground(c).Render(heatCellBlock)
	}
	legend := "  " + dimStyle.Render(fmt.Sprintf("rows %d–%d of %d", first, last, n)) +
		dimStyle.Render(" · req/s per model · older ") + ramp + dimStyle.Render(" newer")
	if m.width > 0 {
		legend = lipgloss.NewStyle().MaxWidth(m.width).Render(legend)
	}
	return legend
}

// compactMeta is a model's identity in as few columns as possible (dimension,
// pooling, quantization) so it fits the row's fixed metadata column without a
// second line.
func compactMeta(ms *embtop.ModelStats) string {
	if ms == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("%dd", ms.Dim)}
	if ms.Pooling != "" {
		parts = append(parts, ms.Pooling)
	}
	if ms.Quantization != "" && ms.Quantization != "none" {
		parts = append(parts, ms.Quantization)
	}
	return strings.Join(parts, "·")
}

func (m tuiModel) gaugesView() []string {
	p := m.sampler.Latest
	// The bars were sized to the row grid, and the separators below are the
	// row's own (one column after identity, two before the strip), so each
	// gauge spans exactly the columns of the zone above it.
	idW, numW, stripW := newRowLayout(m.width).gaugeBands()
	barLine := lipgloss.JoinHorizontal(lipgloss.Top,
		caption("cache", m.cacheBar.View(), p.CacheHitRate, "%", idW),
		" ",
		caption("cpu", m.cpuBar.View(), p.CPUPercent, "%", numW),
		"  ",
		caption("mem", m.memBar.View(), float64(p.MemMB), "MB", stripW),
	)
	texts := fmt.Sprintf("conns %2d · active %2d · goroutines %3d · truncated texts/pairs/images %3d/%3d/%3d",
		p.Conns, p.Active, p.Goroutines, p.TruncatedTexts, p.TruncatedPairs, p.TruncatedImages)
	return []string{barLine, dimStyle.Render("  " + texts)}
}

func caption(title, barView string, val float64, unit string, width int) string {
	// The value sits in a fixed-width column so a growing rate moves no glyph;
	// the column shrinks with a narrow band instead of overflowing it.
	valW := width - len(title) - 1
	if valW > 7 {
		valW = 7
	}
	if valW < 1 {
		valW = 1
	}
	head := title + " " + fmt.Sprintf("%*s", valW, fmtRate(val)+unit)
	if pad := width - len(head); pad > 0 {
		head += strings.Repeat(" ", pad)
	} else if pad < 0 {
		head = head[:width]
	}
	return labelStyle.Render(head) + "\n" + barView
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
		" · " + fmt.Sprintf("%3d texts", ev.Texts) +
		" · " + fmt.Sprintf("%-7s", lat) + " " + mark
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
		"  j / k        scroll the model list",
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
		"events for latency percentiles (p50/p95/p99). One model per row, in a",
		"stable first-seen order, with its activity strip beside its live numbers.",
		"The health line synthesizes error ratio, p95 vs this session's baseline,",
		"CPU, cache hit rate and connection state; the latency panel is a p50–p99",
		"band with a p95 line.",
	}
	return strings.Join(lines, "\n")
}

// ---- formatting helpers ----

func trimModel(s string) string { return trimModelLen(s, colName) }

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
