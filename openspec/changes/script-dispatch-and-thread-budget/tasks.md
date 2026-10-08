## 1. Baseline

- [ ] 1.1 Commit `cmd/evalbench` (RESP load client: corpus, script, ARGV, concurrency list, N). Output: p50/p90/p99, req/s per concurrency level, and an optional reply dump.
- [ ] 1.2 Add `just bench-script BASE=<bin> CAND=<bin>`. It runs interleaved samples for both binaries and fails on any reply difference.
- [ ] 1.3 Record a baseline with the 0.4.3 release on the GLiNER2 corpus (36 texts, `ent_labels`, reply cache off). Cover layouts 4×2, 8×1 and 4×unset; serial and c=4/8/16. Do the same for siglip2 unbatched at c=8/16.

## 2. Script session dispatch

- [ ] 2.1 Add a test showing a request is served by an idle session while another session is held busy. Under round-robin it must currently block.
- [ ] 2.2 Add `idle` channel dispatch to `ScriptResources` (`RunNamed`), and switch `evalScripted` to it.
- [ ] 2.3 Run `go test -race ./internal/registry ./internal/server`. Run the benchmark against the baseline: replies byte-identical; c=8 p99 ≤ 110 ms at 4×2; serial p50 within ±5%.

## 3. Shared sessions and spinning

- [ ] 3.1 Add a race test: concurrent `RunNamed` calls on one `NamedRuntimeSession` with different sequence lengths return the same outputs as serial calls.
- [ ] 3.2 Remove the run mutex and the shape-keyed output cache from `NamedRuntimeSession`; allocate outputs per call.
- [ ] 3.3 Add `script_callers_per_session` (idle channel holds each session N times) and the `allow_spinning` model option, with config tests and docs.
- [ ] 3.4 Benchmark 2×4, 4 callers, spinning off against the baseline: replies identical; serial p50 ≤ 20 ms; c=4 p50 ≤ 28 ms; ≥ 150 req/s.
- [ ] 3.5 Re-run 4×2, 8×1 and the 2×4 shared layout on Fargate (`bench/fargate`), and record the recommended production layout in `docs/configuration.md`.

## 4. Thread budget

- [ ] 4.1 Add tests: unset `intra_op_threads` with `script_workers: 4` yields `max(1, (cores−2)/4)`; an explicit value is unchanged; the boot warning fires when `Σ sessions × threads > GOMAXPROCS`.
- [ ] 4.2 Implement the default and the warning. Update `docs/configuration.md` with the `workers × threads ≈ cores` rule and the measured layout table.
- [ ] 4.3 Benchmark: 4×unset reaches ≥ 4× the baseline req/s at c=8.

## 5. Embedding and image pools

- [ ] 5.1 Replace per-worker round-robin with a shared request channel in the unbatched `pipeline.Pool`. Apply the same idle dispatch as 2.2 in `registry/image.go`.
- [ ] 5.2 Benchmark siglip2 unbatched at c=8/16 against the baseline. Keep the change only if p99 improves and serial is within ±5%; otherwise revert and record the numbers here.

## 6. Observability

- [ ] 6.1 Add per-model `dispatch_wait_us`, `run_us`, `runs`, `sessions_busy`, `sessions_total` (split into script and embed) to `EMB.STATS` and `EMB.INFO`. Add tests that the RESP array count still matches.
- [ ] 6.2 Add a test that waits are ≈ 0 for serial traffic and > 0 when concurrency exceeds the session count.
- [ ] 6.3 Document the fields in `docs/operations.md`, including how to derive average wait and run time from two reads.

## 7. Validation

- [ ] 7.1 Run `go test ./... -count=1`, `just lint`, and `openspec validate script-dispatch-and-thread-budget --strict`.
- [ ] 7.2 Update `BENCHMARK.md` with before/after tables from 1.3, 2.3, 3.4, 3.5, 4.3 and 5.2.
- [ ] 7.3 After release, compare in Datadog: `phrase.entities` `backend:remote` vs `local` p50/p90/p99; `phrase.embeddings` p90/p99 at peak hours against the week before; emb `dispatch_wait_us` per model.
