## 1. Output Tensor Selection

- [x] 1.1 Add `selectOutputTensor` helper: prefer rank-2 outputs → fall back to rank-3 → first available → default `last_hidden_state`
- [x] 1.2 Wire into `ensurePool`: after `GetOutputInfo`, auto-select if `cfg.OutputTensor` not found in available outputs

## 2. Pooling Inference

- [x] 2.1 Add `poolingForRank` helper: rank-2 → `none`, rank-3 → `mean`
- [x] 2.2 Set `cfg.Pooling` automatically when inferred from selected output rank and `cfg.Pooling` is not explicitly configured

## 3. Config Resolution Integration

- [x] 3.1 In `resolveModelConfig`, move `GetOutputInfo` before output tensor/pooling defaults so auto-detection runs before the "empty → default" fallback
- [x] 3.2 Log auto-detected values: `output=%q pooling=%s normalize=%v`

## 4. Tests

- [x] 4.1 Unit test: `selectOutputTensor` picks rank-2 over rank-3
- [x] 4.2 Unit test: `selectOutputTensor` picks only available output
- [x] 4.3 Unit test: `poolingForRank(2)` returns `none`
- [x] 4.4 Unit test: `poolingForRank(3)` returns `mean`
- [x] 4.5 Integration test: model config without output_tensor/pooling loads and runs correctly against known models (minilm + siglip2)
- [x] 4.6 Run `go test ./...` — all tests pass
