package embtop

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// DefaultAddr is the node watched when no node flag is given. It carries a
// port on purpose, so the default stays a single literal node rather than a
// DNS name that could expand to several addresses.
const DefaultAddr = "localhost:6379"

// DefaultPort is applied to a node specification that carries no port.
const DefaultPort = "6379"

// NodeSpec is one parsed node specification: host:port, a bare host (taking
// the default port), or a DNS name to be expanded at watch time.
type NodeSpec struct {
	raw    string // exactly as given, for error messages
	host   string
	port   string
	isName bool // expand host through the resolver to one node per address
}

// Addr is the specification's host:port, with IPv6 brackets handled.
func (s NodeSpec) Addr() string { return net.JoinHostPort(s.host, s.port) }

// ParseNodeSpec parses one specification: host:port, a bare host (taking the
// default port), or a DNS name. An explicit port, or an IP literal, is an
// address; a bare non-IP host is a name that is resolved at watch time.
func ParseNodeSpec(raw string) (NodeSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return NodeSpec{}, fmt.Errorf("empty node specification")
	}
	host, port, hasPort, err := splitSpec(raw)
	if err != nil {
		return NodeSpec{}, err
	}
	if host == "" {
		return NodeSpec{}, fmt.Errorf("node specification %q has no host", raw)
	}
	return NodeSpec{
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
		return s, DefaultPort, false, nil // bare IPv4 or IPv6 literal
	}
	if strings.Contains(s, ":") {
		return "", "", false, fmt.Errorf("node specification %q is not host:port or a host", s)
	}
	return s, DefaultPort, false, nil
}

func validPort(p string) bool {
	n, err := net.LookupPort("tcp", p)
	return err == nil && n > 0 && n <= 65535
}

// Resolver is the DNS surface the fleet uses; a fake stands in for tests.
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// NetResolver resolves names with the default resolver.
type NetResolver struct{}

// LookupHost returns the address records behind host.
func (NetResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

// ResolvedNode is one monitored address a specification produced.
type ResolvedNode struct {
	Key  string // stable row identity: a per-node name, else the address
	Spec string // the specification that produced it
	Name string // DNS name when the row follows a name, "" for a literal address
	Addr string // host:port to dial
}

// ResolveSpecs expands every name specification to its address records and
// returns nodes in first-seen order, deduplicated on host:port. A name that
// resolves to exactly one address is keyed by its specification, so the row
// follows the name when the address changes; a name that expands to several
// addresses contributes one address-keyed row each. A specification that
// yields no address is an error naming it.
func ResolveSpecs(ctx context.Context, r Resolver, specs []NodeSpec) ([]ResolvedNode, error) {
	var out []ResolvedNode
	seen := map[string]bool{}
	add := func(spec NodeSpec, name, addr string, single bool) {
		if addr == "" || seen[addr] {
			return
		}
		seen[addr] = true
		key := addr
		if single {
			key = spec.raw
		} else {
			// A name that expands to several addresses contributes one row per
			// address, and each is named by the address it dials: the shared
			// name would label every row identically.
			name = ""
		}
		out = append(out, ResolvedNode{Key: key, Spec: spec.raw, Name: name, Addr: addr})
	}
	for _, spec := range specs {
		if !spec.isName {
			add(spec, "", spec.Addr(), true)
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

// IsLoopback reports whether addr (host:port) names a loopback host, so a
// plaintext AUTH is only silent for local fleets.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// AllLoopback reports whether every specification is a loopback address.
func AllLoopback(specs []NodeSpec) bool {
	for _, s := range specs {
		if !IsLoopback(s.Addr()) {
			return false
		}
	}
	return true
}

// BuildSpecs assembles the specifications from the node flags. addr is a
// compatibility alias, nodes is repeatable, and nodesCSV is comma-separated;
// the localhost default applies only when none of them is given.
func BuildSpecs(addr string, nodes []string, nodesCSV string) ([]NodeSpec, error) {
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
		raws = []string{DefaultAddr}
	}
	specs := make([]NodeSpec, 0, len(raws))
	for _, r := range raws {
		sp, err := ParseNodeSpec(r)
		if err != nil {
			return nil, err
		}
		specs = append(specs, sp)
	}
	return specs, nil
}
