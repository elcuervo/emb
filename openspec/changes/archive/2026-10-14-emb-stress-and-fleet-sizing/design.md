# Design

## Context

See `proposal.md` — Why. The constraints that shape the approach:

- **The signals are already published.** `EMB.STATS` reports `cpu_allowance`
  (+source), `cpu_budget`, `cpu_active_tokens`/`cpu_peak_tokens`,
  `cpu_admission_wait_us`/`_waiters`/`_timeouts`, `cpu_throttle_periods`/
  `_throttled_periods`/`cpu_throttle_usec`, `cpu_steal_usec`, `mem`,
  `goroutines`, and `cache_restore_limit_bytes`/`_rss_bytes`/
  `_headroom_bytes`/`_skipped_memory` (`internal/server/server.go:1370-1580`).
  Nothing server-side changes.
- **`internal/embtop` is already the shared, CGo-free poll library.** It decodes
  most of those fields into `Point`; it is missing only the throttle, steal and
  cache-restore-headroom fields.
- **The two halves this change needs are trapped in `cmd/emb-top`'s `package
  main`**: node resolution (`nodes.go`) and the health verdict
  (`health.go`). Neither can be imported by another command today.
- **The tokenizer is in-process only** (`registry.go`: `sharedTok` is
  unexported; only `ScriptResources.Tokenizer` exposes one), so a client cannot
  count tokens. `emb-stress` therefore cannot generate token-exact text.
- **`emb-top`'s verdict rules are enumerated in a spec**
  (`emb-top-dashboard:302`): connection, error ratio, p95-vs-baseline, CPU.
  Cache hit rate is explicitly informational.

## Goals / Non-Goals

**Goals:**

- One definition of a node set and one definition of "unhealthy" / "undersized",
  shared by both tools.
- A recommendation that is evidence, not opinion: every claim traceable to a
  named counter.
- `emb-stress` works against a fleet the operator did not deploy from this
  machine — no Docker, no model files, no config the server did not itself read.

**Non-Goals:**

- Open-loop arrival scheduling or an SLO hold. v1 is closed-loop (see Decisions).
- Launching servers or bounding their vCPU/memory. That is `bench/fargate`.
- Recorded, comparable evidence bundles and cross-host diffing. That is
  `performance-validation`.
- Image (`EMB.IMG`) and scripted (`EMB.EVAL`) workloads, and the `EMB.MULTI`
  batch API. v1 drives the `EMB` hot path only.
- Multiple targets in one run; `-target` takes one specification.
- Per-pod memory accounting (see Risks).

## Decisions

### Shared code moves into `internal/embtop`, not a new package

**Decision.** Lift `cmd/emb-top/nodes.go` and `cmd/emb-top/health.go` into
`internal/embtop`; both commands import them. Their tests move with them.

**Why.** `internal/embtop` is already the poll library both tools need and is
already CGo-free. A third package (`internal/fleet`) would split the node-set
notion from the poll client that iterates it.

**Alternative considered.** Duplicating the resolver in `emb-stress` — rejected:
two DNS-expansion semantics that can drift, and the spec requires the name to
expand identically for the observer and the generator.

### `emb-stress` splits into a command and a package

**Decision.** `cmd/emb-stress/` (flags, wiring, output) over
`internal/embstress/` (generator, fan-out, result).

**Why.** The generator must be unit-testable without a binary or a live server,
which is how `internal/embtop` is organized. `cmd/evalbench` is a single package
because it is ~450 lines; `emb-stress` carries a generator, a fan-out and a
report.

**Alternative considered.** A single `cmd/emb-stress` package like `evalbench` —
rejected as the surface grows past what one package keeps tidy.

### Closed-loop concurrency levels, with a dwell and a warm-up

**Decision.** Each load level fixes a worker count per endpoint; each worker
sends its next request only when the previous reply arrives. A level runs for a
dwell, and a leading portion of the dwell is discarded as warm-up.

**Why.** A closed loop needs no arrival scheduler and no in-flight cap, and it
still reaches the fleet's ceiling because the server's batcher aggregates
concurrent single requests — so the `EMB` hot path exercises batching without a
batch API. The warm-up is what makes the reported numbers steady state rather
than the first fill of the cache and the queue.

**Alternative considered.** Open-loop rate with a bounded in-flight window
(`internal/server/inference_bench_test.go`'s shape, which counts dropped pairs).
It answers "does it hold R rps at p99 < X" rather than "where is the knee", and
it needs an arrival scheduler. Deferred: it is an addition, not a correction.

### One worker pool per endpoint; no shared queue

**Decision.** Workers are pinned to an endpoint. Nothing redistributes work
between endpoints during a run.

**Why.** The spec requires per-endpoint attribution. A shared queue makes an
endpoint's share an artifact of scheduling rather than of the fleet, and it
would hide a stalled node behind its peers.

**Alternative considered.** A shared atomic index across all workers — better
spread under stragglers, worse attribution. Rejected: `emb-top` already reports
per-node *observed* share; `emb-stress` reports the *offered* share, and the two
must be comparable.

### Length classes, not token counts

**Decision.** The generator produces texts in length classes derived from the
configured `max_length`, seeded and deterministic, with `-corpus` as an escape
hatch. The result reports the client's own throughput and latency; token
throughput is not claimed by `emb-stress` at all.

**Why.** A client cannot count tokens (see Context), so any client-side token
figure would be an invention. Tokens are already reported by the server, and
`emb-top` is the tool that reads them — which is precisely the split this change
draws.

**Alternative considered.** Building `emb-stress` with CGo to reuse
`internal/tokenizer` — rejected: it would make the load generator undeployable
from the same places `emb-top` runs, for a number the fleet already publishes.

### Reconciliation uses the configuration as an expectation and the fleet as truth

**Decision.** For each (endpoint, model) pair, compare `EMB.MODELS`' reported
`{dim, status}` against the configuration. When the configuration states a
`dim`, that is the expectation. When it does not (a model whose dimension is
auto-detected at load), the endpoints are compared against each other and
disagreement is the drift. A pair that fails reconciliation is dropped from the
workload and named in the result rather than exercised.

**Why.** `resolveModelConfig` derives `dim` from the ONNX graph at load
(`registry.go:1126`), so a config file legitimately may not declare it —
comparing against a missing expectation would report false drift on every such
model, while cross-endpoint agreement still catches a node serving a different
build.

### Recommendations are a separate advisory list, not severity chips

**Decision.** A `recommendation` value rendered as a panel, produced by a pure
function over the node's `Point` series, with its own fixed thresholds. It is
not a `signal`, and it has no severity. `health()`'s existing behaviour and
thresholds are untouched.

**Why.** `signal` carries a `driving` flag into a worst-wins verdict; advice is
orthogonal to severity. A node at 96% CPU with zero errors is both `healthy` and
"add CPU", and the mockups depend on that being expressible.

**Alternative considered.** Adding a `driving: false` chip per recommendation —
rejected: several recommendations with several counters each is a panel, and the
banner's chip row is already the widest thing on screen.

### Memory headroom is reported against the server's own limit, in bytes

**Decision.** The memory recommendation quotes `cache_restore_headroom_bytes`
and its ratio to `cache_restore_limit_bytes`, and names that the limit is the
cache-restore limit. It never reports a percentage of the machine's RAM.

**Why.** The server computes the limit itself (`effectiveRestoreLimit`,
`snapshot_coordinator.go:303`); inventing a RAM denominator would make the panel
disagree with the server. This also sidesteps RSS being a per-process figure
(see Risks).

### Unbounded growth is reported as a suspicion, never as a purchase

**Decision.** An RSS rise across the window's halves at flat offered load and
flat cache occupancy renders as "unexplained growth", not as "add memory". The
halves are compared rather than every step, because RSS dips as the allocator
returns pages to the OS: a strict monotonic test misses the sawtooth a real leak
draws.

**Why.** The two causes need opposite responses: a spent cache budget is bought
away, a leak is fixed. Conflating them is how a fleet gets memory it did not
need and keeps its bug.

## Risks / Trade-offs

- **[The lift disturbs `emb-top`]** → the lifted functions are already pure and
  already have tests (`health_test.go`, `nodes_test.go`); the tests move with
  the code, and `emb-top`'s behaviour is unchanged until the panel renders.
- **[`mem` is process RSS, not pod memory]** → the panel names the limit it
  measures against (the cache-restore limit) instead of implying a RAM
  percentage; a pod running several processes or a sidecar will under-report.
- **[Advice becomes noise]** → fixed thresholds, quiet below them, and at most
  one fleet line per resource class.
- **[Length classes are not tokens]** → documented; the token-exact number is
  the server's, read by `emb-top`.
- **[A closed loop under-states a fleet's true ceiling]** → the loop's knee is
  reported as a knee, not as an SLO guarantee; the open-loop mode is the
  follow-up that turns it into one.
- **[`emb-top` reads DNS weights as equal]** (existing behaviour) → unchanged
  here; `emb-stress` splits workers evenly for the same reason, so the offered
  share and the expected share stay comparable.

## Migration Plan

No schema, wire or on-disk format changes, so there is nothing to migrate. The
sequence is: lift the two files and their tests into `internal/embtop` with
`cmd/emb-top` still passing, then add the throttle/steal/headroom fields to
`Point`, then the recommendation function and its rendering, then `emb-stress`.
Rollback is a revert; no persisted state is touched.

## Open Questions

- The numeric thresholds for the recommendation are defaults in one place, like
  the existing health thresholds; their exact values can move after a run on a
  real fleet without changing any spec.
