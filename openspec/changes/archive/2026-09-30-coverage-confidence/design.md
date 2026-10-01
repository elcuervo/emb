# Design

## Context

See `proposal.md` for motivation. Current state that shapes the approach:

- `just cover` runs `go test -coverprofile=... ./...`, which is package-local: each package is credited only with its own tests. Cross-package coverage is materially higher (`registry` 52.8% local vs. 70.4% measured with `-coverpkg=./...`), so the reported number misleads in both directions depending on the package.
- `ci.yml`'s `test` job runs only CGo-free packages; the CGo suites are not executed on PRs. `release.yml` runs `pipeline` + `registry` on tags with a hand-installed ONNX runtime and `libtokenizers`, but does not download the model fixture.
- The `nix develop` shell already provides the ONNX runtime, `libtokenizers`, and the toolchain; `just download-model` produces `models/minilm`, which the ONNX and registry tests use and skip on when absent.
- The CGo-dependent tests are fake-session based where possible (`recordingSession`, `fakeSession`, `newRuntimeSession`, `newNamedSession`, `newTokenizer` seams), so most new coverage needs no real model.
- Performance validation already exists: Go benchmarks (`just bench`), the Fargate-shaped harness (`bench/fargate`, ±5% req/s / ±10% p50 noise gate), and `redis-benchmark`. System validation already exists: `just all`, `just verify-embeddings`, `just verify-emb-multi`, `just validate-gems`.

## Goals / Non-Goals

**Goals:**

- An honest, enforced coverage number for production packages, with CI executing the suites it measures.
- Visible skip accounting so a green run cannot mean "the model tests did not run".
- Targeted tests for the identified untested paths, without new runtime dependencies and without real-model cost where a fake suffices.
- Explicit acceptance of the work by the existing performance and system tests.

**Non-Goals:**

- Chasing a global percentage target, or testing `cmd/*` entrypoints and the benchmark harness (documented exclusions).
- Replacing the Fargate harness or the Ruby suite; this change consumes them, it does not rebuild them.
- Adding a new coverage service, dashboard, or third-party test framework.

## Decisions

### Decision: cross-package coverage via `-coverpkg=./...`, with a ratchet floor file

`just cover` measures the whole repository with `-coverpkg=./...` and reports two totals: production (excluding `cmd/*` and `bench/*`) and whole-repository. A checked-in floors file records the production baseline per package; the gate fails a package that drops below its floor.

- Why a per-package ratchet over one global threshold: a global percentage lets one package rot while another improves and still passes; the point of this change is to protect the specific boundaries, so the floor is per package.
- Why not `-coverpkg=./internal/...` only: the site bridge (`website/repl`) is deployed production code, so it stays in the measured set; only `cmd/*` and `bench/*` are excluded.
- Cost: `-coverpkg` recompiles every package per test binary and is slower than the local profile. It is a gate target, not the fast inner loop; `just test` stays as-is.

### Decision: CI runs the CGo suites in a dedicated, path-filtered job

A new CI job runs `go test ./internal/... ./website/repl/` with the ONNX runtime and `libtokenizers` present and `just download-model` run first. It is gated on the existing `changes` classifier so docs-only and site-only diffs do not pay for it.

- Why a separate job over widening the existing `test` job: the CGo environment (ORT, `libtokenizers`, model fixture) is heavy and independent of the CGo-free packages; keeping them separate preserves the fast signal and lets the heavy job be cached/filtered.
- Environment: mirror `release.yml`'s ORT + `libtokenizers` setup, or run inside `nix develop` if the runner cost is acceptable. The implementation task picks one and documents it; either satisfies the spec.
- Fixture: `just download-model` writes `models/minilm`; caching it across runs keeps the job bounded.

### Decision: skip accounting from `go test -json`

The CI job runs the suite with `-json` and counts `"Action":"skip"` events (jq/awk), failing or annotating when the count is non-zero for a package whose fixture should be present. No new dependency (`gotestsum` etc.).

- Why not `-v | grep --- SKIP`: `-json` is stable, parseable, and already produced by the toolchain.
- The existing `-short` flag is currently inert (no test calls `testing.Short()`); this change does not rely on it.

### Decision: reuse existing fakes and the minilm fixture for the new tests

- `InferMaxLength` and the `resolveModelConfig` fallback: temp directory + `config.json`, no ONNX.
- `InferDim`/`GetInputInfo` and the detection branches: the `models/minilm` fixture, following the existing gated pattern in `internal/onnx/named_test.go` (now run in CI because the fixture is downloaded).
- `openImageResources`: inject the existing `newNamedSession` seam and assert open/close/rollback, mirroring the script-resource parity test.
- `EncodeReply` null/error: extend `TestEncodeReplyMatchesConvert` in `internal/script/cache_test.go`.
- `rolling.advance`: deterministic table test — the function takes `now time.Time`, so no clock seam is needed.
- `countingConn` 64-bit/`WriteAny` and `doubleText` inf/NaN: extend the existing table in `internal/server/counting_test.go`.

- Why not generate a tiny ONNX fixture: an extra build-time artifact and a new tool for one metadata read; the minilm fixture is already downloaded by CI for the same package's tests.
- Why not a real vision export for image resources: the seam already exists and the gated e2e tests remain for the real path; the unit test is about lifecycle, not inference.

### Decision: acceptance is the existing perf and system harnesses, run before/after

Performance acceptance: `just bench` (Go benchmarks) plus the Fargate harness diff against the recorded baseline (req/s within −5%, p50 within +10%), or `just bench-ruby` on Linux. System acceptance: `just all`, `just verify-embeddings`, `just verify-emb-multi`, `just validate-gems`.

- Why not a new perf test: the noise gate and methodology already exist and are specified; adding a parallel one would drift.
- The new tests are unit-level and deterministic, so their contribution to suite wall-clock is bounded and checkable against a recorded budget.

## Risks / Trade-offs

- [CI wall-clock grows] → path-filter the CGo job on code changes; cache the fixture and ORT; keep the fast CGo-free job separate.
- [Model-dependent tests flake in CI] → the new detection tests use the fixture only for metadata reads; lifecycle tests use fakes. Keep the existing determinism rules (no wall-clock assertions in batcher tests).
- [Floors become stale or aspirational] → floors are generated from the measured cross-package baseline at implementation time (ratchet, not a wish); lowering a floor requires a recorded justification in the change.
- [`-coverpkg` is slow and noisy in output] → confine it to the gate target; report production and whole-repo totals separately; exclude `cmd/*` and `bench/*` explicitly.
- [Skip accounting creates a hard failure on a legitimately absent optional fixture] → report by default; only treat skips as failures for packages whose fixture the job guarantees, and name the skipped tests in the output.

## Migration Plan

1. Land the measurement/gate tooling (`just cover` cross-package, floors file, gate target) and record the baseline floors.
2. Add the targeted tests and confirm the floors hold.
3. Add the CI CGo job and skip accounting; confirm it runs the suites and reports skips.
4. Run the performance harness before/after and the system tests; record the results in the change.
5. Rollback: the tooling and CI changes are additive; reverting the `justfile` and workflow diff restores the previous behavior with no production impact.

## Open Questions

- Whether the CI CGo job runs under `nix develop` or with the hand-installed ORT/`libtokenizers` of `release.yml` — decide at implementation based on runner time and cache behavior; both satisfy the specs.
- The exact per-package floors and the suite wall-clock budget — set from the measured baseline during implementation, not guessed here.
