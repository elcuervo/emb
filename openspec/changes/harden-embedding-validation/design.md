## Context

Three standalone commands compare served embeddings against an independent
reference, and each carries its own RESP reader:

| Command | Reference | Notes |
|---|---|---|
| `cmd/emb-verify` | Python `sentence-transformers` (`reference-embeddings.json`) | hand-rolled bulk-only reader, no timeout |
| `cmd/emb-multi-verify` | sequential `EMB` replies | hardcoded models/dims/port; justfile points at a file `download-model` does not write |
| `cmd/emb-verify-performance` | a second served model (fp32 baseline) | hand-rolled reader, hardcoded thresholds |

`internal/embtop` already contains a tested RESP2 client (`internal/embtop/resp.go`)
that speaks bulk, status, integer, array, error and nil replies, applies I/O
timeouts, supports AUTH/TLS, and builds with `CGO_ENABLED=0`. The verifiers do
not use it.

`internal/config` and `internal/hfhub` own the model/reference inputs the
verification depends on; `hfhub` currently has 0% statement coverage and the
only test there asserts a slice of constants.

See `proposal.md` for motivation and `specs/` for the behavior contract.

## Goals / Non-Goals

**Goals:**
- One wire client and one set of embedding/ranking helpers shared by all
  verifiers, unit-tested without a live server, model, or ONNX runtime.
- A reference artifact that cannot be silently stale, and verifiers whose
  inputs and thresholds are explicit rather than compiled in.
- A working `just verify-emb-multi`.
- Harness-visible dead-code and coverage checks.

**Non-Goals:**
- Changing embedding math, model loading, or any server behavior.
- Adding a model-backed reference run to CI (it needs a model download and a
  Python environment; it stays an explicit `just` step).
- Replacing or refactoring `emb-top`'s higher-level polling logic.

## Decisions

### D1: Extract the RESP wire client to `internal/resp`; verifiers and embtop share it

Move `Reply` and the `Client` wire methods (`Dial`, `WriteArgv`, `Flush`,
`ReadReply`, `Close`, timeouts, AUTH/TLS) out of `internal/embtop` into a new
`internal/resp`, and have `internal/embtop` type-alias or thin-wrap them so its
existing API and tests stay green. The verifiers import `internal/resp`
directly.

- *Why not write a fourth reader?* That is the current defect; four copies of
  RESP framing diverge, and only `embtop`'s has tests.
- *Why not have the verifiers import `embtop`?* It works and is smaller, but
  `embtop` is the dashboard package; coupling verification to it inverts the
  dependency and drags dashboard concepts into the verifiers.
- *Blast radius:* the move is mechanical; `internal/embtop/resp_test.go` and
  `e2e_test.go` are the regression net.

### D2: Embedding/ranking math and artifact schema in `internal/embverify`

`internal/embverify` owns: float32 little-endian decode, cosine, pairwise
cosine, ranking, nDCG@k retention, the reference-artifact struct
(`model`, `dim`, `sentences`, `embeddings`, `generator`, dependency versions,
`checksum`), and load/validate. It imports `internal/resp` for I/O. Its tests
use an in-process `net.Pipe` responder for framing and pure functions for math.

- *Alternative:* keep the math duplicated in each `main`. Rejected — the same
  divergence problem, and untestable without a server.

### D3: Reference artifact is provenance-stamped and validated on load

`generate-reference.py` records the model, dim, sentence list, a generator
version, and the installed `sentence-transformers`/`torch` versions, and writes
a sha256 over a canonical serialization of the embeddings. The verifier
recomputes the checksum and compares the recorded model/dim/sentence set
against its inputs; a mismatch is a hard error naming the field, and
`--refresh` regenerates.

- *Alternative:* regenerate every run. Rejected — it needs a multi-GB torch
  install and network on every invocation, so the artifact must be cacheable.
- *Alternative:* trust the file if present (today). Rejected — that is how a
  reference from a different model or an older sentence set produces a
  meaningless pass.

