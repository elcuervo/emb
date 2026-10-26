# Proposal

## Why

`emb-top` decides whether a node specification is a DNS name from the *absence* of a port
(`internal/embtop/node.go`: `isName: !hasPort && net.ParseIP(host) == nil`), so discovery and a
non-default port are mutually exclusive: `-nodes emb.internal:6399` is taken as one literal
address and the fleet behind it is never expanded, while the only form that expands
(`-nodes emb.internal`) dials the default `6379`. A fleet that listens anywhere else cannot be
discovered at all.

## What Changes

- The **host alone** decides whether a specification is a name: any non-IP host, with or without an
  explicit port, is expanded to one node per address record, and every expanded node dials the
  specification's port (`6379` when it carries none). `-nodes emb.internal:6399` and
  `-nodes emb.internal` therefore differ only in the port dialed.
- The loopback alias `localhost` stays a literal address, so the built-in default
  (`localhost:6379`) and the documented `-addr localhost:6379` keep rendering one row instead of
  expanding to the machine's `::1` and `127.0.0.1` records.
- **BREAKING**: a non-IP `host:port` entry is now a name. It is resolved at startup (inside the
  existing bounded wait), its row is keyed and dialed by the resolved address, and a host that never
  resolves fails the start with the resolver's error instead of appearing as an unreachable row.
- An expanded row carries its specification's DNS name as its TLS server name, so
  `-nodes emb.internal:6399 -tls` verifies the certificate against `emb.internal` rather than the
  pod IP it dials. No spec delta: `emb-top-dashboard` already requires this ("An expanded name
  still verifies its certificate"); expanded rows currently drop the name, so this restores
  conformance.
- `docs/operations.md` documents the port-carrying form beside the existing bare-name example.
- No new flags: the port is the one the specification already carries.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `emb-top-cluster-view`: the requirement "Node specifications accept addresses and DNS names"
  changes — a DNS name is recognized by its host regardless of an explicit port, the
  specification's port is joined to every address record it resolves to, and a loopback name is a
  literal address rather than a fleet name.

## Impact

- `internal/embtop/node.go`: `ParseNodeSpec` reports a name from the host, `ResolvedNode` carries
  the per-row TLS name.
- `cmd/emb-top/fleet.go`: `addNode` passes the row's TLS name to the client.
- `cmd/emb-stress`: `-target` shares `ParseNodeSpec`, so it gains the same resolution for a
  port-carrying name without its own change.
- Tests: `internal/embtop/node_test.go`, `cmd/emb-top/fleet_test.go`.
- Docs: `docs/operations.md` fleet section.
- No wire, protocol, config-file, or gem surface changes; on a name that resolves to one address the
  only observable difference is that the row dials the resolved address instead of resolving at
  connect time.
