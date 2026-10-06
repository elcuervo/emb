# Design

See proposal.md — Why.

## Context

Cache keys are self-describing (`internal/server/cache.go`):

```
text:   "txt:<model>:<sha256>"          value = Dim × 4 bytes (fp32)
image:  "img:<model>:<sha256>"          value = Dim × 4 bytes (fp32)
script: "<model>:<sha1>:<sha256>:<text>" value = JSON bytes, arbitrary length
```

`modelOf` already parses these three shapes positionally. Restore
(`readSnapshot`) validated every value as `len(value) == cur.Dim*4`, which is
only true for the first two. A decision model reports `Dim = -1`, so its script
replies always failed the check; and even for an embeddable model the check
rejects script replies (a rank/graph reply is not `Dim*4` bytes).

The quarantine path (`snapshotRestoreResult.Quarantine`) holds entries for
models that were not `Loaded` at restore. Admission (`admitQuarantine`) verifies
the model's file-based fingerprint and publishes the entries. It is called only
from the embedding paths (`embedTexts`, the single-embed path), so a model whose
pool never loads never admits.

## Goals / Non-Goals

**Goals:**
- Script replies survive a save/restore round-trip for every model shape.
- A restored reply is served on the first scripted request, without inference,
  including for a script-only model.
- Existing embedding validation and all restore bounds are unchanged.

**Non-Goals:**
- The snapshot format or version (key family needs no new field).
- Image entries whose image dimension differs from the model's `Dim` — a
  separate latent issue this change deliberately does not touch.
- What is cached, cache eviction, or save scheduling.

## Decisions

### D1 — Classify by key family, not by model dimension

Add one helper (`embeddingCacheKey(key) bool`, true for the `txt:` and `img:`
prefixes) and use it in both restore branches:

- embedding key: `len(value) == Dim*4` must hold (unchanged behavior);
- opaque key: fingerprint is the only gate.

**Alternatives:** skip the length check whenever `Dim <= 0` — insufficient; it
would still drop script replies on embeddable models like `minilm`, whose cache
holds both embeddings and rank/graph replies. Use the model's cache-key shape
from a stored table — more state for the same information the key already
carries.

### D2 — Admit quarantine from the script path

`evalScripted` already resolves the model entry before its cache lookup; call
`s.admitQuarantine(model, entry)` there. That covers the warm (its first payload
now hits restored entries instead of recomputing) and everyday script calls on
any model, including `Dim > 0` models whose first request is a script.

`admitQuarantine` is safe to call when persistence is off: `s.quarantine` is nil,
the map lookup misses, and it returns. It calls `entry.Fingerprint()`, which is
memoized and only hashes files; it does not require the pool.

**Alternative:** admit when script *resources* open (`openScriptResources`) —
correct too, but it fires after the cache lookup in `evalScripted`, so the
request that triggers admission would still recompute. Admitting before the
lookup is what makes the restored reply serve.

### D3 — Read the header dimension as signed

The header writes `uint32(model.Dim)`, so a `-1` dimension is stored (correctly)
and read back as `4294967295`. Both restore branches compare it to the
configured `Dim`, so a dimension-less model failed there before the key-family
check ever mattered. Read the field as `int32`; `uint32`/`int32` share a width,
non-negative values are byte-identical, and an old snapshot's `4294967295`
decodes to `-1`, so this is compatible in both directions with no version bump.

**Alternative:** bump the format and store a signed field with a per-model flag —
unnecessary, since two's-complement already carries the sign in the same bytes.

## Risks / Trade-offs

- **An opaque value is no longer length-validated against the model.** The
  value's integrity still comes from the whole-file SHA-256 checksum, its length
  is bounded by `readSized` and the restore budget, and a fingerprint match means
  the same model bytes produced it. → No new corruption surface; the removed
  check could not hold for these keys in the first place.
- **Admission does file hashing on the first script call.** The fingerprint is
  memoized per model and the quarantine bucket is deleted on admission, so the
  cost is once per model per process. → Same cost the embed path already pays.
- **Existing snapshot tests use `model:...` keys as stand-ins for embeddings.**
  They keep passing because their fingerprints match; their dim-shaped comments
  become stale. → Add explicit family tests rather than rewriting them.

## Migration Plan

None. The change is read-side; an existing snapshot restores identically except
that previously-dropped script replies now land. Deploy, then verify a scripted
reply survives a graceful restart on the sandbox and that a first script call
is a cache hit.

**Verified on the deployed sandbox (2026-10-06).** The failure was layered, and
the fix addresses all three: the unsigned header dimension, the embedding-only
length check, and admission from the embedding paths only.

| | before | after |
|---|---:|---:|
| restored entries (laya-real) | 0 (`skipped_fingerprint: 12`) | 9 quarantined, 0 skipped |
| warm's scripted calls | 9 misses | **9 hits, 0 misses** |

The observable is `cache_restore_quarantined > 0` with `cache_restore_entries = 0`
(the model is lazy, so entries are admitted on the first script evaluation, not
during restore) and then a warm that takes zero inference.
