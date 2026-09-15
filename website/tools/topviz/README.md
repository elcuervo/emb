# The `emb-top` plate's recording rig

`just website-topviz` records a real `emb-top` session against a real node and
publishes it as the site's plate. This directory is the rig: it is under
`website/` because the plate is part of the site, and it is never served
(`/tools/` is ignored, and `published-tree.py` asserts it).

```
just website-topviz                 # build, record, render, publish, verify
```

## What one run does

```
bin/emb (4 models, cache on)  ──▶  asciinema record (headless, 120x32)
        ▲                                   │  raw: 5s of blank + the dashboard + the exit
        │                                   ▼
   traffic.sh (redis-benchmark)      trim.py  ── take: the dashboard's own screen, t=0 at first paint
        behind emb-top in a pty             │
                                            ▼
                                     agg  ── the site's palette, 1.8x, 10 fps cap
                                            │
                                            ▼
                                   img2webp + pngquant  ── animated WebP + poster frame
                                            │
                                            ▼
                                    publish.py  ── names, placement, caption, figures, served set
```

Files here:

| file | what it is |
|---|---|
| `models.yaml` | the node that gets recorded: four models, all preloaded, cache on, `:16379` |
| `traffic.sh` | the load: one `redis-benchmark` band per model, staggered start and stop |
| `run.sh` | what `asciinema` runs: the dashboard in front, the traffic behind it |
| `trim.py` | cuts the raw recording to the dashboard's alt-screen session |
| `publish.py` | names the artifacts from their bytes, writes the page and the served set |
| `runs/` | the last run's `raw.cast`, `take.cast`, `node.log`, `traffic.log`, `samples.txt` — gitignored, unserved, and the only way to check what the plate shows |

## Three things that are not obvious

**The first five seconds of every take are blank, on purpose.** Bubble Tea's
package init calls `lipgloss.HasDarkBackground()`, which asks the terminal for its
background colour and waits out `termenv.OSCTimeout` — five full seconds — when
nothing answers. A recording pty never answers; a real terminal replies in
microseconds. So `run.sh` waits 8 seconds before loading the node, and `trim.py`
throws the wait away by rebasing the take to the first painted frame. Reading
asciicast **v3** times as if they were v2's absolute ones gives a take that
starts in the past: v3 carries *deltas*, so `trim.py` accumulates, rebases and
re-deltas.

**The capture has no errors because none can be staged.** `emb-top`'s per-model
`err/s` reads `EMB.INFO <model>` → pool counters, which only move on a genuine
tokenizer or inference failure; a scripted model's failures increment
`script_errors` in `EMB.STATS`, which `EMB.INFO` does not report. There is no
request that raises a model's error rate on purpose, so the plate shows the
error column at zero rather than inventing a spike — this is why the illustrative
render it replaced carried `err 18 ↑` under a "not a measured run" caption.

**The gauge bars are emb-top's own rendering, not a theme artefact.** The three
gauges are drawn as full-width blocks regardless of value (`barchart` with
horizontal bars and no axis), and `mem` reads `1.1kMB` rather than `1.1GB`
(`fmtRate` stops at `k`). Both were checked against agg's stock themes and are
identical there, so they are the dashboard's, and out of this change's scope.

## Recording it again

`just website-topviz` is idempotent: it deletes the previous take's assets,
writes new content-hashed names into `index.html` and into
`published-tree.py`'s served set, and ends by running that check. The artifact
name changes on every take, which is what makes the existing
`/assets/img/*  immutable` cache rule honest and leaves `_headers` untouched.

After a re-record, watch the shipped clip once. The clip's shape is structural
(idle → ramp → all four models → drain) and does not depend on the machine, but
its *rates* do: a busy laptop halves them. Everything about the run that a
reader can check is in `runs/`.

The models are ~500 MB on disk. The rig fails before recording, naming the file
and the command, when one is missing:

```bash
just download-model Xenova/bge-small-en-v1.5 ./models/bge-small
just download-model Xenova/jina-embeddings-v2-small-en ./models/jina-small
just download-model Xenova/e5-small-v2 ./models/e5-small
```

`website/README.md`'s tools list is the inventory this directory appears in.
