package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

func TestMonitorMembershipAndFailureRecovery(t *testing.T) {
	spec, _ := embtop.ParseNodeSpec("fleet.example:6379")
	m := newMonitor([]embtop.NodeSpec{spec}, "", false, "")
	at := time.Unix(123456, 0)
	m.now = func() time.Time { return at }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := embtop.ResolvedNode{Key: "fleet.example:6379", Spec: "fleet.example:6379", Name: "fleet.example", Addr: "127.0.0.1:1"}
	m.applyResolution(ctx, resolveResult{nodes: []embtop.ResolvedNode{first}})
	defer func() {
		for _, n := range m.nodes {
			close(n.stop)
		}
	}()
	n := m.nodes[0]
	handle := n.id
	m.applyPoll(pollResult{n: n, generation: n.generation, dialed: true, result: &embtop.PollResult{TotalRequests: 100, UptimeSecs: 50}})
	at = at.Add(time.Second)
	m.applyPoll(pollResult{n: n, generation: n.generation, result: &embtop.PollResult{TotalRequests: 104, UptimeSecs: 51}})
	if got := m.buildSnapshot().Nodes[0].Point.ReqRate; got == nil || *got != 4 {
		t.Fatalf("expected 4 req/s, got %v", got)
	}
	m.applyPoll(pollResult{n: n, generation: n.generation, err: errors.New("timeout")})
	if stale := m.buildSnapshot().Nodes[0]; stale.State != "stale" || stale.Point != nil || stale.LastOK == nil {
		t.Fatalf("lost sample was presented as live: %+v", stale)
	}
	m.applyPoll(pollResult{n: n, generation: n.generation, err: errors.New("auth: invalid credential secret123")})
	v := m.buildSnapshot().Nodes[0]
	if v.State != "auth_failed" || v.Point != nil || strings.Contains(v.Reason, "secret123") {
		t.Fatalf("unsafe authentication state: %+v", v)
	}
	at = at.Add(time.Second)
	m.applyPoll(pollResult{n: n, generation: n.generation, dialed: true, result: &embtop.PollResult{TotalRequests: 2, UptimeSecs: 1}})
	v = m.buildSnapshot().Nodes[0]
	if v.Point == nil || v.Point.ReqRate != nil || len(v.History) != 1 {
		t.Fatalf("reconnect did not rebase counters/window: %+v", v)
	}
	second := embtop.ResolvedNode{Key: "127.0.0.1:1", Spec: first.Spec, Addr: first.Addr}
	m.applyResolution(ctx, resolveResult{nodes: []embtop.ResolvedNode{second, {Key: "127.0.0.1:2", Spec: first.Spec, Addr: "127.0.0.1:2"}}})
	if len(m.nodes) != 2 || m.nodes[0] != n || m.nodes[0].key != second.Key {
		t.Fatal("DNS one-to-many duplicated existing row")
	}
	m.applyResolution(ctx, resolveResult{nodes: []embtop.ResolvedNode{{Key: first.Key, Spec: first.Spec, Name: first.Name, Addr: "127.0.0.1:2"}}})
	if !m.buildSnapshot().Nodes[0].Orphaned {
		t.Fatal("answering peer lost orphan marker")
	}
	if len(m.nodes) != 2 {
		t.Fatal("answering orphan dropped")
	}
	if m.byKey[n.key] != n || m.byKey["fleet.example:6379"] != m.nodes[1] || n.id != handle || m.nodes[1].id == handle {
		t.Fatal("DNS contraction lost stable row handles or duplicated one")
	}
	m.resolveError = "DNS resolution unavailable"
	if m.buildSnapshot().Error == "" {
		t.Fatal("DNS failure hidden")
	}
}

func TestModelOrderAndMissingAttribution(t *testing.T) {
	m := newMonitor(nil, "", false, "")
	at := time.Unix(1000, 0)
	m.now = func() time.Time { return at }
	n := &node{id: "node", key: "node", address: "127.0.0.1:1", resolved: true, sampler: embtop.NewSampler(historyPoints)}
	n.sampler.SetClock(m.now)
	m.nodes = []*node{n}
	m.byKey[n.key] = n
	first := &embtop.PollResult{Models: []embtop.ModelListEntry{{Name: "B", Status: "ready"}, {Name: "A", Status: "ready"}}, PerModel: map[string]*embtop.ModelStats{"A": {Requests: 1}, "B": {Requests: 1}}}
	m.applyPoll(pollResult{n: n, result: first})
	at = at.Add(time.Second)
	second := &embtop.PollResult{Models: []embtop.ModelListEntry{{Name: "A", Status: "ready"}, {Name: "B", Status: "ready"}}, PerModel: first.PerModel}
	m.applyPoll(pollResult{n: n, result: second})
	v := m.buildSnapshot().Nodes[0]
	if len(v.Models) != 2 || v.Models[0].Name != "B" || v.Models[1].Name != "A" {
		t.Fatalf("model order changed with server enumeration: %+v", v.Models)
	}
	if v.Models[0].Point.RunUs != nil {
		t.Fatal("unreported model phases presented as zero")
	}
}

