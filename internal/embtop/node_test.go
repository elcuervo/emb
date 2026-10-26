package embtop

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseNodeSpecTable(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		port   string
		isName bool
		err    bool
	}{
		{"localhost:6379", "localhost", "6379", false, false},
		{"LocalHost:6379", "LocalHost", "6379", false, false},
		{"db-a:16379", "db-a", "16379", true, false},
		{"localhost", "localhost", "6379", false, false},
		{"localhost.:6379", "localhost.", "6379", false, false},
		{"localhost.", "localhost.", "6379", false, false},
		{"10.0.3.7", "10.0.3.7", "6379", false, false},
		{"emb.internal", "emb.internal", "6379", true, false},
		{"emb.internal:16379", "emb.internal", "16379", true, false},
		{"[::1]:16379", "::1", "16379", false, false},
		{"::1", "::1", "6379", false, false},
		{"", "", "", false, true},
		{"host:notaport", "", "", false, true},
	}
	for _, tc := range cases {
		got, err := ParseNodeSpec(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("ParseNodeSpec(%q) = %+v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseNodeSpec(%q): %v", tc.in, err)
			continue
		}
		if got.host != tc.host || got.port != tc.port || got.isName != tc.isName {
			t.Errorf("ParseNodeSpec(%q) = %+v, want host=%q port=%q isName=%v",
				tc.in, got, tc.host, tc.port, tc.isName)
		}
	}
}

// A port in the specification decides the port dialed, never whether the host
// is a name: the records behind a name are expanded on that port.
func TestResolveSpecsExpandsNamesOnTheirPort(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{
		"emb.internal": {"10.0.3.7", "10.0.3.8", "10.0.3.9"},
		"db-a":         {"10.0.4.2"},
	}}
	specs, err := BuildSpecs("", nil, "emb.internal:6399,db-a:6399")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 4 {
		t.Fatalf("resolved %d nodes, want 4: %+v", len(nodes), nodes)
	}
	for i, want := range []string{"10.0.3.7:6399", "10.0.3.8:6399", "10.0.3.9:6399"} {
		n := nodes[i]
		if n.Addr != want || n.Name != "" || n.Key != want {
			t.Errorf("expanded node %d = %+v, want %s labelled and keyed by its address", i, n, want)
		}
		if n.TLSName != "emb.internal" {
			t.Errorf("expanded node %d verifies TLS against %q, want the resolved name", i, n.TLSName)
		}
	}
	if n := nodes[3]; n.Addr != "10.0.4.2:6399" || n.Name != "db-a" || n.Key != "db-a:6399" || n.TLSName != "db-a" {
		t.Errorf("single-address name with a port = %+v, want the specification's identity", n)
	}
}

// A literal address has no name to verify: the client checks the dialed host.
func TestResolveSpecsLiteralRowsCarryNoTLSName(t *testing.T) {
	specs, err := BuildSpecs("", []string{"10.0.0.1:16379", "localhost:6379"}, "")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ResolveSpecs(context.Background(), &fakeResolver{}, specs)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.TLSName != "" || n.Name != "" || n.Addr != n.Key {
			t.Errorf("literal node = %+v, want no name and its own address as identity", n)
		}
	}
}

// fakeResolver is a scripted DNS resolver.
type fakeResolver struct {
	answers map[string][]string
	errs    map[string]error
	calls   int
}

func (r *fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	r.calls++
	if err := r.errs[host]; err != nil {
		return nil, err
	}
	return r.answers[host], nil
}

func TestResolveSpecsExpandsNames(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{
		"emb.internal": {"10.0.3.7", "10.0.3.8", "10.0.3.9"},
	}}
	specs, err := BuildSpecs("", nil, "emb.internal")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("resolved %d nodes, want 3: %+v", len(nodes), nodes)
	}
	for _, n := range nodes {
		if !strings.HasSuffix(n.Addr, ":6379") {
			t.Errorf("node %q did not take the default port", n.Addr)
		}
	}
}

func TestResolveSpecsDedupes(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{
		"a.internal": {"10.0.0.1", "10.0.0.2"},
		"b.internal": {"10.0.0.2", "10.0.0.3"},
	}}
	specs, err := BuildSpecs("", nil, "a.internal,b.internal")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("dedupe kept %d nodes, want 3: %+v", len(nodes), nodes)
	}
}

func TestResolveSpecsEmptyIsError(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{"empty.internal": {}}}
	specs, _ := BuildSpecs("", nil, "empty.internal")
	_, err := ResolveSpecs(context.Background(), r, specs)
	if err == nil || !strings.Contains(err.Error(), "empty.internal") {
		t.Fatalf("err = %v, want an error naming the specification", err)
	}
}

func TestResolveSpecsResolverError(t *testing.T) {
	r := &fakeResolver{errs: map[string]error{"down.internal": errors.New("no such host")}}
	specs, _ := BuildSpecs("", nil, "down.internal")
	_, err := ResolveSpecs(context.Background(), r, specs)
	if err == nil || !strings.Contains(err.Error(), "down.internal") {
		t.Fatalf("err = %v, want an error naming the specification", err)
	}
}