### D4: Thresholds and inputs are flags with the spec's defaults

Each verifier exposes `-addr`, `-model`/`-models`, `-dim`, `-corpus`,
`-min-cosine`, `-min-ndcg`, `-timeout`. Defaults are the values the specs
state: text reference `0.999`; fast-path/quantized retrieval `0.99` mean cosine
and `0.95` nDCG@10 retention; EMB.MULTI byte-equality (no threshold). The README
preprocessing-parity caveat is about image preprocessing and is documented
separately; it does not justify loosening the text gate.

- *Alternative:* one global tolerance. Rejected — the checks answer different
  questions (reference parity vs quantization fidelity) and already differ.

### D5: `verify-emb-multi` consumes the artifact the download step writes

The recipe downloads via `download-model` and then references the file that step
actually writes (`<dir>/model.onnx`), derives model name and dim from the
generated config instead of hardcoding `siglip2`/`768`, depends on `build`, and
polls `EMB.READY`/`PING` with a deadline instead of `sleep 3`.

- *Alternative:* change `download-model` to rename to `text_model.onnx`.
  Rejected — the filename is the repository's, and other configs assume
  `model.onnx`.

### D6: Dead-code and coverage checks are locked tools

Add `golang.org/x/tools` to `go.mod` via a `tool` directive and a `just deadcode`
target running the dead-code analysis over the module's production code
(without `-test`, so test-only helpers are not reported). Add `just cover`
producing per-package statement coverage. The dead-code check reports the real
finding this surfaced: `onnx.NewRuntimeSession` is unreachable, since
`onnx-bytes-session` moved the loader to `NewRuntimeSessionFromBytes`. Its
disposition (delete, or keep as documented API parity) is a task.

- *Alternative:* `go run golang.org/x/tools/cmd/deadcode@latest`. Rejected —
  unpinned and network-bound; the version must be in `go.sum`.

### D7: CI runs the harness unit tests; the model-backed run stays manual

CI adds the `CGO_ENABLED=0` unit tests for `internal/resp` and
`internal/embverify` (no model, no ONNX). `just verify-embeddings`,
`just verify-emb-multi`, `just deadcode`, and `just cover` remain dev-shell
steps documented in `AGENTS.md`, because they need the ONNX toolchain or a
model download.

## Risks / Trade-offs

- [Moving `embtop`'s client breaks the dashboard] → the move preserves the
  exported names and is validated by `internal/embtop`'s existing tests; do it
  as a standalone commit before touching the verifiers.
- [Regenerating the reference changes a pass/fail] → pin the generator's model
  and dependency versions in the recorded provenance, and keep the previous
  artifact until the new one validates.
- [Dead-code tool reports test-only helpers] → run it without `-test` and
  document that intentional test seams are not production dead code.
- [The Python reference still needs a large torch install] → generation stays
  an explicit opt-in step; the artifact is cached and checksum-verified, so
  normal verification does not re-install it.
- [A hard checksum can make an older artifact unusable after a generator
  change] → `--refresh` is the documented recovery, and the error names it.

## Migration Plan

1. Extract `internal/resp` and keep `embtop` green.
2. Add `internal/embverify` with tests; do not change command behavior yet.
3. Port the three verifiers onto the shared client and flag surface.
4. Stamp/validate the reference artifact; regenerate once and commit it only if
   the repository intends to track it (otherwise it stays generated locally per
   `.gitignore`).
5. Fix `just verify-emb-multi`.
6. Add `just deadcode`/`just cover`, resolve `onnx.NewRuntimeSession`, and
   document the gates.

Rollback is per-step: the verifiers keep their old readers until step 3, and
each gate is additive.

## Open Questions

- Whether `reference-embeddings.json` should be committed to the repository (a
  small, reviewable parity fixture) or stay a locally generated, gitignored
  artifact. This affects reproducibility of the manual check but not the specs
  or task breakdown.
- Whether the dead-code check belongs in CI now or only in the dev shell. The
  check itself is specified; only its CI placement is deferred.
