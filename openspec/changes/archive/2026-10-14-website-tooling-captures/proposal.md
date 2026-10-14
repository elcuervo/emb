# Proposal

## Why

The site's only tooling figure is a take recorded on 2026-10-12 at `8ace286`,
before `emb-top` gained the sizing recommendation — so the plate on both
surfaces, and the capture in `docs/operations.md`, show a dashboard missing the
readout this repository just added. `emb-top-capture` already requires a
re-record in the same change as a new readout; nothing satisfies it yet.

Worse, the two things an operator now needs most are absent from every surface:
`emb-stress` (the tool that drives a fleet) appears nowhere on the site, and the
fleet view exists only as a GIF in the repository's own docs. A reader who wants
to know whether a cluster can take production inference, and whether to buy CPU
or memory, reads one node's dashboard and no fleet.

## What Changes

- **Re-record the `emb-top` family** — `just website-topviz` and
  `just website-clusterviz` against the current binary, so the landing plate, the
  documentation plate, the documented capture and their captions show the
  `SIZING` row and the `rec=` token, and the caption names the run it is.
- **A new capture rig for `emb-stress`** — `website/tools/stressviz/` with a
  `just website-stressviz` target: it starts a two-node fleet from a scenario
  config, drives it with a real `emb-stress` run, reads the fleet's own `SIZING`
  lines from `emb-top -frames`, and publishes the run's three forms — the
  documentation surface's measured plate, its caption, and a capture for
  `docs/operations.md` — with every number stamped from the run rather than
  typed into the page.
- **The fleet view becomes a site artifact.** `website-clusterviz` grows the same
  take/frame/publish half `website-topviz` has, so the fleet run is a served
  `.cast` replayed as text on the documentation surface, beside the single-node
  plate. The animated GIF stays where it is: the site MUST serve no image of the
  dashboard, so putting the existing GIF on the site is not an option.
- **The measured record lands in the reference.** The rig's run is recorded in
  `BENCHMARK.md` with its machine, method and reproduction command, and
  `docs/operations.md`'s `Load: emb-stress` section carries the figure and that
  command, so no number on a surface lacks provenance.
- No **BREAKING** changes. No server, protocol, config, or on-disk format
  changes; no new site dependency, script, font, or player.

## Capabilities

### New Capabilities

- `emb-stress-capture`: a reproducible, scripted capture of a real `emb-stress`
  run against a fleet, in the forms the repository needs it — the measured plate
  the documentation surface shows, and the capture the reference docs carry —
  so the fleet's offered-versus-observed behaviour and its sizing advice are
  shown as they were measured rather than drawn from memory.

### Modified Capabilities

- `emb-top-capture`: the fleet run becomes a published site artifact. The rig
  that already produces the page's take and the documented capture also produces
  the fleet take, its still frame and the fleet plate on the documentation
  surface, named from its own bytes and referenced by the capture like the
  single-node pair.
- `product-site`: the documentation surface carries two more tooling figures —
  the fleet take replayed as text, and the measured `emb-stress` plate — under
  the same rules the `emb-top` plate already obeys: the run's own output or its
  recorded numbers, a caption written by the capture, no unsolicited motion, and
  no image of a dashboard.
- `product-docs`: the operations section documents the fleet pairing — what
  `emb-stress` offers, what `emb-top` observes, and which node to look at first —
  and carries the figure with the reproduction command that produced it, with the
  measured values recorded in `BENCHMARK.md`.

## Impact

- New: `website/tools/stressviz/` (scenario config, fleet bring-up, the
  `emb-stress` run, the capture and the publish steps), the `justfile` target,
  and the plate markup on the documentation surface.
- Modified: `website/tools/topviz/publish.py` and `run-cluster.sh` (the fleet
  take's trim, frame, publish and served-set entry, beside the GIF they write
  today), `website/tools/published-tree.py` (the second cast in the served set),
  `docs/operations.md`, `BENCHMARK.md`, and
  `website/.impeccable/surfaces/website-docs-index-html.md` (a brief amendment
  before any markup edit, per the site's Impeccable requirement).
- Prerequisites: the scenario's models on disk (three are missing today,
  ~500 MB) and the website dev shell for the recorder; both rigs already refuse
  to run without them, naming what is missing.
- Unchanged: the server, its RESP surface, the gems, and every on-disk format.
  The GIF in `docs/assets/` keeps its name, its place and its owner.
