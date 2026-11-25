package embtop

import (
	"testing"
	"time"
)

func TestFleetMembershipTransitions(t *testing.T) {
	member := []Member{{Key: "service:6379", Spec: "service:6379", Addr: "10.0.0.1:6379"}}
	key, ok := MatchMember(ResolvedNode{Key: "10.0.0.1:6379", Spec: "service:6379", Addr: "10.0.0.1:6379"}, member)
	if !ok || key != "service:6379" {
		t.Fatalf("one-to-many rekey lost row: %q %v", key, ok)
	}
	if _, ok := MatchMember(ResolvedNode{Key: "10.0.0.2:6379", Spec: "service:6379", Addr: "10.0.0.2:6379"}, member); ok {
		t.Fatal("new peer reused a different address")
	}
	member[0].Key = "10.0.0.1:6379"
	if key, ok := MatchMember(ResolvedNode{Key: "service:6379", Spec: "service:6379", Addr: "10.0.0.1:6379"}, member); !ok || key != "10.0.0.1:6379" {
		t.Fatalf("many-to-one lost row: %q %v", key, ok)
	}
	member = []Member{{Key: "service:6379", Spec: "service:6379", Addr: "10.0.0.1:6379"}, {Key: "10.0.0.2:6379", Spec: "service:6379", Addr: "10.0.0.2:6379"}}
	if key, ok := MatchMember(ResolvedNode{Key: "service:6379", Spec: "service:6379", Addr: "10.0.0.2:6379"}, member); !ok || key != "10.0.0.2:6379" {
		t.Fatalf("name contracted onto wrong peer: %q %v", key, ok)
	}
	if key, ok := MatchMember(ResolvedNode{Key: "alias:6379", Spec: "alias:6379", Addr: "10.0.0.2:6379"}, member); !ok || key != "10.0.0.2:6379" {
		t.Fatalf("configured aliases duplicated peer: %q %v", key, ok)
	}
	at := time.Unix(1000, 0)
	if PruneOrphan(false, true, at, at.Add(time.Hour), 5*time.Minute) || PruneOrphan(false, false, at, at.Add(5*time.Minute), 5*time.Minute) {
		t.Fatal("answering or grace-period peer pruned")
	}
	if !PruneOrphan(false, false, at, at.Add(5*time.Minute+time.Second), 5*time.Minute) {
		t.Fatal("unreachable orphan not pruned")
	}
}
func TestFleetRatesExcludeFailedAndRebase(t *testing.T) {
	at := time.Unix(1000, 0)
	p := []Point{{At: at, ReqRate: 3, TokRate: 9, ErrRate: 1, Active: 2, Conns: 4}, {At: at, ReqRate: 1000, TokRate: 1000, Active: 1000}}
	var total FleetTotals
	total.Add(p[0], true)
	total.Add(p[1], false)
	if total.ReqRate != 3 || total.TokRate != 9 || total.ErrRate != 1 || total.Active != 2 || total.Conns != 4 {
		t.Fatalf("failed peer leaked into sum: %+v", total)
	}
	p[1] = Point{At: at} // first poll after reconnect seeds a fresh counter baseline
	total.Add(p[1], true)
	if total.ReqRate != 3 || total.TokRate != 9 {
		t.Fatalf("rejoining counter spiked sum: %+v", total)
	}
}

func TestTrimEventsSinceHandlesOutOfOrderClock(t *testing.T) {
	s := NewSampler(300)
	s.PushEvents([]Event{{AtUs: time.Unix(12, 0).UnixMicro(), LatencyUs: 1}, {AtUs: time.Unix(9, 0).UnixMicro(), LatencyUs: 2}, {AtUs: time.Unix(13, 0).UnixMicro(), LatencyUs: 3}})
	s.TrimEventsSince(time.Unix(10, 0))
	if len(s.Events) != 2 || s.Events[0].LatencyUs != 1 || s.Events[1].LatencyUs != 3 {
		t.Fatalf("old events retained: %+v", s.Events)
	}
}
