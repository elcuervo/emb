# Tasks

## 1. Measurement and coverage gate

- [x] 1.1 Change `just cover` to measure cross-package coverage (`-coverpkg=./...`, atomic mode) and report the production total (excluding `cmd/*` and `bench/*`) separately from the whole-repository total. Verify: inside `nix develop`, `just cover` prints both totals and `internal/registry` reads materially above its package-local number.
- [x] 1.2 Add a checked-in floors file (per production package) generated from the measured baseline, and a `just coverage-gate` recipe that fails non-zero naming any package below its floor. Verify: the gate passes at baseline; temporarily raising a floor for one package makes it fail and names that package, its coverage, and its floor.
- [x] 1.3 Document the coverage policy (what is measured, the documented `cmd/*`/`bench/*` exclusions, how to run the gate, how floors are regenerated) in the repo docs. Verify: the documented command runs as written and produces the reported totals.

## 2. Model auto-configuration tests

- [x] 2.1 Add `InferMaxLength` tests: a valid `config.json` in a temp dir returns its `max_position_embeddings`; a missing file, malformed JSON, and a non-positive value each return the documented error. Verify: `go test ./internal/onnx/` passes and fails if the fallback/error behavior regresses.
- [x] 2.2 Add `InferDim` and `GetInputInfo` tests against the `models/minilm` fixture, following the existing gated pattern in `internal/onnx/named_test.go` (skip with a named message when the fixture is absent). Verify: `go test ./internal/onnx/` exercises both and reports the skip when the fixture is missing.
- [x] 2.3 Add `resolveModelConfig` tests with `Dim`, `MaxLength`, `OutputTensor`, and `Pooling` left unset, covering each detection path (graph output rank → pooling, graph dim, `config.json` max length) and each fallback (`MaxLength` 512, default output/pooling) when metadata is unavailable. Verify: `go test ./internal/registry/` passes; removing a detection branch fails a test.

## 3. Cached reply and image resource tests

- [x] 3.1 Extend `TestEncodeReplyMatchesConvert` (or add a sibling) with null, `false`, and error-table script results, asserting the encoded cache bytes equal what the live writer produces. Verify: `go test ./internal/script/` passes and the `replyBuffer.WriteNull`/`WriteError` paths are exercised.
- [x] 3.2 Add `openImageResources` lifecycle tests using the existing `newNamedSession` seam: successful open (session count published), every opened session closed exactly once, and rollback when a later session construction fails. Verify: `go test ./internal/registry/` passes; injecting a construction failure asserts the already-opened sessions are closed.
- [x] 3.3 Add `validateImagePairing` tests: matching dimensions pass, mismatched dimensions fail naming both dims, and a missing output tensor fails naming it. Verify: `go test ./internal/registry/` passes.

## 4. Server and sandbox edge tests

- [x] 4.1 Extend the counting table in `internal/server/counting_test.go` with `WriteInt64`, `WriteUint64`, `WriteAny` (RESP2 and RESP3) and `doubleText` with `+Inf`, `-Inf`, and `NaN`, cross-checked against the fork's encoder. Verify: `go test ./internal/server/` passes and the new cases fail if the size arithmetic regresses.
- [x] 4.2 Add `handleCLIENT` tests for the subcommand paths currently untested (unknown subcommand, arity errors, `SETINFO`). Verify: `go test ./internal/server/` passes.
- [x] 4.3 Add a deterministic table test for `rolling.advance` (first observation, `steps <= 0`, `steps >= len(buckets)` full reset, normal rotation) using explicit `now` values. Verify: `go test ./website/repl/` passes and the full-reset and rotation branches are covered.

## 5. CI execution and skip accounting

- [x] 5.1 Add a path-filtered CI job that runs the CGo-dependent suites (`./internal/...` and `./website/repl/`) with the ONNX runtime and `libtokenizers` available. Verify: a PR that breaks a test in `internal/server` (or another CGo package) fails the job; the CGo-free `test` job still runs independently.
- [x] 5.2 Make the job download the model fixture (`just download-model`) and cache it across runs, so model-dependent tests execute rather than skip. Verify: the job log shows the fixture-backed tests running (no skip for the minilm-dependent tests).
- [x] 5.3 Add skip accounting: run the suite with `-json` and report the skipped-test count and names. Verify: when a fixture is deliberately absent, the job reports the skips (or fails for the guaranteed-fixture packages) instead of presenting a clean run.
- [x] 5.4 Record the suite wall-clock budget for the CI job. Verify: the job reports its duration against the recorded budget.

## 6. Validation by performance and system tests

- [x] 6.1 Capture the performance baseline before the change and re-run after (`just bench`, plus the Fargate-shaped harness diff or `just bench-ruby` on Linux). Verify: every cell is within the documented noise gate (req/s within −5%, p50 within +10%); record the before/after numbers.
- [x] 6.2 Run the system tests against a built server: `just all` (Go + Ruby via the unified runner), `just verify-embeddings`, `just verify-emb-multi`, and `just validate-gems`. Verify: all exit zero with no modification to existing tests.
- [x] 6.3 Confirm the coverage gate passes with the new tests in place and the suite wall-clock is within budget. Verify: `just coverage-gate` exits zero; the recorded suite duration is within the budget from 5.4.
- [x] 6.4 Record the validation evidence (baseline diff, system-test results, gate output) in the change directory. Verify: the evidence files are present and referenced from the change.
