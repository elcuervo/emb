## Why

The largest, most architecture-independent CPU win available to `emb` — the one TEI and Infinity lean on — is CTranslate2: layer fusion, **intrinsic padding removal** (CT2 packs variable-length sequences natively, so `PadEncodings` waste disappears entirely), int8, and shared-memory compute. On ARM64/Graviton the x86 MKL/oneDNN story doesn't apply (CTranslate2 uses Ruy/OpenBLAS on aarch64), so the realistic gain is ~1.5-2x over fp32 ONNX rather than the 2-3x x86 case — still the single biggest lever left after int8-weight-quantization, especially for mixed-length traffic where padding removal compounds with the P1 budget.

`emb` already has the right seam: the `onnx.Session` interface (runtime.go) makes a new backend a drop-in implementation with no upstream pipeline changes.

## What Changes

- New `backend: auto|onnx|ctranslate2` model config (`auto` uses CTranslate2 when the model loads in CT2 format, else ONNX).
- New `internal/onnx/ctranslate2.go` implementing `onnx.Session` via CTranslate2's C API (cgo), supporting CT2-format model dirs (`model.bin`, `config.json`, vocab files) — the format shipped by repos like `michaelfeil/bge-small-en-v1.5` (including `int8` variants).
- Reuses the existing tokenizer; produces `int32` token ids + var-length batches directly (no `PadEncodings`), feeding CT2's native variable-length path.
- Pipeline changes: `processBatch` selects per-backend encode/run; async-tokenization (P2) and token-budget batching (P1) both apply; budget still bounds per-run memory.
- Build: add CTranslate2 shared library to `nix develop` and to the Docker builder/runtime stage for `aarch64` (source build or prebuilt arm64 artifact); model conversion (sentence-transformers → CT2) is a documented build-time step (`ct2-transformers-converter`), NOT a runtime path — non-goal.
- `EMB.INFO` reports `backend: onnx|ctranslate2`.

## Capabilities

### New Capabilities

- `ctranslate2-backend`: serve BERT-family embeddings through CTranslate2 for layer fusion, padding removal, and int8 CPU compute.

### Modified Capabilities

- `smart-batching`: the batch window's packed run composes with the CT2 variable-length path (no padding).

## Impact

Files: `internal/onnx/ctranslate2.go` (new), `internal/onnx/runtime.go` (backend selection), `internal/pipeline/pipeline.go` (encode/run split), `internal/config/config.go` (+`Backend`), `internal/registry/registry.go`, `flake.nix` (ct2 lib), Dockerfile (ct2 libs in builder + runtime), `cmd/emb-verify` (cosine fp32-vs-ct2), BENCHMARK.md. Larger image + build surface; protocol and RESP output unchanged.

## Validation

All inside `nix develop`, against the P0/P4 baselines (linux/arm64):

```
$ nix develop
$ just build            # builds with CTranslate2 for aarch64
$ just verify-embeddings -- -backend ctranslate2   # quality vs reference
$ just bench-fargate-diff <int8-baseline> <after-ct2>
```

- **ARM engine gate:** req/s at 8 vCPU ≥ **+50%** vs the ORT-int8 (P4) baseline and ≥ **+150%** vs the fp32 (P0) baseline.
- **Arm latency gate:** single-client p50 at 2 vCPU ≤ **0.65×** the fp32 baseline.
- **Arm memory gate:** RSS with CT2 int8 ≤ fp32 RSS (layer fusion offsets CT2 overhead).
- **Quality gate:** cosine vs the sentence-transformers reference (via extended emb-verify) ≥ **0.99**.
- **Build gate:** `nix develop` shell and Docker (aarch64) both link CTranslate2 cleanly; runtime image usable on Fargate arm64 (startup load < 30s).