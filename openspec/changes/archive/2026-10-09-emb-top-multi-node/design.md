# Design

## Context

`emb-top` today is a single-node Bubble Tea TUI over a shared hand-rolled RESP client
(`internal/resp`) with a per-node metric sampler (`internal/embtop.Sampler`). One tick issues
one pipelined round trip against one address: `EMB.MODELS`, one `EMB.INFO` per known model,
`EMB.STATS`, `MONITOR <afterSeq> <limit>`. Latency percentiles come from the node's bounded
`MONITOR` event ring, not from any server-side histogram. The verdict lives in one pure
function over one node's inputs (`cmd/emb-top/health.go:61`).

Constraints that shape this design: `emb` nodes are independent — private LRU cache, private
snapshot file, no gossip, no slots, no replication; clients round-robin a hand-written `url`
list; `emb-top` is a read-only observer built with `CGO_ENABLED=0`; and the repo keeps its
dependency list small.

See `proposal.md` for why the fleet view is needed.

## Goals / Non-Goals

**Goals**

- One `emb-top` that monitors a hand-specified set of nodes, where a node may be named by a
  DNS name that expands to several addresses.
- A fleet view that makes load distribution and cross-node divergence legible without
  inventing signals the nodes cannot provide.
- No server changes, no new dependencies, no new protocol.

**Non-Goals**

- Discovery for the clients. Traffic still goes where `url` says; this change only changes
  what `emb-top` watches.
- SRV records and DNS weights (deferred; see Open Questions).
- Per-client attribution, gossip/peer lists, server-side histograms.
- A pooled fleet-wide latency percentile (see Decision 8).

## Decisions

### 1. Resolution happens in `emb-top`, on the client side of the fence

Chosen over a node-to-node gossip protocol and over a static peer list served by the nodes
(`EMB.CLUSTER.PEERS`). Gossip would give topology and liveness but not metrics — `emb-top`
must dial every node for counters regardless — while adding outbound dialing, a membership
protocol, and a trust surface to a server whose entire security story is one optional shared
password. A static peer list costs more typing than the `-nodes` flag it replaces. DNS is the
mechanism the platform already runs (headless Service, Service Connect, Consul DNS), and the
stdlib resolver reaches it with no dependency.

Rejected: reading `EMB.CLUSTER.PEERS`; hashicorp/memberlist (one direct dependency, ~15
transitive modules, and encryption defaults off).

### 2. Membership is additive; an answering node is never dropped

A name that stops resolving does not shrink the fleet while that node still answers. Clients
may still hold it in their `url` list, so silently removing its row would corrupt the fleet
total and hide a live node. Orphaned rows persist, labelled, until the node is both unresolved
and unreachable for a grace period.

### 3. Rows are keyed by name, not address

The DNS name is a *pool* name when one name expands to several addresses, so it cannot identify
a row by itself; the address can, but it changes under pod churn. Rows are therefore keyed by
the specification that produced them plus a stable per-node detail — preferring a per-node name
(`-node emb-0.internal`) and otherwise the address — with first-seen order preserved exactly as
the existing model rows do. This keeps the established "rows never move" property (see
`emb-top-dashboard` requirements on fixed columns and stable rows) without needing a server-side
`node_name`.

### 4. Re-resolution is timer-based and context-bound

`net.Resolver.LookupHost` returns addresses only, with no TTL, so "honor the record's TTL" is
not available without pulling in `miekg/dns`. Resolution runs on a fixed 30 s tick and
immediately after a node becomes unreachable (the moment a pod has likely moved), using a
context-bound resolver call so a hung resolver cannot stall the watch. The 1 s poll interval
means a stale record for a few seconds is not operationally interesting.

### 5. One independent poll chain per node

Each node keeps its own `resp.Client`, its own `MONITOR` cursor, its own deadline and its own
goroutine, and publishes into a per-node view. A merged snapshot drives the frame; nothing waits
on a slow node. This reuses `internal/embtop.Client.Poll` unchanged — the fan-out is scheduling,
not new protocol. Rejected: sequential polling in the render loop (one hung node freezes the
whole fleet view, and a same-tick pipeline across all nodes in one connection is impossible
since each node is a separate server).

### 6. `internal/resp` separates the dial address from the TLS server name

Expanding `emb.internal` to `10.0.3.7` and passing `10.0.3.7:16379` to `NewClient` makes the
handshake verify the certificate against the IP (`resp.go` sets `ServerName` from `addr`), which
fails for every certificate carrying DNS names. The client gains a way to dial one address while
verifying another name, plus a `-tls-server-name` override for certificates that match neither.

