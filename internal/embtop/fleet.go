package embtop

import "time"

// Member identifies a monitored row independently of its rendering or connection.
type Member struct {
	Key, Spec, Addr string
}

// MatchMember returns the existing row matching a resolution. A DNS name
// switching between one and several addresses keeps the answering row instead
// of sampling and counting it twice.
func MatchMember(resolved ResolvedNode, members []Member) (string, bool) {
	for _, m := range members {
		if m.Spec == resolved.Spec && m.Addr == resolved.Addr {
			return m.Key, true
		}
	}
	// Two configured names can converge on one peer; never poll the same
	// address twice under different row keys.
	for _, m := range members {
		if m.Addr == resolved.Addr {
			return m.Key, true
		}
	}
	// A single-address name can follow a genuinely new address when no
	// existing row already answers there.
	for _, m := range members {
		if m.Key == resolved.Key {
			return m.Key, true
		}
	}
	return "", false
}

// PruneOrphan waits until a missing DNS peer is also unresponsive beyond grace.
func PruneOrphan(resolved, reachable bool, unreachableSince, now time.Time, grace time.Duration) bool {
	return !resolved && !reachable && !unreachableSince.IsZero() && now.Sub(unreachableSince) > grace
}

// FleetTotals adds independently rebased rates and gauges from answering nodes.
type FleetTotals struct {
	ReqRate, TokRate, ErrRate float64
	Active, Conns             int64
}

// Add includes one reachable, sampled node in the aggregate.
func (f *FleetTotals) Add(p Point, reachable bool) {
	if !reachable || p.At.IsZero() {
		return
	}
	f.ReqRate += p.ReqRate
	f.TokRate += p.TokRate
	f.ErrRate += p.ErrRate
	f.Active += p.Active
	f.Conns += p.Conns
}
