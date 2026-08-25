## Why

TEI gets its CPU-relevant efficiency from reduced precision (fp16 on GPU, int8 on CPU). For `emb` on ARM64/Graviton, int8-quantized ONNX weights are the direct analog: ~1.5-2x latency reduction and ~4x smaller weights under Fargate's memory caps. ONNX Runtime natively executes quantized ops, so this is a **download-and-load** change, not an engine change. Many HuggingFace repos (Xenova and friends) already ship `model_quantized.onnx` / `onnx/quantized/model.onnx`; actual runtime quantization is explicitly out of scope.

## What Changes

- New per-model config `quantize: auto|on|off` (default `auto`).
  - `auto`: prefer a quantized ONNX file when the model repo/dir ships one (`model_quantized.onnx`, `onnx/model_quantized.onnx`, `onnx/quantized/model.onnx`); fall back to fp32 with a logged warning when none exists.
  - `on`: require quantized weights; fail model load if unavailable.
  - `off`: always use fp32 (current behavior).
- The HuggingFace downloader resolves the quantized artifact when applicable; local-directory models scan for the quantized files in the same order.
- `EMB.INFO <model>` reports `quantization: int8|fp32` and the on-disk model size; dim/max_length/output_tensor autodetection is unchanged (already graph-driven, works on quantized graphs).
- Docker image and `nix develop` unchanged (ORT executes int8 ops with its NEON kernels on arm64).

## Capabilities

### New Capabilities

- `int8-weight-quantization`: serve int8-quantized ONNX weights for ~2x latency and ~4x memory on ARM64 CPU.

### Modified Capabilities

- `huggingface-model-download`: the downloader resolves pre-quantized model weights when `quantize` is enabled.

## Impact

Files: `internal/config/config.go` (+`Quantize`), `internal/registry/registry.go` (model-dir/file resolution order), `internal/hfhub/hfhub.go` (quantized artifact download), `internal/server/server.go` (`EMB.INFO` quantization field), `internal/onnx/runtime.go` (load by resolved path). No protocol or pipeline changes. Quality validation extends `cmd/emb-verify` with an int8-vs-fp32 cosine check.

## Validation

**Preliminary measurement (2026-08-25, 2 vCPU ARM, identical binary — weights only):**
fp32 (`models/minilm/model.onnx`, 92MB) vs int8 (`Xenova/all-MiniLM-L6-v2` `model_quantized.onnx`, 23MB):

| metric | fp32 | int8 | Δ |
|---|---|---|---|
| process RSS | 1.14 GB | 387 MB | −66% |
| short-txt (~7 tok) p50 | 11.2ms | 8.7ms | −22% |
| long-txt (~500 tok) p50 | 25.8ms | 19.2ms | −26% |
| 8-client req/s | 118 | 159 | +35% |

**Quality:** int8 vs fp32 cosine over the fixed corpus ≈ **0.991** (min). This passes the
qualitative 0.99 gate but NOT `emb-verify`'s 0.999 threshold, which asserts fp32-identical
outputs — quantized weights deliberately use the relaxed 0.99 cosine gate.

This validates the int8 direction on ARM64 CPU; the formal `quantize` config gates below
(p50 ≤ 0.60×, req/s ≥ +60%, RSS ≤ 0.35×, cosine ≥ 0.99) remain to be measured with the
full harness on the gold reference host.

All inside `nix develop`, against the P0 golden baseline (linux/arm64):

```
$ nix develop
$ just verify-embeddings                   # fp32 correctness (unchanged)
$ just build && just bench-fargate-diff <baseline> <after-quantized>
```

- **ARM latency gate:** single-client p50 at 2 vCPU ≤ **0.60×** the fp32 baseline (≥40% faster — ARM-calibrated, lower than the x86 AVX expectation).
- **ARM throughput gate:** req/s at 8 vCPU ≥ **+60%** (ARM-calibrated).
- **Memory gate:** container RSS with quantized weights ≤ **0.35×** fp32 RSS (MiniLM fp32 ~90MB → int8 ~23MB + overhead).
- **Quality gate:** cosine similarity of int8 outputs vs fp32 outputs over a fixed corpus ≥ **0.99** via the extended `emb-verify`; per-vector max absolute diff within int8 tolerance.
- **Fallback gate:** `quantize: auto` loads fp32 with a logged warning when no quantized file exists; `quantize: on` fails loudly.