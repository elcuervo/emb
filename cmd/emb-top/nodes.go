package main

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// defaultAddr is the node watched when no node flag is given. It carries a
// port on purpose, so the default stays a single literal node rather than a
// DNS name that could expand to several addresses.
const defaultAddr = "localhost:6379"

// defaultPort is applied to a node specification that carries no port.
const defaultPort = "6379"

// nodeList collects repeatable -node flags.
type nodeList []string

func (n *nodeList) String() string { return strings.Join(*n, ",") }

func (n *nodeList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("empty node specification")
	}
	*n = append(*n, v)
	return nil
}

// nodeSpec is one parsed -addr/-node/-nodes entry.
type nodeSpec struct {
	raw    string // exactly as given, for error messages
	host   string
	port   string
	isName bool // expand host through the resolver to one node per address
}

// addr is the specification's host:port, with IPv6 brackets handled.
func (s nodeSpec) addr() string { return net.JoinHostPort(s.host, s.port) }

// parseNodeSpec parses one specification: host:port, a bare host (taking the
// default port), or a DNS name. An explicit port, or an IP literal, is an
// address; a bare non-IP host is a name that is resolved at watch time.
func parseNodeSpec(raw string) (nodeSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nodeSpec{}, fmt.Errorf("empty node specification")
	}
	host, port, hasPort, err := splitSpec(raw)
	if err != nil {
		return nodeSpec{}, err
	}
	if host == "" {
		return nodeSpec{}, fmt.Errorf("node specification %q has no host", raw)
	}
	return nodeSpec{
		raw:    raw,
		host:   host,
		port:   port,
		isName: !hasPort && net.ParseIP(host) == nil,
	}, nil
}

// splitSpec separates host and port, applying the default port when the entry
// carries none. It returns whether the port was explicit.
func splitSpec(s string) (host, port string, hasPort bool, err error) {
	if h, p, e := net.SplitHostPort(s); e == nil {
		if !validPort(p) {
			return "", "", false, fmt.Errorf("node specification %q has an invalid port %q", s, p)
		}
		return h, p, true, nil
	}
	if net.ParseIP(s) != nil {
		return s, defaultPort, false, nil // bare IPv4 or IPv6 literal
	}
	if strings.Contains(s, ":") {
		return "", "", false, fmt.Errorf("node specification %q is not host:port or a host", s)
	}
	return s, defaultPort, false, nil
}

func validPort(p string) bool {
	n, err := net.LookupPort("tcp", p)
	return err == nil && n > 0 && n <= 65535
}

// resolver is the DNS surface the fleet uses; a fake stands in for tests.
type resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

type netResolver struct{}

func (netResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

// resolvedNode is one monitored address a specification produced.
type resolvedNode struct {
	key  string // stable row identity: a per-node name, else the address
	spec string // the specification that produced it
	name string // DNS name when the row follows a name, "" for a literal address
	addr string // host:port to dial
}

// resolveSpecs expands every name specification to its address records and
// returns nodes in first-seen order, deduplicated on host:port. A name that
// resolves to exactly one address is keyed by its specification, so the row
// follows the name when the address changes; a name that expands to several
// addresses contributes one address-keyed row each. A specification that
// yields no address is an error naming it.
func resolveSpecs(ctx context.Context, r resolver, specs []nodeSpec) ([]resolvedNode, error) {
	var out []resolvedNode
	seen := map[string]bool{}
	add := func(spec nodeSpec, name, addr string, single bool) {
		if addr == "" || seen[addr] {
			return
		}
		seen[addr] = true
		key := addr
		if single {
			key = spec.raw
		}
		out = append(out, resolvedNode{key: key, spec: spec.raw, name: name, addr: addr})
	}
	for _, spec := range specs {
		if !spec.isName {
			add(spec, "", spec.addr(), true)
			continue
		}
		addrs, err := r.LookupHost(ctx, spec.host)
		if err != nil {
			return nil, fmt.Errorf("node %q: resolve %s: %w", spec.raw, spec.host, err)
		}
		addrs = dedupeStrings(addrs)
		if len(addrs) == 0 {
			return nil, fmt.Errorf("node %q resolves to no address", spec.raw)
		}
		for _, a := range addrs {
			add(spec, spec.host, net.JoinHostPort(a, spec.port), len(addrs) == 1)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no nodes to monitor")
	}
	return out, nil
}

func dedupeStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		dup := false
		for _, o := range out {
			if o == s {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}

// buildSpecs assembles the specifications from the node flags. -addr is a
// compatibility alias, -node is repeatable, and -nodes is comma-separated;
// the localhost default applies only when none of them is given.
func buildSpecs(addr string, nodes []string, nodesCSV string) ([]nodeSpec, error) {
	var raws []string
	if strings.TrimSpace(addr) != "" {
		raws = append(raws, addr)
	}
	raws = append(raws, nodes...)
	for _, v := range strings.Split(nodesCSV, ",") {
		if s := strings.TrimSpace(v); s != "" {
			raws = append(raws, s)
		}
	}
	if len(raws) == 0 {
		raws = []string{defaultAddr}
	}
	specs := make([]nodeSpec, 0, len(raws))
	for _, r := range raws {
		sp, err := parseNodeSpec(r)
		if err != nil {
			return nil, err
		}
		specs = append(specs, sp)
	}
	return specs, nil
}
