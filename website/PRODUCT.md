# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Backend, platform, and application engineers who **already operate Redis** and
need to turn text (and, via scripts, images) into vectors.

The situation: their client, tooling, connection pools, auth, and monitoring are
already Redis-shaped. They need embeddings without adding a second service, a
new SDK, or a new client library to that stack.

The job: *get vectors out of a model, from inside the stack I already have.*

Secondary audience: people self-hosting `emb` directly — running the binary from
`install.sh`, Docker, or the `emb-server` gem.

## Product Purpose

`emb` is a self-hosted text-embeddings server that speaks the Redis protocol.

It exists so that embeddings become one more thing a Redis client can ask for:
`EMB minilm "hello world"` from `redis-cli`, `redis-py`, `redis-rb`, or anything
else that speaks RESP. No proprietary SDK sits between the user and the model.

Success means a team already running Redis can put embeddings in production
without adopting a new protocol, a new client, or a new operational shape.

## Positioning

**The interface is the integration.** `emb` implements the Redis protocol
instead of shipping a client, so any existing Redis client works unchanged —
the SDK is one the user already has.

Supporting mechanisms, all server-side in a single Go binary with ONNX Runtime
embedded:

- Embeddings leave the wire as raw little-endian float32 bytes by default, with
  a self-describing `VALUES` envelope (`dtype` / `shape` / `values`) for RESP3
  clients that cannot unpack binaries.
- Batching (1 ms window, token budget, async tokenization), an in-process LRU
  cache with optional persistent snapshots, and multi-model queries
  (`EMB.MULTI`) are all built in.
- A sandboxed Lua surface (`EMB.EVAL` / `EMB.EVSHA`) lets users define their own
  pre/post-processing around the same model call, cached by SHA1 per model.

## Operating Context

- A running Redis-literate environment: `redis-cli` for smoke tests, existing
  Redis clients in Ruby, Python, or Go, existing pools and timeouts.
- Configuration via YAML (`-config`) or inline flags; models loaded from a local
  ONNX path or auto-downloaded from HuggingFace.
- Distribution: `install.sh` (curl-to-`/usr/local/bin`), Docker Hub
  (`elcuervo/emb`), RubyGems (`emb` client, `emb-server` precompiled binary).
- Operations through Redis-shaped commands: `INFO`, `CONFIG GET`/`SET`,
  `EMB.READY` health checks, `EMB.STATS`, `MONITOR`, and the `emb-top` TUI.
- Evaluation through `BENCHMARK.md` (measured, reproducible `redis-benchmark`
  runs) and the Lua examples in `examples/scripts/`.

## Capabilities and Constraints

Confirmed surfaces: `EMB`, `EMB.MULTI`, `EMB.MODELS`, `EMB.INFO`, `EMB.STATS`,
`MONITOR`, `EMB.READY`, `EMB.EVAL`, `EMB.EVSHA`, `EMB.SCRIPT *`,
`EMB.CACHE.FLUSH`, `EMB.SAVE`, `EMB.HELP`, `INFO`, `CONFIG GET`/`SET`, `AUTH`,
`HELLO`, `PING`.

Constraints and facts future work must preserve:

- **Pre-1.0.** Current version `0.4.0.pre4` (`VERSION`). Interfaces may still move.
- **MIT licensed**, `Copyright (c) 2026 elcuervo`.
- **Platforms:** macOS (Apple Silicon) and Linux (amd64, arm64).
- **RESP2 by default**; RESP3 is opt-in via `HELLO 3`. `INFO` stays a bulk string
  in both, as in real Redis.
- **Terminology is Redis terminology**, deliberately: commands, `INFO` sections,
  `CONFIG`, keyspace framing.
- **Model behavior is auto-detected** (dim, max length, output tensor, pooling)
  from the ONNX graph plus `config.json`; explicit config overrides it.
- **Scripts are pure compute** — `os`, `io`, `require`/loaders, coroutines and
  `math.random*` are stripped, so identical inputs always produce identical
  replies.
