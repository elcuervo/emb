## 1. Config Optional Fields

- [x] 1.1 Make `tokenizer` path optional in config: infer from ONNX file directory (same dir + `tokenizer.json`)
- [x] 1.2 Make `pooling` default to `"mean"`, `normalize` default to `true` in `LoadModel()` (resolveModelConfig)
- [x] 1.3 Update config validation: `onnx` or `model_repo` is required, everything else is optional

## 2. ONNX Graph Inspection

- [x] 2.1 Implement `InferDim(modelPath string) (int, error)` using `ort.GetInputOutputInfo`
- [x] 2.2 Wire `InferDim` into `registry.LoadModel()`: if `cfg.Dim == 0`, detect from ONNX

## 3. max_length Detection

- [x] 3.1 Implement `InferMaxLength(modelDir string) (int, error)` reading `config.json` `max_position_embeddings`
- [x] 3.2 Wire into `registry.LoadModel()`: if `cfg.MaxLength == 0`, detect from config.json (fallback 512)

## 4. Embedding Validation

- [x] 4.1 Write Python reference embedding generator (`cmd/emb-verify/generate-reference.py`)
- [x] 4.2 Implement `cmd/emb-verify` Go tool: connects to server, sends EMB, compares cosine similarity
- [x] 4.3 Add `verify-embeddings` target to `justfile`
- [x] 4.4 Verification passed: 20/20 sentences with cosine = 1.0

## 5. Integration and Tests

- [x] 5.1 Update config tests for new optional-field behavior (dim/tokenizer no longer required)
- [x] 5.2 Dim inference tested via real server startup (logged dim=384)
- [x] 5.3 max_length inference tested via real server startup (logged from config.json)
- [x] 5.4 Build: `go vet ./...` passes; all `go test ./...` pass
- [x] 5.5 Benchmarks: no regression vs baseline
- [x] 5.6 Minimal config verified: `config.yaml` now just `onnx: ./models/minilm/model.onnx`
