package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

type snapshot struct {
	At    time.Time      `json:"at"`
	Fleet fleetSnapshot  `json:"fleet"`
	Nodes []nodeSnapshot `json:"nodes"`
	Error string         `json:"error"`
}
type fleetSnapshot struct {
	Health     string       `json:"health"`
	Reason     string       `json:"reason"`
	ReqRate    *float64     `json:"req_rate"`
	TokRate    *float64     `json:"tok_rate"`
	P95Us      *int64       `json:"p95_us"`
	History    []fleetPoint `json:"history"`
	Discovered int          `json:"discovered"`
	Active     int          `json:"active"`
	Idle       int          `json:"idle"`
	Orphaned   int          `json:"orphaned"`
	Advice     string       `json:"advice"`
}
type fleetPoint struct {
	At      time.Time `json:"at"`
	ReqRate *float64  `json:"req_rate"`
	TokRate *float64  `json:"tok_rate"`
	P95Us   *int64    `json:"p95_us"`
}
type nodeSnapshot struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Address  string          `json:"address"`
	State    string          `json:"state"`
	Health   string          `json:"health"`
	Reason   string          `json:"reason"`
	Orphaned bool            `json:"orphaned"`
	LastOK   *time.Time      `json:"last_ok"`
	Point    *point          `json:"point"`
	History  []point         `json:"history"`
	Models   []modelSnapshot `json:"models"`
	Events   []eventSnapshot `json:"events"`
	P50Us    *int64          `json:"p50_us"`
	P95Us    *int64          `json:"p95_us"`
	P99Us    *int64          `json:"p99_us"`
	Advice   string          `json:"advice"`
}
type point struct {
	At              time.Time `json:"at"`
	ReqRate         *float64  `json:"req_rate"`
	TokRate         *float64  `json:"tok_rate"`
	ErrRate         *float64  `json:"err_rate"`
	Active          int64     `json:"active"`
	Conns           int64     `json:"conns"`
	MemMB           int64     `json:"mem_mb"`
	CPUPct          *float64  `json:"cpu_pct"`
	CacheHitRate    *float64  `json:"cache_hit_rate"`
	CacheHitPS      *float64  `json:"cache_hit_ps"`
	CacheMissPS     *float64  `json:"cache_miss_ps"`
	CacheEvictPS    *float64  `json:"cache_evict_ps"`
	UptimeSecs      int64     `json:"uptime_secs"`
	CPUBudget       *int64    `json:"cpu_budget"`
	CPUActiveTokens *int64    `json:"cpu_active_tokens"`
	CPUPlanKnown    bool      `json:"cpu_plan_known"`
}
type modelSnapshot struct {
	Name         string       `json:"name"`
	Status       string       `json:"status"`
	Dim          int          `json:"dim"`
	Pooling      string       `json:"pooling"`
	Quantization string       `json:"quantization"`
	Point        *modelPoint  `json:"point"`
	History      []modelPoint `json:"history"`
	P50Us        *int64       `json:"p50_us"`
	P95Us        *int64       `json:"p95_us"`
	P99Us        *int64       `json:"p99_us"`
	Advice       string       `json:"advice"`
}
type modelPoint struct {
	At              time.Time `json:"at"`
	ReqRate         *float64  `json:"req_rate"`
	TokRate         *float64  `json:"tok_rate"`
	ErrRate         *float64  `json:"err_rate"`
	AvgLatencyUs    *int64    `json:"avg_latency_us"`
	CacheHitRate    *float64  `json:"cache_hit_rate"`
	Stale           bool      `json:"stale"`
	Runs            *int64    `json:"runs"`
	DispatchWaitUs  *int64    `json:"dispatch_wait_us"`
	AdmissionWaitUs *int64    `json:"admission_wait_us"`
	RunUs           *int64    `json:"run_us"`
	SessionsBusy    *int64    `json:"sessions_busy"`
	SessionsTotal   *int64    `json:"sessions_total"`
}
type eventSnapshot struct {
	AtUs        int64  `json:"at_us"`
	Model       string `json:"model"`
	LatencyUs   int64  `json:"latency_us"`
	Error       bool   `json:"error"`
	CachedTexts *int   `json:"cached_texts"`
	Attributed  bool   `json:"attributed"`
	DispatchUs  *int64 `json:"dispatch_us"`
	AdmitUs     *int64 `json:"admit_us"`
	RunUs       *int64 `json:"run_us"`
}

