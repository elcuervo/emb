# Design: script production runtime

## Context

See `proposal.md` for motivation. The constraints that shape this design come from the current code:

- **The script path deliberately avoids the embedding pool.** `Registry.Resolve` exists so non-embeddable graphs (GLiNER logits) can be addressed by name, and `runScripted` builds hosts from `ModelEntry.ScriptResources()` — a *separate* named-tensor session pool plus its own tokenizer.
- **`emb.run` materializes every output element into Lua.** `onnx.NamedRuntimeSession.RunNamed` allocates inputs and auto-allocates/destroys outputs per call, `namedTensorFromValue` copies the whole output into a Go slice, and `namedTensorToLua` then `RawSetInt`s every element into a Lua table.
- **The input side already has a packed form.** `emb.run` accepts `{shape, bytes, dtype}` and `emb.image.preprocess` returns exactly that, so packed bytes are an established convention — the output side simply never mirrored it.
- **Scripts are invisible.** `runScripted` emits no `MonitorEvent` and bypasses `Pool.Stats()`.

Measured baseline (MiniLM, dev shell, `workers: 1`, `script_workers: 1`, cold cache):

| tokens | `emb.run` + read nothing | `emb.run` + read all | `EMB` |
|---|---|---|---|
| 12 | 1.68 ms | 2.06 ms | 1.44 ms |
| 34 | 3.74 ms | 4.81 ms | 2.97 ms |
| 66 | 6.38 ms | 8.69 ms | 5.02 ms |
| 128 | 11.72 ms | 15.78 ms | 8.78 ms |

A distance script (tokenize + 2 runs + Lua pool + cosine) measured 3.94 ms against 1.89 ms for `EMB` with the same two texts. RSS after the first `EMB.EVAL`: +185 MB with `script_workers: 1`, +984 MB auto-tuned, on a 90 MB model.

## Goals / Non-Goals

**Goals**

- Make the embedding class of scripts (similarity, rerank, cross-modal scoring) within 1.30× of the native embedding path, with no extra model memory.
- Make the raw-tensor class (GLiNER2-style decode) dominated by inference rather than Lua tensor handling.
- Make scripted traffic observable and its resource cost inspectable.
- Add the similarity/distance surface without breaking any existing script.

**Non-Goals**

- Cross-request batching of arbitrary named-tensor evaluations. Coalescing heterogeneous `emb.run` shapes across connections is a different machine from the text batcher; concurrency is served by the session pool and `max_concurrent_requests`.
- A native `EMB.DISTANCE` command. The script surface is the deliverable; a command can be added later over the same `embedTexts` helper if ergonomics demand it.
- Lazy `.data` userdata for `emb.run`. Packed outputs reach the same goal without breaking `ipairs`/`table.concat` users.
- Changing `EMB`/`EMB.MULTI`/`EMB.IMG` reply shapes or the script reply grammar.

## Decisions

### 1. `emb.embed` publishes through the embedding path, not a new one

`emb.embed` resolves the model with `GetOrInit` (creating the embedding pool on first use) and calls a new `Server.embedTexts(entry, model, texts)` helper that owns the cache-read → miss-compute → cache-write sequence currently inlined in `handleEMB`. `handleEMB` is refactored to call the same helper.

This is what delivers the parity budget: the batcher (coalescing with `EMB`), the `model:text` cache (shared entries), the Go-side mean-pool/normalize, and the existing ORT sessions all come for free.

*Alternatives considered.* (a) A dedicated "script" embedding path — rejected: duplicates pooling logic and reintroduces the memory cliff. (b) Exposing pooling as `emb.pool.mean` and keeping raw tensors — rejected as the primary mechanism: it still marshals `seq×dim` into Lua, so it cannot meet the budget; it survives as `emb.math.mean_pool` for models whose pooled output is not the embedding (Decision 4).

### 2. `emb.embed` is bound conditionally, on the `emb.image` precedent

