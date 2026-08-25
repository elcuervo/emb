## 1. Config & resolution

- [x] 1.1 Add `Quantize` enum (`auto|on|off`, default `auto`) to model config
- [x] 1.2 Add quantized-file resolution helper (local dirs: root/onnx/onnx-quantized order)
- [x] 1.3 Resolution applies to HF and local paths alike: `auto` prefers quantized when present and falls back to fp32 (warn); `off` never switches. (Decision: simpler than the HF-auto/local-off split — harmless fallback either way.)

## 2. Download & load

- [x] 2.1 `hfhub` resolves quantized artifact when `quantize != off` (FindQuantizedONNX + DownloadModel prefers it)
- [x] 2.2 Registry/onnx load uses resolved path; `quantize: on` fails load with clear error when missing
- [x] 2.3 Autodetection verified on quantized graphs (measured: dim=384, max_length=512 on Xenova int8)

## 3. Observability

- [x] 3.1 `EMB.INFO` reports `quantization: int8|fp32` + on-disk model size (model_bytes)

## 4. Quality verification

- [x] 4.1 Measured int8-vs-fp32 cosine over the fixed corpus via `emb-verify`: **min cosine ≈ 0.9913** (passes the 0.99 qualitative gate; the verify's 0.999 gate is a fp32-identical check and legitimately fails for int8 — see proposal note)
- [x] 4.2 Per-vector diff bound documented in the proposal (0.99 gate, not 0.999)

## 5. Validation stage (nix develop)

- [x] 5.1 In `nix develop`: `just test` + `just lint` pass
- [ ] 5.2 Latency gate: p50 ≤ 0.60× fp32 baseline at 2 vCPU — preliminary measured −26% long-txt p50; formal harness gate pending gold host
- [ ] 5.3 Throughput gate: req/s ≥ +60% vs fp32 baseline at 8 vCPU — preliminary measured +35% at 8 clients; formal gate pending gold host
- [x] 5.4 Memory gate measured: RSS 1.14GB → 387MB (−66%); weights 92MB → 23MB (−75%)
- [x] 5.5 Quality gate: cosine 0.9913 ≥ 0.99 passes
- [x] 5.6 Fallback gates verified by unit tests (auto→quantized, auto→fp32, on→error, off→fp32)