func TestResolveSpecsMixesAddressesAndNames(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{"emb.internal": {"10.0.0.9"}}}
	specs, _ := BuildSpecs("", []string{"10.0.0.1:16379"}, "emb.internal")
	nodes, err := ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Addr != "10.0.0.1:16379" || nodes[1].Addr != "10.0.0.9:6379" {
		t.Fatalf("mixed fleet = %+v", nodes)
	}
}

// TestResolveSpecsNamesExpandedRowsByAddress guards row identity: a name that
// expands to several addresses must produce rows named by the address they
// dial, or every row of the cluster view carries the same label.
func TestResolveSpecsNamesExpandedRowsByAddress(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{
		"emb.internal": {"10.0.3.7", "10.0.3.8"},
	}}
	specs, err := BuildSpecs("", nil, "emb.internal")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("resolved %d nodes, want 2", len(nodes))
	}
	for _, n := range nodes {
		if n.Name != "" {
			t.Errorf("node %s is labelled %q; an expanded row is named by its address", n.Addr, n.Name)
		}
		if n.Key != n.Addr {
			t.Errorf("node %s is keyed %q, want its address", n.Addr, n.Key)
		}
	}

	// A name that resolves to exactly one address still follows the name, so
	// the row survives the record moving.
	r = &fakeResolver{answers: map[string][]string{"one.internal": {"10.0.0.9"}}}
	specs, _ = BuildSpecs("", nil, "one.internal")
	nodes, err = ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Name != "one.internal" || nodes[0].Key != "one.internal" {
		t.Fatalf("a single-address name did not keep its identity: %+v", nodes)
	}
}

// flakyResolver fails the first failN lookups, then answers: the DNS record
// that appears while the fleet starts.
type flakyResolver struct {
	failN int
	err   error
	calls int
}

func (r *flakyResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	r.calls++
	if r.calls <= r.failN {
		return nil, r.err
	}
	return []string{"10.0.3.7"}, nil
}

func TestWaitForSpecsRetriesUntilResolved(t *testing.T) {
	r := &flakyResolver{failN: 2, err: notFound("late.internal")}
	specs, _ := BuildSpecs("", nil, "late.internal")
	nodes, err := WaitForSpecs(context.Background(), r, specs, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != 3 {
		t.Errorf("calls = %d, want 3", r.calls)
	}
	if len(nodes) != 1 || nodes[0].Addr != "10.0.3.7:6379" {
		t.Fatalf("nodes = %+v, want the resolved address", nodes)
	}
}

// TestWaitForSpecsIsAllOrNothing guards the invariant the capability states:
// a specification that stays unresolvable fails the call, and the addresses
// its siblings produced are not presented as a fleet.
func TestWaitForSpecsIsAllOrNothing(t *testing.T) {
	r := &fakeResolver{
		answers: map[string][]string{"early.internal": {"10.0.0.9"}},
		errs:    map[string]error{"late.internal": notFound("late.internal")},
	}
	specs, _ := BuildSpecs("", nil, "late.internal,early.internal")
	nodes, err := WaitForSpecs(context.Background(), r, specs, 2*time.Second)
	if nodes != nil {
		t.Errorf("nodes = %+v, want none: a name never resolved", nodes)
	}
	if err == nil || !strings.Contains(err.Error(), `node "late.internal"`) {
		t.Fatalf("err = %v, want an error naming the failing specification", err)
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) || dnsErr.Name != "late.internal" {
		t.Fatalf("err = %v, want the resolver's *net.DNSError reachable", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2: one attempt and one retry within the budget", r.calls)
	}
}

// TestWaitForSpecsStopsAtBudget guards against a wait that outlives its
// budget: a budget shorter than the spacing buys no retry.
func TestWaitForSpecsStopsAtBudget(t *testing.T) {
	r := &fakeResolver{errs: map[string]error{"late.internal": notFound("late.internal")}}
	specs, _ := BuildSpecs("", nil, "late.internal")
	nodes, err := WaitForSpecs(context.Background(), r, specs, 100*time.Millisecond)
	if nodes != nil || err == nil {
		t.Fatalf("nodes = %+v, err = %v, want no nodes and an error", nodes, err)
	}
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1", r.calls)
	}
}

// notFound is the resolver failure a name that does not exist yet produces.
func notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// TestIsLoopbackAndAllLoopback covers the plaintext-AUTH guard both tools use.
func TestIsLoopbackAndAllLoopback(t *testing.T) {
	for _, addr := range []string{"localhost:6379", "127.0.0.1:6379", "[::1]:6379", "127.0.0.2:1"} {
		if !IsLoopback(addr) {
			t.Errorf("IsLoopback(%q) = false, want true", addr)
		}
	}
	for _, addr := range []string{"10.0.0.1:6379", "emb.internal:6379", "0.0.0.0:6379"} {
		if IsLoopback(addr) {
			t.Errorf("IsLoopback(%q) = true, want false", addr)
		}
	}
	if !AllLoopback(nil) {
		t.Error("an empty specification list is trivially all loopback")
	}
	local, _ := BuildSpecs("", nil, "localhost:6379")
	if !AllLoopback(local) {
		t.Error("AllLoopback(localhost) = false")
	}
	mixed, _ := BuildSpecs("", nil, "localhost:6379,10.0.0.1:6379")
	if AllLoopback(mixed) {
		t.Error("AllLoopback(mixed) = true, want false")
	}
}
