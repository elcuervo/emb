## 1. Baseline

- [x] 1.1 Commit `cmd/evalbench` (RESP load client: corpus, script, ARGV, concurrency list, N). Output: p50/p90/p99, req/s per concurrency level, and an optional reply dump.
- [x] 1.2 Add `just bench-script BASE=<bin> CAND=<bin>`. It runs interleaved samples for both binaries and fails on any reply difference.
- [x] 1.3 Record a baseline with the previous (pre-change) binary on the GLiNER2 corpus (36 texts, `ent_labels`, reply cache off). Cover layouts 4×2, 8×1 and 4×unset; serial and c=4/8/16. Do the same for siglip2 unbatched at c=8/16. Baseline recorded in `BENCHMARK.md` (pre-change `HEAD` binary, 5-label set `PERSON ORG PRODUCT LOCATION EVENT`).

## 2. Script session dispatch

- [x] 2.1 Add a test showing a request is served by an idle session while another session is held busy. Under round-robin it must currently block.
- [x] 2.2 Add `idle` channel dispatch to `ScriptResources` (`RunNamed`), and switch `evalScripted` to it.
- [x] 2.3 Run `go test -race ./internal/registry ./internal/server`. Run the benchmark against the baseline: replies byte-identical; c=8 p99 ≤ 110 ms at 4×2; serial p50 within ±5%. Race clean; 4×2 c=8 p99 97.9 ms, serial p50 +0.2%, replies identical.

## 3. Shared sessions and spinning

- [x] 3.1 Add a race test: concurrent `RunNamed` calls on one `NamedRuntimeSession` with different sequence lengths return the same outputs as serial calls.
- [x] 3.2 Remove the run mutex and the shape-keyed output cache from `NamedRuntimeSession`; allocate outputs per call.
- [x] 3.3 Add `script_callers_per_session` (idle channel holds each session N times; default 4) and the `allow_spinning` model option (default off while a session is shared), with config tests and docs.
- [x] 3.4 Benchmark 2×4, 4 callers, spinning off against the baseline: replies identical; serial p50 ≤ 20 ms; c=4 p50 ≤ 28 ms; ≥ 150 req/s. Verify an unset config (`script_workers: 2`, everything else default) reaches the same layout out of the box. Replies identical; serial p50 15.1 ms; c=4 p50 29.0 ms; c=8 145.5 req/s (c=16 145.6).

## 4. Thread budget

- [x] 4.1 Add tests: unset `intra_op_threads` with `script_workers: 4` yields `max(1, (cores−2)/4)`; an explicit value is unchanged; the boot warning fires when `Σ sessions × threads > GOMAXPROCS`.
- [x] 4.2 Implement the default and the warning. Update `docs/configuration.md` with the `workers × threads ≈ cores` rule and the measured layout table.
- [x] 4.3 Benchmark: 4×unset reaches ≥ 4× the baseline req/s at c=8. 33.4 → 149.4 req/s (4.47×).

## 5. Embedding and image pools

- [x] 5.1 Replace per-worker round-robin with a shared request channel in the unbatched `pipeline.Pool`. Apply the same idle dispatch as 2.2 in `registry/image.go`.
- [x] 5.2 Benchmark siglip2 unbatched at c=8/16 against the baseline. Keep the change only if p99 improves and serial is within ±5%; otherwise revert and record the numbers here. Kept: c=8 p99 144.0 → 117.4 ms, c=16 282.6 → 242.0 ms, serial unchanged (31.73 ms), embeddings identical.

## 6. Observability

- [x] 6.1 Add per-model `dispatch_wait_us`, `run_us`, `runs`, `sessions_busy`, `sessions_total` (split into script and embed) to `EMB.STATS` and `EMB.INFO`. Add tests that the RESP array count still matches.
- [x] 6.2 Add a test that waits are ≈ 0 for serial traffic and > 0 when concurrency exceeds the session count.
- [x] 6.3 Document the fields in `docs/operations.md`, including how to derive average wait and run time from two reads.

## 7. Validation

- [x] 7.1 Run `go test ./... -count=1`, `just lint`, and `openspec validate script-dispatch-and-thread-budget --strict`.
- [x] 7.2 Update `BENCHMARK.md` with before/after tables from 1.3, 2.3, 3.4, 4.3 and 5.2.
