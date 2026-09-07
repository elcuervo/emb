## Why

The three-model measurement (Emb gem, 4 threads, int8 testbed, M4) showed scripted embedding of the fused siglip2 CLIP export costs **182 ms p50 / 21 req/s** — and profiling the path found two removable server-side costs: the script rebuilds a **150,528-element Lua zeros table on every request** (the `pixel_values` feed for the fused graph), and a 768-dim vector is replied as **768 individual RESP bulk strings** (≈19 KB of `$nn\r\n0.037665342547944\r\n` framing) instead of one 3 KB float32 buffer. The embed path returns the latter; the script surface currently cannot.

## What Changes

- **`fill` input specs**: `{shape = {1, 3, 224, 224}, fill = 0, dtype = "f32"}` builds a constant-filled tensor in the host without a Lua `data` table — zero allocation round-trip through Lua, no 150k-element table per request. Works in `emb.run` and `emb.run_batch`; `fill = n` covers ones/constants (masks, padding).
- **`emb.math.float32_bytes(vals)`**: packs a Lua number array into a raw little-endian float32 string. Scripted embeddings can return `emb.math.float32_bytes(embedding)` — a single bulk a client decodes with `unpack('e*')`, byte-identical layout to the embed path's replies.
- **Example adoption**: `examples/scripts/siglip2.lua` rewritten to `fill` the `pixel_values` tensor and return packed bytes (normalized); `gems/emb/bench/bench_models.rb` decodes via `unpack('e*')` and re-measures siglip2 (before/after captured: 182 ms p50 baseline).
- No command, reply-shape, or cache-key changes: a bulk-string reply is the same grammar as today.

## Capabilities

### New Capabilities
- `script-tensor-utils`: constant-filled tensor specs (`fill`) for \( \text{emb.run} \)/`emb.run_batch`, and `emb.math.float32_bytes` for byte-compatible vector replies.

### Modified Capabilities
(none — additive host blocks; no config or command changes)

## Impact

- **`internal/script`**: `namedTensorFromLua` accepts `fill` (constant tensor construction, no Lua data table); `emb.math.float32_bytes` host fn (little-endian pack via `encoding/binary`); tests (fill shapes/dtypes in run + batch; byte round-trip).
- **`examples/scripts/siglip2.lua`**: pixel_values via `fill`, embedding reply via `float32_bytes`.
- **`gems/emb/bench/bench_models.rb`**: siglip2 scenario decodes raw bytes (`unpack('e*')`) — client measurement harness, no gem API change needed.
- **Benchmarks**: siglip2 wire p50 before/after (baseline recorded this turn: 182 ms p50, 21 req/s, 4 threads).