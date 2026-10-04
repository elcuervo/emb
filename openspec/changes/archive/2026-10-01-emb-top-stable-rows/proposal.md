# Proposal

## Why

`emb-top`'s per-model rows still jump every poll. The client-side sort was
removed, but the real cause is the server: `Registry.List()` ranges over a Go
map, so `EMB.MODELS` returns a different order on every call, and `emb-top`
rebuilds its row list from that reply each poll. The dashboard also has room to
be clearer about per-model traffic: the header repeats the health line, and the
heatmap encodes intensity in color alone with no per-row number.

## What Changes

- `EMB.MODELS` SHALL return models in a deterministic order (by name) so the
  listing is stable across calls. Other listing consumers already sort
  defensively; this fixes the source.
- `emb-top` SHALL keep its own **first-seen model order**, independent of the
  server's enumeration order: existing rows never move, new models append,
  removed models drop out. Traffic can never reorder a row.
- `emb-top`'s activity heatmap SHALL show each model's current req/s beside its
  strip, and its legend SHALL spell out the scale and time direction.
- The `emb-top` header SHALL drop the values now duplicated by the health line
  (req/s, p95, connection state) so the banner is the single status surface.

## Capabilities

### New Capabilities

_(none)_

### Modified Capabilities

- `emb-cmds`: `EMB.MODELS` gains a deterministic ordering guarantee.
- `emb-top-dashboard`: per-model rows follow a stable first-seen order rather
  than the server's enumeration order; heatmap rows show each model's current
  req/s.

## Impact

- `internal/registry/registry.go` — `List()` returns models sorted by name.
- `cmd/emb-top/main.go` — first-seen model reconciliation; header/heatmap
  rendering.
- Tests: `internal/registry` ordering test, `cmd/emb-top/main_test.go`,
  `hmcheck_test.go`.
- `website/tools/topviz/` — the recorded plate is re-run after the UI change.
- No new dependencies; `emb-top` stays a pure RESP2 client.