func healthName(status embtop.HealthStatus) string {
	return strings.ToLower(strings.ReplaceAll(status.String(), " ", "_"))
}

func pointOf(p embtop.Point, measured bool, hasCache bool) point {
	out := point{At: p.At, Active: p.Active, Conns: p.Conns, MemMB: p.MemMB, UptimeSecs: p.UptimeSecs, CPUPlanKnown: p.CPUPlanKnown}
	if measured {
		out.ReqRate, out.TokRate, out.ErrRate = &p.ReqRate, &p.TokRate, &p.ErrRate
		cpu := p.CPUPercent / float64(embtop.NodeParallelism(p.GoMaxProcs))
		out.CPUPct = &cpu
		if hasCache {
			out.CacheHitPS, out.CacheMissPS, out.CacheEvictPS = &p.CacheHitPS, &p.CacheMissPS, &p.CacheEvictPS
		}
	}
	if hasCache && p.CacheHits+p.CacheMisses > 0 {
		out.CacheHitRate = &p.CacheHitRate
	}
	if p.CPUPlanKnown {
		out.CPUBudget, out.CPUActiveTokens = &p.CPUBudget, &p.CPUActiveTokens
	}
	return out
}
func modelPointOf(p embtop.ModelPoint, measured, hasCache bool) modelPoint {
	out := modelPoint{At: p.At, Stale: p.Stale}
	if measured && !p.Stale {
		out.ReqRate, out.TokRate, out.ErrRate = &p.ReqRate, &p.TokRate, &p.ErrRate
	}
	if p.Requests > 0 && p.AvgLatencyUs > 0 {
		out.AvgLatencyUs = &p.AvgLatencyUs
	}
	if hasCache && p.CachedReqs+p.CacheMisses > 0 {
		out.CacheHitRate = &p.CacheHitRate
	}
	if p.PhasesKnown {
		out.Runs, out.DispatchWaitUs, out.AdmissionWaitUs, out.RunUs = &p.Runs, &p.DispatchWaitUs, &p.AdmissionWaitUs, &p.RunUs
		out.SessionsBusy, out.SessionsTotal = &p.SessionsBusy, &p.SessionsTotal
	}
	return out
}
func eventOf(e embtop.Event) eventSnapshot {
	out := eventSnapshot{AtUs: e.AtUs, Model: e.Model, LatencyUs: e.LatencyUs, Error: e.Err, Attributed: e.Attributed}
	if e.Attributed {
		out.CachedTexts, out.DispatchUs, out.AdmitUs, out.RunUs = &e.CachedTexts, &e.DispatchUs, &e.AdmitUs, &e.RunUs
	}
	return out
}
func percentiles(events []int64) *int64 {
	if len(events) == 0 {
		return nil
	}
	sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
	return &events[(len(events)*95+99)/100-1]
}

