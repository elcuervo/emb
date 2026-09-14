# Make scripting a production inference path

## Why

Scripting is the only way to serve non-embedding models (GLiNER2 extraction, reranking, QA) and one-off operations, and today GLiNER2 already ships as a script. But scripts cannot be used in production: a scripted evaluation is a serial side-channel rather than a peer of the embedding path.

Measured on MiniLM (dev shell, `workers: 1`, `script_workers: 1`, cache cold):

| workload | scripted | native | ratio |
|---|---|---|---|
| similarity / distance (2 texts) | 3.94 ms | 1.89 ms | 2.1× |
| `emb.run` + read, 128 tokens | 15.78 ms | 8.78 ms | 1.8× |
| `emb.run` + read *nothing*, 128 tokens | 11.72 ms | 8.78 ms | 1.3× |

The overhead is not evaluation (a no-op script costs 0.071 ms) — it is **per-element tensor materialization**: every output value is copied ORT→Go→Lua and then pooled in interpreted Lua. A script that reads only `.shape` still paid the full marshal.

Three further gaps make scripts unusable as a production path:

- **No embedding primitive.** Scripts can address a model but cannot obtain a pooled, normalized, cached embedding, because the script path deliberately uses `Registry.Resolve` (no embedding pool) so GLiNER-style graphs work.
- **A memory cliff.** Scripts open a *second* ORT session pool, eagerly, on the first `EMB.EVAL`: **+185 MB** (`script_workers: 1`) to **+984 MB** (auto-tune, ~10 sessions) for a 90 MB model. Native embedding commands cost +0.
- **No observability.** `runScripted` emits no `MONITOR` event and bypasses `Pool.Stats()`, so `MONITOR`, `EMB.INFO` and `EMB.STATS` never see scripted traffic.

Finally, the API has no similarity primitive — the operation that started this investigation — so every user re-implements cosine in Lua on top of marshalled tensors.

## What Changes

- **`emb.embed`** — pooled, normalized, cached, batched embeddings for scripts, routed through the model's existing embedding pool (so it shares the batcher, the `model:text` cache, and the ORT sessions). Bound only for embeddable models, mirroring the existing conditional `emb.image` binding.
- **`emb.similarity(a, b [, metric])`** — higher is more similar; `cosine` (default), `dot`. **`emb.distance(a, b [, metric])`** — lower is closer; `l2` (default), `l2sq`, `cosine` (`1 − cos`). Both accept vectors as Lua number tables or packed float32 byte strings. Polarity is uniform within each function.
- **`emb.image.embed`** — pooled image vectors via the existing image resources, so text↔image scoring is symmetric with text↔text.
- **Packed tensor I/O.** `emb.run`/`emb.run_batch` gain opt-in `{bytes = true}` (packed float32 Lua string) and `{outputs = {...}}` (selective) result forms, mirroring the `bytes` form the input side already accepts.
- **`emb.math` over packed buffers.** `dot`, `cosine`, `l2`, `norm`, `mean_pool`, `cls`, `topk`, `gather`, `slice`, `scale`, `add`, and byte-operand forms of `sigmoid`/`softmax`/`argmax` — all executed in Go, never per element in Lua.
- **Lazy script resources.** Named-tensor sessions open on first `emb.run`/`emb.run_batch`, not on the first script; the tokenizer is shared with the embedding pool; `script_workers` auto-tune is bounded by the embedding pool size.
- **Script observability.** `MONITOR` events and per-model `EMB.STATS`/`EMB.INFO` counters for scripted evaluations, with eval-vs-inference time split.
- **`emb.API_VERSION`** for script-side capability negotiation.
- **Docs and reference implementations.** A script API reference and a "production scripts" guide; `gliner2.lua` promoted from demo to maintained reference implementation and rewritten against the new primitives.

All additions are opt-in or new; existing scripts execute with identical replies and cache keys.

## Capabilities

### New Capabilities

- `script-embed`: pooled embedding primitive, cross-modal image embeddings, and the similarity/distance methods over vectors or packed bytes.
- `script-tensor-io`: packed (`bytes`) and selective outputs for `emb.run`/`emb.run_batch`, and Go-side math over packed buffers.

### Modified Capabilities

- `script-eval`: sandbox host-function whitelist gains `emb.embed`/`emb.image.embed`/`emb.similarity`/`emb.distance`; `emb.math` gains byte-operand and reduction operations; `emb.API_VERSION` and the determinism contract for the new primitives.
- `script-inference-performance`: `script_workers` auto-tune bounded by the embedding pool, lazy session/tokenizer creation, and the new scripted-inference parity budgets (latency, memory, per-element tax).
- `server-stats-observability`: scripted evaluations counted in `MONITOR` and `EMB.STATS`/`EMB.INFO`, including scripted request/error/latency fields and script resource footprint.
- `product-docs`: script API reference and production scripting guide; docs state which example scripts are maintained.

## Impact

- **Lua surface (additive):** `emb.embed`, `emb.image.embed`, `emb.similarity`, `emb.distance`, `emb.math.*` additions, `emb.run`/`emb.run_batch` option table, `emb.API_VERSION`.
- **Server:** `internal/script/{host,math,batch}.go`, `internal/server/{script,info,server}.go` (cache-aware embed factoring shared by `EMB` and `emb.embed`), `internal/registry/registry.go` (lazy/shared script resources), `internal/onnx/named.go` (output buffer reuse).
- **Config:** `script_workers` default becomes bounded by the embedding pool; no new required keys.
- **Clients:** none. `EMB.EVAL`/`EMB.EVSHA`/`EMB.SCRIPT *` grammar, reply conversion, and cache keys are unchanged.
- **Tests/bench:** new benchmarks for parity budgets; GLiNER reference-script golden test; existing script tests remain green.
- **Docs/site:** README scripting section, `website/docs/` script reference.
