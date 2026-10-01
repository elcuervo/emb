# Design

## Context

See `proposal.md - Why`. `Registry.List()` builds its slice by ranging the
`models` map, so Go's per-call map iteration randomization makes `EMB.MODELS`
return a different order every call. `infoSnapshot()` and `configModels()` already
sort defensively; `handleMODELS` does not, and `emb-top` rebuilds its row list
from that reply each poll.

## Goals / Non-Goals

**Goals**

- `EMB.MODELS` is deterministic for every client.
- `emb-top` rows can never move because of traffic or the server's enumeration.
- Each model's current traffic is legible from its own row.

**Non-Goals**

- Preserving config-file declaration order (the registry stores a map; name
  order is the deterministic choice).
- New flags or dependencies.

## Decisions

### 1. Sort in `Registry.List()` (server)

Return models sorted by name. Every caller stays correct: `Fingerprints` and
`FingerprintState` build maps, `infoSnapshot`/`configModels` already sort,
`handleSTATS` per-model output becomes deterministic too.

- *Alternatives:* sort only in `handleMODELS` — leaves other listing commands
  non-deterministic and keeps the landmine; keep an insertion-ordered slice in
  the registry — more state and ordering semantics for no extra value.

### 2. First-seen order in `emb-top` (client)

Stop rebuilding `modelOrder` from `res.Models` every poll. Keep a client-side
list: drop names no longer announced, append newly announced names, and poll
`EMB.INFO` in that order. Rows never move while a model stays loaded, so the
tool is stable even against a node whose `EMB.MODELS` is unordered.

- *Alternatives:* trust the server now that it is sorted — no protection against
  an older/other node; alphabetical — reorders existing rows when a new model
  sorts before them.

### 3. Heatmap legibility

Show each row's current req/s after its strip (fixed width, right-aligned) and
replace the terse legend with one naming the scale and direction
(`req/s per model · older ░…█ newer`), plus `(+N more)` when rows are capped.
Drop req/s, p95, and connection state from the header, which the health line
already reports.

## Risks / Trade-offs

- **Name-sorting changes RESP2 `EMB.MODELS` output order** → not previously
  documented or asserted, and RESP3 maps are unordered; a spec scenario now
  pins it.
- **First-seen order is arbitrary on the first poll** → acceptable; it is stable
  thereafter, and the server now returns name order anyway.
- **Heatmap loses strip width to the new number** → `hotW()` shrinks by the
  column width so the panel keeps its footprint.
- **Recorded plate is now stale** → re-run `just website-topviz`.