`runScripted` binds `emb.embed`/`emb.image.embed` only when the model declares an embedding configuration (`dim > 0`, pooling ≠ `none`) or an image configuration. The pool is not opened during host construction, only on the first `emb.embed` call. This preserves the `Resolve`-without-pool contract for GLiNER-style models exactly: a model with no embedding config simply has no `emb.embed`.

### 3. `similarity` and `distance` are separate functions with uniform polarity

`emb.similarity(a, b, metric)` returns higher-is-more-similar (`cosine` default, `dot`); `emb.distance(a, b, metric)` returns lower-is-closer (`l2` default, `l2sq`, `cosine` = `1 − cos`). Sum of `similarity(a,b,"cosine")` and `distance(a,b,"cosine")` is 1.

*Alternatives considered.* (a) One function with mixed polarity — the wart identified when this started: CLIP users expect `0.87`, `l2` users expect `0.0`; a single name cannot be honest about both. (b) Cosine-only — leaves the Euclidean case to hand-written loops, which is the status quo we are removing. (c) Returning distances negated so "higher is always similar" — rejected: negative distances are unreadable and break the `1 = similar + distance` identity.

Operands are vectors (arrays or packed float32 strings), never validated against the model dimension, so a script can score vectors it computed itself. Length mismatches and non-multiple-of-4 packed buffers are errors; a zero-magnitude cosine returns `0` rather than erroring (matching `image_zeroshot.lua`'s existing convention).

### 4. Packed outputs mirror the existing packed inputs

`emb.run(spec, {bytes = true, outputs = {...}})` returns `{shape, bytes, dtype}` per tensor — the exact inverse of the accepted input form — and `{outputs = ...}` skips converting unnamed graph outputs entirely. `emb.run_batch` takes the same options.

`emb.math` gains host-side operations that consume either form: `dot`, `cosine`, `l2`, `norm`, `mean_pool`, `cls`, `topk`, `gather`, `slice`, `scale`, `add`, and byte-operand `sigmoid`/`softmax`/`argmax`. `mean_pool` deliberately recomputes masked mean + L2 normalization in the host so a script can reproduce the embedding pipeline's pooling for models whose embedding cannot be exposed as `emb.embed` (for example a fused graph whose pooled output is not the configured embedding).

*Alternatives considered.* (a) Lazy `.data` userdata — rejected as breaking for `ipairs`, `table.concat`, `#`, and deep copies; and per-access metamethod cost would still apply to interpreted loops. (b) Only host math over existing arrays — rejected: the array is already materialized, so the dominant cost remains.

### 5. Script resources become lazy and bounded

`ScriptResources` currently opens its session pool and tokenizer eagerly for any script. Three changes:

1. `runScripted` builds the host table with a *lazy* `emb.run` binding: the first `emb.run`/`emb.run_batch` call triggers `ScriptResources()`.
2. `openScriptResources` reuses the embedding pool's tokenizer when the model has one, instead of loading a second tokenizer.
3. `script_workers` auto-tune is bounded by the embedding pool's worker count, so scripting cannot silently multiply a model's footprint.

*Alternatives considered.* (a) Keep eager creation and only document `script_workers` — rejected: a script that returns a constant should cost zero model memory. (b) Unify the named and narrow sessions — rejected: `RuntimeSession` assumes fixed inputs and a reused output tensor, and GLiNER needs arbitrary named I/O; the two contracts are genuinely different.

> **Superseded (`script-runtime-parity`).** The auto-tuned bound above is now derived from the embedding pool's *actual* session count — one for a batcher pool (the batching default), one per worker otherwise — not from `autoTuneWorkers` alone. An explicitly configured `script_workers` is an **operator override honoured verbatim**, never clamped; only the auto-tuned default is bounded. Image resources are lazy too: `emb.image.info`/`emb.image.preprocess` resolve only the preprocessing plan, and image sessions open on first `emb.image.embed`.

### 6. Reduce per-call tensor cost in `NamedRuntimeSession`

Independent of the packed API: reuse output tensors by shape (mirroring `RuntimeSession.outTensor`) instead of `nil` auto-allocation per call, and marshal from the ORT buffer directly when a packed representation is requested, skipping the intermediate Go copy. This narrows the residual per-call overhead that remains after packed I/O.

### 7. Observability is a prerequisite, not a follow-up

`runScripted` emits `MonitorEvent`s and maintains script counters (requests, errors, latency, per-model split, session footprint) exposed through `EMB.STATS`/`EMB.INFO`. Without this, none of the budgets can be regression-tracked in production and the change cannot be called production-grade.

### 8. Benchmarks are the specification's teeth

The parity budgets are enforced by `go test -bench` benchmarks committed alongside the code and compared to a captured baseline in `just bench`:

- `BenchmarkScriptEmbedParity` — `emb.embed`+`emb.similarity` vs `EMB`, at 8/32/128 tokens.
- `BenchmarkScriptPackedRead` — packed output + host reduction vs no-output run, at 32/128 tokens; derives the per-element overhead.
- `BenchmarkScriptSessionFootprint` — sessions opened for an `emb.embed`-only script vs `emb.run` script.
- `just bench-gliner` gains a reference-script variant that exercises the packed path end to end.

Ratio budgets (1.30×, 1.35×, 0.02 µs/element) are asserted from the benchmark outputs; the raw numbers are recorded in `benchmark-baseline.txt`.

## Risks / Trade-offs

- **[`emb.embed` makes a model's memory profile path-dependent]** → the pool already loads lazily, so this is not new; document preload behaviour and report script session counts in stats.
- **[Packed outputs invite large Lua strings]** → the per-evaluation tensor-element budget counts packed and array forms equally, and a byte bound is added so a single evaluation cannot materialize an unbounded buffer.
- **[Packed `bytes` + host math is a second way to do what `emb.embed` does]** → documented roles: `emb.embed` for embeddings, `emb.math.mean_pool` for pooled outputs that are not the model's configured embedding.
- **[`script_workers` bound could reduce scripted parallelism on memory-rich hosts]** → the bound is the embedding pool's worker count, which is already the tuned value for that model; operators can still raise it explicitly.
- **[New host functions widen the sandbox]** → all additions are pure compute over model/request inputs; no clock, randomness, network, or server state, so the reply cache stays correct. `emb.API_VERSION` is the escape hatch for scripts written against older servers.
- **[Baseline drift]** → benchmarks are ratio-based where possible (script vs native) so absolute machine speed does not gate them; absolute baselines are recorded but the CI gate is the 10% regression threshold.

## Migration Plan

1. Land observability (workstream 1) first; it is additive and immediately useful.
2. Land lazy resources + shared tokenizer + `script_workers` bound: behaviour change is limited to *when* sessions open and *how many*; document it in the changelog. Rollback is a revert with no data migration.
3. Land `emb.embed`/`similarity`/`distance` (additive). Cache keys and reply grammar unchanged, so no cache invalidation is required.
4. Land packed I/O + math (additive; defaults unchanged).
5. Rewrite `gliner2.lua` against the new primitives and re-point the golden test at the rewritten reference; keep the old script under `snippets/` for one release so operators can compare.
6. Docs and `emb.API_VERSION`.

Rollback at any step is a revert; no persisted state depends on the new surface (the reply cache is content-addressed by script SHA, so old and new scripts coexist).

## Open Questions

- Whether to also expose a `emb.similarity` convenience that embeds texts directly (accepting text operands), which would collide with packed byte strings and reintroduce the text/image ambiguity we deliberately avoided. Deferred; the two-line `emb.embed` + `emb.similarity` composition covers it.
- Exact byte bound for packed output materialization; the element budget is the primary guard and a byte bound can be derived once the benchmarks land.
