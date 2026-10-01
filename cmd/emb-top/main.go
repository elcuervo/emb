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
	"github.com/NimbleMarkets/ntcharts/sparkline"
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

	known      []string // models polled via EMB.INFO
	modelOrder []string // server EMB.MODELS order
	lastSeq    uint64   // last MONITOR event seq seen

	reqChart streamlinechart.Model
	sparks   map[string]*sparkline.Model

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
		sparks:   map[string]*sparkline.Model{},
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

// hotW derives the heatmap grid width from the terminal.
func hotW(width int) int {
	if width <= 0 {
		width = 80
	}
	// Reserve the model label (14) and the row's rate column (2+colHeat).
	if w := width - 38; w > 10 {
		return w
	}
	return 10
}

// heatContentW is the heatmap panel's inner width: label + strip + rate column.
func (m tuiModel) heatContentW() int {
	return 13 + 1 + m.hotW() + 2 + colHeat
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

	// Reconcile known models with a stable first-seen order. The server's
	// EMB.MODELS order is not something to render by: keep rows where they are,
	// append newly discovered models, and drop the ones that vanished, so
	// traffic can never make a row jump.
	announced := make(map[string]bool, len(res.Models))
	for _, mod := range res.Models {
		announced[mod.Name] = true
	}
	kept := m.modelOrder[:0]
	for _, name := range m.modelOrder {
		if announced[name] {
			kept = append(kept, name)
		}
	}
	m.modelOrder = kept
	for _, mod := range res.Models {
		if slices.Contains(m.modelOrder, mod.Name) {
			continue
		}
		m.modelOrder = append(m.modelOrder, mod.Name)
		sp := sparkline.New(m.sparkW(), 1,
			sparkline.WithStyle(modelStyle(len(m.modelOrder)-1)))
		m.sparks[mod.Name] = &sp
	}
	m.known = append(m.known[:0], m.modelOrder...)
	for name := range m.sparks {
		if !slices.Contains(m.modelOrder, name) {
			delete(m.sparks, name)
		}
	}

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
	b.WriteString(m.bannerView())
	b.WriteString("\n")

	if len(m.modelOrder) > 0 {
		b.WriteString(borderStyle.Render(m.heatmapView()))
		b.WriteString("\n")
	}

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
		uptime = fmt.Sprintf("uptime %s", fmtDuration(p.UptimeSecs))
	}
	// Health (connection, rates, latency) lives in the banner immediately
	// below; the header is identity only, so it does not repeat it.
	line := headerStyle.Render("emb-top v"+version) +
		" · " + labelStyle.Render(m.client.Addr()) +
		" · " + uptime +
		" · " + fmt.Sprint(p.RegisteredModels) + " models" +
		" · poll " + m.interval.String()
	if m.paused {
		line += " " + warnStyle.Render("(paused)")
	}
	return lipgloss.NewStyle().Width(m.width).Render(line)
}

// heatmapView renders a hand-rolled model-activity heatmap: rows = models in
// stable first-seen order, columns = recent polls, cell = colored block
// intensity by req/s, with each row's current rate spelled out. Built by hand
// because lipgloss v1 does not style whitespace-only cells (the ntcharts
// heatmap widget colors space cells, which render as unstyled), so each cell is
// a colored non-space rune.
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
		} else {
			for x := 0; x < w; x++ {
				idx := x * len(pts) / w
				if idx >= len(pts) {
					idx = len(pts) - 1
				}
				frac := pts[idx].ReqRate / maxV
				line += heatCellStyle(frac).Render(cell)
			}
		}
		// The current rate is the row's readable value; the cells are the trend.
		value, _ := fixedCol(labelStyle, fmtRate(latestRate(pts))+" r/s", colHeat)
		lines = append(lines, line+"  "+value)
	}
	ramp := ""
	for _, c := range heatColors {
		ramp += lipgloss.NewStyle().Background(c).Foreground(c).Render(heatCellBlock)
	}
	legend := " " + labelStyle.Render("req/s per model") + dimStyle.Render(" · older ") + ramp + dimStyle.Render(" newer")
	if n := len(m.modelOrder) - len(rows); n > 0 {
		legend += dimStyle.Render(fmt.Sprintf("   (+%d more)", n))
	}
	legend = lipgloss.NewStyle().MaxWidth(m.heatContentW()).Render(legend)
	return legend + "\n" + strings.Join(lines, "\n")
}

