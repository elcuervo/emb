# The `emb-stress` figure's recording rig

`just website-stressviz` brings up a real two-node fleet, drives it with one
real `emb-stress` run, and publishes three things from that one run: the
measured figure on the documentation surface (`website/docs/index.html`), the
capture `docs/operations.md` carries, and the run's record in `BENCHMARK.md`.
Nothing on any of those three surfaces is typed by hand.

```
just website-stressviz      # build, bring the fleet up, record, publish, verify
bash bringup.sh             # just the fleet, for a manual run against it
```

## What one run does

```
bin/emb (127.0.0.1:P) ─┐
                       ├─▶ emb-stress -target localhost:P ─▶ asciinema record ─▶ agg ─▶ docs/assets/
bin/emb ([::1]:P)     ─┘        (one run, levels 1,2,4,8)                          stressviz-<sha8>.gif
        ▲                                  │
        │                                  ├─▶ runs/stress.txt        (the run's own report)
        │                                  └─▶ publish.py            (draws and writes)
        │                                             ▲
   emb-top -node localhost:P -frames ────────────────┘
        (the same fleet, polled once a second: the sizing lines)
```

| artifact | where | why that form |
|---|---|---|
| the measured figure | the documentation surface's Operations section | a `.plate`, drawn from the run's own numbers: the level curve, each endpoint's served share beside its offered mark, and the sizing lines the frames carried |
| the capture | `docs/assets/stressviz-<sha8>.gif` | the surface that renders markdown cannot replay a recording, and a picture of a report is what a reader there can read |
| the record | `BENCHMARK.md`, between `<!-- stressviz:begin -->` and `<!-- stressviz:end -->` | the machine, the fleet, the method, the invocation and every value the figure draws |

Files here:

| file | what it is |
|---|---|
| `bringup.sh` | the scenario: two nodes from `topviz`'s `models.yaml`, one on each loopback family, one port, ready-checked before anything drives them |
| `run.sh` | what `asciinema` runs: the one `emb-stress` invocation, echoed first, with its report kept in `runs/stress.txt` |
| `publish.py` | reads the report and the frames, draws the figure, and writes the three surfaces |
| `runs/` | the last run's `stress-raw.cast`, `stress.txt`, `frames.jsonl` and node logs — gitignored, unserved, and the only way to check what the surfaces show |

## The scenario

Two nodes, one port, one per address family:

```
bin/emb -config website/tools/topviz/models.yaml -listen 127.0.0.1:6379
bin/emb -config website/tools/topviz/models.yaml -listen [::1]:6379
```

The target is the bare name `localhost`, which resolves to both addresses, so one
`emb-stress -target` and one `emb-top -node` cover the fleet — which is the
shape a name-expanded deployment has and the only one in which the offered share
and the served share are comparable at all.

It has to be *bare*: `ParseNodeSpec` treats `host:port` as an address and expands
only a name, which takes the default port. So the fleet listens on 6379, and
`just website-dev` — which points the site's console at the same port — is the
one thing that can be in the way; the guard below says so rather than recording
whatever was already there.

The models, their widths, the cache size and the preload flags all come from
`topviz`'s `models.yaml`, so the fleet the stress figure describes is configured
like the node the dashboard plate describes.

The rig refuses to start when either address already answers, and refuses to
record a fleet whose nodes did not answer `EMB.READY` inside the deadline
(printing the node's log tail). The run itself is the scenario's: four levels
(1, 2, 4, 8 workers per endpoint), 12s held per level with the leading 4s
discarded, seed 3. `publish.py` refuses to write anything when the report does
not show two endpoints with non-zero work, when a pair is reported under
`NOT EXERCISED`, or when a marker is missing from a surface.

## Why the bring-up runs as `bash bringup.sh`

`emb` loads ONNX Runtime through `DYLD_LIBRARY_PATH`, and macOS hands a
`#!/usr/bin/env bash` script an environment with every `DYLD_*` variable removed:
the loader strips them when it launches a protected binary, and `/usr/bin/env`
is one. So a rig script that starts `emb` has to be run by the dev shell's own
bash, which keeps the variable — `bash website/tools/stressviz/bringup.sh`, which
is exactly how the justfile calls it. Running `./bringup.sh` says so and stops,
rather than failing with the loader's message, which names no cause. (The other
rigs never hit this: their scripts start `emb-top` and `redis-benchmark`, neither
of which loads a model.)

## The figure is a drawing; the capture is the run's own output

The documentation surface gets a drawing because the figure's job there is the
*shape*: the level curve, and the gap between what each node was offered and what
it served. It is composed only from the atoms that surface already uses — ruled
graticule, mono labels, a faint ink area, one accent signal on the measured
throughput — and it is captioned with every value it draws, so the surface's
claims survive the drawing being removed.

The capture is not a drawing: it is the `emb-stress` report's own characters,
recorded from the run and rendered by `agg` with an idle-time cap (a run works
for its levels and prints the table at the end, so the cap is what makes the
capture short without speeding the run up). `docs/operations.md` carries it
because that surface cannot replay a recording.

## Recording it again

`just website-stressviz` is idempotent: it re-records the run, replaces the
capture under a name made of its own bytes, rewrites the figure, the caption and
the figures on the documentation surface, rewrites the reference in
`docs/operations.md` and the record in `BENCHMARK.md`, and ends by running the
site's published-tree check.

The rates depend on the machine — a busy host halves them — but the shape does
not: the curve rises with the level, the offers stay even, and the split is the
point. Everything a reader can check is in `runs/`.
