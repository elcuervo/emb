# Proposal

## Why

`emb-top` says what is *wrong* — a health verdict and a sizing diagnosis naming a
resource — but never what to *do* about it, and every headline number it shows
blends two populations that need opposite responses: requests the embedding cache
answered and requests that actually ran. A warm node renders `p50 2µs · p95 1.9s`
in one row, `cpu 0%` beside it, and the diagnosis `host is the constraint` for a
node that is idle — three readouts that cannot be acted on: is the tail
inference, the CPU gate, the batch window, or one cold run? is the fix a wider
CPU budget, more workers, or a bigger cache? Are the 0.0 req/s a dead node or a
cache serving everything?

The numbers that would answer it already exist inside the process and are
discarded. `internal/pipeline` measures, per run, the wait for a worker or batch
window (`dispatch_wait_us`), the wait at the process CPU admission gate
(`admission_wait_us`) and the ORT run (`run_us`), and `internal/server` knows per
request how many texts the cache answered — but the per-request values are folded
into cumulative per-model atomics, published only as means (`EMB.INFO`), and the
MONITOR event carries only a wall-clock total. `emb-top`'s percentiles are
therefore computed over both paths at once (`latenciesLocked` filters only
errors), so the p90/p95 answers neither "is the cache fast" nor "is inference
slow"; and `EMB.INFO <model>` reports the *process-wide* cache counters for every
model rather than that model's own, so a per-model cache story is not readable at
all.

## What Changes

- **Per-request attribution becomes observable.** A `MONITOR` event gains
  `cached_texts` (how many of its texts the cache answered), `dispatch_us`,
  `admit_us` and `run_us` (the phases its *misses* paid). The phases are carried
  out of `internal/pipeline` on the response value instead of being dropped into
  atomics, and the server times the cache-answered portion where it already knows
  it. The ring stays bounded; the per-event cost is bounded ints, no clock read
  is added to the cache path.
- **`EMB.INFO <model>` reports that model's own cache counters.** The cache
  already keeps per-model hit/miss/eviction/entry/byte counters
  (`internal/server/cache.go` `byModel`); `EMB.INFO` publishes the process-wide
  ones today, which is why every model row shows the same hit rate. `EMB.STATS`
  keeps the aggregate. Field names and RESP array counts are unchanged.
- **The dashboard's advice becomes an action.** The advisory line names the
  single next step and the knob behind it — `raise cpu_budget`, `raise workers`,
  `lower intra_op_threads`, `grow the cache`, `quantize the model`, `raise the
  host's CPU entitlement`, `add nodes`, `fix the leak`, or nothing to buy — with
  the counter that decided it, and the runner-up it rejected and why. Rules gain
  magnitude floors: a claim of a starving host needs steal/throttle that is a
  measurable share of the window, and a latency claim needs enough events behind
  it, so an idle node with 10 ms of steal no longer reports "host is the
  constraint".
- **Every headline metric is read over the path split.** Latency renders the
  service percentile (all events, unchanged) *and* the uncached-path percentile,
  so the tail compute causes is separable from the tail queueing causes; the CPU
  readout is a share of the node's capacity *and* per uncached run, so a warm
  cache reads as the traffic it is rather than as idle; throughput reports the
  offered rate beside the rate that actually ran. The advice attributes the
  uncached p95 to its dominant phase.
- **The fleet view names the fleet's action**, beside the existing fleet sizing
  line, using the same vocabulary and the same per-node evidence.
- **Headless output carries the attribution**: action token and reason, the
  cached share, the per-path percentiles and the per-phase means, per node and
  per model. The existing `rec=` tokens are kept as-is, so script consumers do
  not break.

**BREAKING** — `MONITOR` events grow from six fields to ten under both encodings
(RESP2 flat arrays append after `err`, RESP3 maps gain the same four keys), and
`EMB.INFO <model>`'s `cache_*` values change from process-wide to per-model. A
consumer that pins MONITOR's element count or compares cache counters across
models is affected; `emb-top`, the Ruby client and the verification binaries index
positionally and are unaffected.

## Capabilities

### New Capabilities

None. The per-request attribution model belongs where per-model timings already
live, and the dashboard's advice capability already exists in both the node and
the fleet specs; a new capability would duplicate them.

### Modified Capabilities

- `emb-cmds`: the `MONITOR` event carries the request's cache-served text count
  and its dispatch/admission/run phases under both RESP2 and RESP3.
- `lru-cache`: `EMB.INFO <model>`'s cache counters are scoped to the requested
  model (the cache already keeps them per model); `EMB.STATS` keeps the aggregate.
- `server-stats-observability`: the per-model dispatch/run timings it already
  requires gain their per-request counterpart — attribution is observable per
  completed request, the phases never exceed the event's latency, and the ring's
  bound and the no-allocation-on-the-request-path rules hold.
- `emb-top-dashboard`: the recommendation becomes an action naming the knob and
  rejecting the runner-up, with magnitude floors and an evidence floor for
  latency claims; percentiles render per path and the CPU readout is read as a
  share of node capacity and per uncached run.
- `emb-top-cluster-view`: the fleet recommendation names the fleet's action and
  the node to change first, derived from the same per-node evidence.

## Impact

- Modified: `internal/pipeline/` (response carries the phase triple; worker and
  batcher fill it), `internal/server/` (event fields and their encoding, cache
  split per request, `EMB.INFO` cache scope, the ring's element count),
  `internal/embtop/` (decode the phases and the path, per-path percentiles,
  per-model window means), `cmd/emb-top/` (the action line, the split readouts,
  the fleet action, the headless tokens), `docs/operations.md` and
  `docs/commands.md`, and the recorded takes (`emb-top-capture` requires a
  re-record when a readout changes).
- Unchanged: the RESP surface beyond the appended event fields and the cache
  scope, the configuration schema, the cache's own semantics, and every on-disk
  format. No new flags; thresholds stay built-in defaults. The server-side cost is
  the event's four extra ints per entry in a ring that is already bounded at 8192.
