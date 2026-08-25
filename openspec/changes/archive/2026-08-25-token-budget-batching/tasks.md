## 1. Config

- [x] 1.1 Add `MaxBatchTokens` field to `Batching` config struct (default 16384)
- [x] 1.2 `MaxBatchTokens: 0` → disable token accounting (count-only behavior)
- [x] 1.3 Update `config-batching.yaml` example with `max_batch_tokens`

## 2. Batcher

- [x] 2.1 Accumulate real tokens per enqueued request in `Batcher.run`
- [x] 2.2 Flush on `budget reached OR count ≥ max_batch OR timer`
- [x] 2.3 Keep single-command texts unsplit (flush the whole Request as one run)
- [x] 2.4 Track padding efficiency per run (real tokens / batch_size × seq_len)

## 3. Wiring & stats

- [x] 3.1 Wire `MaxBatchTokens` through registry → NewPool → NewBatcher
- [x] 3.2 Expose `batching_max_tokens` + padding efficiency in `EMB.INFO`
- [x] 3.3 Surface budget + efficiency in `EMB.STATS`

## 4. Validation stage (nix develop)

- [x] 4.1 In `nix develop`: `just test` + `just lint` pass
- [ ] 4.2 Mixed-length harness gate on 8 vCPU: req/s ≥ +30% vs P0 baseline; padding efficiency ≥ 0.5
- [ ] 4.3 Fixed-length regression gate: req/s ≥ −5%
- [ ] 4.4 Latency gate: p50 ≤ baseline + 2ms
- [x] 4.5 `just verify-embeddings` passes (correctness unchanged)