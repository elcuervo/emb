## Context

Greenfield Go server for Redis-compatible text embedding generation. Zero existing code. All new.

## Goals / Non-Goals

**Goals:**
- RESP-protocol server accepting any Redis client (redis-cli, redigo, etc.)
- Multiple ONNX text embedding models served concurrently
- Auto-tuned worker pool per model (one worker per CPU core, IntraOpNumThreads=1)
- True batch embedding in a single ONNX `Run()` call
- Zero-alloc hot path via pre-allocated tensors per worker
- Mean pooling + L2 normalization (sentence-transformers convention)
- flake.nix providing Go, ONNX Runtime C library, build dependencies
- Tests and benchmarks

**Non-Goals:**
- GPU acceleration (future)
- Model training or export to ONNX
- HTTP API (RESP only)
- Persistent storage or caching of embeddings
- Authentication, TLS (add later if needed)

## Decisions

### Framework: tidwall/redcon
Goroutine-per-connection, each handler goroutine sends requests to model worker pools via channels. redcon handles all RESP parsing and response writing.

### Concurrency: Channel-based worker pool per model
A single `DynamicAdvancedSession` is not safe for concurrent `Run()`. Instead of a mutex (serializes) or per-connection session (explodes memory), each model gets N workers communicating via a channel:

```
conn handler ──req──▶ model.reqChan ──▶ worker₁ (session₁, tok₁, tensors₁)
                                      ──▶ worker₂ (session₂, tok₂, tensors₂)
                                      ──▶ ...
```

Workers auto-tune to `GOMAXPROCS`. The request includes a `chan []byte` for the response.

### IntraOpNumThreads=1 for small embedding models
Small models (MiniLM, SigLIP base) have low compute-per-operator cost. Spawning threads inside `Run()` overhead. Better to run N standalone sessions on N cores. Configurable per-model for larger models.

### true model batching
`EMB <model> <t1> <t2> <t3>` → single `session.Run()` with batch_size=3. Worker pads to `max(sequence_lengths)` within the batch, uses per-text attention masks for mean pooling.

### Response format
Raw float32 bytes in little-endian order. Dimension is not in the response — the client knows it from the model name. This is the standard pattern for vector search clients (pgvector, RedisVL, qdrant).

- Single: `$<dim*4>\r\n<bytes>`
- Batch: `*N\r\n$<dim*4>\r\n<bytes>\r\n$<dim*4>\r\n<bytes>\r\n...`
- Errors: standard RESP error `-ERR <msg>\r\n`

### Model config at startup
YAML config loaded at boot. Maps model names to paths and options. No runtime load/unload.

```yaml
models:
  siglip2:
    onnx: ./models/siglip2/model.onnx
    tokenizer: ./models/siglip2/tokenizer.json
    pooling: mean
    normalize: true
    max_length: 64
    dim: 768
```

### Pooling: mean pooling in Go
Sentence-transformers models output `last_hidden_state` with shape `(batch, seq_len, hidden)`. Mean pooling applies the attention mask to average only real tokens, not padding tokens. This is simpler and faster in Go post-processing than embedding in the ONNX graph (which would require model-specific export).

### Tokenizer: dauler/tokenizers
Goroutine-safe (Rust under the hood), supports WordPiece/BPE/SentencePiece from HuggingFace tokenizer.json files. One instance per model, shared across all workers.

## Risks / Trade-offs

- Session weight duplication → Each worker creates its own `AdvancedSession`, which loads its own copy of model weights from the ONNX file. 4 workers × 1GB model = 4GB RSS. Mitigation: use `ort.SessionOptions.SetMemoryPattern(true)` and `SetCpuMemArena(true)`; consider weight sharing API in future ORT versions.
- CGo overhead → Both ONNX runtime and tokenizer use CGo. The actual compute dwarfs CGo marshaling for reasonable batch sizes. Mitigation: batch within a single request rather than making many tiny calls.
- SigLIP2 max_length=64 → Text longer than 64 tokens gets truncated silently. The worker should accept a `truncation` config option and/or log a warning.
- ONNX Runtime versioning → The C shared library ABI must match the Go binding version. Mitigation: precise version pinning in flake.nix.
