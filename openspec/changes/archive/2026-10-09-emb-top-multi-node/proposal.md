# Proposal

## Why

`emb-top` watches exactly one node (`-addr`). But `emb` is deployed as N interchangeable
replicas: each keeps its own LRU cache and its own snapshot file, and load balancing lives
in the client (`gems/emb` round-robins a configured `url` list). No single node's dashboard
can answer the two questions an operator actually has — *is traffic landing where I think it
is*, and *is any node degraded relative to its peers*. Every node looks locally healthy while
the fleet is unbalanced, a restarted node serves from a cold cache, or a node that no client
was ever pointed at sits at 0%, and the only current remedy is one terminal per node.

## What Changes

- `emb-top` accepts multiple nodes. `-node` is repeatable, `-nodes` takes a comma-separated
  list, and each entry is a `host:port`, a bare `host` (default port), or a **DNS name that is
  expanded to every A/AAAA record it resolves to**, so a headless Service, Service Connect name
  or Consul DNS becomes the node list without a new protocol.
- Names are re-resolved on a fixed interval and immediately after a node goes unreachable, so an
  autoscaled node joins the fleet without restarting `emb-top`, and a pod that moved is followed
  rather than stranded.
- Fleet membership is additive and honest: a node that stops resolving is **never dropped while
  it still answers** (it is marked orphaned, because clients may still be configured with it),
  and a node that is reachable but receives no traffic is labelled **idle**, never
  "underutilized" — `emb-top` cannot see which nodes the clients were given.
- A cluster view renders the fleet: one aggregate band plus one row per node in a stable
  first-seen order, each with the node's share of fleet traffic against its `1/N` expectation,
  its own verdict and chips, and a drill-down into the existing per-model dashboard.
- A fleet verdict is synthesized only from actionable cross-node signals (nodes unreachable or
  refusing auth, a node far slower than its peers, a node whose cache is cold after a restart,
  load spread that cannot be explained by the number of nodes).
- The headless modes become fleet-aware: `-once` prints one fleet line plus a section per node,
  and `-frames` streams the fleet frame, so the website's live view shows the whole cluster.
- `internal/resp` separates the dial address from the TLS server name, so a name expanded to IP
  addresses still verifies its certificate; `-tls-server-name` overrides it where needed.
- Explicit non-goals: gossip or any node-to-node protocol, service discovery for the *clients*
  (traffic still goes where `url` says), SRV records, per-client attribution, server-side
  metric additions, and fleet-wide latency percentiles (percentiles do not add across nodes).

## Capabilities

### New Capabilities

- `emb-top-cluster-view`: turns a set of node specifications (addresses and DNS names) into a
  monitored fleet — resolution and refresh, additive membership, orphan and idle semantics,
  per-node polling isolation — and renders/verdicts that fleet.

### Modified Capabilities

- `emb-top-dashboard`: the poll contract, aggregate throughput and headless modes become
  node-scoped; authentication and TLS gain a per-node dial/server-name split; the health banner
  gains a fleet-level verdict above the per-node one.

## Impact

- `cmd/emb-top` (`main.go`, `health.go`): multi-node flags, resolution, layout, fleet verdict,
  per-node drill-down; the single-node view stays the drill-down target.
- `internal/embtop`: unchanged in shape — one existing pipelined `Poll` per node per tick; the
  sampler is instantiated per node.
- `internal/resp`: `NewClient` gains a dial-address/server-name split (TLS only); no wire change.
- `openspec/specs/emb-top-dashboard`: requirements modified by the delta files in this change.
- Consumers: the website's live `-frames` view (`emb-top-capture` pipeline, `cli.emb.is/stats`)
  renders the fleet frame; CI's `-once` consumers gain per-node sections.
- No server changes, no new dependencies (stdlib `net.Resolver`), no CGo; `emb-top` stays a
  read-only RESP client.
