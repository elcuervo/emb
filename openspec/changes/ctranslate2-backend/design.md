## Context

ONNX Runtime is `emb`'s only engine. Its `processBatch` pads every window to the longest sequence (`PadEncodings`), so mixed-length traffic wastes GEMM work — the exact problem TEI/Infinity solve with CTranslate2's native variable-length path plus layer fusion. On Graviton (aarch64) CTranslate2 uses Ruy/OpenBLAS (oneDNN/MKL are x86-only), so we calibrate expectations to ~1.5-2x fp32 rather than the 2-3x x86 case, but the padding-removal benefit is architecture-independent and compounds with P1's budget. The `onnx.Session` interface (runtime.go) is the drop-in seam.

## Goals / Non-Goals

**Goals:**
- CT2 `Session` implementation behind `onnx.Session` (cgo, C API)
- `backend: auto|onnx|ctranslate2` config with format detection
- Variable-length runs without padding; composes with P1 budget and P2 async tokenization
- Buildable in `nix develop` and the Docker image for aarch64

**Non-Goals:**
- Runtime conversion sentence-transformers→CT2 (build-time/`ct2-transformers-converter` only; documented)
- Supporting non-BERT architectures in CT2 (falls back to ONNX)
- GPU/accelerator support (CPU-only Fargate)
- Replacing ONNX; backends coexist per model

## Decisions

### CTranslate2 via its C API through cgo
CTranslate2 exposes a C API (`include/ctranslate2/translator.h`) usable from Go exactly as `onnxruntime_go` is used today. The Go `Session` wraps a CT2 encoder model handle; `Run` passes `int32` token id batches + lengths and receives pooled-layer output for mean-pooling/normalization in Go (reuses existing `MeanPoolAndNormalize`).

### Padding removal is the core win — keep the budget but drop PadEncodings for CT2
CT2 packs variable-length sequences per run. `processBatch` branches: ONNX path keeps `PadEncodings`; CT2 path packs `[]int32` + lengths per sequence and lets CT2 handle alignment internally. P1's token budget still bounds per-run memory (CT2 materializes the batch internally), so the two compose.

### Int8 default for CT2 where shipped
CT2 models are commonly pre-converted already int8 (e.g. `michaelfeil/bge-small-en-v1.5` `ct2_int8`). `backend: ctranslate2` prefers the available int8 variant; `EMB.INFO` reports it via the same `quantization` field.

### Build: nix dev shell + Docker, aarch64
- `flake.nix`: add `ctranslate2` (or a derivation building the arm64 lib) to devShell buildInputs + CGO flags.
- Dockerfile builder: fetch/build CTranslate2 aarch64 shared lib; runtime stage copies it alongside `libonnxruntime.so`.
- Model conversion is NOT executed by the server; `just convert-model` (wraps `ct2-transformers-converter`) is the documented path to produce CT2 model dirs.

## Risks / Trade-offs

- [Build complexity of CT2 for aarch64] → nix derivation or prebuilt arm64 artifact pinned in flake inputs; Docker build pinned to VTAG. Mitigation: keep ONNX backend as default fallback so the build can land incrementally.
- [CT2 coverage limited to BERT-family] → all current reference models (MiniLM, bge) are BERT-family; non-supported architectures auto-fall back to ONNX via `backend: auto`.
- [Two engines inflate image size] → runtime image grows by CT2 libs (~20-40MB); acceptable under Fargate image-size limits; measured in the image gate.
- [Amortized-vs-latency tradeoff] → CT2 int8 may increase first-call load time; mitigated with `preload: true` guidance + startup-load gate (<30s).
- [Pooling equivalence risk] → CT2 exposes per-token hidden states for the same mean-pooling math; quality gate (cosine ≥ 0.99) is the backstop vs silent drift.

## Migration Plan

1. Add `Backend` config + CT2 lib to nix shell; prove a smoke `Session` in tests.
2. Implement CT2 `Session`; branch `processBatch` encode/run per backend.
3. Docker builder/runtime CT2 libs for aarch64; document conversion via `just convert-model`.
4. Extend `emb-verify` with a `-backend ctranslate2` mode.
5. Run ARM gates vs P0 (fp32) and P4 (int8) baselines; flip `auto` default recommendation in config docs.

## Open Questions

- Should CTranslate2 be built from source in nix/Docker or pulled as a pinned prebuilt arm64 artifact? (Recommendation: pinned artifact for Docker, nix derivation from source for the dev shell — matches how ORT/libtokenizers are handled today.)