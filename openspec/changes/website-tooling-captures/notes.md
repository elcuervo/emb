# Verification notes

Recorded observations from running the change on a real local rig. These are
evidence, not a spec: nothing here is asserted by CI.

## 1. The re-recorded takes

### 1.1 Models

Three models were missing and were fetched (~500 MB on disk):
`Xenova/bge-small-en-v1.5` → `./models/bge-small`,
`Xenova/jina-embeddings-v2-small-en` → `./models/jina-small`,
`Xenova/e5-small` → `./models/e5-small` (whose ONNX lives under `onnx/`, which
`just download-model` falls back to). All four `onnx:` paths in
`website/tools/topviz/models.yaml` exist and begin with the ONNX protobuf
header; all four `tokenizer.json` files are beside them.

### 1.2 The single-node re-record

`just website-topviz`, inside the dev shell, on 2026-10-14:

| | before | after |
|---|---|---|
| take | `assets/cast/emb-top-a5fe39e7.cast` | `assets/cast/emb-top-243fc292.cast` (117 KiB) |
| capture | `docs/assets/emb-top-bb0d714c.gif` | `docs/assets/emb-top-9d750c12.gif` (198 KiB) |
| caption | 2026-10-12 · emb-top v0.4.3.pre4 · 8ace286 | 2026-10-14 · emb-top v0.4.3.pre5 · 9334f4f · 4 models · 127.0.0.1:16379 |

Both pages carry the new cast and the new caption, the previous cast and capture
are gone, the served-tree check passed (77 served paths), and the captured
version matches `VERSION`.

**The `SIZING` row is present in the new frame, and blank.** The frame gained the
row's reserved line — 23 lines against 22 at `HEAD`, the extra one directly under
the health banner, which is where `recommendView` (`cmd/emb-top/health.go`)
renders it — but no rule of `embtop.RecommendNode` fires for this run, so the row
carries no text and `runs/samples.txt` carries no `rec=` token:

- *host is the constraint* needs a cgroup throttle or steal delta; this run is on
  macOS, where neither exists.
- *cpu budget is the limit* needs the budget **held** (≥95%) with an admission
  timeout. The run peaks at `cpu_peak_tokens=6` of a budget of 8 and ends with
  `cpu_admission_timeouts=0`, so neither half holds. (Probed separately with the
  recorded node pinned to `cpu_budget: 1` under a sustained four-model burst: the
  frame then reads `tokens 3/1 · peak 3 · queued 2 · timeouts 0` — still no
  recommendation, because nothing reaches the 10 s admission bound.)
- *memory* needs the cache-restore headroom spent, and *growth* needs a rise of
  ≥10% of the window's starting RSS (and ≥8 MB) while load and cache stay flat;
  the run's RSS moved 1044 MB → 1061 MB under load that was not flat.

The proposal's "so the plate … show the `SIZING` row and the `rec=` token"
therefore over-states what an untuned re-record produces: the row is current, the
readout is honest and empty. Accepted as such — the plate's job here is to be a
recording of the current binary, and a recommendation is a claim a quiet node
must not make. Nothing in the change's specs requires the single-node plate to
carry a recommendation; the fleet view's own line is conditional in the same way
(see 1.4).

### 1.3 The still frame is in the sustained phase

`trim.py --until-pct 65` still lands where the plate needs it: the recorded take
is 41.1 s and the frame is its 26.0 s screen. Reading the same raw recording at
other percentages shows the frame is neither edge:

| `--until-pct` | the four model rows |
|---|---|
| 20 | no model rows at all (the preroll, before the node is loaded) |
| 50 | three rows active, the fourth band not yet started |
| **65** | **all four active — bge-small 20.0 r/s, e5-small 60.9 r/s, jina-small 17.0 r/s, minilm 36.9 r/s, CPU 193%, cache 43.3%** |
| 90 | the drain — bge-small and minilm at 0.0 r/s |

No re-derivation was needed.

### 1.4 The fleet re-record

`just website-clusterviz`, on 2026-10-14, twice (the second run is 2.4's check
that a re-run replaces both artifacts):

