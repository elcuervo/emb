## Why

The only checks that compare the running server's embeddings against an
independent reference are `just verify-embeddings` (Go vs Python
sentence-transformers) and `just verify-emb-multi` (EMB.MULTI vs sequential
EMB). Today neither is trustworthy as a gate: the Python reference is generated
once, unpinned, and silently reused without validation; `just verify-emb-multi`
is broken (`download-model` writes `./models/siglip2/model.onnx` but the
generated config points at `./models/siglip2/text_model.onnx`, which does not
exist in that repo); and all three verifier commands duplicate a hand-rolled
RESP reader with no timeouts, array handling, or error handling. The
"implementation vs reference" claim is therefore unverified, and harness rot
(e.g. the dead `onnx.NewRuntimeSession`) goes unnoticed.

## What Changes

- Make the Python reference artifact reproducible and self-describing: pin the
  generator's model and dependency versions, record provenance (model, dim,
  sentence set, generator version, creation metadata), store a checksum, and
  **validate the artifact on use** — regenerate or fail loudly rather than
  silently trusting a stale file.
- Route every verifier through one shared, unit-tested RESP client that speaks
  both bulk and array replies, applies connect/read timeouts, surfaces server
  errors and nulls, and takes address, model, dimension, and corpus from flags
  instead of hardcoded constants.
- Fix `just verify-emb-multi`: consume the artifact `download-model` actually
  writes, depend on `build`, and derive the second model from the harness
  config rather than assuming `siglip2`/`text_model.onnx`.
- Make the comparison contract explicit and consistent with the documented
  preprocessing-parity tolerance (`README.md` states Go and Python pipelines are
  within tolerance, not bit-identical; `> 0.999` as an unconditional gate is
  stricter than the `≥ 0.99` class used by the other reference checks).
- Add harness hygiene: unit tests for the shared client and the comparison math
  that run with no live server and no ONNX, a `just` entry point, and a
  dead-code + coverage check for the verifier packages so unreachable code is
  caught by the harness rather than by review.

## Capabilities

### New Capabilities
- `embedding-verification-harness`: the shared RESP verification client, the
  verifier commands (`emb-verify`, `emb-multi-verify`,
  `emb-verify-performance`) built on it, and the `just`/CI gates (unit coverage,
  dead-code detection) that keep the harness itself trustworthy.

### Modified Capabilities
- `embedding-validation`: reference generation becomes pinned, provenance
  stamped, and validated; the comparison tolerance is documented; missing-model
  and missing/invalid-reference failures are specified; the verifier reads its
  inputs from configuration rather than hardcoded constants.
- `onnx-bytes-session`: the session-options parity wording no longer names the
  removed path-based `NewRuntimeSession`, which the dead-code gate found
  unreachable.

## Impact

- `cmd/emb-verify/`, `cmd/emb-verify-performance/`, `cmd/emb-multi-verify/` —
  refactored onto the shared client with flags and clear failures.
- New shared package (e.g. `internal/embverify`) holding the RESP client,
  float decoding, cosine/ranking math, and their unit tests.
- `cmd/emb-verify/generate-reference.py` — pinned, provenance-emitting
  generator; `reference-embeddings.json` gains a schema/checksum.
- `justfile` — `verify-embeddings`, `verify-emb-multi` fixed; new `deadcode`
  and `cover` targets.
- `.github/workflows/ci.yml` — a harness job for the unit-tested pieces (the
  model-backed comparison stays an explicit manual/network-gated step).
- `openspec/specs/embedding-validation/spec.md` and the new
  `openspec/specs/embedding-verification-harness/spec.md`.
- `README.md` — verification and tolerance documentation.