### 7. CPU is measured against the node's own parallelism

`healthState` currently divides reported CPU by `runtime.NumCPU()` — the count of the machine
running `emb-top`. In a fleet this is wrong, and worst on a homogeneous-looking fleet of
different instance sizes. The node's own `gomaxprocs` (already in `INFO cpu`) becomes the
denominator, falling back to the local count only when a node does not report it.

### 8. Percentiles stay per node; divergence is the fleet signal

Every node's `MONITOR` events are available locally, so a pooled fleet percentile is technically
possible — and is rejected, because pooling averages a straggler into the mass that hides it.
Per-node percentiles render in the node rows, and the fleet-level latency signal is the spread
between the worst node and the fleet median.

### 9. The fleet verdict extends the existing one rather than replacing it

The per-node verdict keeps the current pure-function rules and thresholds verbatim (connection,
error ratio, p95 versus session baseline, CPU; cache informational only). A fleet verdict sits
above it, derived only from actionable cross-node signals — nodes unreachable or refusing auth,
a node's tail latency far above its peers, a node whose cache is cold after a restart, and load
spread beyond what the node count explains — with the same `no data` guard and precedence
ordering by severity. Cache *spread* never drives the fleet verdict, for the same reason a low
hit rate never drove the node verdict.

### 10. Expected share is `1/N`, and weights are ignored

The clients round-robin without consulting DNS, so a DNS weight would set an expectation the
traffic never satisfies, producing permanent phantom skew. `1/N` over the monitored nodes.

### 11. An idle node is a state, not a fault

Which nodes any client was configured with is not observable from the nodes. A monitored node
with zero traffic is therefore labelled idle and excluded from skew judgements, and the header
reports membership and traffic separately (`discovered · receiving traffic · idle · orphaned`),
so the DNS-set versus client-set gap is visible instead of misread.

### 12. The existing single-node view becomes the drill-down, unchanged

The per-model dashboard, its gauges, its grid, its fixed columns and its heat strips are scoped
to one node and remain the detail view for the selected row. The fleet view is a new top-level
alternative render, so the existing single-node layouts and their tests keep their meaning.

## Risks / Trade-offs

- [DNS set ≠ client set, so 0% is ambiguous] → explicit idle/orphaned states plus separate
  membership and traffic counts; the fleet verdict never calls an idle node unbalanced.
- [Rows keyed by address churn under autoscaling] → prefer per-node names; show the address as
  detail; keep first-seen ordering.
- [N nodes × N samplers grow memory and chart cost] → samplers stay bounded by the existing
  `-window`; only the aggregate band and node rows are drawn at fleet level; per-model charts
  render only in the drill-down.
- [A 30 s re-resolution adds a failure mode in the render loop] → resolution runs off the render
  path with a context deadline, and a resolver failure degrades to "keep the known fleet".
- [Fleet verdict noise from transient skew] → skew is computed over a rolling window rather than
  a single tick, and a single degraded node alone cannot make the fleet `critical` unless it is
  unreachable or refusing auth.
- [Per-node TLS against IPs silently weakens verification] → the dial/server-name split keeps
  verification on the name; the override is opt-in and named in the help text.

## Migration Plan

1. `-node`/`-nodes` land alongside `-addr`; `-addr` remains a compatibility alias, and the
   `localhost:6379` default applies only when no node flag is given. Existing single-node
   invocations keep working unchanged.
2. Fleet rendering is the default when more than one node resolves; a single-node run renders
   the single-node dashboard exactly as today, so screenshots, the `-frames` website view and
   the capture pipeline stay valid until the fleet is configured.
3. Rollback is `-addr`-only or `-node <one address>`; no server state, config file or snapshot
   is touched, so there is nothing to undo on the nodes.

## Open Questions

- SRV (`-srv _emb._tcp.<zone>`) is deferred: it would carry ports and priorities, but priorities
  have no consumer while the clients ignore DNS. Revisit if the clients ever resolve their own
  target list.
- Whether `-once` should gain an explicit JSON schema for fleet lines, or keep the current
  machine-readable line format extended per node. Deferrable: consumers are this repo's CI and
  the website, and both read the line format today.
- Whether a fleet of one should render the fleet chrome or fall through to the single-node
  layout verbatim. Leaning to the latter for capture stability; either choice is spec-neutral.