func TestBoundedWindowMeasuredFields(t *testing.T) {
	s := embtop.NewSampler(historyPoints)
	at := time.Unix(45678, 0)
	s.SetClock(func() time.Time { return at })
	for i := range historyPoints + 5 {
		s.Push(&embtop.PollResult{TotalRequests: int64(i), PerModel: map[string]*embtop.ModelStats{"model": {Requests: int64(i)}}})
		at = at.Add(time.Second)
	}
	_, hist, mods := s.Snapshot()
	if len(hist) != historyPoints || len(mods["model"]) != historyPoints {
		t.Fatal("sample window grew past 300")
	}
	p := pointOf(hist[len(hist)-1], true, false)
	if p.CPUBudget != nil || p.CacheHitRate != nil {
		t.Fatal("unreported CPU plan or cache fabricated")
	}
	mp := modelPointOf(mods["model"][len(hist)-1], true, false)
	if mp.DispatchWaitUs != nil || mp.RunUs != nil {
		t.Fatal("absent phase attribution fabricated")
	}
	m := newMonitor(nil, "", false, "")
	m.now = func() time.Time { return at }
	for range historyPoints + 5 {
		m.record()
		at = at.Add(time.Second)
	}
	if got := len(m.buildSnapshot().Fleet.History); got != historyPoints {
		t.Fatalf("fleet history grew to %d", got)
	}
}

func TestEventsExpireWithIdlePublication(t *testing.T) {
	m := newMonitor(nil, "", false, "")
	at := time.Unix(1000, 0)
	m.now = func() time.Time { return at }
	n := &node{id: "n", key: "n", address: "127.0.0.1:1", resolved: true, sampler: embtop.NewSampler(historyPoints)}
	n.sampler.SetClock(m.now)
	m.nodes = []*node{n}
	m.byKey[n.key] = n
	m.applyPoll(pollResult{n: n, result: &embtop.PollResult{Events: []embtop.Event{{AtUs: at.UnixMicro(), LatencyUs: 12}}}})
	if v := m.buildSnapshot().Nodes[0]; len(v.Events) != 1 || v.P95Us == nil {
		t.Fatal("measured event missing from recent window")
	}
	at = at.Add(301 * time.Second)
	m.record()
	if v := m.buildSnapshot().Nodes[0]; len(v.Events) != 0 || v.P95Us != nil || v.Point != nil {
		t.Fatalf("idle publication retained expired live evidence: %+v", v)
	}
}

func TestFlagTargetsRequiredAndTLS(t *testing.T) {
	if _, err := parseFlags(nil); err == nil || !strings.Contains(err.Error(), "missing target") {
		t.Fatalf("missing target: %v", err)
	}
	o, err := parseFlags([]string{"-node", "127.0.0.1:6379", "-nodes", "fleet.example:6380,127.0.0.2:6379", "-tls"})
	if err != nil || len(o.specs) != 3 || !o.useTLS || o.listen != "127.0.0.1:8080" {
		t.Fatalf("targets/TLS/default listen: %+v %v", o, err)
	}
	if _, err := parseFlags([]string{"-node", "127.0.0.1:6379", "-nodes", "bad,"}); err == nil {
		t.Fatal("empty CSV target accepted")
	}
	if _, err := parseFlags([]string{"-node", "127.0.0.1:6379", "-password", "bad"}); err == nil {
		t.Fatal("password CLI flag exposed")
	}
	targets := make([]string, 0, (maxNodes+1)*2)
	for range maxNodes + 1 {
		targets = append(targets, "-node", "127.0.0.1:6379")
	}
	if _, err := parseFlags(targets); err == nil {
		t.Fatal("unbounded upstream node count accepted")
	}
}

// An untrusted certificate names an "unknown authority", which contains "auth":
// it must not be reported as a rejected credential.
func TestUnknownAuthorityIsTLSNotAuthentication(t *testing.T) {
	spec, _ := embtop.ParseNodeSpec("fleet.example:6379")
	m := newMonitor([]embtop.NodeSpec{spec}, "", false, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.applyResolution(ctx, resolveResult{nodes: []embtop.ResolvedNode{{Key: "fleet.example:6379", Spec: "fleet.example:6379", Addr: "127.0.0.1:1"}}})
	defer func() {
		for _, n := range m.nodes {
			close(n.stop)
		}
	}()
	n := m.nodes[0]
	m.applyPoll(pollResult{n: n, generation: n.generation, err: errors.New("x509: certificate signed by unknown authority")})
	if got := m.buildSnapshot().Nodes[0]; got.Reason != "TLS verification failed" || got.State == "auth_failed" {
		t.Fatalf("unknown authority classified as %q/%q", got.State, got.Reason)
	}
}
