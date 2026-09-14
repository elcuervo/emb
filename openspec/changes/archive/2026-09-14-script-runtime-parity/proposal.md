# Scripted runtime parity, resource bounds, and leak safety

## Why

The production-scripting change (`2026-09-14-script-production-runtime`) closed the main
performance gap — embedding-class scripts now run at 1.11–1.30× the native `EMB`
command and the packed-tensor path removed the per-element Lua tax — but a review
left three resource/parity defects, one of which violates a requirement that
change added.

Measured on MiniLM (90 MB), default config (batching is **on** by default, so the
embedding pool is a *batcher* holding exactly **one** session):

```
boot                                      RSS=191 MB
after EMB (pool: batcher, 1 session)      RSS=292 MB   script_sessions=0
after one emb.run script                  RSS=1265 MB  script_sessions=10   ← +973 MB
```

1. **Raw-tensor scripts still duplicate the model.** `script_workers` auto-tunes
   to `autoTuneWorkers` (10 here) while the pool holds 1 session, so the archived
   requirement *"a model's scripted session count SHALL NOT exceed its embedding
   pool worker count"* — whose scenario reads *"pool has 1 worker → at most one
   named-tensor session"* — fails in the default configuration. Only
   embed-only/constant scripts are free.
2. **`emb.image.embed` bypasses the image cache.** `EMB.IMG` consults
   `imageCacheKey`; the script binding added by that change has no cache access
   at all. Verified with the fake image session: `EMB.IMG` twice → 1 inference
   run, then `emb.image.embed` on the same bytes → **2** runs. Text shares its
   cache; images do not.
3. **Image resources open eagerly.** `runScripted` resolves
   `entry.ImageResources()` before evaluating, so even `return 1` on an
   image-configured model opens the image session pool.

The same change also introduced new ownership that has no leak contract yet: the
per-session output-tensor cache in `NamedRuntimeSession`, a tokenizer now shared
between the embedding pool and the scripted path, and lazily-created resource
handles. "Scripting is a production path" is only true if those are provably
bounded and provably released.

## What Changes

- **Bound scripted sessions by the pool's real concurrency.** `script_workers`
  unset SHALL open no more named-tensor sessions than the model's embedding pool
  actually holds (1 for a batcher pool, `len(workers)` otherwise). An explicit
  `script_workers` SHALL be honoured as an operator override (not silently
  clamped), documented as trading memory for scripted parallelism.
- **Share the image cache.** Factor `Server.embedImages(entry, model, images)`
  (the image analogue of `embedTexts`) and route both `EMB.IMG` and
  `emb.image.embed` through it, so an image embedded by either path is a hit for
  the other and repeated identical images infer once.
- **Open image resources lazily** inside the `emb.image.*` host closures, so a
  script that never touches the image surface allocates nothing image-related.
- **Explicit resource ownership and leak guarantees** for everything the
  scripted path allocates: named-tensor sessions, the shared tokenizer, cached
  output tensors, and script-source/bytecode caches. Every allocation SHALL be
  released on eviction or on `Registry.Close`, every cache SHALL be bounded by a
  documented constant, and the guarantees SHALL be enforced by leak tests rather
  than by inspection.
- **Metal-parity success criteria.** Scripted evaluation SHALL approach the cost
  of the inference it wraps: the *non-inference* overhead of a scripted
  evaluation (interpreter, host calls, tensor marshalling, reply conversion)
  SHALL be a small bounded fraction of the same graph run, at every supported
  sequence length, for both the embedding class and the raw-tensor class.

All changes are internal or additive; no Lua surface is removed and no reply
shape changes.

## Capabilities

### New Capabilities

- `script-resource-lifecycle`: ownership, bounds, and leak guarantees for every
  resource the scripted path allocates (named-tensor sessions, shared tokenizer,
  cached output tensors, script caches), plus lazy acquisition of the image
  surface.

### Modified Capabilities

- `script-embed`: `emb.image.embed` SHALL share the content-addressed image cache
  with `EMB.IMG`, mirroring the text path's shared-work requirement.
- `script-inference-performance`: the scripted session bound is restated in terms
  of the pool's *actual* session count (with an explicit-override path), and the
  parity budgets gain a metal-parity overhead criterion and a throughput-scaling
  criterion.

## Impact

- **Server:** `internal/server/script.go` (session bound, lazy image resources,
  `emb.image.embed` wiring), `internal/server/image.go` (extract `embedImages`),
  `internal/registry/registry.go` (session-count accessor, `script_workers`
  override semantics, ownership/close paths), `internal/onnx/named.go`
  (output-tensor cache lifetime).
- **Config:** `script_workers` semantics change from "clamped" to "override";
  no new keys. Defaults open strictly fewer sessions than today.
- **Tests/bench:** new leak tests over scripted evaluations and close cycles;
  session-count and image-cache-parity tests; RSS and throughput-scaling
  benchmarks.
- **No client-visible change:** command grammar, reply conversion, cache keys,
  and the Lua surface are unchanged.
- **Operational:** RSS for raw-tensor scripting drops to the embedding pool's
  footprint by default; the memory/parallelism trade-off becomes an explicit
  operator choice.
