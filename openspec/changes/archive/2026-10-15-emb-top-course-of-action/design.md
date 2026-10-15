# Design

## Context

See `proposal.md` — Why. The facts that shape the approach (all verified in the
tree at the time of writing):

- **The phases are already measured and then thrown away.** `Pool.EmbedContext`
  takes `waitStart := time.Now()` before handing a request to a worker
  (`internal/pipeline/pool.go:258`) and the batcher does the same
  (`batcher.go:140`); the run's admission wait and its own duration are measured
  per run (`pool.go:75-82`, `batcher.go:233-235`). Each value is then folded into
  an `atomic.Int64` (`Pool.Stats` / `Batcher`, exposed as EMB.INFO's
  `embed_dispatch_wait_us`, `embed_admission_wait_us`, `embed_run_us`,
  `embed_runs`) and the local variable is discarded. `Response` carries only
  `{Embeddings, Err}` (`pipeline.go:58-61`), and `embedTexts` returns only
  `([][]byte, error)` (`internal/server/server.go:891`).
- **The event already has the total and nothing else.** `MonitorEvent` is
  `{Seq, AtUs, Model, Texts, LatencyUs, Err}` (`internal/server/monitor.go:9-17`);
  `LatencyUs` spans argument parsing through reply writing (`server.go:982-1007`),
  and its encoder writes six fields under both encodings (`server.go:1074-1121`).
- **A cache hit is a normal event with no marker.** `Cache.Get` counts the hit
  (`internal/server/cache.go:182-190`) but the request path records nothing about
  it; emb-top's percentiles therefore mix a µs hit and a second-long run in one
  distribution (`internal/embtop/stats.go:389-400`, which filters only errors).
- **EMB.INFO's cache counters are process-wide, not the model's.** The reply uses
  `s.cache.Stats()` for every model (`server.go:1348-1357`), so every model row
  in the dashboard shows the same hit rate even though the cache keeps per-model
  buckets (`cache.go:24-28`, `ByModel` in `cache.go:59-62`).
- **The dashboard's advice is a diagnosis, not an action.** `RecommendNode`
  (`internal/embtop/recommend.go:67`) returns a `Kind` + one line naming the
  counters; the `host` rule fires on any steal at all
  (`recommend.go:85`), which is why an idle node with 10 ms of stolen time
  reports `host is the constraint`.
