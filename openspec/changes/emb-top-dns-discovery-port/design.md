# Design

## Context

See `proposal.md` — Why. The current shape that matters for the approach:

- `ParseNodeSpec` (`internal/embtop/node.go`) decides name-ness from the port's presence and hands
  `ResolveSpecs` a `NodeSpec{host, port, isName}`. `ResolveSpecs` already joins `spec.port` to every
  address record; only the literal branch (`spec.Addr()`) bypasses it.
- Row identity is already correct for names: a single-address name is keyed by the specification
  (so the row follows a moving record) and a multi-address name produces one address-keyed row per
  record, labelled by the address. Nothing in `fleet.applyResolved`, the 30s refresh, dedupe or the
  orphan grace depends on how the specification was spelled.
- `fleet.addNode` passes `rn.Name` as the TLS server name; `ResolvedNode.Name` is empty for
  multi-address rows because it is the row label. `emb-top-dashboard` already requires the
  handshake to verify a resolved node against its name, so an expanded fleet does not have to
  gain a requirement — it has to stop dropping the name.
- `emb-stress -target` parses through the same `ParseNodeSpec`, so one parser change reaches both
  binaries.

## Goals / Non-Goals

**Goals:**

- `-nodes emb.internal:6399` expands to one row per address record, each dialing port `6399`.
- The default invocation (`localhost:6379`) and the documented `-addr localhost:6379` keep rendering
  exactly one row.
- Discovered rows verify TLS against the name that was resolved, per the existing dashboard
  requirement.
- No new flag, and no change to the resolution, refresh, dedupe, orphan or keying machinery.

**Non-Goals:**

- Collapsing a pod's `A` and `AAAA` records into one row (records are deduped on `host:port`, and
  the tool cannot know two families are one process).
- SRV, weighted or priority-aware discovery; the cluster-view spec forbids consulting them.
- Resolving a Service Connect alias differently from any other name: it still resolves to the
  proxy in the watching task, so it stays one node.

## Decisions

### The host alone decides name-ness; the loopback alias stays literal

`isName` becomes `net.ParseIP(host) == nil && !equalFold(host, "localhost")`.

- Alternatives considered. (a) Plain `net.ParseIP(host) == nil`, relocating the default to
  `127.0.0.1:6379`: rejected — `-addr localhost:6379` is in the help text and every documented
  example, and would start showing a second row for the machine's `::1` record that usually
  refuses the connection, which the fleet verdict would call degraded. (b) A `-expand`/`-dns` flag
  to opt a port-carrying entry into expansion: rejected — the entry has no other reasonable
  reading, and a flag would leave `-nodes emb.internal:6399` silently wrong. (c) Treating
  `localhost` as a name but filtering loopback answers: rejected — a host-name check is decided at
  parse time; filtering needs a resolution and hides a real record.
- The alias predicate is the same special case `IsLoopback` already carries, so it is expressed
  once and reused there.
- Consequence: any other non-IP `host:port` becomes a name and therefore needs its records at
  startup (see Risks). That is the semantic the spec already gives names; literals stay available
  by spelling an IP.

### A resolved row carries its specification's name as the TLS name

`ResolvedNode` gains the specification's host as a separate field (`TLSName`); `addNode` passes it
to the client, and `tlsServerName` still lets `-tls-server-name` override it.

- `Name` cannot be reused: it is the row label, and a multi-address expansion must keep labelling
  rows by address or every row of the fleet reads the same.
- Literal rows leave it empty, so verification falls back to the dialed host exactly as today.
- Alternative considered: deriving the name from `ResolvedNode.Spec` — the spec string carries the
  port, so it would need the same split the resolver already performed.

### Keying and the literal branch are untouched

A single-address name stays keyed by the specification (the row follows a moving record); a
multi-address name stays keyed by address. `ResolveSpecs` continues to hand IP literals and
`localhost` through `spec.Addr()`, dialed as written, resolved by the dialer at connect time.

## Risks / Trade-offs

- **Startup now waits on DNS for a port-carrying name** → Mitigation: the wait is the existing
  bounded `WaitBudget` (30s, sized for records that appear with an autoscaling fleet), and the
  failure names the specification, the queried name and the resolver that answered. A deployment
  that wants the old lazy behaviour spells the IP.
- **Dual-stack pods double the row count** → Mitigation: documented; the alternative is guessing
  that two records are one process, which the tool cannot observe. Use one address family per
  deployment, or accept two rows.
- **TLS verification regresses for expanded rows if the name is not carried** → Mitigation: the
  `TLSName` field above; the existing `-tls-server-name` override still wins when a certificate
  names something else.
- **A name resolving to one address is now pinned to that address until the next resolve** →
  Mitigation: the refresh loop already re-resolves every 30s and immediately when a row goes
  unreachable, and the row key is the specification, so the row survives the record moving.
- **`-nodes localhost` stops producing a `::1` row** → Mitigation: it now dials the alias, and Go's
  dialer tries both loopback families, so a v6-only listener is still reached; `::1:6379` and
  `127.0.0.1:6379` remain explicit spellings.

## Migration Plan

Single-binary, flag-semantics change: no protocol, config-file or gem surface moves, and
`emb-top`/`emb-stress` are the only consumers of the parser. Document the port-carrying form in
`docs/operations.md`, and roll back by reinstalling the previous binary, which reads
`emb.internal:6399` as one literal address again.
