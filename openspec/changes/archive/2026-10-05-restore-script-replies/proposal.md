# Proposal

## Why

The persisted cache silently discards every **scripted reply** on restore. In
`readSnapshot`, each entry is validated as an embedding vector:

```
internal/server/snapshot.go  len(value) != cur.Dim*4  →  SkippedFingerprint
```

A script reply is a JSON blob of arbitrary length, and a decision model like
`laya-real` reports `Dim = -1` (it has no pooled output), so `len(value) != -4`
is always true. Measured on the deployed sandbox: after a graceful restart,
`cache_restore_skipped_fingerprint: 12`, `cache_restore_entries: 0` — the twelve
entries were exactly the plate's cached replies. The archived `sandbox-service`
requirement *"an entry restored from it answers without re-running inference"*
therefore does not hold for the model that motivated it.

The warm masks the loss by recomputing every payload at boot (~20 s of CPU),
but the persisted cache is doing none of the work it exists for.

A second, related gap: admission of quarantined (lazy-model) entries is
triggered only from the embedding paths (`embedTexts`, the single-embed path).
A script-only model's embedding pool never loads, so even correctly-classified
quarantined replies would never be admitted — and a model whose first request
is a script (rather than an embed) waits for an embed that may never come.

## What Changes

- **Validate by key family, not by one shape.** Restored entries whose key
  addresses a float32 embedding (`txt:` / `img:`) keep the `dim*4` length check;
  opaque entries — script replies — are validated by model fingerprint only. The
  whole-file SHA-256 checksum and the per-entry length bound still apply.
- **Round-trip the model's dimension as signed.** The header stores `Dim` as an
  unsigned 32-bit field, so `Dim = -1` reads back as `4294967295` and every
  fingerprint comparison for that model fails before the key-family check can
  help. Read it as two's-complement; non-negative dimensions are byte-identical,
  so existing snapshots restore unchanged.
- **Admit quarantined entries when the script path first resolves the model.**
  `evalScripted` resolves the model before its cache lookup; admission moves
  there, so a restored reply is published before the lookup that would re-run
  inference. This also covers `Dim > 0` models whose first request is a script.
- **Pin the behavior in the spec** so it cannot regress: a requirement that
  restored validation is per key family, and that script-load admission occurs.
- **No snapshot format change.** Key family is self-describing (`txt:`/`img:`
  prefixes), so no version bump and no migration. A snapshot written before
  this change restores identically; the entries that were dropped now land.

Not in scope: restoring **image** entries with an image dimension different from
the model's configured `Dim` (a separate latent question), and anything about
what gets cached in the first place.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `cache-snapshots`: restored-entry validation becomes per key family (embedding
  length check vs fingerprint-only for opaque replies), and quarantine admission
  is triggered by the model's first script evaluation as well as its first
  embedding.

## Impact

- Code: `internal/server/snapshot.go` (the restore validation branch),
  `internal/server/cache.go` (a key-family helper), `internal/server/script.go`
  (admission before the scripted cache lookup).
- Tests: new restore/admission coverage for a `Dim = -1` model and for an
  opaque key on an embeddable model; the existing snapshot suite must stay green.
- No config, no snapshot version, no wire change. On the sandbox it makes the
  warm's first boot cheaper and the archived restart requirement true.
