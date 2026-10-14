package embtop

import (
	"context"
	"errors"
	"strings"
	"testing"
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
		{"db-a:16379", "db-a", "16379", false, false},
		{"localhost", "localhost", "6379", true, false},
		{"10.0.3.7", "10.0.3.7", "6379", false, false},
		{"emb.internal", "emb.internal", "6379", true, false},
		{"emb.internal:16379", "emb.internal", "16379", false, false},
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
	specs, _ := BuildSpecs("", []string{"db-a:16379"}, "emb.internal")
	nodes, err := ResolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Addr != "db-a:16379" || nodes[1].Addr != "10.0.0.9:6379" {
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
