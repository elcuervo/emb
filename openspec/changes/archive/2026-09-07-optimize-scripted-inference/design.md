# Design: optimize-scripted-inference

## Context

Baseline measurements (Apple M4, int8 GLiNER2, 4 intra threads, warm session): 15-150 ms per extraction by text length; 10→180 words is the dominant cost axis (~8×); decode ≈ 8% of end-to-end; wire path adds ~40 ms/request over raw eval (Lua compile + state + conversion); concurrent distinct-text requests scale only 1.5× for 4 connections (single mutex-serialized ORT session per model); multi-text `EMB.EVSHA` runs N serialized inferences; first request pays ~300-500 ms session creation. See proposal.md and `internal/script/gliner_bench_test.go` (plus `just bench-gliner`).

Constraint: the script surface stays the same shape (blocks, not adapters), reply shapes and cache semantics are frozen — this change only makes the *same work* faster. Scripts remain deterministic pure compute.

## Goals / Non-Goals

**Goals:**
- Concurrency scaling for distinct-text scripted requests (session pool).
- One inference call per multi-text request (batched block + example adoption).
- Remove per-request Lua recompilation; clear cold-start latency via preload.
- Measured before/after via the existing benchmark.

**Non-Goals:**
- Long-document chunking (`emb.text.chunks`) — deferred rung; GLiNER2's own repo does chunking outside the graph, and it composes on top of these blocks later.
- Changing reply shapes, cache keys, or the conversion grammar.
- KV-cache-style decoder acceleration (seq2seq remains expressible-but-slow).
- Pooling Lua states across evaluations (isolation-by-fresh-state stays; the compile cache keeps the win without the risk).

## Decisions

### D1: Session pool — mirror the embed worker pool

`ModelEntry` grows a pool of `NamedRuntimeSession`s: `script_workers` workers created from the same session factory, selected round-robin under a mutex (each session already serializes its own runs, so a ring + per-session lock is enough; no queueing beyond steady state). Auto-tune reuses the embed path's RAM heuristic (availMem/2 ÷ per-session estimate). `ScriptResources.Session` becomes the pool; the `RunNamed` host hook picks the next session per `emb.run`/`emb.run_batch` call. Sessions close on registry close. Rationale over alternatives: a shared mutex over one session (current) bounds throughput to one run at a time; per-request session creation is far too slow; a full work-queue adds no value when runs are uniform.

### D2: `emb.run_batch` — merge in the host, no new ORT layer

`emb.run_batch(items)` validates equal tensor names/dtypes across items, pads each named tensor to the per-dimension max (zero-fill), concatenates along the batch axis, and issues ONE `RunNamed` against a pooled session. Outputs split back along the batch dim into per-item maps (slice [i*n, (i+1)*n) per element). This reuses `RunNamed` unchanged — the merge/split lives in the Lua-facing host. GLiNER fits immediately: `label_positions`/`label_mask` are identical across items, `text_lengths` varies per item (concat on dim 0), `input_ids`/`attention_mask`/`words_mask` pad along seq. The example `gliner2.lua` collects all KEYS texts, builds one batch, decodes each item — same replies, verified by the unchanged golden test (single text = batch of one).

Caveat: padding to the longest seq in a batch means a mixed-length batch costs ~max length, not sum — the intended trade. Measured on the int8 testbed (M4, intra=4, ~20-word texts, labels 3): batch-8 is neutral vs serial (268ms vs 279ms per 8 texts) because equal-length padding cancels the savings; batch-16 is 2.3x faster (331ms vs 777ms) once ORT's intra-op parallelism amortizes batch processing. So run_batch pays off at meaningful batch sizes and is architecture-enabling (one session call, per-text caching) even when neutral.

### D3: Compiled-script cache — proto per (model, SHA), fresh state stays

gopher-lua separates compilation from instantiation: `lua.CompileString(src)` → `*FunctionProto`, then `NewFunctionFromProto(proto)` per fresh state. The script cache (per model, sha) stores the compiled proto alongside the source; `EMB.EVAL` (new source) compiles and caches, `EMB.EVSHA` reuses. `EMB.SCRIPT FLUSH` clears both. The sandbox, budgets, and fresh-state isolation are untouched — same bytecode, same capabilities, no cross-request state. This is why a pooled-state approach was rejected: it would trade the isolation guarantee for marginal parsing savings.

### D4: `script_preload` — warm resources at load

`LoadModel` calls `ScriptResources()` when `cfg.ScriptPreload` is set, dropping first-request session creation (~300-500 ms for the int8 testbed, worse for fp32) into startup, which is the pattern `preload: true` already establishes for the embed pool. It does not create sessions beyond `script_workers` — the pool size still governs steady state.

### D5: Measurement contract

`just bench-gliner` gains a multi-text batched cell (e.g., 8 × 20-word texts: serial baseline vs batched) and a concurrency cell. Acceptance is relative: batched multi-text must beat the serial sum, and 4-connection distinct-text throughput must scale roughly linearly with `script_workers` up to cores. The Go benchmark (which bypasses the wire) plus the wire probe from the baseline turn are the before/after instruments.

## Risks / Trade-offs

- **RAM:** each scripted session holds a full model copy (int8 ≈ 400-500 MB runtime). → Auto-tune mirrors the embed heuristic; explicit `script_workers` overrides; document the cost.
- **Batch padding pathology:** a request mixing 10- and 500-token texts pays ~500-token cost for all. → Padded cost ≈ max(seq); scripts control batching (they choose when to batch), and the cache still derisks repeat calls; document in the example.
- **Compile-cache invalidation:** a proto cached under a SHA is immutable by construction (source-addressed), so staleness is impossible; FLUSH clears evicted/edited states. → Content-addressed, no TTL needed.
- **Pool sizing vs correctness:** more sessions mean more concurrent ORT runs per model; ONNX Runtime shares the intra-op thread pool per session, so >cores sessions oversubscribe. → Cap pool at GOMAXPROCS like the embed path, let auto-tune respect RAM first.

## Migration Plan

Additive: config keys default to current behavior (1 session, no preload, per-request compile). Deploy order: (1) compiled-script cache + tests; (2) session pool + `script_workers`/`script_preload` config + tests; (3) `emb.run_batch` + host tests; (4) example `gliner2.lua` batched adoption (golden + wire tests must stay green); (5) benchmark cells + sweep. Rollback: revert commits; without the keys configured, behavior equals the pre-change baseline.

## Open Questions

(Deferrable without changing specs, approach, or task breakdown.)

- Expose per-scripted-session stats (requests/latency per worker in `EMB.INFO`/`EMB.STATS`) — currently scripted runs don't update pool stats at all; adding them is a follow-up.
- Whether auto-tune should also cap by intra-op thread count (cores−2 like embed intra; sessions × intra threads could oversubscribe).