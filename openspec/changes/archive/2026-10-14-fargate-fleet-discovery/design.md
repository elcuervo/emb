# Design

## Context

See `proposal.md` — Why. What shapes the approach here:

- The failure is a startup ordering problem, not a resolution problem. The
  resolution itself works: `internal/embtop/node.go` resolves names with the
  platform resolver, and the reported line already names the resolver that
  answered and the name as queried, because `net.DNSError`'s own text is kept by
  the `%w` wrap at `node.go:129`. Nothing in `emb-top` needs to learn about
  Cloud Map, Service Connect or AWS.
- `emb-top` already tolerates a resolution failure *while watching*:
  `fleet.go:212-227` keeps the known fleet, records `refreshErr` and renders a
  `(resolve failed)` marker. The strict path is only the startup one
  (`main.go:159-164`), which exits 1 before the TUI starts. The two paths are
  the same `ResolveSpecs` call with different failure policy.
- `cmd/emb-stress/main.go:185-189` takes the same target name through the same
  `ResolveSpecs` and fails the same way, and `docs/operations.md` documents the
  two tools as one workflow on that one target.
- `openspec/specs/emb-top-cluster-view/spec.md` pins the strict startup behavior
  on purpose ("does not start presenting an empty fleet as healthy"). The
  invariant is worth keeping; the immediacy of the failure is not, and the spec
  delta in this change moves exactly that and nothing else.
- The capability already declares that thresholds are built-in defaults
  requiring no new flags, so this change adds no flag surface.

## Goals / Non-Goals

**Goals:**

- A watch started before a fleet's discovery records exist ends up monitoring
  that fleet, without a restart and without an operator loop.
- The failure that remains is the same failure, with the same text shape and
  exit code, only later.

**Non-Goals:**

- Teaching the tool any cloud-specific discovery: no AWS SDK, no Cloud Map
  client, no Service Connect endpoint awareness, no metadata lookups.
- Changing what a *resolved* name does: expansion, deduplication, refresh,
  orphan grace and the fleet verdict are untouched.
- Presenting an unresolved fleet as if it were a fleet (see D1).
- Any change to the server, its commands, or its protocol.

## Decisions

### D1. Wait for the first resolution; do not start with an empty fleet

A bounded retry on the startup path, then the existing error.

Alternatives considered and rejected:

- **Enter the TUI with zero nodes and keep re-resolving.** The runtime refresh
  path already tolerates failure, so this is superficially the smaller change,
  but it starts presenting an unresolved fleet with a header reading `0 nodes`
  and a fleet verdict that would have to be *taught* not to be a health claim.
  The empty-fleet frame branch (`fleet.go:1045`) is currently unreachable —
  `ResolveSpecs` errors before it — so the branch, its verdict and its rendering
  would all need new asserted behavior. That is more behavior to define than
  waiting, and it weakens the invariant the capability states.
- **Keep failing fast and improve the message only.** The message is already
  as informative as it can be without an AWS API call (D4), so this leaves the
  rolling-start case — the reason for the change — unaddressed.
- **Retry inside `ResolveSpecs` itself.** It would change the meaning of a
  function that callers use for one-shot resolution, including the refresh path
  that must *not* block waiting.

### D2. One shared, bounded wait in `internal/embtop`, used by both tools

`ResolveSpecs` gains a sibling that retries until every specification yields an
address or the budget expires, returning the same `[]ResolvedNode` or the same
wrapped error. `cmd/emb-top`'s startup and `cmd/emb-stress`'s target expansion
both call it; `emb-top`'s in-watch refresh keeps calling `ResolveSpecs`, since a
failure there is already tolerated and must never block the frame.

Rationale: the package already owns `ResolveSpecs` and the `Resolver` interface,
so the wait is testable with the fakes `node_test.go` and `fleet_test.go`
already use, and one call site change per tool keeps a single behavior for the
one workflow the docs describe. The alternative — waiting in `cmd/emb-top` and
leaving `emb-stress` fatal — would make the two halves of the documented pairing
disagree about the same name.

