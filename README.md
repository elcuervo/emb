# emb

A simple yet powerful text embeddings generator.

[![GitHub Release](https://img.shields.io/github/v/release/elcuervo/emb?logo=github&color=blue)](https://github.com/elcuervo/emb/releases)
[![Docker Hub](https://img.shields.io/docker/v/elcuervo/emb?logo=docker&color=blue&label=docker)](https://hub.docker.com/r/elcuervo/emb)
[![emb gem](https://img.shields.io/gem/v/emb?logo=rubygems&color=red&label=emb)](https://rubygems.org/gems/emb)
[![emb-server gem](https://img.shields.io/gem/v/emb-server?logo=rubygems&color=red&label=emb-server)](https://rubygems.org/gems/emb-server)

<a href="https://emb.is">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/emb-wordmark-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="assets/emb-wordmark-light.svg">
    <img alt="emb — bytes in. vectors out." src="assets/emb-wordmark-light.svg" width="270" height="115">
  </picture>
</a>

`emb` is a text-embeddings server speaking the Redis protocol. Every Redis
client — `redis-cli`, `redis-py`, `redis-rb`, … — can call it with no special
library. Embeddings come back as raw float32 bytes by default:

```bash
redis-cli EMB minilm "hello world"
# → \x7c\x8e\x80\xbd...   (384 float32s × 4 bytes)

# RESP3 clients can ask for a self-describing decimal reply instead:
redis-cli -3 EMB minilm VALUES "hello world"
# → dtype FLOAT / shape [1 384] / values [-0.1974, 0.1776, ...]
```

## Features

- **Redis protocol** — works with any Redis client. RESP2 by default, RESP3 via
  `HELLO 3`. Embeddings are compact little-endian float32 bytes, or a readable
  `VALUES` envelope for clients that can't decode raw floats.
- **ONNX Runtime** — CPU/GPU inference through CGo, with optional int8
  quantization.
- **HuggingFace integration** — auto-download models and auto-detect dim,
  max_length, output tensor, and pooling from the ONNX graph + `config.json`.
- **Smart batching** — a 1 ms window coalesces concurrent requests into shared
  ONNX runs, with a token budget and async tokenization (on by default).
- **Embeddings cache** — in-process LRU with per-model stats; sized in bytes,
  percentages, or `auto`, and optionally snapshotted across restarts.
- **Multi-model queries** — `EMB.MULTI` calls different models in one command,
  with MGET-style partial failures.
- **Image embeddings** — `EMB.IMG` / `EMB.IMGMULTI` take raw JPEG/PNG/GIF/WebP
  bytes (no base64, no URLs), decode and preprocess server-side, and return
  embeddings in the same `BLOB` / `VALUES` grammar.
- **Lua scripting** — pre/post-process around the model call: custom
  tokenization, pooling, argmax/softmax, similarity, and more.
- **Ops-ready** — Redis-style `INFO` and `CONFIG`, `EMB.READY` health checks,
  connection lifecycle knobs, full server stats, and the `emb-top` dashboard.

## Contents

| Document | What's in it |
|---|---|
| [Commands](docs/commands.md) | Every command, reply formats (`BLOB` / `VALUES`), RESP3 negotiation |
| [Configuration](docs/configuration.md) | Config file, model options, images, batching, cache, snapshots |
| [Scripting](docs/scripting.md) | Lua `EMB.EVAL` / `EMB.EVSHA` surface and examples |
| [Operations](docs/operations.md) | Health checks, limits, observability, `emb-top` |
| [Clients](docs/clients.md) | Ruby, Python, and Go recipes |
| [Development](#development) | Build, test, and dev-shell commands |

## Install

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | sh
```

Installs to `/usr/local/bin` (set `EMB_INSTALL_DIR` to change the target):

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | EMB_INSTALL_DIR=~/.local/bin sh
```

Platforms: macOS (Apple Silicon) and Linux (amd64, arm64). You can also install
the [`emb-server`](https://rubygems.org/gems/emb-server) gem and run `emb`
directly.

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
| `EMB <model> [BLOB\|VALUES] <text> [text...]` | Embed one or more texts |
| `EMB.MULTI [BLOB\|VALUES] <model> <text> [<model> <text>...]` | Embed across different models in one call; per-pair nulls on failure |
| `EMB.IMG <model> [BLOB\|VALUES] <bytes> [<bytes>...]` | Embed raw JPEG/PNG/GIF/WebP bytes (URLs rejected) |
| `EMB.IMGMULTI [BLOB\|VALUES] <model> <bytes> [<model> <bytes>...]` | Embed images across different models |
| `EMB.MODELS` | List loaded models with dimensions and status |
| `EMB.INFO <model>` | Model details, requests served, latency, live cache stats |
| `EMB.STATS` | Uptime, requests, connections, per-model breakdown, RSS, CPU, goroutines |
| `MONITOR [seq] [limit]` | Recent completed-request events from a bounded ring |
| `EMB.READY` | Health check: `+OK` or `-ERR <reason>` |
| `EMB.EVAL` / `EMB.EVSHA` | Evaluate a Lua script inline / from the script cache |
| `EMB.SCRIPT LOAD\|EXISTS\|FLUSH` | Manage cached scripts |
| `EMB.CACHE.FLUSH [model]` | Drop all cached embeddings, or one model's |
| `EMB.SAVE` | Trigger an asynchronous cache snapshot |
| `EMB.HELP` | Command reference |
| `INFO [section...]` | Redis-style INFO sections |
| `CONFIG GET [glob]` / `CONFIG SET` | Read or live-tune runtime settings |
| `AUTH <password>` | Authenticate the connection |
| `HELLO [2\|3]` | Negotiate the RESP version |
| `PING` | PONG |

Reply formats, `EMB.IMG` semantics, and RESP3 differences are documented in
[Commands](docs/commands.md).

## Development

```bash
just format          # Format all Go code (gofmt + goimports)
just lint            # Linters (golangci-lint + go vet)
just test            # Run tests
just deadcode        # Fail on unreachable production functions
just cover           # Per-package statement coverage + total
just bench           # Run Go benchmarks (just bench-all for the redis-benchmark suite)
just build           # Build the emb binary
just dev             # Build and run the server
just download-model  # Download a model from HuggingFace
just verify-harness  # Unit-test the shared verification harness (no server/model/ONNX)
just verify-embeddings # Compare served embeddings to a Python reference
just verify-emb-multi  # EMB.MULTI byte-equality vs sequential EMB
```

Everything runs inside the Nix dev shell, which provides Go, ONNX Runtime,
golangci-lint, just, and all the CGo configuration:

```bash
nix develop
```

Docker:

```bash
docker run -v ./models:/models elcuervo/emb -config /models/config.yaml
```

See [BENCHMARK.md](BENCHMARK.md) for benchmarks and
[examples/kitchensink/](examples/kitchensink/) for a full application that
combines `emb` with Redis vector search.
