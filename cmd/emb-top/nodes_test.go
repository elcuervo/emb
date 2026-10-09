package main

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
		got, err := parseNodeSpec(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("parseNodeSpec(%q) = %+v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNodeSpec(%q): %v", tc.in, err)
			continue
		}
		if got.host != tc.host || got.port != tc.port || got.isName != tc.isName {
			t.Errorf("parseNodeSpec(%q) = %+v, want host=%q port=%q isName=%v",
				tc.in, got, tc.host, tc.port, tc.isName)
		}
	}
}

func TestParseFlagsNodeForms(t *testing.T) {
	cases := []struct {
		args []string
		want []string // expected specs, as host:port
	}{
		{nil, []string{"localhost:6379"}},
		{[]string{"-addr", "a:1"}, []string{"a:1"}},
		{[]string{"-node", "a:1"}, []string{"a:1"}},
		{[]string{"-node", "a:1", "-node", "b:2"}, []string{"a:1", "b:2"}},
		{[]string{"-nodes", "a:1,b:2"}, []string{"a:1", "b:2"}},
		{[]string{"-addr", "a:1", "-nodes", "b:2"}, []string{"a:1", "b:2"}},
		{[]string{"-node", "name"}, []string{"name:6379"}},
	}
	for _, tc := range cases {
		o, err := parseFlags(tc.args, func(string) string { return "" })
		if err != nil {
			t.Errorf("parseFlags(%v): %v", tc.args, err)
			continue
		}
		var got []string
		for _, s := range o.specs {
			got = append(got, s.addr())
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("parseFlags(%v) specs = %v, want %v", tc.args, got, tc.want)
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
	specs, err := buildSpecs("", nil, "emb.internal")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := resolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("resolved %d nodes, want 3: %+v", len(nodes), nodes)
	}
	for _, n := range nodes {
		if !strings.HasSuffix(n.addr, ":6379") {
			t.Errorf("node %q did not take the default port", n.addr)
		}
	}
}

func TestResolveSpecsDedupes(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{
		"a.internal": {"10.0.0.1", "10.0.0.2"},
		"b.internal": {"10.0.0.2", "10.0.0.3"},
	}}
	specs, err := buildSpecs("", nil, "a.internal,b.internal")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := resolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("dedupe kept %d nodes, want 3: %+v", len(nodes), nodes)
	}
}

func TestResolveSpecsEmptyIsError(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{"empty.internal": {}}}
	specs, _ := buildSpecs("", nil, "empty.internal")
	_, err := resolveSpecs(context.Background(), r, specs)
	if err == nil || !strings.Contains(err.Error(), "empty.internal") {
		t.Fatalf("err = %v, want an error naming the specification", err)
	}
}

func TestResolveSpecsResolverError(t *testing.T) {
	r := &fakeResolver{errs: map[string]error{"down.internal": errors.New("no such host")}}
	specs, _ := buildSpecs("", nil, "down.internal")
	_, err := resolveSpecs(context.Background(), r, specs)
	if err == nil || !strings.Contains(err.Error(), "down.internal") {
		t.Fatalf("err = %v, want an error naming the specification", err)
	}
}

func TestResolveSpecsMixesAddressesAndNames(t *testing.T) {
	r := &fakeResolver{answers: map[string][]string{"emb.internal": {"10.0.0.9"}}}
	specs, _ := buildSpecs("", []string{"db-a:16379"}, "emb.internal")
	nodes, err := resolveSpecs(context.Background(), r, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].addr != "db-a:16379" || nodes[1].addr != "10.0.0.9:6379" {
		t.Fatalf("mixed fleet = %+v", nodes)
	}
}