Both call sites pass the production `NetResolver`; the wait takes the resolver
as a parameter so tests drive it, matching the existing seam.

### D3. All-or-nothing, with a built-in budget

The wait ends the moment *every* specification resolves; it does not proceed
with a partial fleet. A mixed fleet whose dead name outlives the budget is still
the error it is today, so "an unresolved fleet is never presented" survives
without qualification.

Budget: the existing `resolveInterval` (30s) as the total, attempts no more
frequently than once a second, each attempt bounded by the existing
`resolveTimeout` (5s) — the two constants the package already uses for "how
stale is tolerable" and "how long one resolution may take". A name that does not
exist costs one round trip per attempt and fails at the budget; a resolver that
never answers costs one attempt per 5s. No new flag: the capability's built-in
defaults stance applies, and an operator who disagrees can start the tool after
the fleet.

Ordering within the attempt is unchanged: specs are attempted in the order
given, deduplicated on `host:port`, first-seen order preserved.

### D4. The error text is pinned, not rewritten

`node.go:129`'s `fmt.Errorf("node %q: resolve %s: %w", …)` already produces
`node "emb.internal": resolve emb.internal: lookup emb.internal on 10.0.0.2:53:
no such host` — specification, queried name, resolver address and Go's
classification, in one line, with no wrapper of our own to keep in sync. The
change keeps the `%w`, so the `*net.DNSError` stays reachable with `errors.As`
for a caller that ever wants the resolver address as a field, and the docs carry
the `dig` that interprets it. Rewriting the message to add advice was rejected:
it duplicates what the docs say, and any hand-rolled text loses the resolver
address that `net.DNSError` supplies.

### D5. The docs state the prerequisites where the name is introduced

`docs/operations.md`'s monitoring section (inside the existing operations
section, no new top-level section, per the product-docs requirement) gains, next
to the `-nodes <name>` examples, what makes a name resolvable inside a VPC and
the one query that tells the two NXDOMAIN cases apart. The table shapes
themselves belong there rather than in the tool, because they are properties of
the operator's discovery, not of the client.

## Risks / Trade-offs

- **A wrong name now costs the budget before failing** (a script that failed in
  milliseconds waits up to 30s) → bounded, one constant, and the exit code and
  message are unchanged; the wait only ever delays a failure, never hides it.
- **A wait that swallows the error** (a retry loop that reports success on an
  empty result) → the wait returns nodes only when every spec produced one, and
  returns the last resolution error otherwise; both outcomes are asserted with
  fake resolvers.
- **Flaky tests from wall-clock assertions** → the wait takes its budget as a
  parameter and tests use a tiny one, asserting call counts and the returned
  error, never elapsed time (the rule this repo already applies to the batcher
  tests).
- **`emb-stress` changes behavior for a load run started before its fleet** →
  same budget, and a name that never appears still exits 1; the run's own report
  is unchanged.
- **A second exported helper in `internal/embtop`** → it has two production
  callers (`cmd/emb-top`, `cmd/emb-stress`), so `just deadcode` has nothing to
  whitelist.
- **An operator who relied on the fast failure to detect a typo** → they still
  get the error, with the resolver address that says whether the name or the
  zone is the problem; the docs now name the `dig` that settles it.

## Migration Plan

No persisted state, configuration or protocol is involved, so there is nothing
to migrate: the tool waits before it speaks, and the address, refresh, orphan
and verdict behaviors are untouched.

- Ship as an ordinary change: `just test`, `just lint`, `just build`, then the
  release path that already carries `emb-top` (per-platform tarballs and the
  `emb-server` gem's `emb-top-binary-*`).
- Rollback is reverting the commit: the flag surface is unchanged, so a new and
  an old `emb-top` are interchangeable against the same nodes, and a rollback
  restores fail-fast startup with no other difference.
- The matrix change this change does not require: no version bump beyond the
  normal release, since no user-facing interface changes.
