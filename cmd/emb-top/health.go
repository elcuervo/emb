package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/elcuervo/emb/internal/embtop"
)

// nodeCPUPct reads a sample as CPU share of the node's whole capacity. The
// sampler reports percent of one core, so every surface that shows it to a
// reader divides by the node's own parallelism; the raw figure is for the
// machine-readable paths.
func nodeCPUPct(p embtop.Point) float64 {
	return p.CPUPercent / float64(embtop.NodeParallelism(p.GoMaxProcs))
}

// healthState gathers the derived inputs the banner and verdict are computed
// from.
func (m tuiModel) healthState() embtop.HealthInput {
	p := m.sampler.Latest
	in := embtop.HealthInput{
		Connected:  m.connected,
		Polls:      m.polls,
		CPUPct:     nodeCPUPct(p),
		BaselineUs: m.p95Base,
	}
	if _, _, p95, ok := m.sampler.Latency(); ok {
		in.P95Us = p95
	}
	if p.ReqRate > 0 {
		in.ErrRatio = p.ErrRate / p.ReqRate
	}
	for _, ms := range m.sampler.RawModels() {
		if ms.CacheMaxBytes > 0 {
			in.HasCache = true
			in.CachePct = p.CacheHitRate
			break
		}
	}
	return in
}

// bannerView renders the one-line health verdict and its signal chips.
func (m tuiModel) bannerView() string {
	st, sigs := embtop.Health(m.healthState())

	dot := okStyle.Render("●")
	switch st {
	case embtop.HealthNoData:
		dot = warnStyle.Render("○")
	case embtop.HealthDegraded:
		dot = warnStyle.Render("●")
	case embtop.HealthCritical:
		dot = errStyle.Render("●")
	}

	chips := make([]string, 0, len(sigs))
	for _, s := range sigs {
		text := s.Text
		if s.Text == "reconnecting" && !m.lastGood.IsZero() {
			text += " · last sample " + fmtDuration(int64(time.Since(m.lastGood).Seconds())) + " ago"
		}
		chips = append(chips, styleFor(s.Level).Render(text))
	}
	line := dot + " " + headerStyle.Render(fmt.Sprintf("%-8s", st.String())) + "  " +
		strings.Join(chips, dimStyle.Render(" │ "))

	max := m.width
	if max <= 0 {
		max = 120
	}
	return lipgloss.NewStyle().MaxWidth(max).Render(line)
}

func styleFor(st embtop.HealthStatus) lipgloss.Style {
	switch st {
	case embtop.HealthCritical:
		return errStyle
	case embtop.HealthDegraded:
		return warnStyle
	default:
		return okStyle
	}
}

// recommendView renders the advisory sizing line: the resource to change and
// the counters it is derived from. It is always present, so the fixed chrome
// budget stays exact; a quiet node renders it blank. It carries no severity
// and never touches the banner above it.
// advice gathers the node's evidence the way the headless output does, so the
// panel and the tokens describe the same poll.
func (m tuiModel) advice() embtop.Advice {
	return embtop.Advise(embtop.SamplerAdviceInput(m.sampler))
}

// recommendView renders the advisory panel: the action and the counters that
// decided it on the first line, the action the same evidence rejects and the
// window's own facts — the cache's share of the traffic, the CPU each uncached
// run cost — on the second. Both lines are always rendered so the chrome budget
// stays exact; a node with nothing to advise leaves them blank.
func (m tuiModel) recommendView() string {
	adv := m.advice()
	width := widthOr(m.width, 120)
	head, tail := "", ""
	if text := embtop.AdviceHead(adv); text != "" {
		head = labelStyle.Render("SIZING") + "  " + text
	}
	tail = embtop.AdviceRejected(adv)
	if adv.CachedPct > 0 {
		tail = appendClause(tail, fmt.Sprintf("served %.0f%% from cache", adv.CachedPct))
	}
	if adv.CPUPerRun > 0 && adv.Phases.Runs > 0 {
		tail = appendClause(tail, fmt.Sprintf("%.1fms cpu/run", float64(adv.CPUPerRun.Microseconds())/1000))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(head) + "\n" +
		lipgloss.NewStyle().MaxWidth(width).Render(dimStyle.Render(tail))
}

// appendClause adds one dim clause to the sizing panel's second line.
func appendClause(line, clause string) string {
	if line == "" {
		return clause
	}
	return line + " · " + clause
}

// modelHealthState is the per-model indicator: idle when the model saw no
// traffic anywhere in the window, otherwise a status and rising arrows.
type modelHealthState struct {
	idle    bool
	status  embtop.HealthStatus
	latRise bool
	errRise bool
}

// modelHealth derives a model's indicator from its recent history. A model
// with no traffic in the window reads as idle, not unhealthy.
func modelHealth(hist []embtop.ModelPoint) modelHealthState {
	st := modelHealthState{status: embtop.HealthHealthy, idle: true}
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
		st.status = embtop.HealthDegraded
		st.errRise = true
		if last.ErrRate >= 1 {
			st.status = embtop.HealthCritical
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
	case mh.status >= embtop.HealthCritical:
		return errStyle.Render("●")
	case mh.status >= embtop.HealthDegraded:
		return warnStyle.Render("●")
	default:
		return okStyle.Render("●")
	}
}