- **The unit inconsistency in the CPU readout is being closed in the same
  series**: the gauge and the banner now read the same denominator (share of the
  node's capacity), so the delta's requirement states what the tree already does.

## Goals / Non-Goals

**Goals:**

- A tail that can be explained: which path (cache-served or ran) and which phase
  (dispatch, gate, run) the p90/p95 came from, per model and per node.
- Advice that is an action with a knob, and that names the action it rejected.
- Advice that stays quiet when the evidence is thin, including on an idle node.
- The cache read as part of the *service's* performance: the headline latency is
  every request, the split is the attribution.

**Non-Goals:**

- `emb-top` never applies anything: no config writing, no restart, no autotuning.
  It observes and names.
- No new flags; thresholds stay built-in defaults, as the existing health and
  sizing thresholds do.
- No changes to scheduling, admission, batching or cache semantics: the server
  reports what it already does.
- No per-phase histograms and no new introspection command; a percentile needs
  per-request values, which the event now carries.
- No latency SLO claim: the dashboard reports what the window measured, and a
  thin window is reported as thin.
- Per-key or per-namespace cache introspection; the model is the unit.

## Decisions

### The phases ride the response, they are not recomputed

**Decision.** `pipeline.Response` gains the request's own dispatch wait, admission
wait and run duration; `Pool.EmbedContext`/`Batcher.EmbedContext` fill the
dispatch wait, the worker and the batcher's flush fill admission and run, and the
server passes the triple into the event it already records.

**Why.** The values are measured on the request path today and discarded. The only
alternatives are reconstruction (differencing cumulative counters attributes to
one request a wait it did not pay, and cannot attribute a p95 at all) or a second
measurement (a new clock read on the hot path for a number the code already has).

**Alternative considered.** A per-phase histogram in the server plus a command to
read it — rejected: it answers "the mean dispatch wait rose", not "the requests in
the tail waited at the gate", and it needs a new command, a new encoding and a
retention policy for the same question.

### Attribution is per event because the question is about a percentile

**Decision.** The event carries the phases; the client computes per-path and
per-phase percentiles from the same bounded event window it already keeps.

**Why.** p90/p95 are per-request statistics. A mean over the window cannot tell an
operator whether the tail waited at the gate or ran long, which is exactly the
difference between raising the CPU budget and quantizing the model.

### `cached_texts` is a count, and the cache lookup is not separately timed

**Decision.** An event reports how many of its texts the cache answered
(`cached_texts`), not a boolean; the time the lookup itself took stays in the
unattributed remainder.

**Why.** `EMB` takes several texts and `EMB.MULTI` re-checks the cache after
resolving the model (`server.go:1736,1752`), so a request can be partly cached; a
boolean would misreport it. Timing each lookup would add a clock read per text to
the path that exists to be cheap, for a number the cache's own hit counters
already bound. The remainder is not noise: the delta requires the split to be
published as "the measured phases", never as "the whole latency".

### The wire change is append-only

**Decision.** RESP2 arrays keep their first six elements in the existing order and
append the four new ones; the RESP3 map gains the same four keys.

**Why.** `internal/embtop` reads events by index (`resp.go:239-257`) and the Ruby
client and the verification binaries read RESP2 positionally. Appending keeps every
existing reader correct, including one that pins the element count in its own
tests: it will notice, which is the desired failure, rather than silently
misreading a reordered field.

### EMB.INFO's cache counters become the model's

**Decision.** `cache_hits`/`cache_misses`/`cache_hit_rate`/`cache_evictions`/
`cache_entries`/`cache_max_bytes`/`cache_memory_bytes` in `EMB.INFO <model>` report
that model's entries; `EMB.STATS` keeps the aggregate. Field names and RESP array
counts are unchanged.

**Why.** A per-model row showing the process's hit rate cannot support a per-model
cache story — two models with different behaviour render identically — and the
dashboard's whole point is to attribute. The cache already keeps the buckets, so
this is a reporting fix, not new accounting.

**Alternative considered.** A new command or new keys for the per-model split —
rejected: it would leave `cache_hits` in `EMB.INFO` wrong and give every consumer
two numbers to reconcile.

### The headline metric stays every request; the split is a second series

**Decision.** The percentile band and the per-model p50/p95 keep describing all
completed requests — cache hits included — and the uncached p95 is rendered as an
additional marked level, with the cached share beside it.

**Why.** The service's latency is what clients experienced, and on a warm node
that is mostly the cache path; replacing it with the uncached path would report a
number no client ever saw. But a single blended percentile also cannot be acted
on, so the second series is what makes the first explainable. This also keeps the
fixed-column requirements intact: no column changes meaning, one level is added to
the band the layout already reserves.

### The CPU readout carries both denominators

**Decision.** The gauge and its value are the share of the node's own capacity, and
the attribution line states the CPU spent per uncached request over the window.

**Why.** Capacity share answers "is this node out of CPU", the per-run figure
answers "is this node's CPU going to the work the cache could not do". With a 77%
hit rate a small CPU number is a *result*, not an idle node — and the advice must
not read cache-served traffic as a reason to buy anything.

### The action vocabulary mirrors what the server already says

**Decision.** Actions: `host_cpu`, `cpu_budget`, `workers`, `batch_window`,
`intra_op_threads`, `cache`, `model`, `memory`, `growth`, `nodes`, `none`. The
dashboard names the knob the winning rule points at, plus the rejected runner-up.

**Why.** The server already gives the same advice in its own logs — "lower
intra_op_threads to overlap runs" when a model's threads fill the admission budget
(`internal/registry/registry.go:823`), "consider raising script_workers or
intra_op_threads" when a scripted path is CPU-saturated
(`internal/registry/sampler.go:67`). A dashboard that invented a different
vocabulary would contradict the server's own diagnosis.

**Alternative considered.** Keeping the resource kinds (`cpu`/`host`/`memory`) as
the only vocabulary — rejected: "more CPU" is not an action, and the whole point of
this change is that the difference between more CPU and more processing is
decidable from the phases.

### Every rule needs a magnitude floor, and a latency claim needs evidence

**Decision.** The host rule requires throttling or stolen time that is a material
share of the window's own wall time *and* the node to be carrying inference load;
the tail rules require a minimum number of events and runs before naming a phase,
and say "thin" instead of naming one.

**Why.** The frame that motivated this change is an idle node, no throttled
periods, `0µs throttled`, 10 ms stolen, and the verdict `host is the constraint`.
Advice that fires on noise is advice operators learn to ignore.

**Alternative considered.** A flag to tune the floors — rejected: the existing
health and sizing thresholds are built-in defaults, and a knob here would be tuned
once and forgotten.

### The headless vocabulary grows, it does not rename

**Decision.** `-once`/`-frames` keep the existing `rec=` tokens and add the action,
the reason, the cached share, the per-path percentiles and the per-phase means.

**Why.** The headless output is a documented machine interface
(`docs/operations.md`, the orchestrator samples under `website/`), and renaming a
token is a silent break for whoever parses it. Two tokens that mean different
things — the resource and the action — are worth the width.

### Decisions taken while implementing

Three things the build settled differently from the plan, recorded here because
each is a contract a reader would otherwise have to infer from the code:

- **Absent attribution is a flag, not a missing field.** The image path runs
  named-tensor sessions of its own and keeps no dispatch or run accounting, so it
  cannot report phases at all. Rather than a ragged event (some rows with ten
  fields, some with six) or zeroes that read as "the run was free", the event
  carries `attributed`: 0 means the server did not measure this request's phases.
  A reader must treat `attributed 0` as unknown, which is also exactly how an old
  server's six-field row is read.
- **A repair is checked before the phase rules, and capacity is not a repair.**
  Entries the cache dropped are work the node did not have to do, so the cache
  rule is evaluated before the layout and model rules, and a named repair
  suppresses the fleet's capacity answer. `cpu_budget` and `nodes` are *both*
  capacity, so they are still named together rather than one suppressing the
  other.
- **The panel is two lines, always.** The rejected action and the window's facts
  did not fit on one line at the widths the dashboard is read at, and truncating
  them would have made the rejected action invisible on narrow terminals. Both
  lines are always rendered and the chrome budget counts both, the same way the
  CPU plan line is always present.

## Risks / Trade-offs

- **[The ring grows by four ints per event]** → ~8192 events × 32 bytes ≈ 256 KiB
  at the bound, in a buffer whose bound and non-allocation discipline are already
  spec'd; the per-event cost is captured in the observability requirement.
- **[Attribution is partial by construction]** → the phases do not cover protocol,
  tokenization, the cache lookup or encoding, so a phase can explain most of a tail
  but never all of it. The specs require the remainder to stay unattributed rather
  than be assigned, and the advice requires dominance before it names a phase.
- **[A cache-served window makes the uncached sample small]** → on a node with a
  high hit rate the uncached p95 rests on few runs, which is exactly when the
  evidence floor must speak: "n runs" instead of a capacity recommendation.
- **[EMB.INFO's cache counters change meaning]** → any consumer comparing models'
  `cache_hits` sees different (correct) values. Called out as breaking in the
  proposal; the field names and counts are unchanged, and `EMB.STATS` still holds
  the aggregate.
- **[The recorded takes must move with the readouts]** → `emb-top-capture`
  requires a re-record when a readout changes, and CI checks the published
  artifacts: the tasks end with the take, the still frame and the docs section
  re-published together, not after.
- **[A new level in the band can disturb the fixed grid]** → the band's grid is
  required to stay a function of terminal width alone; the level is drawn inside
  the existing plot and the capture is what proves it.
- **[Old and new in either order]** → an old dashboard against a new server reads
  the first six fields and ignores the rest; a new dashboard against an old server
  renders without attribution rather than reading absent phases as zero. Both are
  stated as scenarios.

## Migration Plan

No persisted state, so there is nothing to migrate. Server first or dashboard
first both work. The sequence for the apply phase: carry the phases on the
response (pipeline, with its tests), encode and populate the event (server, with
the ring and encoding tests), scope EMB.INFO's cache counters, decode in
`internal/embtop` with per-path percentiles and the phase means, then the
dashboard's action line and split readouts, then the headless tokens, then the
docs and the re-recorded takes. Rollback is a revert; an old server and a new
dashboard degrade to today's readouts.

## Open Questions

- The floors' exact values (minimum events, minimum runs, steal as a share of
  wall time) are defaults in one place, like the existing health thresholds; they
  can move after a run on a real node without changing any spec.
- Whether the uncached p95 renders as a level inside the band or as a value beside
  it is settled by the capture (the grid must not move); the spec requires the
  number to be visible and separable, not its glyph.
- `cached_texts` versus `cache_hits` as the event field name: the event's own
  vocabulary is `texts`, so `cached_texts` reads as its fraction; renaming the
  existing `cache_hits` counter is out of scope.
