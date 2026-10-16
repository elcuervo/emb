# Verification notes

Recorded observations from running the change against this machine. These are
evidence, not a spec: nothing here is asserted by CI.

## The check fails on the tree that shipped the defect

`python3 website/tools/published-tree.py` on the tree before the fleet take was
re-recorded, with the new check in place:

```
published-tree: 78 served, 91 ignored, 2 platform config, 32 sandbox service file(s)
published-tree: the published tree does not match what we mean
  drifted:      website/docs/index.html plays ../assets/cast/emb-top-e348a08c.cast, labelled emb-top v0.4.3.pre6
  drifted:      website/docs/index.html plays ../assets/cast/emb-top-cluster-29458d0f.cast, labelled emb-top v0.4.3.pre5
  drifted:      website/index.html plays assets/cast/emb-top-e348a08c.cast, labelled emb-top v0.4.3.pre6
```

Every plate is named with the version it declares, so the take to re-record is
the one the message points at. The two pre6 plates are the single-node take on
both surfaces; the pre5 one is the fleet take, still the 2026-10-14 recording.

## The check is falsifiable in both directions

Run on a scratch copy of the tree (`rsync -a` of the working tree, no `.git`,
`models` or `bin`), so nothing here is a change to the repository.

A copy whose fleet label is set to the single-node version passes:

```
published-tree: 3 takes agree on emb-top v0.4.3.pre6
published-tree: ok (78 served paths, one origin at https://emb.is)
```

A copy whose fleet label is removed fails, naming the fleet plate rather than
one of its siblings:

```
published-tree: the published tree does not match what we mean
  unlabelled:   website/docs/index.html plays ../assets/cast/emb-top-cluster-29458d0f.cast with no recorded-run label in its plate
```

A copy whose fleet label keeps its text but loses the version fails the same way:

```
  unlabelled:   website/docs/index.html plays ../assets/cast/emb-top-cluster-29458d0f.cast; the label beside it names no emb-top version
```

The pairing is per plate, not per count: each mount reads the next label inside
its own `<figure>`, so a missing label is reported against its own take instead
of shifting every plate after it onto its neighbour's version.

## The fleet take is a recording of this build

`just website-clusterviz` inside `nix develop`: the two nodes came up on
127.0.0.1:16379 and :16380 with the four topviz models preloaded, `emb-top -once
-samples 44` sampled the fleet while the take was recorded, and the rig published
all three artifacts from that one run:

```
publish: 24-line frame into the documentation plate
publish: website/assets/cast/emb-top-cluster-b0476372.cast (231 KiB)
publish: docs/assets/emb-top-cluster-35e5250e.gif (221 KiB)
publish: 2026-10-15 · emb-top v0.4.3.pre6 · ce6fda7 · 2 nodes · 4 models · 127.0.0.1:16379,127.0.0.1:16380
published-tree: 3 takes agree on emb-top v0.4.3.pre6
published-tree: ok (78 served paths, one origin at https://emb.is)
```

The rig's own final check is the agreement the change asks for: the fleet plate
and the single-node plate now declare one `emb-top` version. The previous pair
(`emb-top-cluster-29458d0f.cast`, `emb-top-cluster-94bf476c.gif`) was deleted by
the publish, and both new names are referenced — the cast by the fleet region of
`website/docs/index.html`, the GIF by the `clusterviz` block of
`docs/operations.md`, which the same run rewrote.

## The fleet frames carry no sizing line

The redesign this change follows retunes the sizing rules, so the `emb-stress`
plate's title — "the sizing lines emb-top rendered during the run (none)" — was
re-read rather than assumed. Decoding every `o` event of the re-recorded take and
stripping its ANSI:

```
cluster-raw.cast: SIZING=0 ACTION=0
cluster-take.cast: SIZING=0 ACTION=0
```

The fleet view rendered no sizing line at any point of the run, and its still
frame and metrics line agree (fleet banner, `IN`/`OUT`, `cpu budget`, `TRAFFIC`,
`LOAD`, `INGRESS`, `IMBALANCE` — no `SIZING` row). The stress plate's "none"
therefore still holds, and `just website-stressviz` was not re-recorded.

## The served-tree family agrees on the final tree

```
stamp-version: ok (7 stamped value(s), all '0.4.3.pre6')
stamp-presets: ok (20 marker(s), all 12 digests current)
stamp-examples-check: 25 examples, byte for byte
published-tree: 3 takes agree on emb-top v0.4.3.pre6
published-tree: ok (78 served paths, one origin at https://emb.is)
```

## The surface's rows name the fields the wire carries

Every `<code>` field the two edited rows name was checked three ways: against the
reply builders in `internal/server`, against `docs/commands.md` and
`docs/operations.md`, and against a running node's replies. All ten `EMB.INFO`
names and all five `MONITOR` names exist in the served set of `internal/server`'s
bulk strings, and the reference files name the same groups and the same scope
("that model's own cache stats").

The live check ran against `bin/emb -config test-two-models.yaml -listen
127.0.0.1:16379 -cache 64MB`, reading both encodings:

```
EMB.INFO minilm: 52 fields, 10 checked
MONITOR event: 11 RESP2 elements, 11 RESP3 keys, 5 checked
RESP3 attribution: {'cached_texts': 0, 'attributed': 1, 'dispatch_us': 27, 'admit_us': 0, 'run_us': 3014}
cache-scoped keys: ['cache_hits', 'cache_misses', 'cache_hit_rate', 'cache_evictions', 'cache_entries', 'cache_max_bytes', 'cache_memory_bytes']
every field the surface names exists in the live replies, in the order it states
```

The RESP3 map carries the five names, and the RESP2 event is the same eleven
values in the order the row describes — which is the property the row states when
it says they follow `err`. `cache_max_bytes` is the model's own limit; the
`EMB.INFO` cache block is that model's entries only, so the row's scope matches
the code.

## The surface renders through the repository's own loop

`just website` (the repository's dev loop, serving `website/` on :8080), then the
documentation surface at `/docs/`:

- **Scripting on**: the fleet plate builds the real player
  (`ap-player … asciinema-player-theme-plate`) from the new take
  (`../assets/cast/emb-top-cluster-b0476372.cast`), drawn on a canvas in the
  dashboard's own colours. Held: two canvas reads 1.5s apart are byte-identical.
  Clicking the player's own control (`aria-label="Play"`) advances it — the
  control bar goes `00:02` → `00:04` and the canvas changes, with the button
  flipping to `Pause`.
- **Scripting off**: the plate falls back to the take's still frame and the
  caption `Measured fleet run · 2026-10-15 · emb-top v0.4.3.pre6 · ce6fda7 · 2
  nodes · 4 models · …`, and the frame it holds is the new run's (fleet banner,
  `IN 193 req/s`, both ingress rows).

Both edited rows render with their new text on the served page, read from the
rendered DOM rather than the source.
