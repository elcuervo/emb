## Why

Building a fast, Redis-compatible text embedding server that speaks RESP protocol so any Redis client can generate embeddings without needing Python or custom SDKs. Existing embedding services are HTTP-based with high overhead; this aims for raw TCP throughput with ONNX models and minimal allocation.

## What Changes

- New `emb` binary: a RESP-protocol server using `tidwall/redcon`
- Custom `EMB` command family for text embedding generation
- ONNX model loading from config (HuggingFace-exported models)
- Worker-pool architecture for concurrent request handling
- Session pooling per model with pre-allocated tensors (zero-alloc hot path)
- True batch embedding (single ONNX `Run()` for multiple texts)
- `flake.nix` for reproducible dev environment
- Tests and benchmarks

## Capabilities

### New Capabilities
- `emb-cmds`: RESP command family (`EMB`, `EMB.MODELS`, `EMB.INFO`, `EMB.STATS`, `EMB.HELP`) for generating and inspecting text embeddings
- `model-loading`: Load ONNX models + tokenizers from config at startup, auto-tune worker pools to CPU count
- `embedding-pipeline`: Tokenize → batch pad → ONNX inference → mean pooling → L2 normalization, returning raw float32 bytes

### Modified Capabilities

- None (greenfield project)

## Impact

New codebase under this repo. Dependencies: `tidwall/redcon` (Go), `yalue/onnxruntime_go` (CGo ONNX binding), `daulet/tokenizers` (CGo tokenizer binding), ONNX Runtime C library (from flake). No existing code modified.
