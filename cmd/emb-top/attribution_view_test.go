package main

import (
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

// causedTUI builds a node whose window shows the gate holding the uncached
// tail, with a mixed event window behind it: cache answers in microseconds and
// runs that waited at the gate.
func causedTUI(t *testing.T) tuiModel {
	t.Helper()
	m := newSizedTUI(120, 30)
	m.sampler.Window = []embtop.Point{
		{At: time.Unix(0, 0), GoMaxProcs: 2, CPUBudget: 2, CPUPlanKnown: true, ReqRate: 50, TotalRequests: 0, CPUPercent: 80},
		{At: time.Unix(1, 0), GoMaxProcs: 2, CPUBudget: 2, CPUPlanKnown: true, ReqRate: 50, TotalRequests: 50, CPUPercent: 80},
		{At: time.Unix(2, 0), GoMaxProcs: 2, CPUBudget: 2, CPUPlanKnown: true, ReqRate: 50, TotalRequests: 100, CPUPercent: 80,
			CPUActiveTokens: 2, CacheHits: 50},
	}
	m.sampler.Latest = m.sampler.Window[len(m.sampler.Window)-1]
	m.sampler.ModHist = map[string][]embtop.ModelPoint{
		"m": {
			{At: time.Unix(0, 0), Requests: 0, Runs: 1},
			{At: time.Unix(2, 0), Requests: 100, Runs: 9, CachedReqs: 20,
				AdmissionWaitUs: 1_800_000, RunUs: 200_000, SessionsBusy: 1, SessionsTotal: 1},
		},
	}
	m.sampler.LatestModels = map[string]embtop.ModelPoint{"m": m.sampler.ModHist["m"][1]}
	m.sampler.LatestRaw = map[string]*embtop.ModelStats{"m": {Dim: 384, EmbedRuns: 9, EmbedSessionsTotal: 1}}
	// Six cache answers among twelve events: enough of a window for the tail to
	// mean something.
	events := make([]embtop.Event, 0, 12)
	for i := range 6 {
		events = append(events, embtop.Event{
			Seq: uint64(i + 1), Model: "m", Texts: 1, CachedTexts: 1, Attributed: true, LatencyUs: int64(2 + i),
		})
	}
	for i := range 6 {
		events = append(events, embtop.Event{
			Seq: uint64(i + 7), Model: "m", Texts: 1, Attributed: true,
			LatencyUs:  int64(700_000 + i*200_000),
			DispatchUs: 100, AdmitUs: int64(600_000 + i*180_000), RunUs: 90_000,
		})
	}
	m.sampler.PushEvents(events)
	m.polls = 3
	return m
}

// The dashboard names the course of action and the paths behind it, so an
// operator can tell a cache-served window from an idle one and the tail's cause
// from its size.
func TestViewRendersTheCourseOfAction(t *testing.T) {
	m := causedTUI(t)
	view := m.View()

	for _, want := range []string{"SIZING", "intra_op_threads", "served", "cpu/run"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	// The action names what it rejected and why.
	if !strings.Contains(view, "not more cpu") {
		t.Errorf("view does not state the rejected action:\n%s", view)
	}
	// The band separates the service tail from the runs' tail.
	if !strings.Contains(view, "runs p95") || !strings.Contains(view, "n=6") {
		t.Errorf("the band legend does not carry the run tail and the cached share:\n%s", view)
	}
	// The header's CPU chip and the gauge's value stay on the same unit.
	if !strings.Contains(view, "cpu  40%") {
		t.Errorf("the cpu readout is not the node's capacity share:\n%s", view)
	}
}

// An older server, or a window with nothing attributed, renders its existing
// readouts without inventing an action or a tail.
func TestViewWithoutAttributionSaysNothingNew(t *testing.T) {
	m := newSizedTUI(120, 30)
	m.applyResult(multiPoll([]string{"alpha"}, map[string]int64{"alpha": 10}))
	view := m.View()
	if strings.Contains(view, "intra_op_threads") || strings.Contains(view, "runs p95") {
		t.Errorf("an unattributed window rendered an attribution:\n%s", view)
	}
	if !strings.Contains(view, "p50") {
		t.Errorf("the band legend disappeared:\n%s", view)
	}
}
