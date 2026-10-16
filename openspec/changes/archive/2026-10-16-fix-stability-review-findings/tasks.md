# Tasks

## 1. Gate-wait attribution (specs: server-stats-observability, inference-capacity-autotune)

- [x] 1.1 Stop stashing the admission wait on the session: have the admission-aware wrappers in `internal/registry/cpu_budget.go` return the wait to their caller with the result, and delete the per-session `pending` field — verify with `go test ./internal/registry/ ./internal/permit/ -count=1` and by grepping that no wait-bearing field remains on the session types
- [x] 1.2 Sum the wait per run in `internal/pipeline/pool.go` (inside the per-text loop) and keep the `run < 0` clamp at both summing sites, matching `pipeline/batcher.go` — verify `go test ./internal/pipeline/ ./internal/server/ -count=1`
- [x] 1.3 Add a concurrency attribution test (gated fakes and channels, no `time.Sleep`, the `batcher_budget_test.go` pattern): concurrent callers on one shared session, asserting `run_us` and `admission_wait_us` never decrease across reads and each caller's wait is attributed to that caller — verify the test fails against the pre-fix tree and passes after 1.1–1.2
- [x] 1.4 Make `permit.Classify` refuse to classify a window with a zero or negative per-run time as `latency` — verify a unit test in `internal/permit` feeds the inverted window and asserts the class is not `latency` and the allowance is unchanged
- [x] 1.5 Document the per-run rule where `admit_us`/`run_us` are described (`docs/commands.md`) — verify the documented wording matches the emitted behavior for a multi-text request

## 2. Named-run lifetimes (spec: script-resource-lifecycle)

- [x] 2.1 Register the input-value destroy `defer` before the first value is created in `internal/onnx/named.go` — verify a table test that fails on an unknown, duplicated and invalid input *after* a valid one, and asserts the created values were destroyed
- [x] 2.2 Clear the reused output tensor before re-creating it in `internal/onnx/runtime.go` — verify an injected creation failure leaves the next run re-creating the tensor rather than reading the destroyed one
- [x] 2.3 Add the input-assembly-failure arm to the scripted leak gate (in-process heap/goroutine probe, repeated failing runs) — verify it fails on the pre-fix tree and passes after 2.1, and that its tolerance is documented next to the existing probe

## 3. Image batching (spec: image-embeddings)

- [x] 3.1 Reject an output whose batch dimension disagrees with the request's, at the pooling entry points, with an error naming both dimensions — verify a table test over short and long output batches returns an error and never panics
- [x] 3.2 State the refusal in the image command's documented error list if that list names per-image failures — verify the documented error text matches the returned one

## 4. emb-top (specs: emb-top-dashboard, emb-top-cluster-view)

- [x] 4.1 Validate `-window >= 1` in `cmd/emb-top`'s flag parsing, before any mode takes the screen — verify `emb-top -frames -samples 4 -window -1` exits non-zero with a message naming `-window` (it currently panics in `pushLat`) and `-window 1` still samples
- [x] 4.2 Build the fleet aggregate's record input with the same helper `internal/embtop/once.go` uses, including the node's advice — verify a test where every node's headroom is small and one node's advice is a cheap repair: the fleet line names that repair, and `-once` and `-frames` agree
- [x] 4.3 Read `-once`'s cache share from the same window function the panel renders — verify a test where the ring and the window disagree: both surfaces report the window's value

## 5. Gate evidence (specs: performance-validation, coverage-gate)

- [x] 5.1 Emit one `emb-smoke` summary per arm, naming the arm it summarizes and reporting skipped alongside pass/fail — verify `EMB_BENCH_SMOKE=1 go test ./internal/server/ -run Smoke -v` prints two attributed summaries and no assertion that did not run is counted as passed
- [x] 5.2 Make the CI check read the autotune arm's own summary and fail when that arm produced none — verify by running the check's command with the arm excluded (`-run` selecting only the other arm) and observing the check fail
- [x] 5.3 Regenerate `bench/smoke/mutants/*.patch` against `HEAD` — verify `bash bench/smoke/verify-gate.sh` reports every mutant caught, names its assertion, and ends with the all-clear (it currently fails on `03-cache-hits-count-as-traffic.patch` with "the mutant patch did not apply")
- [x] 5.4 Add the sweep as a path-filtered required check on the paths it guards (the gate, admission, autotune, scripted runtime) — verify the job runs `bench/smoke/verify-gate.sh` and that the same command passes locally on the final tree
- [x] 5.5 Add the race-instrumented pass over the CGo packages to `test-cgo` and give the two laya episode tests the test-only deadline seam so the race pass is not decided by a wall-clock bound — verify `go test -race ./internal/server/ ./internal/registry/ ./internal/pipeline/ ./internal/onnx/ ./internal/script/ ./internal/permit/ -count=1` is green (measured ~229 s here) and that the job's wall-clock budget is raised accordingly

## 6. Spec and doc resync (specs: emb-info-array-count, emb-cmds, model-lifecycle)

- [x] 6.1 Stop asserting a literal element total for `EMB.INFO`: compare the declared count against the elements parsed, in the no-cache and cache cases, and keep the key groups named — verify the test passes with the current 90/104 elements and still passes after adding a field
- [x] 6.2 Assert `attributed` in both `MONITOR` shapes (11 elements under RESP2, 11 keys under RESP3) and in `docs/commands.md` — verify `go test ./internal/server/ -run Monitor -count=1` and that a positional read of the first six fields is unaffected
- [x] 6.3 Correct the lazy-loading prose so it matches `docs/operations.md`: `README.md` and `docs/configuration.md`'s `preload` row state that a cold lazy model refuses the first request with a not-ready error instead of loading on it — verify by grepping for the old promise and by reading the three documents side by side
- [x] 6.4 Correct `docs/scripting.md` on `emb.API_VERSION` (the server version, not a separate host-surface version) and confirm the website reference still derives its value from `VERSION` — verify `python3 website/tools/stamp-version.py --check` and `python3 website/tools/stamp-presets.py --check` stay green

## 7. Integration

- [x] 7.1 Run the project's full sweep on the final tree — `just test`, `just lint`, `just deadcode`, `just cover`, `just verify-harness`, `EMB_BENCH_SMOKE=1 just bench-smoke` — verify each is green and that the coverage floors still hold
- [x] 7.2 Re-run `bash bench/smoke/verify-gate.sh` and the smoke gate on the final tree — verify the sweep is still all-clear after the accounting change, since the gate asserts attribution
- [x] 7.3 `openspec validate fix-stability-review-findings --strict` — verify the change validates (the only tolerated warning is a requirement-text length warning)

## Workflow follow-up

- Sync the deltas into `openspec/specs/` and archive the change once the release's review is done.
- Note in the release's upgrade section that a cold lazy model refuses its first requests and that `emb-top -window < 1` is now rejected, since both are visible to someone upgrading from 0.4.2.
