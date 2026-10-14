# Design

## Context

The publishing machinery this change extends is `website/tools/topviz/`: one
recorded run becomes three artifacts (the take the plate plays, its still frame,
the documentation's animated capture), and `publish.py` writes every reference to
them by marker. Three properties of that code shape the approach, all verified
in it:

- `stamp_text` and `stamp_attr` replace **the first** match (`count=1`). A second
  plate on the same page is therefore stamped by whichever plate comes first —
  the marker vocabulary is per-page, not per-figure.
- `publish.py`'s cast cleanup globs `emb-top-*.cast`, which also matches the
  fleet's `emb-top-cluster-<sha8>.cast`; only the *GIF* cleanup carries a
  cluster-prefix guard today, so publishing a single-node take would delete the
  fleet take.
- `topviz.js` already drives every `[data-topviz-cast]` mount
  (`querySelectorAll`, one loop), and `_headers` already pins `/assets/cast/*`
  immutable as a directory rule, so a second take needs neither new script nor
  new header.

`emb-stress` prints its result as a table per level (aggregate and per endpoint),
and `emb-top -node … -frames` prints one JSON object per poll; the recorded
two-node observation in `openspec/changes/emb-stress-and-fleet-sizing/notes.md`
is the layout this rig reproduces.

## Goals / Non-Goals

**Goals:**

- One command per run, and every published value written by that command rather
  than by hand.
- The fleet run joins the single-node run as a first-class site artifact, without
  adding a script, a font, a dependency, a header rule, or a second visual
  language.
- The `emb-stress` figure and the capture beside it describe **one** run, so a
  reader cannot compare two numbers that came from different measurements.

**Non-Goals:**

- No change to `emb-top`, `emb-stress`, the server, or the site's runtime.
- No new top-level documentation section, and no reflow of the landing page
  beyond the re-recorded plate.
- Not a general figure-authoring system: one plate, one rig, one scenario.

## Decisions

### The capture rig is a sibling of `topviz`, not a growth of it

`website/tools/stressviz/` owns the fleet scenario, the recorded `emb-stress`
invocation and the publishing of the plate. `topviz`'s contract is one node under
`redis-benchmark` bands (`run.sh`, `traffic.sh`); this scenario is a different
tool driving a fleet, and folding it in would make one script answer two
questions. The new rig reuses `topviz`'s *idioms* — content-named artifacts,
marker publishing, a `runs/` directory that is never served, a final
`published-tree.py` check — and its `trim.py`/`agg` invocations verbatim.

*Alternative:* extend `website-topviz` with a `--fleet` mode. Rejected: the
single-node target's whole value is that one command reproduces the landing's
plate; a mode flag makes the common run carry the uncommon one's machinery.

### The plate is drawn, the capture is the run's own output

The documentation surface gets an inline `.plate` — ruled graticule, mono caps
labels, one accent signal — with its values stamped from the run, following the
three adaptive-capacity plates already on that surface. `docs/operations.md`,
which cannot replay a cast, gets the run's own terminal output rendered to a GIF
by `agg`, exactly as the `emb-top` capture is.

*Alternatives:* replaying the stress run as a cast on the docs surface (its
output is a table per level, and a drawing is what makes the offered-versus-
observed split legible at a glance); a hand-authored figure (forbidden by the
surface's own measured-figures requirement).

### The fleet plate is stamped inside its own marked region

The fleet block sits between `<!-- fleetviz:begin -->` and
`<!-- fleetviz:end -->` comments, and the publisher stamps **within that slice**
using the existing helpers, then splices it back. That keeps the single-match
stampers safe with two plates on one page and lets the mount keep the
`data-topviz-*` vocabulary, so `topviz.js` needs no change.

*Alternative:* `data-fleetviz-*` attributes. Rejected: it duplicates the
vocabulary and would need a second branch in the page's script for no reader-
visible gain.

### The single-node publisher stops owning the fleet take's bytes

`publish.py`'s cast cleanup gains the cluster-prefix guard its GIF cleanup
already has, and the fleet cast is written into `published-tree.py`'s served set
by its own anchored stamp (`emb-top-cluster-<sha8>.cast`, beside the single-node
entry) rather than by the existing `emb-top-[0-9a-f]+\.cast` regex, which is
deliberately narrow.

*Alternative:* give the fleet cast a different prefix so no rule overlaps.
Rejected: the fleet GIF already establishes `emb-top-cluster-<sha8>`, and the two
artifacts of one run should read as a pair.

### The fleet scenario is the layout the change already proved

Two nodes on one port, one per address family (`127.0.0.1` and `[::1]`), watched
by `emb-top -node <name>`, because that is what makes a bare name expand into two
endpoints — which is the fleet shape the tools were built for and the one the
recorded observation used. The rig parameterizes the port and refuses to start
when either address answers, as `topviz` does for its own.

*Alternative:* two ports on one host and `-nodes a,b`. Rejected for the *stress*
run: `emb-stress` takes one target specification, and a name that resolves to
both endpoints is the input the tool exists to accept.

### One run feeds the plate, the caption and the capture

`stressviz` records the `emb-stress` table run once (asciinema), keeps that
run's output, and derives the plate's curve and shares from **that** output; an
`emb-top -frames` sidecar on the same fleet supplies the sizing lines the plate
annotates. The rig does not run the scenario twice.

*Alternatives:* `-json` for the plate and a table run for the capture — the two
runs' rates differ, and a plate whose numbers do not match the capture beside it
is the defect the specs exist to prevent; `-json` alone — machine-readable but
unreadable as the capture the repository shows.

### The re-record comes first, and stays independent

`just website-topviz` / `just website-clusterviz` are re-run as their own step,
before the new figures land, because `emb-top-capture` already requires a take
re-recorded in the same change as a new readout. Doing it first means the
surfaces are never in a state where a new figure sits beside a plate that
contradicts it, and it isolates "the dashboard changed" from "the page gained a
figure" if either half needs revisiting.

### Impeccable governs the plate, and the brief is amended first

`impeccable context` runs once for `website/docs/index.html`; the existing brief
at `website/.impeccable/surfaces/website-docs-index-html.md` gains an amendment
with the direction contract before any markup edit; the detector runs once over
the changed files after the build and its findings are resolved or recorded.
`DESIGN.md` moves only if a durable system change is approved. The v0.1.5 CLI
from the Pi skill is used directly (`~/.pi/agent/skills/impeccable/scripts/impeccable`),
since this harness does not register that skill.

## Risks / Trade-offs

- **[The single-node publish deletes the fleet take]** → the cluster-prefix
  guard lands before any fleet cast exists, and `published-tree.py` fails if the
  cast is missing.
- **[A busy host halves the rates]** → the record and the caption carry the date,
  the host's shape and the invocation, and the plate's argument is the shape of
  the curve and the split, not a magnitude; the rig is re-runnable in one command.
- **[The plate drifts from the capture]** → one run, parsed once; the publisher
  fails on a missing marker or an unparseable level rather than writing half.
- **[`localhost` does not expand to two families on some host]** → the rig
  asserts both endpoints appear in the run's own output and fails naming the
  resolution it actually saw.
- **[A second take grows the docs surface's payload]** → the fleet take is
  trimmed to its sustained window like the single-node one, the plate fetches
  nothing where it is not drawn, and `/assets/cast/*` is already pinned and
  declared compressible; the task measures the served size.
- **[Two figures, one accent rule]** → each figure carries at most one accent
  signal; the fleet plate is the run in its own colours and draws no accent of
  its own.
- **[The plate's drawing is read as the tool's output]** → the figure is
  captioned as a drawing of the run's measured values, and the run's own output
  is kept and shown by the repository's capture.

## Migration Plan

Nothing to migrate: the re-record replaces the take and the capture under new
content-derived names, and the fleet take is additive. Rollback is `git revert` —
the previous artifacts are named from their own bytes and are recoverable from
history.

## Open Questions

- Whether the fleet recording's traffic should come from `emb-stress` rather than
  the cluster traffic script, collapsing the two fleet runs into one fleet and
  one load source. It changes which script drives a recording, not which
  artifacts exist or what they must show.
- Whether the landing page should also carry the fleet plate, or keep exactly one
  tooling plate as its brief currently requires.
