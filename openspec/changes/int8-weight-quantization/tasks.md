## 1. Config & resolution

- [ ] 1.1 Add `Quantize` enum (`auto|on|off`, default `auto`) to model config
- [ ] 1.2 Add quantized-file resolution helper (local dirs: root/onnx/onnx-quantized order)
- [ ] 1.3 Default `auto` for HF downloads, `off` for explicit local paths (see Open Questions)

## 2. Download & load

- [ ] 2.1 `hfhub` resolves quantized artifact when `quantize != off`
- [ ] 2.2 Registry/onnx load uses resolved path; `quantize: on` fails load with clear error when missing
- [ ] 2.3 Autodetection (dim/max_length/output_tensor) verified working on quantized graphs

## 3. Observability

- [ ] 3.1 `EMB.INFO` reports `quantization: int8|fp32` + on-disk model size

## 4. Quality verification

- [ ] 4.1 Extend `emb-verify` with an int8-vs-fp32 cosine comparison over the fixed corpus
- [ ] 4.2 Document per-vector diff bounds in the verify output

## 5. Validation stage (nix develop)

- [ ] 5.1 In `nix develop`: `just test` + `just lint` pass
- [ ] 5.2 Latency gate: p50 ≤ 0.60× fp32 baseline at 2 vCPU (single client)
- [ ] 5.3 Throughput gate: req/s ≥ +60% vs fp32 baseline at 8 vCPU
- [ ] 5.4 Memory gate: RSS ≤ 0.35× fp32 RSS
- [ ] 5.5 Quality gate: cosine ≥ 0.99 (fp32 vs int8) passes
- [ ] 5.6 Fallback gates: `auto` warns+fp32; `on` fails loudly; `off` loads fp32