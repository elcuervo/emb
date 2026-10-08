## Context

Three independent agents reviewed cache correctness, scheduling/performance, and stability. The shared LRU stores text/image embeddings and variable-sized scripted replies. `Cache.Set` copies before admission, skips eviction on replacement, and inserts a too-large entry after evicting everything else. Script keys inline short inputs but encode long inputs as `#<sha256>`; a literal short input can equal that representation.

The existing `script-eval` specification explicitly excludes sibling-dependent per-text results from its cache contract. The earlier review overstated that behavior as a contract violation. Expanding that contract is separate work.

### ROI assessment

Ratings are qualitative engineering judgments, not measured speedups or delivery estimates.

| Priority | Candidate | Benefit and confidence | Effort / risk | Decision |
| --- | --- | --- | --- | --- |
| 1 | Cache budget enforcement | Prevents over-budget occupancy and destructive cache churn; directly established in `internal/server/cache.go:194` | Small / low | Include |
| 1 | Script literal/hash separation | Prevents incorrect cached replies; directly established in `internal/script/cache.go:62` | Small / moderate compatibility work | Include |
| 2 | Repair incomplete model downloads | Weights cause download short-circuit even after best-effort tokenizer download failed (`internal/registry/registry.go:542`, `internal/hfhub/hfhub.go`) | Moderate / setup and restart behavior | Separate stability change |
| 3 | Per-request embedding miss deduplication | Avoids repeated inference for duplicate texts in `embedTexts`; gain depends on traffic | Small / all-unique overhead | Measure before proposing |
| 3 | Available-worker dispatch | Fixed round-robin dispatch can wait on a busy worker while another is idle | Moderate / lifecycle-sensitive | Separate measured change |
| 3 | Batching timer reconciliation | Idle-flush makes timer-start branch unreachable; config/spec semantics need resolution | Moderate / latency-sensitive | Separate investigation |
| 3 | Download cancellation/deadlines | Bounds stalled setup network operations | Moderate / large-download policy | Combine with download repair later |
| 4 | Full ordered-KEYS reply caching | Expands supported script semantics | Moderate / reduced cross-request reuse | Defer contract expansion |

## Goals / Non-Goals

**Goals:** Keep accounted retained cache bytes within every positive budget after writes; distinguish script input representations; prevent old ambiguous keys from serving replies; preserve immutable snapshot values and existing command behavior.

**Non-Goals:** Change per-text script semantics, inference scheduling, metrics definitions, cache locking strategy, snapshot encoding, dependencies, or configuration. Cache accounting remains an approximation of retained entry storage, not a hard process-RSS limit; in-progress snapshots and callers can retain old values.

## Decisions

### Enforce admission in the existing shared Set method

Use the existing mutex to inspect the current budget and compute entry cost with the existing accounting formula. If an entry cannot fit a positive budget by itself, return before copying, changing recency, evicting, or updating counters/generation. For a rejected replacement, retain the old value unchanged.

For an admissible replacement, publish an owned copy, update its byte delta and MRU position, then evict other LRU entries until the budget holds. The replacement itself fits and is MRU, so it survives. Reuse `evictTailLocked` for consistent accounting. Inserts follow the same admission and eviction invariant. Preserve existing non-positive internal budget semantics and snapshot immutability.

This avoids per-caller fixes and new cache abstractions. Removing an old value on rejected replacement was considered but adds needless mutation to a skipped cache write; callers still receive their newly computed response independently of cache admission.

### Domain-separate script key derivation, preserving its outer shape

Add a fixed reply-key derivation-version domain and an explicit input representation discriminator to the existing length-framed metadata hash. Continue inlining inputs up to `CacheKeyInlineLimit`, and digest larger inputs into the bounded tail. Preserve `model:scriptSHA:metadataDigest:tail` so model extraction, flush, and snapshot handling keep working.

The derivation version makes every new script key distinct from its legacy equivalent. The discriminator prevents a digest tail and literal tail from aliasing. Do not bump `script.APIVersion`: the host-function API did not change. Do not change the snapshot file version. Hashing all input tails would also work but unnecessarily removes the existing short-input representation and its compatibility expectations.

### Treat legacy script entries as cold, not as a migration project

Snapshots remain readable. Old script entries may be restored under the existing bounded admission policy, but current requests cannot address them; ordinary eviction removes them. Compatible text/image keys remain usable. No legacy lookup fallback is allowed, since it could return an ambiguous value. New script entries round-trip through snapshots normally.

## Risks / Trade-offs

- Copying admitted values under the cache mutex can extend write critical sections → compare existing cache Set/Get and cached/uncached EMB benchmarks against the unchanged baseline; retain the simplest implementation unless a measured regression requires adjustment.
- Script cache warmup after upgrade → document the intentional cold-cache transition; preserve embedding hits.
- Legacy restored script entries consume budget until eviction → accept bounded temporary occupancy instead of adding a snapshot migration layer.
- Retained snapshot references and allocator overhead can exceed accounted live bytes → state and test the live-cache accounting invariant, not an RSS ceiling.
- Restoring an older binary reintroduces the original bugs → rollback restores prior behavior; retain the original snapshot format and document that rollback is not a correctness mitigation.

## Migration Plan

Deploy as a normal server upgrade. No configuration or client changes are needed. Existing snapshots remain structurally compatible. Document intentional script reply misses after upgrade and confirm text/image hits remain possible. Update pinned key compatibility tests to the new derivation and keep explicit legacy fixtures to prove separation.

## Validation

Use deterministic table-driven tests for oversized new entries, oversized replacements, growing/shrinking replacements, exact-fit boundaries, LRU order, accounting, counters, generation, and immutable snapshot values. Verify collision separation using a payload longer than 256 bytes and a literal `#` plus that payload's SHA-256; test both cache priming orders through the scripted endpoint. Preserve binary safety and the 256-byte boundary.

Use snapshots containing legacy script keys and compatible text/image entries to verify legacy script misses and embedding hits; separately verify new script reply hits after restore. Run focused race checks, full Go tests, lint/vet, and existing cache benchmarks inside `nix develop`. Follow the existing persistence disabled-path benchmark gate (no added allocation/op; median throughput regression at most 2 percent), using an isolated baseline and repeated samples. No inference speedup is claimed.

## Open Questions

None blocking implementation. Deferred candidates need their own workload evidence or contract decisions.