| | before | after |
|---|---|---|
| fleet take | — (the rig wrote only a GIF) | `assets/cast/emb-top-cluster-ed7eaaae.cast` (95 KiB) |
| fleet capture | `docs/assets/emb-top-cluster-04bf4380.gif` | `docs/assets/emb-top-cluster-d67d35a5.gif` (199 KiB) |
| caption | none (the block's prose was hand-written) | `2026-10-14 · emb-top v0.4.3.pre5 · 9334f4f · 2 nodes · 4 models · 127.0.0.1:16379,127.0.0.1:16380` |

`docs/operations.md`'s fleet block names the new capture, exactly one cluster GIF
exists, the served set carries the fleet take beside the single-node one, and the
documentation page's fleet region plays it (`../assets/cast/…`). The second run
replaced both files and left one of each.

**The fleet run rendered no sizing line either.** The frame at 65% of the take
carries none, for the same reason as 1.2 in the fleet's own terms: the fleet CPU
headroom at the run's peak is ≈25% (`cpu budget 16 · tokens 12/16 · peak 12`
across the two nodes), above `fleetHeadroomPct` (20%), no node is bound on a
resource its peer is not, and neither node's p95 is 2× its peer's. So neither the
capacity clause ("add nodes: every node's cpu headroom is small") nor the
inspect clause fires, and the fleet plate shows the cluster view as it was —
which is the honest state, and the one the documentation draws. A separate
40-frame probe of the same fleet under a longer traffic run agreed (0 sizing
lines).

## 3. The `emb-stress` rig

### 3.1 The scenario

Two nodes from `topviz`'s `models.yaml` on one port, one per loopback family:
`127.0.0.1:6379` and `[::1]:6379`. The target is the bare name `localhost`,
which is what makes both tools expand it into two endpoints — and the name must
be *bare*: `ParseNodeSpec` treats `host:port` as an address (`isName` is
`!hasPort && ParseIP(host) == nil`), so the first run of the rig against
`localhost:16391` drove exactly **one** endpoint and the publisher refused it
(`publish: the run reports 1 endpoint(s) — the scenario starts two`). With the
bare name the fleet listens on the default port 6379.

Verified by running the bring-up alone: both nodes answer `EMB.READY`, a second
bring-up over the live fleet refuses (`something already answers on
[127.0.0.1]:16391`), and holding `::1:6379` with a listener that answers `PONG`
makes it refuse with `something already answers on [::1]:6379`.

### Non-obvious: the rig's bring-up runs as `bash bringup.sh`

`emb` finds ONNX Runtime through `DYLD_LIBRARY_PATH`, and macOS hands a
`#!/usr/bin/env bash` script an environment with every `DYLD_*` variable
**removed** — the loader strips them when it launches a protected binary, and
`/usr/bin/env` is one. Measured directly:

```
direct DYLD=[/nix/store/…-onnxruntime-1.26.0/lib:]
script DYLD=[]                      # ./probe.sh, shebang #!/usr/bin/env bash
script DYLD=[/nix/store/…-onnxruntime-1.26.0/lib:]   # bash probe.sh
```

So a rig script that starts `emb` has to be run by the dev shell's own bash; the
justfile calls `bash website/tools/stressviz/bringup.sh`, and the script refuses
with that instruction when it finds the variable empty. The other rigs never hit
it: their scripts start `emb-top` and `redis-benchmark`, neither of which loads a
model.

### 3.2 The run

One `emb-stress` run at the scenario's levels and dwell —
`-target localhost -levels 1,2,4,8 -dwell 12s -warmup 4s -seed 3` — recorded by
`asciinema`, with its report kept in `runs/stress.txt` and an
`emb-top -node localhost -frames` sidecar in `runs/frames.jsonl`. The publisher
asserted, on the run's own output: two endpoints with per-level and per-endpoint
values, non-zero completed work at every level, nothing under `NOT EXERCISED`,
and the fleet line equal to the sum of the endpoints. The record names both
endpoints by the address each dials (`[::1]:6379`, `127.0.0.1:6379`) — the
name-expansion the rig exists for.

### 3.4 The capture

