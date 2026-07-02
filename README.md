# emb

[![GitHub Release](https://img.shields.io/github/v/release/elcuervo/emb?logo=github&color=blue)](https://github.com/elcuervo/emb/releases)
[![Docker Hub](https://img.shields.io/docker/v/elcuervo/emb?logo=docker&color=blue&label=docker)](https://hub.docker.com/r/elcuervo/emb)
[![emb gem](https://img.shields.io/gem/v/emb?logo=rubygems&color=red&label=emb)](https://rubygems.org/gems/emb)
[![emb-server gem](https://img.shields.io/gem/v/emb-server?logo=rubygems&color=red&label=emb-server)](https://rubygems.org/gems/emb-server)

![](https://images.unsplash.com/photo-1625768376503-68d2495d78c5?q=80&w=2225&auto=format&fit=crop&ixlibrb=rb-4.1.0&ixid=M3wxMjA3fDB8MHxwaG90by1wYWdlfHx8fGVufDB8fHx8fA%3D%3D)

```
redis-cli EMB minilm "hello world"
→ \x7c\x8e\x80\xbd...   (384 float32s × 4 bytes)
```

## Install

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | sh
```

Installs to `/usr/local/bin`. Set `EMB_INSTALL_DIR` to change the target:

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | EMB_INSTALL_DIR=~/.local/bin sh
```

**Platforms:** macOS (Apple Silicon), Linux (amd64, arm64).

## Quick start

```bash
# Auto-downloads a model from HuggingFace and starts the server
emb -model-repo Xenova/all-MiniLM-L6-v2

# In another terminal:
redis-cli EMB minilm "hello world"
→ \x7c\x8e\x80\xbd...   (384 float32s × 4 bytes)
```

## Features

- **Redis protocol**: any Redis client works (`redis-cli`, `redis-py`, `redis-rb`, etc.)
- **ONNX Runtime**: fast CPU/GPU inference via CGo bindings
- **HuggingFace integration**: auto-download models and auto-detect dim, max_length, output tensor, pooling strategy from ONNX graph + config.json
- **Multi-model queries**: `EMB.MULTI` calls different models in one command (MGET-style partial failures)

## Quick start

### One-liner (no config file)

```bash
# Auto-downloads a model from HuggingFace and starts the server
emb -model-repo Xenova/all-MiniLM-L6-v2

# In another terminal:
redis-cli EMB model "hello world"
```

### Two models inline

```bash
emb \
  -model minilm -model-onnx ./models/minilm/model.onnx -model-tokenizer ./models/minilm/tokenizer.json \
  -model bge   -model-repo Xenova/bge-small-en-v1.5

redis-cli EMB.MULTI minilm "hello" bge "world"
```

### Local development (with config file)

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
| `PING` | PONG |

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
  minilm:
    onnx: ./models/minilm/model.onnx

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
require "redis_client"

redis = RedisClient.new(port: 6379)
raw = redis.call("EMB", "minilm", "hello world")
emb = raw.unpack("e*")
```

Or use the [`emb`](gems/emb/README.md) gem:

```ruby
require "emb"

Emb[:minilm]["hello world"]
# => [0.0123, -0.0456, 0.0789, ...]
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

## Ruby Gems

Ruby gems for emb:

- [`emb`](https://rubygems.org/gems/emb) — Client library with connection pooling, proxy, and multi-model support. Auto-decodes float32 responses.
  [README](gems/emb/README.md)

- [`emb-server`](https://rubygems.org/gems/emb-server) — Precompiled server binary. Install and run `emb` directly.
  [README](gems/emb-server/README.md)

## Development

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
# Run with a model mounted:
docker run -v ./models:/models elcuervo/emb \
  -config /models/config.yaml
