# emb

A simple yet powerful text embeddings generator.

[![GitHub Release](https://img.shields.io/github/v/release/elcuervo/emb?logo=github&color=blue)](https://github.com/elcuervo/emb/releases)
[![Docker Hub](https://img.shields.io/docker/v/elcuervo/emb?logo=docker&color=blue&label=docker)](https://hub.docker.com/r/elcuervo/emb)
[![emb gem](https://img.shields.io/gem/v/emb?logo=rubygems&color=red&label=emb)](https://rubygems.org/gems/emb)
[![emb-server gem](https://img.shields.io/gem/v/emb-server?logo=rubygems&color=red&label=emb-server)](https://rubygems.org/gems/emb-server)

![](https://images.unsplash.com/photo-1582137696617-4031a8e3e268?q=80&w=2428&auto=format&fit=crop&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxwaG90by1wYWdlfHx8fGVufDB8fHx8fA%3D%3D)

`emb` is a text-embeddings server speaking the Redis protocol. Every Redis
client: `redis-cli`, `redis-py`, `redis-rb`, … — can call it without special
libraries, and embeddings come back as raw float32 bytes:

```bash
redis-cli EMB minilm "hello world"
# → \x7c\x8e\x80\xbd...   (384 float32s × 4 bytes)
```

## Contents

- [Features](#features)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Configuration](#configuration)
- [Operations](#operations)
- [Clients](#clients)
- [Development](#development)

## Features

- **Redis protocol** — drop-in for any Redis client; RESP2 responses are raw
  little-endian float32 vectors.
- **ONNX Runtime** — fast CPU/GPU inference via CGo bindings, with optional
  int8 weight quantization.
- **HuggingFace integration** — auto-download models and auto-detect dim,
  max_length, output tensor, and pooling strategy from the ONNX graph + `config.json`.
- **Smart batching** — a 1 ms window coalesces concurrent requests into shared
  ONNX runs, with a token budget and async tokenization (on by default).
- **Embeddings cache** — in-process LRU with per-model stats; sized in bytes,
  percentages, or `auto`.
- **Multi-model queries** — `EMB.MULTI` calls different models in one command
  (MGET-style partial failures).
- **Ops-ready** — Redis-style `INFO` and `CONFIG`, health checks
  (`EMB.READY`), connection lifecycle knobs, and full server stats.

## Install

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | sh
```

Installs to `/usr/local/bin`. Set `EMB_INSTALL_DIR` to change the target:

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | EMB_INSTALL_DIR=~/.local/bin sh
```

**Platforms:** macOS (Apple Silicon), Linux (amd64, arm64).

Or install the [`emb-server`](https://rubygems.org/gems/emb-server) gem and run
`emb` directly.

## Quick start

### One-liner (no config file)

```bash
# Auto-downloads a model from HuggingFace and starts the server
emb -model-repo Xenova/all-MiniLM-L6-v2

# With password authentication
emb -model-repo Xenova/all-MiniLM-L6-v2 -password "hunter2"

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
just download-model   # Download a model from HuggingFace
just dev              # Build and start the server

# In another terminal:
redis-cli EMB minilm "hello world"
```

## Commands

| Command | Description |
|---------|-------------|
| `EMB <model> <text> [text...]` | Embed one or more texts. Single text → bulk string, multiple → array of bulk strings |
| `EMB.MULTI <model> <text> [<model> <text>...]` | Embed texts across different models in one call |
| `EMB.MODELS` | List loaded models with dimensions and status |
| `EMB.INFO <model>` | Model details: dim, workers, requests served, avg latency, live cache stats |
| `EMB.STATS` | Server statistics: uptime, total requests, live connections, active requests, per-model breakdown |
| `EMB.READY` | Health check: `+OK` (ready), `-ERR <reason>` (loading, draining, no models) |
| `EMB.HELP` | Command reference |
| `INFO [section...]` | Redis-style INFO: `server`, `cache`, `keyspace`, `stats`, `clients` |
| `CONFIG GET [glob]` / `CONFIG SET` | Read or live-tune runtime settings (see [Operations](#operations)) |
| `AUTH <password>` | Authenticate the connection (required if `password` is set) |
| `PING` | PONG |

### EMB.MULTI

`EMB.MULTI` embeds texts against different models in a single round trip and
answers with MGET-style partial failures: each reply is the vector for its
model, and a failed pair yields an error in its slot without failing the rest.

```
redis-cli EMB.MULTI minilm "hello" siglip2 "a photo of a cat"
1) \x7c\x8e\x80\xbd...   (minilm, 384 floats)
2) \x4a\x9f\x31\xc2...   (siglip2, 768 floats)
```

## Configuration

Server settings live in a YAML file (`-config config.yaml`) or inline flags:

```yaml
listen: ":6379"

# password: "hunter2"
# tls_cert: /etc/emb/cert.pem
# tls_key:  /etc/emb/key.pem
# cache: "auto"   # or "1GB", "256MB", "25%". Empty = disabled
# idle_timeout: 15m, max_connections: 100, max_concurrent_requests: 32

models:
  minilm:
    onnx: ./models/minilm/model.onnx

  # Pre-pooled siglip2 text encoder (int8 used in production). The graph
  # exports its final embedding as "text_embeds" (768-dim); if you configure
  # an output tensor name that is not in the graph, the server falls back to
  # the detected output and logs which name it auto-selected.
  siglip2:
    onnx: ./models/siglip2/text_model_int8.onnx
    tokenizer: ./models/siglip2/tokenizer.json
    output_tensor: text_embeds
    pooling: none
    normalize: true
    dim: 768
    max_length: 64
    pad_output: true   # graph has a fixed [batch, 64] input; pad to max_length

  # Custom e5 export with pooling + output layers baked into the graph (2D
  # pooled output, already normalized), served with no server-side arithmetic:
  # pooling: none + normalize: false is a pure buffer export.
  e5:
    onnx: ./models/e5/model.onnx
    tokenizer: ./models/e5/tokenizer.json
    output_tensor: pooled_sentence_embeddings_debiased_normalized
    pooling: none
    normalize: false

  # WARNING: the stock HuggingFace export intfloat/e5-small-v2 (and similar)
  # outputs a 3D last_hidden_state. It must NOT be configured with
  # pooling: none — that would slice a 3D buffer (correct only at batch=1).
  # Use pooling: mean (auto-detected) or a pre-pooled 2D export instead.
```

### Model options

| Field | Default | Description |
|-------|---------|-------------|
| `onnx` | — | Path to ONNX model file |
| `tokenizer` | `<model-dir>/tokenizer.json` | Path to HuggingFace tokenizer JSON |
| `model_repo` | — | HuggingFace repo (auto-downloads ONNX + tokenizer) |
| `dim` | auto-detected | Embedding dimension |
| `max_length` | auto-detected (or 512) | Max token sequence length |
| `pooling` | auto-detected | `mean` (3D output), `cls` (first token) or `none` (2D pre-pooled) |
| `normalize` | `false` | L2-normalize the output |
| `output_tensor` | auto-detected | ONNX output tensor name |
| `preload` | `false` | Load model at startup instead of on first request |
| `pad_output` | `false` | Pad sequences to `max_length` with trailing zeros (compatibility with legacy implementations that don't pass attention mask) |
| `workers` | auto-tuned | Number of worker goroutines |
| `intra_op_threads` | `cores−2` | ONNX intra-op threads per session. Defaults to `cores−2` to reserve cores for request parsing/dispatch; set explicitly to override |
| `batching` | `{timeout: 1, max_batch: 32, max_batch_tokens: 16384}` | Smart batching settings. **Enabled by default** (1 ms window) for every model; set `timeout: 0` to use the worker pool. With batching on, `tokenize_workers` defaults to `min(4, cores)` and the token budget auto-applies |

### Batching

Batching is **on by default** for every model, so no config is needed for the
performance path: a window coalesces concurrent requests (including `EMB.MULTI`
pairs) into shared ONNX runs, a token budget bounds each run, and dedicated
tokenizer workers hide tokenization behind inference. `timeout: 0` opts out.
`EMB.MULTI` processes pairs with bounded concurrency (≤ the machine's `GOMAXPROCS`),
so request storms can't spawn unbounded goroutines that starve inference.

### Caching

Embeddings are cached by `model:text` key in an in-process LRU, so repeated
texts skip ONNX inference entirely. Configure via the `cache:` YAML key or the
`-cache` CLI flag:

| Value | Behavior |
|-------|----------|
| *(empty)* | Cache disabled (default) |
| `auto` | ~13% of total RAM: 20% of memory left after a 10% safety margin and a 25% model reserve, floored at 64MB. No fixed byte cap — scales with the machine |
| `1GB`, `256MB`, … | Explicit byte budget (`docker/go-units` sizes) |
| `25%` | Percentage of total system RAM (explicit operator choice — no safety margins applied) |

```bash
./bin/emb -config config.yaml -cache auto   # ~13% of RAM on this machine
./bin/emb -config config.yaml -cache 25%    # a quarter of RAM
./bin/emb -config config.yaml -cache 2GB    # fixed budget
```

Invalid sizes or percentages (e.g. `150%`, `abc`) fail startup with a clear
error. Live cache stats — hits, misses, hit rate, evictions, entries, and byte
usage — are visible per model via `EMB.INFO <model>` and globally via
[`INFO`](#operations) and `EMB.STATS`. See `BENCHMARK.md` → *Cache* for
hit-rate measurements.

## Embeddings & vector indexes (OpenSearch)

`emb` always emits **fp32 little-endian float vectors** (`dim * 4` bytes per
text), so vectors can be stored directly in an OpenSearch `knn_vector` field
with `data_type: float` (or any float-vector index). This holds **even when the
backbone runs int8** (e.g. `siglip2`/`text_model_int8.onnx`): quantization
applies to the model's compute; pooling/normalization and the wire format stay
fp32. There is no `byte`-vector mode and no on-wire precision loss.

Retrieval-correctness notes:

- **Near-identical is the contract, not byte-identity.** SIMD pooling,
  pooling-in-graph, and int8 backbones shift vectors within ~0.99 cosine of the
  fp32 reference while preserving top-k retrieval ranking (validated by
  `cmd/emb-verify-performance`; siglip2 int8 measures 0.9992 mean cosine vs its
  fp32 export with identical top-10 ranking). If you require exact byte equality
  with a previous deployment, reindex with the same configuration instead.
- **Changing precision requires a one-time reindex.** fp32→int8 (or any
  pooling/normalize change) alters vector values. **Version your embedding
  function** (model id + precision + normalizer) into the index metadata and
  rebuild the index in the new precision; never mix vectors of different
  precisions in one `knn_vector` field.
- **Int8 artifact discovery is deliberately unchanged** (`quantize: auto` still
  picks `model_quantized.onnx`/`onnx/quantized/*` by name); quantized-model
  autodiscovery is out of scope for this change.

## Operations

### Health checks

`EMB.READY` returns `+OK` when the server is ready to serve traffic, or `-ERR`
with a reason (`loading`, `draining`, `no models`). Point your load balancer's
TCP health check at it, or use the client's `ready?`/`ready` helpers:

```bash
redis-cli EMB.READY
# → OK
```

```ruby
Emb.ready?  # => true
Emb.ready   # => "ready"
```

### Connection lifecycle

Three knobs bound the server's connection and request surface. `idle_timeout`
defaults to **15 minutes** (set `0` to disable reaping entirely); the two caps
default to `0` (unlimited).

```yaml
idle_timeout: 15m
max_connections: 100
max_concurrent_requests: 32
```

Equivalent flags: `-idle-timeout 15m`, `-max-connections 100`,
`-max-concurrent-requests 32`.

- `idle_timeout` reaps connections that stop sending commands, bounding file
  descriptors and zombie-diagnosis noise. Pooled Redis clients reconnect
  transparently; a long-idle interactive session is closed and must reconnect.
- `max_connections` refuses connections at the cap; refused sockets are closed
  immediately without being counted.
- `max_concurrent_requests` answers `EMB`/`EMB.MULTI` with `ERR busy ...` while
  at the cap, giving consumers backpressure instead of unbounded queueing.
  Control commands (`PING`, `AUTH`, `EMB.READY`, `EMB.STATS`, `EMB.MODELS`,
  `EMB.INFO`, `EMB.HELP`) always answer so ops can still probe a saturated
  server.

### Observability

`EMB.STATS` reports uptime, total requests, live `connections` and
`active_requests` (the real in-flight count), a per-model breakdown
(requests, avg latency, tokens, errors, memory, pooling), the cache counters,
and the effective `idle_timeout_ms`/`max_connections`/`max_concurrent_requests`
— a 10-second check to classify a CPU/stuck-traffic incident as volume,
saturation, or churn.

`INFO [section...]` renders Redis-format sections; with no argument it returns
all of them:

- **# Server** — `redis_version`, `emb_version`, `uptime_secs`, `process_id`
- **# Cache** — hits, misses, hit rate, evictions, entries, byte usage
- **# Keyspace** — per-model `db0:model=…,keys=…,hits=…,misses=…,hit_rate=…`
- **# Stats** — `total_requests`, `total_tokens`, `total_errors`, `models_loaded`
- **# Clients** — `active_requests`

`CONFIG GET [glob]` reads the live configuration registry (`CONFIG GET cache*`),
and `CONFIG SET` tunes it at runtime — including **live cache resizing**
(`CONFIG SET cache 1GB` evicts immediately) and swapping the `password` (affects
new connections only). Read-only parameters (listen address, TLS, models) are
reported by `GET` but rejected by `SET`. Both require authentication, matching
Redis semantics.

## Clients

The response is raw little-endian float32 bytes, so any Redis client works:

**Ruby:**

```ruby
require "redis_client"

redis = RedisClient.new(port: 6379)
raw = redis.call("EMB", "minilm", "hello world")
emb = raw.unpack("e*")
```

Or use the [`emb`](gems/emb/README.md) gem — connection pooling, proxy, and
multi-model support with automatic float32 decoding:

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

Ruby gems:

- [`emb`](https://rubygems.org/gems/emb) — client library ([README](gems/emb/README.md))
- [`emb-server`](https://rubygems.org/gems/emb-server) — precompiled server binary ([README](gems/emb-server/README.md))

## Development

```bash
just format          # Format all Go code (gofmt + goimports)
just lint            # Linters (golangci-lint + go vet)
just test            # Run tests
just bench           # Run Go benchmarks
just bench-all       # redis-benchmark suite (see BENCHMARK.md)
just bench-ruby      # End-to-end Ruby client harness against a live server
just build           # Build the emb binary
just dev             # Build and run the server
just download-model  # Download a model from HuggingFace
```

Nix provides a reproducible dev shell with Go, ONNX Runtime, golangci-lint,
just, and all CGo configuration:

```bash
nix develop
```

Docker:

```bash
# Run with a model mounted:
docker run -v ./models:/models elcuervo/emb \
  -config /models/config.yaml
```