func (m *monitor) nodeView(n *node, now time.Time) (nodeSnapshot, embtop.FleetNodeInput) {
	name := n.name
	if name == "" {
		name = n.address
	}
	v := nodeSnapshot{ID: n.id, Name: name, Address: n.address, State: "starting", Health: "no_data", Orphaned: !n.resolved,
		History: make([]point, 0), Models: make([]modelSnapshot, 0), Events: make([]eventSnapshot, 0)}
	if !n.lastOK.IsZero() {
		v.LastOK = &n.lastOK
	}
	n.sampler.TrimEventsSince(now.Add(-historyPoints * pollEvery))
	p, hist, models := n.sampler.Snapshot()
	current := n.reachable && !n.lastOK.IsZero() && now.Sub(n.lastOK) <= 2*pollEvery
	switch {
	case n.failure == "authentication rejected":
		v.State = "auth_failed"
		v.Reason = n.failure
	case !n.lastOK.IsZero() && !current && now.Sub(n.lastOK) <= 10*pollEvery:
		v.State = "stale"
		v.Reason = n.failure
	case !n.unreachableSince.IsZero() || !n.lastOK.IsZero() && !current:
		v.State = "disconnected"
		v.Reason = n.failure
	case current && len(hist) >= 2 && p.ReqRate <= 0 && p.TokRate <= 0:
		v.State = "idle"
	case current:
		v.State = "connected"
	}
	if v.State == "stale" && v.Reason == "" {
		v.Reason = "Sample overdue"
	}
	if v.State == "disconnected" && v.Reason == "" {
		v.Reason = "No recent sample"
	}
	if v.State == "starting" {
		v.Reason = "Waiting for first sample"
	}
	if v.State == "idle" {
		v.Reason = "Connected; no recent traffic"
	}
	if v.State == "connected" {
		v.Reason = "Connected"
	}
	if v.Orphaned && current {
		v.Reason = "Answering but absent from DNS"
	}
	if n.failure == "TLS verification failed" {
		v.Reason = n.failure
	}
	rawModels := n.sampler.RawModels()
	hasCache := p.CacheHits+p.CacheMisses > 0
	for _, ms := range rawModels {
		if ms.CacheMaxBytes > 0 {
			hasCache = true
			break
		}
	}
	for i, past := range hist {
		v.History = append(v.History, pointOf(past, i > 0 || len(hist) == historyPoints, hasCache))
	}
	if current && len(v.History) > 0 {
		v.Point = &v.History[len(v.History)-1]
	}
	if p50, p95, p99, ok := n.sampler.Latency(); ok {
		v.P50Us, v.P95Us, v.P99Us = &p50, &p95, &p99
	}
	input := embtop.HealthInput{Connected: current, Polls: n.samples, CPUPct: p.CPUPercent / float64(embtop.NodeParallelism(p.GoMaxProcs)), HasCache: hasCache, CachePct: p.CacheHitRate, BaselineUs: n.p95Base}
	if p.ReqRate > 0 {
		input.ErrRatio = p.ErrRate / p.ReqRate
	}
	if v.P95Us != nil {
		input.P95Us = *v.P95Us
	}
	status, signals := embtop.Health(input)
	if v.State == "auth_failed" || v.State == "disconnected" {
		status = embtop.HealthCritical
	}
	if v.State == "stale" || v.Orphaned && current {
		if status < embtop.HealthDegraded {
			status = embtop.HealthDegraded
		}
	}
	v.Health = healthName(status)
	if current && status >= embtop.HealthDegraded {
		for _, sig := range signals {
			if sig.Driving && sig.Level == status {
				v.Reason = sig.Text
				break
			}
		}
	}
	for _, modelName := range n.modelOrder {
		model, loaded := n.models[modelName]
		if !loaded {
			continue
		}
		ms := rawModels[model.Name]
		mv := modelSnapshot{Name: model.Name, Status: model.Status, Dim: model.Dim, History: make([]modelPoint, 0)}
		if ms != nil {
			mv.Pooling, mv.Quantization = ms.Pooling, ms.Quantization
		}
		for i, past := range models[model.Name] {
			mv.History = append(mv.History, modelPointOf(past, i > 0 || len(models[model.Name]) == historyPoints, ms != nil && ms.CacheMaxBytes > 0))
		}
		if current && len(mv.History) > 0 {
			mv.Point = &mv.History[len(mv.History)-1]
		}
		if p50, p95, p99, ok := n.sampler.ModelLatency(model.Name); ok {
			mv.P50Us, mv.P95Us, mv.P99Us = &p50, &p95, &p99
		}
		v.Models = append(v.Models, mv)
	}
	events := n.sampler.Events
	if len(events) > 128 {
		events = events[len(events)-128:]
	}
	for _, e := range events {
		v.Events = append(v.Events, eventOf(e))
	}
	if current {
		v.Advice = embtop.AdviceText(embtop.Advise(embtop.SamplerAdviceInput(n.sampler)))
		if v.Advice == "" {
			v.Advice = embtop.RecommendNode(hist).Text
		}
	}
	return v, embtop.FleetNodeInput{Label: name, Health: input, ReqRate: p.ReqRate, Idle: v.State == "idle", Orphaned: !n.resolved, Unreachable: !n.reachable && !n.unreachableSince.IsZero(), UptimeSecs: p.UptimeSecs, HasCache: hasCache, CachePct: p.CacheHitRate}
}

