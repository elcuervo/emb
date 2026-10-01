<a href="https://emb.is">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/emb-wordmark-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="assets/emb-wordmark-light.svg">
    <img alt="emb — bytes in. vectors out." src="assets/emb-wordmark-light.svg" width="270" height="115">
  </picture>
</a>

A simple yet powerful inference server.

[![GitHub Release](https://img.shields.io/github/v/release/elcuervo/emb?logo=github&color=blue)](https://github.com/elcuervo/emb/releases)
[![Docker Hub](https://img.shields.io/docker/v/elcuervo/emb?logo=docker&color=blue&label=docker)](https://hub.docker.com/r/elcuervo/emb)
[![emb gem](https://img.shields.io/gem/v/emb?logo=rubygems&color=red&label=emb)](https://rubygems.org/gems/emb)
[![emb-server gem](https://img.shields.io/gem/v/emb-server?logo=rubygems&color=red&label=emb-server)](https://rubygems.org/gems/emb-server)

`emb` is a simple yet powerful inference server speaking the Redis protocol. Every Redis
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

## Laya decision models

[Laya](https://github.com/NandhaKishorM/laya) is a multilingual, non-autoregressive System 1 decision engine: typed decisions (`choice`/`score`/`noul`) over any state in one forward pass, with calibrated probabilities. The reference implementations serve three ONNX checkpoints (exports of `convaiinnovations/laya*` in [`codenamev/laya-onnx`](https://huggingface.co/codenamev/laya-onnx)): `english` (ModernBERT-large, 421M, context 512), `multilingual` (mmBERT-base, 322M, context 1024 — 100+ languages), and `typed-decisions` (ModernBERT-large, fine-tuned workflows). Mount one the way the gem does, and preload the preset:

```yaml
# config.yaml
models:
  laya:
    onnx: ./models/laya-english/model.onnx
    tokenizer: ./models/laya-english/tokenizer/tokenizer.json
    script_preload: true
    scripts:
      - path: ./scripts/laya.lua
        config:                 # the checkpoint's envelope, read as emb.script.config
          max_len: 64
          head_max_len: 32
          temperature: [1.6, 1.25, 1.98]
          temperature_by_options: { "choice:2": 1.9 }
```

```bash
# In another terminal (the reply below is the vendored tiny export's own bytes):
EMBSHA=$(redis-cli -p 6379 EMB.SCRIPT LOAD laya "$(cat scripts/laya.lua)")
redis-cli -p 6379 EMB.EVSHA laya "$EMBSHA" 1 \
  '"we were charged twice for the invoice please refund"' \
  '{"department": {"type": "choice", "instructions": "which team", "criteria": {"billing": "invoices, refunds", "technical": "bugs, outages", "other": null}}}'
# → {"answers":{"department":{"action":{"act_probability":0.3582},"choice":"technical","confidence":0,
#     "probabilities":{"billing":0.3333,"other":0.33,"technical":0.3367},"type":"choice"}},
#     "usage":{"input_tokens":32,"output_tokens":0}}
# (The production checkpoints take max_len 512 / head_max_len 192 — 1024 / 192 for
# multilingual — and answer meaningfully; the envelope on the model entry carries them.)
```

Every question in a request is answered in one forward pass. The reply is a JSON bulk with one answer per question id, the gem's payload shapes, and `usage.input_tokens` accounting.

The input is a decision tree drawn as rows — breadth, not depth: each question becomes its own sequence (`[CLS] <question> [SEP]`, every option a `[MASK]` marker, the state at the tail), all rows are batched, and one pass scores every marker at its own position. No question waits on another's answer, so the whole round is one call and one reply (see [the decision](website/demos/laya.html) plate).

Which checkpoint reads a ticket is a routing decision made before the call — the gem's `Laya::Router` detects script and language (Devanagari, Han, Spanish…) and sends non-English tickets to `multilingual`; the same state, questions and config envelope work against any of the three model entries, so a deployment just picks the entry by the ticket's language. In the reference client, that is `Laya.load("convaiinnovations/laya", subfolder: "multilingual")`.

Every answer also carries `action.act_probability`, the act/escalate head's read on the same answer (act on it, or escalate it). The reference gating pattern reads **confidence** against a threshold — `>= 0.85` routes automatically, below goes to a human — because confidence has a true floor (0 for a uniform choice, 0.5 for a yes/no coin flip).

The reference implementations' own question sets are the canonical starting points — `Laya::Presets.triage_questions` / `email_questions` / `guard_questions` / `moderation_questions` / `router_questions` in the gem, byte-identical to upstream `laya.presets.*`; the site demo runs `email_questions` verbatim.

**Wire contract** (parity with [ruby-laya](https://github.com/codenamev/ruby-laya) 0.3.7):

- `KEYS[1]` is the state, serialized Python-style — a plain string passes through; a JSON object/array must use spaces after every comma and colon (`{"from": "a@b", "body": "x"}`, not `{"from":"a@b","body":"x"}`) because the checkpoints were trained on exactly those strings.
- `ARGV[1]` is the questions JSON; criteria label order is significant (marker order maps to label order), so criteria objects are sent as JSON objects (not pre-sorted).
- `ARGV[2]` (optional) overrides the checkpoint's config envelope for one call. The envelope itself — `max_len`, `head_max_len`, `min_seq`, `min_markers`, `temperature` (choice/score/noul), `temperature_by_options` (bucket → value, buckets `2`/`3-5`/`6-10`/`11+`) — is declared once on the model entry's `scripts` config and read by the preset as `emb.script.config`, so a client sends only the state and the questions. Calibration clamps follow the gem: temperatures outside [0.5, 5.0] clamp, non-numeric values answer with 1.0.

`scripts/laya.lua` ports the gem's `build_sequence`, marker accounting, temperature calibration, softmax/confidence and answer assembly; the vendored corpus (`testdata/laya/expected.json`) pins it byte-for-byte over the wire (`TestLayaParityCorpus`). The decision graph is a scripted model like GLiNER: there is no new command — `EMB.EVAL`/`EMB.EVSHA` and the script reply cache (keyed on state + questions + config) do the work. `test-laya.yaml` is the running example, with the vendored tiny export in `testdata/laya/`.

A decision *loop* runs where the model runs. `scripts/snake.lua` is the worked example: it owns the board, a Hamiltonian safety planner and three typed questions, and one `EMB.EVSHA laya <snake-sha> 1 '{"ticks": 96, "board": null}'` returns 96 frames — each with the move probabilities and the shielded result — plus the board to resume from. `scripts/pacman.lua` is the second: its own maze, pellets and four ghosts, the same one-call episode contract. The host learns nothing about either game; a task keeps its own rules the same way. Every decision carries the model call's own `inference_ms` — the host measures `emb.run` around the session run and returns it as a second value — and Pac-Man's planner prefers the shortest path to the nearest pellet and refuses an immediate reversal, so the random-weight checkpoint clears pellets instead of oscillating in place.

## Development

```bash
just format          # Format all Go code (gofmt + goimports)
just lint            # Linters (golangci-lint + go vet)
just test            # Run tests
just deadcode        # Fail on unreachable production functions
just cover           # Cross-package statement coverage (production + whole-repo totals)
just coverage-gate   # Enforce coverage-floors.txt
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
