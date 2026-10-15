# Proposal

## Why

An operator running the documented fleet workflow against a Fargate cluster —
`bundle exec emb-top -node emb.internal` (`docs/operations.md:219`) — gets

```
emb-top: node "emb.internal": resolve emb.internal: lookup emb.internal on 10.0.0.2:53: no such host
```

and a non-zero exit before any dashboard renders. Two gaps produce that single
line. First, `emb-top` treats "this name is not registered yet" identically to
"this name does not exist": `main.go:159-164` resolves once, synchronously, and
exits 1, while the in-flight refresh that runs every 30s tolerates the same
failure (`fleet.go:212-227`) — so the tool survives DNS disappearing mid-watch
and dies on DNS not existing yet, which is the one case a rolling or autoscaling
fleet produces. Second, the docs present `-nodes <name>` as a bare capability
without stating what makes a name resolvable inside a VPC, and Go's `no such
host` cannot distinguish a wrong name from a namespace whose private zone is not
associated with the task's VPC.

## What Changes

- **A DNS name that is not resolvable *yet* stops being fatal at startup.**
  `emb-top` waits for the name within a built-in bounded window and reports the
  error only when it stays unresolvable, keeping the existing guarantee that an
  unresolved fleet is never presented as healthy (the requirement it replaces
  says exactly that). The wait window is a built-in default: no new flag, in
  line with this capability's existing "thresholds SHALL be built-in defaults
  requiring no new flags" stance.
- **The failure keeps naming what locates the cause.** The message already
  carries the queried name and the resolver that answered (`no such host` is
  `*net.DNSError` text, preserved by the wrap at `internal/embtop/node.go:129`),
  which is what distinguishes a wrong name from a zone this VPC cannot see. The
  wait must not swallow it, reword it, or drop the `%w` wrap; the docs point at
  the `dig` that confirms which case it is.
- **The operations docs state the Fargate prerequisites** for the fleet
  workflow, inside the existing monitoring section (no new top-level section):
  a private Cloud Map namespace's hosted zone must be associated with the task's
  VPC; per-task `A` records come from Cloud Map DNS service discovery
  (`serviceRegistries`), whereas a Service Connect client alias resolves to the
  proxy in the *same task* — which would be monitored as one node, not the
  fleet; and how to tell the two NXDOMAIN cases apart with `dig`.

Not in scope: no AWS-side change; no Service Connect client-alias discovery in
`emb-top`; no server, protocol or `EMB.*` command change; no new `emb-top` flag;
no new top-level docs section.

## Capabilities

### New Capabilities

None. Both gaps are behavior of capabilities the project already declares.

### Modified Capabilities

- `emb-top-cluster-view`: the requirement "A specification that resolves to
  nothing is an error" becomes a requirement about a name that *stays*
  unresolvable — a not-yet-registered name is waited for within a bounded window
  instead of being reported immediately, and the error names the resolver and
  the queried name. The invariant (never present an empty fleet as healthy) is
  unchanged.
- `product-docs`: the operations section requirement gains the discovery
  prerequisites for the documented `-nodes <name>` fleet workflow, within the
  section and figure provenance rules that already govern it.

## Impact

- **Code**: `cmd/emb-top/main.go` (startup resolution and its exit path),
  `internal/embtop/node.go` (`ResolveSpecs` and the resolution error), and
  `cmd/emb-top/fleet.go` if the bounded wait reuses the tolerant refresh path.
  `cmd/emb-stress/main.go:185-189` shares the same fatal startup resolution for
  `-target <name>`; whether a load run waits or still fails fast is a design
  decision this change must state, not leave implicit.
- **Tests**: `cmd/emb-top/main_test.go`, `cmd/emb-top/fleet_test.go` and
  `internal/embtop/node_test.go` already drive resolution through fake
  resolvers, so the new wait/retry path and the error text are testable the same
  way; no timing-sensitive sleeps against wall clock.
- **Docs**: `docs/operations.md` monitoring section (the `emb-top` fleet
  examples and their explanations). `just website-presets-check` is unaffected —
  no preset bytes change.
- **Operators**: a watch that previously failed fast now starts after the wait
  window; a genuinely wrong name fails with a more specific message after that
  same window. Nothing changes for addresses, for names that resolve, or for
  `-once`/`-frames` runs against an already-resolvable name.