// latestRate returns the last sampled req/s for a model, or 0.
func latestRate(pts []embtop.ModelPoint) float64 {
	if len(pts) == 0 {
		return 0
	}
	return pts[len(pts)-1].ReqRate
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

// hotRows returns the heatmap's model rows in stable server EMB.MODELS order,
// capped to the visible height. Rows never reorder as traffic fluctuates.
func (m tuiModel) hotRows() []string {
	rows := append([]string(nil), m.modelOrder...)
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

// Fixed column widths (plain-text) for per-model metric segments, so a value
// changing width cannot shift the columns after it.
const (
	colRate = 12
	colLat  = 12
	colErr  = 10
	colHeat = 10
)

// fixedCol renders plain text right-aligned in a constant-width column and
// returns the styled segment plus the column width it occupies.
func fixedCol(style lipgloss.Style, plain string, width int) (string, int) {
	if n := width - len(plain); n > 0 {
		plain = strings.Repeat(" ", n) + plain
	} else if n < 0 {
		plain = string([]rune(plain)[:width])
	}
	return style.Render(plain), width
}

func (m tuiModel) modelsView() []string {
	if len(m.modelOrder) == 0 {
		return []string{dimStyle.Render("  no models loaded on node")}
	}
	// Reserve vertical space for header + heatmap + charts + gauges + ticker.
	maxRows := m.height - 22 // header + banner + heatmap + charts + gauges + ticker + footer
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
	// Stable server EMB.MODELS order: rows never reorder as rates fluctuate.
	rows := m.modelOrder
	var out []string
	for i := m.scroll; i < m.scroll+maxRows && i < len(rows); i++ {
		name := rows[i]
		mp := m.sampler.LatestModels[name]
		if mp.At.IsZero() {
			mp.At = time.Now()
		}
		sp := m.sparks[name]
		mh := modelHealth(m.sampler.ModHist[name])

		row := healthDot(mh) + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(modelColorIdx(name, m.modelOrder))).Bold(true).
				Width(13).Render(trimModel(name))
		row += " " + sp.View()

		// Fit the row to the terminal: append fixed-width segments while they
		// fit so numbers never wrap and columns never shift mid-row.
		budget := m.width
		if budget <= 0 {
			budget = 120
		}
		appendSeg := func(seg string, width int) {
			if lipgloss.Width(row)+1+width > budget {
				return
			}
			row += " " + seg
		}
		rateSeg, rateW := fixedCol(labelStyle, fmtRate(mp.ReqRate)+" r/s", colRate)
		appendSeg(rateSeg, rateW)
		tokSeg, tokW := fixedCol(labelStyle, fmtRate(mp.TokRate)+" t/s", colRate)
		appendSeg(tokSeg, tokW)

		latArrow := " "
		if mh.latRise {
			latArrow = "↑"
		}
		if p50, _, p95, ok := m.sampler.ModelLatency(name); ok {
			p50Seg, p50W := fixedCol(dimStyle, "p50 "+fmtLatency(p50), colLat)
			appendSeg(p50Seg, p50W)
			p95Seg, p95W := fixedCol(labelStyle, "p95 "+fmtLatency(p95)+latArrow, colLat)
			appendSeg(p95Seg, p95W)
		} else {
			avgSeg, avgW := fixedCol(dimStyle, "avg "+fmtLatency(mp.AvgLatencyUs)+latArrow, colLat)
			appendSeg(avgSeg, avgW)
		}

		errArrow := " "
		if mh.errRise {
			errArrow = "↑"
		}
		errText := fmt.Sprintf("err %d%s", mp.Errors, errArrow)
		errSeg, errW := fixedCol(dimStyle, errText, colErr)
		if mh.errRise || mh.status >= healthDegraded {
			errSeg, errW = fixedCol(errStyle, errText, colErr)
		}
		appendSeg(errSeg, errW)
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
	texts := fmt.Sprintf("conns %d · active %d · goroutines %d · truncated texts/pairs/images %d/%d/%d",
		p.Conns, p.Active, p.Goroutines, p.TruncatedTexts, p.TruncatedPairs, p.TruncatedImages)
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
		"",
		"The health line synthesizes error ratio, p95 vs this session's baseline,",
		"CPU, cache hit rate and connection state; the latency panel is a p50–p99",
		"band with a p95 line.",
	}
	return strings.Join(lines, "\n")
}

// ---- formatting helpers ----

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