func (m *monitor) buildSnapshot() snapshot {
	now := m.now()
	s := snapshot{At: now, Nodes: make([]nodeSnapshot, 0, len(m.nodes)), Error: m.resolveError}
	f := fleetSnapshot{History: append([]fleetPoint(nil), m.fleetHistory...)}
	var total embtop.FleetTotals
	measuredRates := false
	inputs := make([]embtop.FleetNodeInput, 0, len(m.nodes))
	recs := make([]embtop.FleetNodeRecInput, 0, len(m.nodes))
	var latencies []int64
	for _, n := range m.nodes {
		v, in := m.nodeView(n, now)
		s.Nodes = append(s.Nodes, v)
		inputs = append(inputs, in)
		if n.resolved {
			f.Discovered++
		} else {
			f.Orphaned++
		}
		if v.State == "idle" {
			f.Idle++
		}
		if v.State == "connected" {
			f.Active++
		}
		reachable := v.State == "connected" || v.State == "idle"
		total.Add(n.sampler.Latest, reachable)
		if reachable && n.samples >= 2 {
			measuredRates = true
		}
		if reachable {
			recs = append(recs, embtop.SamplerFleetRecInput(v.Name, n.sampler))
			for _, e := range n.sampler.Events {
				if !e.Err && e.LatencyUs > 0 {
					latencies = append(latencies, e.LatencyUs)
				}
			}
		}
	}
	if measuredRates {
		f.ReqRate, f.TokRate = &total.ReqRate, &total.TokRate
	}
	f.P95Us = percentiles(latencies)
	status, signals := embtop.FleetHealth(inputs)
	f.Health = healthName(status)
	switch {
	case len(signals) > 0:
		f.Reason = signals[0].Text
	case status == embtop.HealthNoData:
		f.Reason = "Waiting for node samples"
	default:
		f.Reason = "All nodes responding"
	}
	f.Advice = embtop.RecommendFleet(recs).Text
	s.Fleet = f
	return s
}

func (m *monitor) publish() { m.publishSnapshot(m.buildSnapshot()) }

func (m *monitor) record() {
	s := m.buildSnapshot()
	m.fleetHistory = append(m.fleetHistory, fleetPoint{At: s.At, ReqRate: s.Fleet.ReqRate, TokRate: s.Fleet.TokRate, P95Us: s.Fleet.P95Us})
	if len(m.fleetHistory) > historyPoints {
		m.fleetHistory = m.fleetHistory[len(m.fleetHistory)-historyPoints:]
	}
	s.Fleet.History = append([]fleetPoint(nil), m.fleetHistory...)
	m.publishSnapshot(s)
}

const maxFrameBytes = 8 << 20

func (m *monitor) publishSnapshot(s snapshot) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	for len(data) > maxFrameBytes {
		shortened := false
		shrink := func(n int) int {
			if n < 2 {
				return n
			}
			return n / 2
		}
		if n := len(s.Fleet.History); n > 1 {
			s.Fleet.History = s.Fleet.History[shrink(n):]
			shortened = true
		}
		for i := range s.Nodes {
			node := &s.Nodes[i]
			if n := len(node.History); n > 1 {
				node.History = node.History[shrink(n):]
				shortened = true
			}
			if n := len(node.Events); n > 1 {
				node.Events = node.Events[shrink(n):]
				shortened = true
			}
			for j := range node.Models {
				model := &node.Models[j]
				if n := len(model.History); n > 1 {
					model.History = model.History[shrink(n):]
					shortened = true
				}
			}
		}
		if !shortened {
			s = snapshot{At: s.At, Fleet: fleetSnapshot{Health: "critical", Reason: "Snapshot exceeds frame limit", History: []fleetPoint{}}, Nodes: []nodeSnapshot{}, Error: "Snapshot exceeds frame limit"}
		}
		data, err = json.Marshal(s)
		if err != nil {
			return
		}
	}
	m.mu.Lock()
	m.latest = data
	for ch := range m.viewers {
		select {
		case ch <- data:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- data:
			default:
			}
		}
	}
	m.mu.Unlock()
}
