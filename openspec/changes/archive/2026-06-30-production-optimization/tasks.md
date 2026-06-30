## 1. Update config-prod.yaml with production settings

- [x] 1.1 Add `batching.timeout: 1` to both siglip2 and e5 models
- [x] 1.2 Add `intra_op_threads: 4` to both models
- [x] 1.3 Verify both models start successfully with new config

## 2. Verify

- [x] 2.1 Run PoC benchmark — both models compatible (cosine=1.0)
- [x] 2.2 Compare single-request speed with previous benchmark
- [x] 2.3 Test concurrent requests to verify batching throughput
