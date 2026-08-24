# Tasks: Server Inference Isolation

## 1. Bound EMB.MULTI fan-out

- [ ] 1.1 Replace the unbounded `go func` per pair in `handleEMBMULTI` with a bounded
  concurrency semaphore sized to the command's total model worker budget
  (`internal/server/server.go`); keep index-based result ordering, `wg.Wait`, and per-pair
  `nil` (MGET) semantics
- [ ] 1.2 Add a Go unit test: a large `EMB.MULTI` with an unknown model + mixed known/unknown
  pairs returns ordered results with `nil` for failures and does not spawn more than the
  bound of concurrent pair workers (goroutine-count assertion)
- [ ] 1.3 (Revisit) If the pool API exposes bounded execution, evaluate routing pairs to the
  model worker pool directly instead of the semaphore; adopt only if the regression gate
  shows a real win

## 2. Default thread isolation

- [ ] 2.1 Default a model's `intra_op_threads` to `max(1, cores−2)` when unset (in the
  registry/session-option default), leaving explicit config to override
  (`internal/onnx` + registry)
- [ ] 2.2 Add unit tests: unset → `cores−2` (floor 1 on ≤2 CPUs); explicit value wins
- [ ] 2.3 Confirm the CPU-partitioned `bench-cpu-partition.yaml` guidance still holds and
  update config docs (`intra_op_threads` now redundant when set, document the default)

## 3. Compose MULTI with the batcher window

- [ ] 3.1 When a model has `batching: {timeout, max_batch}` enabled, route that model's
  MULTI pairs through the existing batcher window (merge same-model texts) instead of
  always calling `Pool.Embed` directly; bounded fan-out still applies
- [ ] 3.2 Verify embeddings from windowed MULTI equal sequential `EMB` embeddings
  (multi-pair same-model case) in a Go/end-to-end test

## 4. Storm stability gate

- [ ] 4.1 Add a storm mode to `gems/emb/bench/bench.rb` (reuse `EMB_BENCH_LOAD_WORKERS`/
  `EMB_BENCH_LOAD_PAIRS` at the measured storm scale, e.g. 2 workers × 400 pairs) that
  reports the storm stability ratio
- [ ] 4.2 Run constant-load + storm gates under the CPU partition on the reference machine
  and record the ratios; confirm storm ratio ≤ 1.5 (the previously-failing mode) and
  publish numbers
- [ ] 4.3 Wire the storm gate as a non-zero-exit regression check in the harness and
  `just bench-ruby`

## 5. Docs & validation

- [ ] 5.1 Update `BENCHMARK.md`: constant-load and storm stability numbers with the
  partition layout, and the bounded-fan-out/intra_op_threads notes
- [ ] 5.2 Update the server README: `intra_op_threads` default (cores−2) and batcher-window
  composition with `EMB.MULTI`
- [ ] 5.3 `go test ./...`, `just bench-ruby` (gate green), and `openspec validate
  server-inference-isolation --type change` all pass