`agg` renders the recording itself with `--idle-time-limit 2` and a 10 s hold on
the last frame: a run works for its levels and prints the table at the end, so
without the cap the capture is 49 s of two static lines, and with it the report
is what the reader sees and has time to read. Three frames, 46 KiB, and the
report's widest line (135 characters) fits the 140-column window unwrapped.

## 5. Integration

### 5.1 Detector

`impeccable detect --json` over `website/docs/index.html`,
`website/index.html` and `website/assets/css/docs.css`, compared finding by
finding against the same run in a worktree at `HEAD` (`git worktree add --detach
/tmp/emb-detector-head HEAD`):

| file | `HEAD` | this change |
|---|---|---|
| `website/docs/index.html` | 19 | **21** |
| `website/index.html` | 19 | 19 |
| `website/assets/css/docs.css` | 0 | 0 |

The two differences are the fleet plate repeating what the single-node plate
already produces: `cramped-padding` on the second `.topviz__play` (children
flush against the frame's border) and `tight-leading` for the player's own
terminal leading (`terminalLineHeight` 1.05). The landing page is
finding-for-finding identical, so the re-recorded take adds none. Recorded, not
changed — the atoms and the player are the surface's own, and the earlier
adaptive-capacity amendment accepted the same two kinds on the same plate.

### 5.2 The surfaces

Both tooling plates were checked in a real browser (`just website` on `:8080`):

- **Scripting available, no reduced-motion preference** (emulated at the CDP
  level, since this host has macOS's *Reduce motion* on and headless Chromium
  reports `reduce` by default): both plates build the vendored player
  (`ap-wrapper`, one child in each mount), the `<pre>` still frame is hidden, the
  player sits at `00:00` with a **Play** control, and clicking that control moves
  it to `00:02` and the control to **Pause** — the recording starts when the
  reader asks, not before.
- **`prefers-reduced-motion: reduce`**: neither plate creates a player; both
  still frames are visible and nothing plays.
- **Scripting off** (`page.setJavaScriptEnabled(false)`): both figures keep
  their still frame, caption, screen-reader figures and drawing — the fleet
  plate shows the cluster view's own text and its caption, the measured plate its
  SVG, its legend and its full caption.

### 5.3 Served bytes

| artifact | where | raw | gzip -9 |
|---|---|---|---|
| the fleet take (**added**, served by the site) | `website/assets/cast/emb-top-cluster-ed7eaaae.cast` | 97,416 B | **5,514 B** |
| the single-node take (replaced) | `website/assets/cast/emb-top-243fc292.cast` | 119,897 B | 5,729 B |
| the run's capture (docs only) | `docs/assets/stressviz-f45ee7a7.gif` | 46,645 B | 45,692 B |
| the fleet capture (docs only) | `docs/assets/emb-top-cluster-d67d35a5.gif` | 203,789 B | 182,983 B |

`website/_headers` already pins `/assets/cast/*` before this change and needed no
edit:

```
/assets/cast/*
  Cache-Control: public, max-age=31536000, immutable
  Content-Type: text/plain; charset=utf-8
```

So the site's added payload is the fleet take's ~5.5 KiB compressed (an asciicast
is escape sequences over cells and compresses ~18×), and the two GIFs live under
`docs/assets/`, which the site does not serve — they are the repository's own
documentation, rendered by GitHub.

**The one rule this change adds to `docs.css`.** The `.plate` atoms draw their
labels, marks and areas in `--fg`, which is ink on the paper blocks the existing
measured plates sit on; the operations block's ground is inverted, so the figure
there would have drawn near-black on near-black. `.block--dark .plate{
--fg: var(--bg); }` is the same scoped remap `styles.css` already does for
`--muted` in that block: it introduces no value, no hue and no second accent, and
the measured plate now reads as paper on the dark ground.

**The published values are checked against the runs, not by eye.** After the
final re-record the two `emb-top` plates' frame, cast URL, caption and figures
were compared against `runs/frame.txt`, the take's own bytes and
`busiest(runs/samples.txt)` — all six equal. That check was added to the review
because one hand edit to the page (adding the fleet region, whose anchor was a
stale copy of the figures line) had put the previous run's figures back on the
docs surface; the publisher rewrote them, and the fresh re-record confirms it
does, so nothing on a surface is left describing a run that is not the recorded
one.