- **Snapshot files contain original input text and embedding bytes** and are a
  disposable warm-start optimization, not a durable database.
- **No hosted or managed cloud offering exists.** There is no pricing, tier, or
  account system to describe.

## Brand Commitments

- **Name:** `emb` — always lowercase, including in the wordmark and headings.
- **Tagline:** `TEXT IN. FLOATS OUT.` — used as the masthead tagline, the hero
  annotation, and the footer mark (`TEXT IN. FLOATS OUT.  /  EMB`).
- **Footer sign-off:** `© 2026 emb. Open source, forever.` (2026 confirmed by the
  maintainer; the poster reference still reads 2024 and must not be copied back.)
- **Positioning line the maintainer has committed to:** "A fast embedding server
  that speaks the Redis protocol." / "SAME PROTOCOL. A MORE SEMANTIC WORLD."
- Visual direction is owned by `DESIGN.md` and the poster reference; it is not
  recorded here.

## Evidence on Hand

Real, usable in future work:

- `BENCHMARK.md` — measured throughput, latency, cache hit-rate, and BLOB-vs-VALUES
  numbers, with reproduction commands.
- `examples/scripts/` — five working Lua scripts: `siglip2.lua`, `sst2.lua`,
  `qa.lua`, `rerank.lua`, `gliner2.lua`.
- `gems/emb` — Ruby client with unit/integration specs; `gems/emb-server` —
  precompiled server distribution.
- `cmd/` — server, `emb-top` dashboard, and the performance verification tool.
- `.github/workflows/ci.yml` and `release.yml` — build, test, and release paths.
- `README.md` — the full command, configuration, and operations reference.

Absences that must **not** be fabricated: customers, logo walls, testimonials,
case studies, press mentions, funding, adoption metrics, pricing, service-level
commitments, hosted cloud, or benchmark numbers not present in `BENCHMARK.md`.

The page now loads generated `website/assets/img/terrain-matte.png`, an alpha
cut-out derived from the generated `website/assets/img/terrain-v2.png`; prompts
and provenance for the source are recorded in
`website/assets/img/terrain-v2.md`. The original
`mountain.jpg` remains in the repository but is unused by the page; its
**placeholder pending licensing review** caveat still applies to any reuse.

## Product Principles

1. **The interface is the integration.** If it already speaks Redis, it is a
   client. Never make someone adopt an SDK to get a vector.
2. **Bytes stay bytes.** The default reply is the fastest, most faithful form of
   the number; self-describing envelopes are opt-in, never the cost of admission.
3. **One binary, no ceremony.** Inference, batching, caching, scripting, and ops
   all live server-side; nothing extra to run alongside.
4. **Boring operations win.** Redis-shaped `INFO`, `CONFIG`, and health checks
   mean existing habits, dashboards, and tooling transfer without translation.
5. **Extensible without forking.** The Lua surface is how users bend the pipeline
   to their problem instead of asking upstream to.

## Accessibility & Inclusion

Committed for the web surface (`DESIGN.md`, `website/`):

- Real semantic structure — `<header>`, `<nav>`, `<main>`, `<section>`,
  `<h1>`/`<h2>`/`<h3>`, `<footer>`; the oversized `emb` wordmark is decorative
  and the semantic `<h1>` carries the value proposition.
- Animations honour `prefers-reduced-motion`; no information depends on motion.
- Technical mono annotations stay readable — a **12px desktop / 14px mobile**
  floor, clamped in the stylesheet so no breakpoint can compute below it; the
  smallest text on the 1086px frame and on a 390px phone was measured against
  it.
- Text must meet contrast requirements; the muted grey (`#6B6963`, 4.82:1 on
  the paper) is a known, recurring risk and should be checked against WCAG AA
  (4.5:1) whenever it is used. `#FF5A1F` is 2.74:1 and is a surface colour
  only: text, hover states and focus rings use `--accent-ink` (`#C23D00`,
  4.66:1) instead.
