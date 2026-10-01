# Proposal

## Why

The measured coverage of the production packages is 85.5% (`go test -coverpkg=./... ./...`, excluding `cmd/*` and `bench/*`), but that number overstates confidence in two ways.

First, **it is local-only**. `.github/workflows/ci.yml` runs only the CGo-free packages (`config`, `hfhub`, `resp`, `embverify`, `tokenizer`, `cmd/emb-verify`, `cmd/emb-multi-verify`). The core — `internal/server`, `registry`, `pipeline`, `onnx`, `script`, `imageproc`, `embtop` (~5,150 statements) — is never executed on a pull request. The CGo suites run only in `release.yml`, on tags, and only `pipeline` + `registry`. A green PR therefore says nothing about most of the product.

Second, **several behavior-defining paths are untested and some are silently skipped**. Model auto-configuration (`InferMaxLength`, `GetInputInfo`, `InferDim`, and the detection branches of `resolveModelConfig`) is never exercised because every `LoadModel` test sets `Dim`/`MaxLength`/`OutputTensor`/`Pooling` explicitly. The cached-reply serializer's null/error encoding (`replyBuffer.WriteNull`) is never called because its tests use a different writer. The real image-session pool (`openImageResources`) runs only under fixture-gated e2e tests that `t.Skip` when `EMB_VISION_MODEL` is absent. Fixture-gated tests skip silently, so "all tests pass" is indistinguishable from "the model-dependent tests did not run".

This matters now because emb's contract is embedding bytes and lifecycle behavior; a silent auto-config misread or an untested cache-replay encoding is exactly the class of bug the suite is supposed to catch. The coverage work must be *validated* — by the existing performance harnesses (Go benchmarks, the Fargate-shaped harness, `redis-benchmark`) proving it does not regress throughput or add wall-clock, and by the system tests (`just all`, `just verify-embeddings`, `just verify-emb-multi`, `just validate-gems`) proving the product still behaves end to end.

## What Changes

- **A coverage bar with an honest measurement.** `just cover` SHALL measure cross-package coverage (`-coverpkg=./...`) and a new gate target SHALL enforce a documented floor for production packages with documented exclusions (`cmd/*`, `bench/*`), instead of the current package-local number that under-reports cross-package exercise (e.g. `registry` 52.8% local vs. 70.4% real).
- **CI runs the CGo suites.** The CGo-dependent packages SHALL be exercised on pull requests (with the ONNX runtime and the minilm fixture available), so coverage is enforced, not just reported locally.
- **Skips are accounted for.** Model/fixture-gated tests SHALL be visible in the run (reported/summarized) so a green job cannot mean "the interesting tests did not run".
- **Targeted tests for the identified gaps** (no production behavior change):
  - `internal/onnx` + `internal/registry`: model auto-configuration (`InferMaxLength`, `GetInputInfo`, `InferDim`, `resolveModelConfig` detection) driven by a real graph fixture and a `config.json`.
  - `internal/script`: `EncodeReply` null and error-table encodings (the bytes replayed from the script reply cache).
  - `internal/registry`: `openImageResources` open/close/partial-failure via the existing `newNamedSession` seam; `validateImagePairing`.
  - `website/repl`: `rolling.advance` window rotation and the `bridge` error/limit paths.
  - `internal/server`: `countingConn` 64-bit/`WriteAny` and `doubleText` inf/NaN edges; `handleCLIENT` subcommand paths.
- **Validation is part of the change.** The coverage work SHALL be accepted only when the performance tests and the system tests pass, as specified.

No production behavior changes; no new runtime dependency.

## Capabilities

### New Capabilities

- `coverage-gate`: the coverage bar (cross-package measurement, documented exclusions and floor, CI execution of the CGo suites, skip accounting) and the requirement that the bar is validated by the project's performance tests and system tests.

### Modified Capabilities

- `runtime-quality-validation`: add focused regression tests for the model auto-configuration, cached script reply encoding, and image-resource construction boundaries.

## Impact

- Tooling: `justfile` (`cover` measured cross-package; new `coverage-gate` target), `.github/workflows/ci.yml` (CGo test job with ONNX runtime + fixture), possibly `.github/workflows/release.yml` (align the suite).
- Tests: new/extended tests in `internal/onnx`, `internal/registry`, `internal/script`, `internal/server`, `website/repl`; new fixtures (a tiny ONNX graph / `config.json`) or reuse of the existing gated `models/minilm` fixture.
- Docs: a short coverage section in the repo docs (what is measured, what is excluded, how to run the gate).
- Performance: new tests are unit-level (fake sessions, temp files, no model download, no network); they SHALL stay within the suite's wall-clock budget and SHALL NOT move the benchmark baseline outside the Fargate noise gate.
- No production code, API, config, or wire-format changes.
