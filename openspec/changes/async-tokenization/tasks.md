## 1. Config

- [x] 1.1 Add `TokenizeWorkers` field to model config (default `min(4, cores)`, 0 = serial)
- [x] 1.2 Document in config.yaml example

## 2. Producer stage

- [x] 2.1 Create producer queue (`Encoding` + tokens per request) with bounded capacity
- [x] 2.2 Tokenizer workers consume texts → emit encodings (goroutine-safe tokenizer reuse)
- [x] 2.3 Feed per-request real-token counts to the P1 budget flush

## 3. Run loop integration

- [x] 3.1 Batcher consumes encodings, packs, flushes, distributes (order preserved)
- [x] 3.2 `tokenize_workers: 0` → serial fallback; pool path keeps per-worker parallelism (already overlaps tokenize+infer across workers — see design note)
- [x] 3.3 `Close` drains queue without deadlock (producers exit on `done`)

## 4. Validation stage (nix develop)

- [x] 4.1 In `nix develop`: `just test` + `just lint` pass
- [ ] 4.2 Overlap gate: tokenization wall share ≤ 5% of request wall time (unit proxy passes: overlap test 0.08s vs 105ms serialized; pending harness-level measurement)
- [ ] 4.3 Throughput gate: fixed-length req/s ≥ +15% and mixed-length ≥ +15% at 8 vCPU vs P0 baseline (pending gold-host paired A/B, same limitation as P1)
- [ ] 4.4 Tail gate: stability p99 (loaded/idle ≤ 1.5) holds (pending harness)
- [x] 4.5 `just verify-embeddings` passes unchanged (20/20, cosine 1.0, tokenize_workers=4 active)