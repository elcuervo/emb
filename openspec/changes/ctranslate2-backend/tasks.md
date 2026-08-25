## 1. Build integration (nix + Docker)

- [ ] 1.1 Add CTranslate2 to `flake.nix` devShell (aarch64) with CGO flags exported in shellHook
- [ ] 1.2 Dockerfile builder fetches/builds CTranslate2 aarch64 lib; runtime stage copies it alongside ORT
- [ ] 1.3 Verify `nix develop` + Docker builds both link cleanly

## 2. Backend implementation

- [ ] 2.1 Implement `Session` in `internal/onnx/ctranslate2.go` (cgo, C API) with variable-length batch input
- [ ] 2.2 Add `Backend` config (`auto|onnx|ctranslate2`) + format detection (CT2 files present?)
- [ ] 2.3 Branch `processBatch`: ONNX keeps `PadEncodings`; CT2 packs `[]int32` + lengths (no padding)
- [ ] 2.4 Pooling/normalization reuse (mean-pool on CT2 hidden states)

## 3. Composition & observability

- [ ] 3.1 Verify P1 token budget + P2 async tokenization compose with the CT2 path
- [ ] 3.2 `EMB.INFO` reports `backend: onnx|ctranslate2` (+ quantization variant)
- [ ] 3.3 Prefer int8 CT2 variant when shipped; `quantize` field reflects it

## 4. Model conversion tooling

- [ ] 4.1 Add `just convert-model` wrapping `ct2-transformers-converter`
- [ ] 4.2 Document conversion in README + config.yaml examples (runtime conversion is out of scope)

## 5. Validation stage (nix develop)

- [ ] 5.1 In `nix develop`: `just test` + `just lint` pass with both backends
- [ ] 5.2 Build gate: `nix develop` shell + aarch64 Docker image build cleanly
- [ ] 5.3 Quality gate: `emb-verify -backend ctranslate2` cosine ≥ 0.99 vs sentence-transformers reference
- [ ] 5.4 Throughput gate: req/s at 8 vCPU ≥ +50% vs ORT-int8 baseline and ≥ +150% vs fp32 baseline
- [ ] 5.5 Latency gate: p50 at 2 vCPU ≤ 0.65× fp32 baseline
- [ ] 5.6 Memory gate: RSS ≤ fp32 baseline (int8 CT2)
- [ ] 5.7 Startup gate: model load in container < 30s (< preload guidance)
- [ ] 5.8 Image gate: Docker runtime image still Fargate-arm64-ready