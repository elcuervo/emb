# Tasks

## 1. Carry the phases out of the pipeline

- [x] 1.1 Give `pipeline.Response` the request's own phase triple
      (`DispatchWait`, `AdmissionWait`, `Run`), fill the dispatch wait where it is
      already measured (`Pool.EmbedContext`, `Batcher.EmbedContext`) and the
      admission wait and run where they are already measured (`Worker.run`, the
      batcher's flush), and return it to the caller. Verify: a test drives one
      request through each path and asserts the response's phases equal the delta
      of that model's cumulative `DispatchWaitUs`/`AdmissionWaitUs`/`RunUs` over
      the same request, and that the response's run time excludes the admission
      wait.
- [x] 1.2 Keep the hot path unchanged: no new allocation and no new clock read on
      the request path (the dispatch start, the admission wait and the run
      duration are already taken). Verify: the pipeline benchmarks
      (`just bench` over `internal/pipeline/`) show no regression beyond noise,
      and `go test ./internal/pipeline/ -count=1` is green.
- [x] 1.3 Count the cache-served texts of one request where the cache already
      counts the hit (`embedTexts`, `processMultiPair`). Verify: an `EMB` with one
      cached and one uncached text reports one cached text and phases that cover
      only the uncached one.

## 2. Publish the attribution

- [x] 2.1 Add `CachedTexts`, `DispatchUs`, `AdmitUs` and `RunUs` to
      `server.MonitorEvent`, clip nothing, and keep the ring's bound (8192) and its
      eviction order. Verify: the ring's bound and eviction tests are unchanged and
      green.
- [x] 2.2 Append the four fields to `MONITOR`'s reply under both encodings — RESP2
      after `err`, RESP3 as four map keys with the same names — for every command
      that records an event (`EMB`, `EMB.MULTI`, `EMB.IMG`, `EMB.EVAL`/`EMB.EVSHA`).
      Verify: a test reads one event under each encoding, asserts the order and the
      key names, and asserts `dispatch_us + admit_us + run_us <= latency_us`.
- [x] 2.3 Scope `EMB.INFO <model>`'s cache counters to that model (hits, misses,
      rate, evictions, entries, bytes) from the cache's per-model buckets, leaving
      `EMB.STATS` on the aggregate. Verify: two models looked up different numbers
      of times report different, model-correct counters, and the `EMB.INFO`
      array-count parity tests (22 / 36) and `EMB.STATS`' (67 pairs) still pass.

## 3. Decode and split in `internal/embtop`

- [x] 3.1 Decode the four event fields, the per-model `embed_*` phases and the
      per-model cache counters into `Event`/`ModelStats`, and keep a per-model
      window of cached and uncached request counts beside the existing event
      window. Verify: a unit test feeds a mixed event stream and asserts the
      per-model counts, with a partly cached batch counted on both sides.
- [x] 3.2 Add per-path percentiles (every completed request, and the requests that
      were not fully cache-answered) through the existing nearest-rank computation,
      and per-phase means per model and per node. Verify: a test with a bimodal
      window (µs cache answers, second-long runs) asserts the uncached p95 is the
      run path's and that a fully cached window renders without an uncached p95.

## 4. Turn the recommendation into an action

- [x] 4.1 Extend the sizing rules into the action vocabulary (`host_cpu`,
      `cpu_budget`, `workers`, `batch_window`, `intra_op_threads`, `cache`,
      `model`, `memory`, `growth`, `nodes`, `none`), each naming the knob, the
      deciding counter and the rejected runner-up. Verify: table-driven tests per
      rule assert the action, the reason and the runner-up.
- [x] 4.2 Add the magnitude floors: the host rule needs throttling or stolen time
      that is a material share of the window's wall time and a node carrying
      inference load; a latency rule needs a minimum number of events and runs to
      attribute at all. Verify: the frame that motivated the change — an idle node,
      `0/0` throttled periods, `0µs` throttled, 10 ms stolen — renders no
      recommendation, and a window with a single uncached run reports it as thin.
- [x] 4.3 Order cache before capacity: a node whose tail is dominated by cache
      misses while the fleet has CPU headroom names the cache, not more CPU.
      Verify: a test asserts the cache action wins over `cpu_budget` on that
      evidence.

## 5. Render it

- [x] 5.1 Render the action line in the sizing line's place (same chrome budget,
      same one line), the cached share and the per-run CPU in the gauges row, and
      the uncached p95 with the cached share beside the band. Verify:
      `go test ./cmd/emb-top/ -count=1` is green, including the fixed-column,
      gauge-grid and label-column tests, and a frames run shows the new line
      without moving the grid.
- [x] 5.2 Put the same attribution in the headless output: the action token, its
      reason, the cached share, the per-path percentiles and the per-phase means,
      per node and per model, keeping every existing token name. Verify: a
      `-frames`/`-once` run against a live node carries the new tokens and the
      existing ones unchanged.

## 6. The fleet line

- [x] 6.1 Render the fleet's action and the first node's attributed tail, keeping
      the per-resource reporting and the "do not average" behaviour. Verify: the
      fleet tests cover different binding resources, a fallen-behind peer (named
      with its phase) and a quiet fleet (nothing rendered).

## 7. Docs and the recorded takes

- [x] 7.1 Document the attribution and the action vocabulary in
      `docs/operations.md`'s dashboard and sizing sections (no new top-level
      section) and the new `MONITOR` fields in `docs/commands.md`, including the
      cache-scope change to `EMB.INFO`. Verify: every number quoted in the prose
      comes from a recorded run, and the docs introduce no field the server does
      not emit.
- [x] 7.2 Re-record the takes so the plate, the still frame and the documented
      capture show the new readout (`just website-topviz`, and the fleet take).
      Verify: the published artifact names change, `docs/operations.md`'s
      references are rewritten by the capture, and the served set's check passes.
- [x] 7.3 Note the cache-scope change and the event's new fields in the release
      notes for the next version. Verify: the note names both breaks and the
      server/dashboard compatibility in either order.
