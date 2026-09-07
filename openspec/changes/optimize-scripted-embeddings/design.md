# Design: optimize-scripted-embeddings

## Context

Measurement (M4, int8 fused siglip2 CLIP, 4 client threads, `script_preload` + 4 sessions): scripted embedding p50 **181.9 ms**, p99 419 ms, 20.6 req/s aggregate. Two server-side costs identified in the script's hot path: (1) `pixel_values` needs a 1×3×224×224 = 150,528-element float tensor, and the example builds it as a Lua array table on every request — 150k `RawSetInt` calls plus the Go-side conversion at the host boundary; (2) the embedding reply is 768 Lua numbers → 768 separate RESP bulks (~19 KB of framing + strconv per element), versus the embed path's single 3 KB float32 buffer.

Constraint: no command/reply-shape/cache changes; the outcome is *the same bytes as the embed path* on the wire.

## Goals / Non-Goals

**Goals:**
- Constant-filled tensors built host-side (no Lua data table round-trip).
- Scripted embedding replies as a single raw float32 bulk, byte-identical to the embed path.
- siglip2 measured improvement over the recorded 182 ms p50 baseline.

**Non-Goals:**
- A text-only siglip2 ONNX (repo-side; the fused export stays the mount).
- Client/gem API changes (unpacking works via existing bulk-string replies).
- Changing the number-array reply style for extractive models (gliner2 hashes stay as-is — their values are few).

## Decisions

### D1: `fill` is a spec field, not a `emb.tensor` module

A `{shape, fill, dtype}` input spec lets `namedTensorFromLua` allocate the flat buffer directly from the shape (Go `make` + constant) with zero Lua participation beyond parsing the shape table. Rationale vs alternatives: a `emb.tensor.zeros(shape)` host fn would still have to return a Lua table to feed `emb.run` (defeating the purpose); a dedicated run variant duplicates the tensor pipeline. `fill` composes with everything already there (`dtype` override included) and with `emb.run_batch` (per-item constant tensors merge trivially). `data`/`fill` exclusivity is enforced with an explicit error rather than precedence.

### D2: `emb.math.float32_bytes` — little-endian pack, no unpack

`encoding/binary.LittleEndian.AppendUint32(math.Float32bits(v))` per element into one Go buffer → Lua string. Little-endian matches the embed path's `unpack('e*')` contract (verified by the gem's `unpack('e*')` decode). No reverse block (`float32_to_array`) for v1 — clients needing structured numbers can keep the numeric reply; the packed form targets the embedding use case.

### D3: siglip2 example + measurement adoption

`examples/scripts/siglip2.lua` becomes: `pixel_values` via `fill = 0, dtype = "f32"`, normalize (L2), `return emb.math.float32_bytes(vec)`. `bench_models.rb` siglip2 scenario decodes `.unpack('e*')` (a single bulk arrives as a String through `parse_script_reply`). The before number is this turn's recorded run; after is measured in the change's benchmark task.

## Risks / Trade-offs

- **`fill` abuse**: constant tensors are only a convenience — correctness (masking semantics) is unchanged; scripts own masking. No new surface for misuse beyond a data-less spec.
- **Byte replies lose in-band structure**: a packed embedding is opaque; clients must know the dim. Mitigation: it's opt-in per script, and the embed path already works this way (dim from EMB.MODELS or script contract).
- **Big-endian hosts**: little-endian is the RESP tradition here (gem `unpack('e*')`); documented, not configurable.

## Measured result

Recorded in the change's benchmark task (M4, GOMAXPROCS=6, quiet ~2 load, 4 threads × 50 texts): after **138.3 ms p50 / 24.2 req/s** vs the 181.9 ms p50 / 20.6 req/s baseline (~24% p50 reduction). Interleaved NEW-vs-OLD A/Bs on this machine were load-sensitive (a concurrent worktree agent spiked load to 6–19 during the session): 168 vs 169 ms p50 in the quiet window (within noise), 396 vs 430 ms mid-load. The wins are real but dominated by machine-state variance on this testbed; the single-bulk reply and host-side fill remove the server-side per-request costs as designed.

## Migration Plan

Additive host blocks; nothing existing changes. Order: (1) `fill` support + tests (run + run_batch); (2) `emb.math.float32_bytes` + round-trip tests; (3) siglip2.lua rewrite; (4) bench harness decode + before/after measurement; (5) sweep/docs. Rollback: revert; scripts keep working with `data`-style tables unchanged.

## Open Questions

(Deferrable.) Whether `emb.math.float64_bytes`/`int64_bytes` join the pack for completeness when a second use case appears.