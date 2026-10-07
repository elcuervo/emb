package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

// healthStatus is the synthesized dashboard verdict, ordered by severity so
// the worst signal wins by simple comparison.
type healthStatus int

const (
	healthNoData healthStatus = iota
	healthHealthy
	healthDegraded
	healthCritical
)

// Fixed health thresholds. Defaults on purpose: no new flags, one place to
// tune.
const (
	errRatioDegraded = 0.01 // 1% of requests erroring
	errRatioCritical = 0.05 // 5%
	p95RiseDegraded  = 2.0  // × session baseline
	p95RiseCritical  = 4.0
	cpuDegradedPct   = 85.0
	cpuCriticalPct   = 95.0
)

// signal is one health chip: its value text and its own severity. Only
// signals marked driving can raise the overall verdict; informational chips
// are shown but never influence it.
type signal struct {
	text    string
	level   healthStatus
	driving bool
}

// healthInput is everything the verdict is derived from, so health() stays a
// pure function.
type healthInput struct {
	connected  bool
	polls      int
	errRatio   float64 // err/s ÷ req/s; 0 when unknown
	p95Us      int64   // current p95; 0 when unknown
	baselineUs int64   // session baseline p95; 0 when unknown
	cpuPct     float64
	hasCache   bool
	cachePct   float64
}

// health synthesizes the overall verdict and the per-signal chips. The overall
// status is the worst signal level, except that fewer than two polls reads as
// no-data rather than a verdict.
func health(in healthInput) (healthStatus, []signal) {
	conn := signal{text: "connected", level: healthHealthy, driving: true}
	if !in.connected {
		conn = signal{text: "reconnecting", level: healthCritical, driving: true}
	}
	sigs := []signal{conn}
	if in.polls < 2 {
		return healthNoData, sigs
	}

	errSig := signal{text: fmt.Sprintf("err %5.1f%%", in.errRatio*100), level: healthHealthy, driving: true}
	switch {
	case in.errRatio > errRatioCritical:
		errSig.level = healthCritical
	case in.errRatio > errRatioDegraded:
		errSig.level = healthDegraded
	}
	sigs = append(sigs, errSig)

	if in.p95Us > 0 {
		latSig := signal{text: fmt.Sprintf("p95 %-7s", fmtLatency(in.p95Us)), level: healthHealthy, driving: true}
		if in.baselineUs > 0 {
			ratio := float64(in.p95Us) / float64(in.baselineUs)
			switch {
			case ratio > p95RiseCritical:
				latSig.level = healthCritical
			case ratio > p95RiseDegraded:
				latSig.level = healthDegraded
			}
		}
		sigs = append(sigs, latSig)
	}

	cpuSig := signal{text: fmt.Sprintf("cpu %3.0f%%", in.cpuPct), level: healthHealthy, driving: true}
	switch {
	case in.cpuPct > cpuCriticalPct:
		cpuSig.level = healthCritical
	case in.cpuPct > cpuDegradedPct:
		cpuSig.level = healthDegraded
	}
	sigs = append(sigs, cpuSig)

	// Cache hit rate is a workload property, not node health: it stays a
	// visible chip but never drives the verdict.
	if in.hasCache {
		sigs = append(sigs, signal{text: fmt.Sprintf("cache %3.0f%%", in.cachePct), level: healthHealthy})
	}

	worst := healthHealthy
	for _, s := range sigs {
		if s.driving && s.level > worst {
			worst = s.level
		}
	}
	return worst, sigs
}

// healthState gathers the derived inputs the banner and verdict are computed
// from.
func (m tuiModel) healthState() healthInput {
	p := m.sampler.Latest
	in := healthInput{
		connected:  m.connected,
		polls:      m.polls,
		cpuPct:     p.CPUPercent / float64(runtime.NumCPU()),
		baselineUs: m.p95Base,
	}
	if _, _, p95, ok := m.sampler.Latency(); ok {
		in.p95Us = p95
	}
	if p.ReqRate > 0 {
		in.errRatio = p.ErrRate / p.ReqRate
	}
	for _, ms := range m.sampler.RawModels() {
		if ms.CacheMaxBytes > 0 {
			in.hasCache = true
			in.cachePct = p.CacheHitRate
			break
		}
	}
	return in
}

// bannerView renders the one-line health verdict and its signal chips.
func (m tuiModel) bannerView() string {
	st, sigs := health(m.healthState())

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
		text := s.text
		if s.text == "reconnecting" && !m.lastGood.IsZero() {
			text += " · last sample " + fmtDuration(int64(time.Since(m.lastGood).Seconds())) + " ago"
		}
		chips = append(chips, styleFor(s.level).Render(text))
	}
	line := dot + " " + headerStyle.Render(fmt.Sprintf("%-8s", healthLabel(st))) + "  " +
		strings.Join(chips, dimStyle.Render(" │ "))

	max := m.width
	if max <= 0 {
		max = 120
	}
	return lipgloss.NewStyle().MaxWidth(max).Render(line)
}

func styleFor(st healthStatus) lipgloss.Style {
	switch st {
	case healthCritical:
		return errStyle
	case healthDegraded:
		return warnStyle
	default:
		return okStyle
	}
}

func healthLabel(st healthStatus) string {
	switch st {
	case healthNoData:
		return "NO DATA"
	case healthDegraded:
		return "DEGRADED"
	case healthCritical:
		return "CRITICAL"
	default:
		return "HEALTHY"
	}
}

// modelHealthState is the per-model indicator: idle when the model saw no
// traffic anywhere in the window, otherwise a status and rising arrows.
type modelHealthState struct {
	idle    bool
	status  healthStatus
	latRise bool
	errRise bool
}

// modelHealth derives a model's indicator from its recent history. A model
// with no traffic in the window reads as idle, not unhealthy.
func modelHealth(hist []embtop.ModelPoint) modelHealthState {
	st := modelHealthState{status: healthHealthy, idle: true}
	if len(hist) == 0 {
		return st
	}
	for _, p := range hist {
		if p.ReqRate > 0 || p.TokRate > 0 {
			st.idle = false
			break
		}
	}
	if st.idle {
		return st
	}
	last := hist[len(hist)-1]
	if last.ErrRate > 0 {
		st.status = healthDegraded
		st.errRise = true
		if last.ErrRate >= 1 {
			st.status = healthCritical
		}
	}
	if len(hist) > 1 {
		prev := hist[len(hist)-2]
		st.latRise = last.AvgLatencyUs > prev.AvgLatencyUs && prev.AvgLatencyUs > 0
	}
	return st
}

// healthDot renders the per-model status indicator.
func healthDot(mh modelHealthState) string {
	switch {
	case mh.idle:
		return dimStyle.Render("○")
	case mh.status >= healthCritical:
		return errStyle.Render("●")
	case mh.status >= healthDegraded:
		return warnStyle.Render("●")
	default:
		return okStyle.Render("●")
	}
}
