# emb

```
redis-cli EMB minilm "hello world"
→ \x7c\x8e\x80\xbd...   (384 float32s × 4 bytes)
```

## Features

- **Redis protocol** — any Redis client works (`redis-cli`, `redis-py`, `redis-rb`, etc.)
- **Pure Go** — no Python or Node.js runtime dependency
- **ONNX Runtime** — fast CPU/GPU inference via CGo bindings
- **HuggingFace integration** — auto-download models and auto-detect dim, max_length, output tensor, pooling strategy from ONNX graph + config.json
- **Smart batching** — coalesce concurrent requests into a single ONNX inference batch with configurable timeout
- **Multi-model queries** — `EMB.MULTI` calls different models in one command (MGET-style partial failures)
- **Pre-pooled models** — supports 2D-output models like SigLIP2, E5 via `output_tensor` + `pooling: none`
- **Auto-tuned workers** — pool size automatically capped to fit within half of available RAM
- **Lazy loading** — models initialized on first request by default (optional `preload`)
- **Pure Go tokenizer** — HuggingFace WordPiece and BPE tokenization, no `tokenizers` library needed

## Quick start

```bash
# Download a model from HuggingFace
just download-model

# Start the server
just dev

# In another terminal:
redis-cli EMB minilm "hello world"
```

## Commands

| Command | Description |
|---------|-------------|
| `EMB <model> <text> [text...]` | Embed one or more texts. Single text → bulk string, multiple → array of bulk strings |
| `EMB.MODELS` | List loaded models with dimensions and status |
| `EMB.INFO <model>` | Model details: dim, workers, requests served, avg latency |
| `EMB.STATS` | Server statistics: uptime, total requests, per-model breakdown |
| `EMB.MULTI <model> <text> [<model> <text>...]` | Embed texts across different models in one call |
| `EMB.HELP` | Command reference |
| `PING` | Redis compatibility |

### EMB.MULTI example

```
redis-cli EMB.MULTI minilm "hello" siglip2 "a photo of a cat"
1) \x7c\x8e\x80\xbd...   (minilm, 384 floats)
2) \x4a\x9f\x31\xc2...   (siglip2, 768 floats)
```

## Configuration

```yaml
listen: ":6379"

models:
  # Standard sentence-transformer (3D output → mean pooled)
  minilm:
    onnx: ./models/minilm/model.onnx

  # Pre-pooled model (2D output, skip mean pooling)
  siglip2:
    onnx: ./models/siglip2/text_model.onnx
    tokenizer: ./models/siglip2/tokenizer.json
    output_tensor: pooler_output
    pooling: none
    normalize: true
    dim: 768

  # Auto-download from HuggingFace
  e5:
    model_repo: intfloat/e5-small-v2
    pooling: none
    normalize: false
```

### Model options

| Field | Default | Description |
|-------|---------|-------------|
| `onnx` | — | Path to ONNX model file |
| `tokenizer` | `<model-dir>/tokenizer.json` | Path to HuggingFace tokenizer JSON |
| `model_repo` | — | HuggingFace repo (auto-downloads ONNX + tokenizer) |
| `dim` | auto-detected | Embedding dimension |
| `max_length` | auto-detected (or 512) | Max token sequence length |
| `pooling` | auto-detected | `mean` (3D output) or `none` (2D pre-pooled) |
| `normalize` | `false` | L2-normalize the output |
| `output_tensor` | auto-detected | ONNX output tensor name |
| `preload` | `false` | Load model at startup instead of on first request |
| `pad_output` | `false` | Pad sequences to `max_length` with trailing zeros (compatibility with legacy implementations that don't pass attention mask) |
| `workers` | auto-tuned | Number of worker goroutines |
| `batching` | `{timeout: 1, max_batch: 32}` | Smart batching settings (set `timeout: 0` to disable) |

## Clients

The response is raw little-endian float32 bytes. Any Redis client works.

**Ruby:**
```ruby
require "redis"

r = Redis.new(port: 6379)
raw = r.call("EMB", "minilm", "hello world")
emb = raw.unpack("e*")
# → [0.0123, -0.0456, 0.0789, ...]
```

**Python:**
```python
import struct
raw = redis.execute_command("EMB", "minilm", "hello world")
emb = list(struct.unpack(f"<{len(raw)//4}f", raw))
```

**Go:**
```go
var vec []float32
binary.Read(bytes.NewReader(raw), binary.LittleEndian, &vec)
```

## Architecture

```
redis-cli ──RESP──▶ tidwall/redcon ──▶ Model Registry ──▶ Pool
                                                           │
                                              ┌────────────┼────────────┐
                                              ▼            ▼            ▼
                                         Tokenizer   ONNX Runtime   Pool+Norm
                                         (pure Go)   (CGo binding)  (mean+L2)
                                                           │
                                              ┌────────────┘
                                              ▼
                                         Smart Batcher
                                    (coalesces concurrent
                                     requests into batches)
```

- **Concurrency**: goroutine-per-connection, round-robin worker pool per model
- **Session pooling**: one ONNX session per worker (safe for concurrent use)
- **Smart batching**: when enabled, concurrent requests are collected for up to `timeout` ms, then run as a single ONNX inference with batch_size = N
- **Lazy loading**: models load on first request; ONNX metadata and config.json are read upfront for auto-detection
- **Memory auto-tune**: workers capped to fit within half of available RAM
- **Auto-download**: `model_repo` triggers a pure-Go HuggingFace download (ONNX file + tokenizer.json + config.json)
- **Auto-config**: dim, max_length, output tensor, and pooling strategy are detected from the ONNX graph and config.json

## Performance

Microbenchmarks (Apple M1 Pro, all-MiniLM-L6-v2):

| Benchmark | Result |
|-----------|--------|
| MeanPool batch=1 | 26µs |
| L2Normalize dim=768 | 1.5µs |
| RESP roundtrip | 28µs |
| Pool embed (parallel) | 656ns |
| **End-to-end p50** | **1.8ms** |
| **End-to-end p95** | **4.4ms** |

## Development

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [ONNX Runtime](https://github.com/microsoft/onnxruntime/releases) shared library
- Python 3 (for `just verify-embeddings` reference generation)
- [Just](https://github.com/casey/just) command runner

### Commands

```bash
just format          # Format all Go code
just lint            # Run linters
just test            # Run tests
just bench           # Run benchmarks
just build           # Build the emb binary
just dev             # Build and run the server
just download-model  # Download a model from HuggingFace
```

### Nix

A `flake.nix` is provided for reproducible development shells:

```bash
nix develop
```

This provides Go, ONNX Runtime, golangci-lint, just, and all CGo configuration.

### Docker

```bash
# Build for native architecture
just docker-build

# Build and push multi-arch (amd64 + arm64)
just docker-push

# Run with a model mounted:
docker run -v ./models:/models elcuervo/emb-server \
  -config /models/config.yaml
```

### Verifying correctness

```bash
# End-to-end response time benchmark
just bench-e2e

# Verify embeddings match Python sentence-transformers reference
just verify-embeddings

# Verify multi-model EMB.MULTI across different model types
just verify-emb-multi
```

The verification tools compare byte-level output against sequential `EMB` calls and Python-generated reference embeddings using cosine similarity.

## License

MIT
