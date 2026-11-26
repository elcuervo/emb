package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elcuervo/emb/internal/embtop"
)

const (
	pollEvery     = time.Second
	resolveEvery  = 30 * time.Second
	orphanGrace   = 5 * time.Minute
	historyPoints = 300
	maxNodes      = 64
)

type node struct {
	id, key, spec, name, address string
	tlsName                      string
	resolved, reachable          bool
	unreachableSince, lastOK     time.Time
	failure                      string
	generation                   int
	stop                         chan struct{}
	ticks                        chan struct{}
	sampler                      *embtop.Sampler
	samples                      int
	p95Base                      int64
	modelOrder                   []string
	modelSeen                    map[string]bool
	models                       map[string]embtop.ModelListEntry
}

type pollResult struct {
	n          *node
	generation int
	result     *embtop.PollResult
	dialed     bool
	err        error
}

type resolveResult struct {
	nodes []embtop.ResolvedNode
	err   error
}

// monitor is the sole sampling owner. HTTP readers only see immutable published JSON.
type monitor struct {
	specs     []embtop.NodeSpec
	password  string
	useTLS    bool
	tlsName   string
	resolver  embtop.Resolver
	newClient func(address, password string, tls bool) *embtop.Client
	now       func() time.Time

	nodes        []*node
	byKey        map[string]*node
	nextID       int
	resolveError string
	fleetHistory []fleetPoint
	results      chan pollResult
	resolved     chan resolveResult

	mu      sync.Mutex
	latest  []byte
	viewers map[chan []byte]struct{}
}

func newMonitor(specs []embtop.NodeSpec, password string, useTLS bool, tlsName string) *monitor {
	m := &monitor{
		specs: specs, password: password, useTLS: useTLS, tlsName: tlsName,
		resolver: embtop.NetResolver{}, newClient: embtop.NewClient, now: time.Now,
		byKey: make(map[string]*node), results: make(chan pollResult), resolved: make(chan resolveResult, 1),
		viewers: make(map[chan []byte]struct{}),
	}
	m.publish()
	return m
}

