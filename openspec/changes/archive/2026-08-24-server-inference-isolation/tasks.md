# Tasks: Server Inference Isolation

## 1. Bound EMB.MULTI fan-out

- [x] 1.1 Replace the unbounded `go func` per pair in `handleEMBMULTI` with a bounded
  concurrency semaphore sized to the command's total model worker budget
  (`internal/server/server.go`); keep index-based result ordering, `wg.Wait`, and per-pair
  `nil` (MGET) semantics

  Bounded fan-out: `processMultiPair` runs under a worker pool of at most
  `multiPairFanOut` goroutines (default GOMAXPROCS, capped at pair count) pulling from a
  jobs channel — O(fanOut) goroutines regardless of pair count; ordering via index
  writes + `wg.Wait`; MGET `nil` and cache behavior preserved.
- [x] 1.2 Add a Go unit test: a large `EMB.MULTI` with an unknown model + mixed known/unknown
  pairs returns ordered results with `nil` for failures and does not spawn more than the
  bound of concurrent pair workers (goroutine-count assertion)

  `TestServerEMBMULTIFanOutBounded` (64 pairs, fanOut=2, counting session asserts max
  concurrent Runs ≤ 2) + `TestServerEMBMULTIMGETSemantics` (mixed known/unknown, null at
  the right index).
- [x] 1.3 (Revisit) If the pool API exposes bounded execution, evaluate routing pairs to the
  model worker pool directly instead of the semaphore; adopt only if the regression gate
  shows a real win

  `Pool.Embed` is already internally bounded (N workers round-robin, or the single
  batcher session), so routing pairs directly adds no bound the semaphore doesn't; the
  fan-out cap matches inference parallelism. Revisit via the task-4 gate if it shows a win.

## 2. Default thread isolation

- [x] 2.1 Default a model's `intra_op_threads` to `max(1, cores−2)` when unset (in the
  registry/session-option default), leaving explicit config to override
  (`internal/onnx` + registry)

  Added `defaultIntraOpThreads()` in `internal/registry`, applied in `ensurePool` when
  `cfg.IntraOpThreads <= 0`; explicit config wins; `newSessionOptions` keeps its floor of 1.
- [x] 2.2 Add unit tests: unset → `cores−2` (floor 1 on ≤2 CPUs); explicit value wins

  `TestDefaultIntraOpThreads` (registry).
- [x] 2.3 Confirm the CPU-partitioned `bench-cpu-partition.yaml` guidance still holds and
  update config docs (`intra_op_threads` now redundant when set, document the default)

  Updated `bench-cpu-partition.yaml` to document the new cores−2 default.

## 3. Compose MULTI with the batcher window

- [x] 3.1 When a model has `batching: {timeout, max_batch}` enabled, route that model's
  MULTI pairs through the existing batcher window (merge same-model texts) instead of
  always calling `Pool.Embed` directly; bounded fan-out still applies

  Already satisfied: `Pool.Embed` routes through the batcher window whenever `timeout>0`
  (see `pipeline/pool.go`); each MULTI pair already calls `Pool.Embed`, so windowed
  composition happens with no code change. Verified by test 3.2.
- [x] 3.2 Verify embeddings from windowed MULTI equal sequential `EMB` embeddings
  (multi-pair same-model case) in a Go/end-to-end test

  `TestServerMULTIBatchingMatchesSequential` (batching pool; windowed MULTI == sequential
  EMB byte-for-byte).

## 4. Storm stability gate

- [x] 4.1 Add a storm mode to `gems/emb/bench/bench.rb` (reuse `EMB_BENCH_LOAD_WORKERS`/
  `EMB_BENCH_LOAD_PAIRS` at the measured storm scale, e.g. 2 workers × 400 pairs) that
  reports the storm stability ratio

  Added a storm phase (`STORM_WORKERS`/`STORM_PAIRS`, default 2w × 400p) with its own
  `EMB_BENCH_STORM_RATIO` threshold (1.75); constant load keeps `EMB_BENCH_STABILITY_RATIO`
  (1.5).
- [x] 4.2 Run constant-load + storm gates under the CPU partition on the reference machine
  and record the ratios; confirm storm ratio ≤ 1.5 (the previously-failing mode) and
  publish numbers

  Measured under the partition: constant-load ratio **1.14 PASS** (≤ 1.5); storm ratio
  **1.61**. The bounded fan-out cut the storm ratio below the pre-change measurements
  (~1.87–1.93 default, ~2.57–2.67 old batching), but the full 2×400-pair storm cannot
  reach 1.5 under a per-command bound (parse/registry churn across concurrent commands).
  Decision (user-approved): storm SLO set to **1.75** (still well under every pre-fix
  measurement); constant load keeps 1.5. Spec/design updated to match.
- [x] 4.3 Wire the storm gate as a non-zero-exit regression check in the harness and
  `just bench-ruby` — `run_stability_gate` aborts (non-zero) if either ratio exceeds its
  threshold, and `just bench-ruby` propagates it; re-run confirms green.

## 5. Docs & validation

- [x] 5.1 Update `BENCHMARK.md`: constant-load and storm stability numbers with the
  partition layout, and the bounded-fan-out/intra_op_threads notes

  Updated the Partitioned section: constant ratio 1.13 (≤ 1.5), storm ratio 1.61
  (≤ 1.75), with a Server-isolation note (bounded fan-out cuts storm ratio from ~1.9–2.6
  to 1.61; intra_op_threads defaults to cores−2).
- [x] 5.2 Update the server README: `intra_op_threads` default (cores−2) and batcher-window
  composition with `EMB.MULTI`

  Added `intra_op_threads` (default `cores−2`) to the model-options table + a note that
  `batching` composes with `EMB.MULTI` and pair fan-out is bounded.
- [x] 5.3 `go test ./...`, `just bench-ruby` (gate green), and `openspec validate
  server-inference-isolation --type change` all pass

  go vet clean + `go test ./...` all packages ok; `just bench-ruby` gate green (constant
  1.13 / storm 1.61); rubocop 0 offenses; rspec 52/0; `openspec validate` passes.
