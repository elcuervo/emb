## Context

`emb` always loads fp32 ONNX. On ARM64 (Graviton), ONNX Runtime's MLAS implements NEON kernel paths for quantized ops, so an int8 model typically runs ~1.5-2x faster and occupies ~4x less weight memory — the latter matters under Fargate memory caps. The Xenova-family repos the project already recommends ship quantized variants, so the change is file-resolution logic plus config, not engine work. Runtime quantization (fp32→int8 on load) is a Python/`onnxruntime.quantization` concern and is explicitly out of scope.

## Goals / Non-Goals

**Goals:**
- `quantize: auto|on|off` per model, default `auto`
- Quantized-file resolution order for local dirs and HF downloads
- Quality gate: cosine ≥ 0.99 vs fp32 over the validation corpus
- Latency/memory gates calibrated for ARM64 (conservative vs x86 AVX expectations)

**Non-Goals:**
- Runtime fp32→int8 quantization/conversion (Python tooling; document repos that ship quantized artifacts)
- Weight calibration or quantization heuristics
- Changing pooling/output semantics; changing the Docker image or dev shell

## Decisions

### Resolution order for local model dirs
`model_quantized.onnx` → `onnx/model_quantized.onnx` → `onnx/quantized/model.onnx` → `model.onnx` (fp32 fallback, warning). Autodetection (dim, max_length, output_tensor) runs on whatever file is selected — it already reads the graph, which is identical topology to the fp32 file.

### HF download resolved per quantize setting
`hfhub` gains a quantized-artifact resolve step used only when `quantize != off`; otherwise behavior is byte-for-byte the current downloader. No tokenizer/config changes.

### ARM-calibrated gates
Do NOT port the x86 expectation (~2x latency). On aarch64 the int8 MLAS speedup is real but smaller; gates are p50 ≤ 0.60× and req/s ≥ +60%, with memory ≤ 0.35× which is architecture-independent.

### Quality pinned to cosine, not bit-exactness
int8 is lossy by design. The gate is semantic: cosine ≥ 0.99 over the corpus plus a documented per-vector diff bound. `emb-verify` gains an fp32-vs-int8 comparison mode (reuses the existing reference harness).

## Risks / Trade-offs

- [int8 accuracy regressions on sensitive models] → `quantize` is per-model and opt-in via `auto` fallback; users can pin `off`.
- [Repos with only fp32 onnx] → `auto` warns and falls back; `on` fails loudly at load with a resolvable message.
- [ORT aarch64 int8 kernel coverage for exotic ops] → falls back to fp32 execution per-operator inside ORT; correctness preserved, perf gain reduced; discovered by the harness memory/latency cells.
- [Weights fetched from unknown repos] → resolution trusts the model config owner; same trust model as today's arbitrary ONNX path.

## Migration Plan

1. Add `Quantize` config + resolution helpers.
2. Wire `hfhub` quantized download; local-dir scan order.
3. Extend `EMB.INFO` with `quantization` + model size.
4. Extend `emb-verify` with int8-vs-fp32 cosine mode.
5. Run ARM harness gates; update README/config examples + BENCHMARK.md.

## Open Questions

- Should `quantize` default to `auto` (may surprise existing expect-fp32 users) or stay `off` until opt-in? (Proposal: default `auto` only for HF downloads, `off` for explicit local paths, to avoid silent behavior change on pinned models.)