func (m *monitor) startWorker(ctx context.Context, n *node, rn embtop.ResolvedNode) {
	n.generation++
	generation := n.generation
	if n.stop != nil {
		close(n.stop)
	}
	n.stop = make(chan struct{})
	stop := n.stop
	n.ticks = make(chan struct{}, 1)
	tick := n.ticks
	client := m.newClient(rn.Addr, m.password, m.useTLS)
	name := m.tlsName
	if name == "" {
		name = rn.TLSName
	}
	if name != "" {
		client.SetTLSServerName(name)
	}
	client.SetTimeout(3 * time.Second)
	go func() {
		defer func() { _ = client.Close() }()
		select {
		case tick <- struct{}{}: // the first sample need not wait one interval
		default: // the poll ticker already queued a sample
		}
		var known []string
		var cursor uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-tick:
			}
			dialed, err := client.EnsureConn()
			var result *embtop.PollResult
			if err == nil {
				if dialed {
					cursor = 0
				}
				result, err = client.Poll(known, cursor)
			}
			if err == nil {
				cursor = result.NextSeq
				for _, model := range result.Models {
					found := false
					for _, k := range known {
						if k == model.Name {
							found = true
							break
						}
					}
					if !found {
						known = append(known, model.Name)
					}
				}
			} else {
				_ = client.Close() // never reuse a stream after an incomplete poll
			}
			select {
			case m.results <- pollResult{n: n, generation: generation, result: result, dialed: dialed, err: err}:
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()
}

func (m *monitor) applyResolution(ctx context.Context, result resolveResult) {
	if result.err != nil {
		m.resolveError = "DNS resolution unavailable"
		m.publish()
		return
	}
	if len(result.nodes) > maxNodes {
		m.resolveError = "Resolved fleet exceeds 64-node limit"
		m.publish()
		return
	}
	m.resolveError = ""
	members := make([]embtop.Member, 0, len(m.nodes)+len(result.nodes))
	for _, n := range m.nodes {
		members = append(members, embtop.Member{Key: n.key, Spec: n.spec, Addr: n.address})
	}
	seen := make(map[*node]bool, len(result.nodes))
	for _, rn := range result.nodes {
		key, found := embtop.MatchMember(rn, members)
		if !found {
			if len(m.nodes) >= maxNodes {
				m.resolveError = "Fleet exceeds 64-node limit; new peers are waiting for orphan expiry"
				continue
			}
			// The id is the browser's row handle, not the resolution key: a
			// rekeyed row keeps its id so two rows can never share one.
			m.nextID++
			n := &node{id: fmt.Sprintf("node-%d", m.nextID), key: rn.Key, spec: rn.Spec, name: rn.Name, address: rn.Addr, tlsName: rn.TLSName, resolved: true, sampler: embtop.NewSampler(historyPoints)}
			n.sampler.SetClock(m.now)
			m.nodes = append(m.nodes, n)
			m.byKey[n.key] = n
			m.startWorker(ctx, n, rn)
			members = append(members, embtop.Member{Key: rn.Key, Spec: rn.Spec, Addr: rn.Addr})
			seen[n] = true
			continue
		}
		n := m.byKey[key]
		if n.key != rn.Key {
			if displaced := m.byKey[rn.Key]; displaced != nil && displaced != n {
				displaced.key = displaced.address
				m.byKey[displaced.key] = displaced
				for i := range members {
					if members[i].Key == rn.Key {
						members[i].Key = displaced.key
						break
					}
				}
			}
			delete(m.byKey, n.key)
			n.key = rn.Key
			m.byKey[n.key] = n
		}
		if n.address != rn.Addr || n.tlsName != rn.TLSName {
			n.address, n.tlsName = rn.Addr, rn.TLSName
			n.reachable = false
			n.failure = ""
			n.lastOK = time.Time{}
			n.sampler.Reset()
			n.samples, n.p95Base = 0, 0
			m.startWorker(ctx, n, rn)
		}
		n.spec, n.name, n.resolved = rn.Spec, rn.Name, true
		for i := range members {
			if members[i].Key == key {
				members[i].Key, members[i].Spec, members[i].Addr = rn.Key, rn.Spec, rn.Addr
				break
			}
		}
		seen[n] = true
	}
	for _, n := range m.nodes {
		if !seen[n] {
			n.resolved = false
		}
	}
	m.prune()
	m.publish()
}

func (m *monitor) prune() {
	kept := m.nodes[:0]
	for _, n := range m.nodes {
		if embtop.PruneOrphan(n.resolved, n.reachable, n.unreachableSince, m.now(), orphanGrace) {
			close(n.stop)
			delete(m.byKey, n.key)
			continue
		}
		kept = append(kept, n)
	}
	m.nodes = kept
}

func (m *monitor) applyPoll(msg pollResult) {
	n := msg.n
	if n == nil || m.byKey[n.key] != n || n.generation != msg.generation {
		return
	}
	now := m.now()
	if msg.err != nil {
		n.reachable = false
		if n.unreachableSince.IsZero() {
			n.unreachableSince = now
		}
		// TLS first: "certificate signed by unknown authority" contains "auth".
		reason := strings.ToLower(msg.err.Error())
		switch {
		case strings.Contains(reason, "tls"), strings.Contains(reason, "certificate"), strings.Contains(reason, "x509"):
			n.failure = "TLS verification failed"
		case strings.Contains(reason, "auth"), strings.Contains(reason, "noauth"):
			n.failure = "authentication rejected"
		default:
			n.failure = "connection unavailable"
		}
	} else {
		if msg.dialed || (!n.sampler.Latest.At.IsZero() && msg.result.UptimeSecs < n.sampler.Latest.UptimeSecs) {
			n.sampler.Reset()
			n.samples, n.p95Base = 0, 0
		}
		n.sampler.Push(msg.result)
		n.sampler.PushEvents(msg.result.Events)
		n.sampler.TrimEventsSince(now.Add(-historyPoints * pollEvery))
		n.samples++
		if _, p95, _, ok := n.sampler.Latency(); ok && p95 > 0 {
			if n.p95Base == 0 {
				n.p95Base = p95
			} else {
				n.p95Base = (n.p95Base*9 + p95) / 10
			}
		}
		if n.modelSeen == nil {
			n.modelSeen = make(map[string]bool)
			n.models = make(map[string]embtop.ModelListEntry)
		}
		clear(n.models)
		for _, model := range msg.result.Models {
			if !n.modelSeen[model.Name] {
				n.modelOrder = append(n.modelOrder, model.Name)
				n.modelSeen[model.Name] = true
			}
			n.models[model.Name] = model
		}
		n.lastOK, n.reachable = now, true
		n.unreachableSince = time.Time{}
		n.failure = ""
	}
	m.prune()
	m.refreshLatest()
}

func (m *monitor) refresh(ctx context.Context) {
	go func() {
		resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		nodes, err := embtop.ResolveSpecs(resolveCtx, m.resolver, m.specs)
		select {
		case m.resolved <- resolveResult{nodes, err}:
		case <-ctx.Done():
		}
	}()
}

func (m *monitor) run(ctx context.Context) {
	poll := time.NewTicker(pollEvery)
	resolve := time.NewTicker(resolveEvery)
	defer poll.Stop()
	defer resolve.Stop()
	defer func() {
		for _, n := range m.nodes {
			close(n.stop)
		}
	}()
	m.refresh(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-m.resolved:
			m.applyResolution(ctx, result)
		case result := <-m.results:
			m.applyPoll(result)
		case <-resolve.C:
			m.refresh(ctx)
		case <-poll.C:
			for _, n := range m.nodes {
				select {
				case n.ticks <- struct{}{}:
				default:
				}
			}
			m.prune()
			m.record()
		}
	}
}
