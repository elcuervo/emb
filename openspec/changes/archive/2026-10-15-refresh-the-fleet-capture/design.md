# Design

## Context

Two rigs publish the dashboard's takes: `just website-topviz` (one warm node) and
`just website-clusterviz` (two nodes, the fleet view). Each runs
`website/tools/topviz/publish.py`, which is the only writer of a plate's frame,
its cast URL, its caption and its metrics line, and of the served-set entry in
`website/tools/published-tree.py`. Artifacts are named from their own bytes, so a
re-record replaces a take and deletes the previous one — but only the take that
ran. The two rigs are independent, so one can be re-recorded while its sibling is
left at an older build, which is what `emb-top-course-of-action` did on
2026-10-15: the single-node take is `v0.4.3.pre6` at 70c7730, the fleet take is
still `v0.4.3.pre5` at 38556b1.

Two checks run over this tree — in `ci.yml` and `site.yml`, both as
`python3 website/tools/stamp-*.py --check` steps followed by
`python3 website/tools/published-tree.py`. Neither can see this: a capture's
version is deliberately outside the stamped set (its version is frozen in its
frames), and `published-tree.py` asserts the tree's shape, not its age.

The prior change's deltas are not yet synced into `openspec/specs/`, so this
change's deltas stack on its.

## Goals / Non-Goals

**Goals:**
- The fleet take in the served tree and in `docs/assets/` is a recording of the
  dashboard as it renders now.
- A take left behind by a change that re-recorded its sibling fails a check in
  CI, instead of passing every gate and shipping.
- The hosted docs surface names the fields and the scope the shipped reference
  documentation names for `EMB.INFO` and `MONITOR`.

**Non-Goals:**
- Any change to the server, the protocol, or what `emb-top` renders. The
  dashboard is correct; the captures are behind it.
- A prose-diff gate between `website/docs/index.html` and `docs/*.md`. The
  surface is a hand-written condensation, so any such check is a heuristic over
  prose and would be turned off rather than fixed.
- Making the check prove a take is *current*. That needs the recorder; see
  Decisions.

## Decisions

**Re-record with the rig, never by editing the plate.** The captions, the frame,
the metrics line and the served-set entry are all written from the run's own
sample log by `publish.py`, and `emb-top-course-of-action`'s verify note ("the
published artifact names change") is exactly that property. Editing the caption's
version by hand would leave frames recorded from the older build, which is the
defect. Alternative — regenerate only the gif — rejected: the plate's frame and
the cast come from the same trimmed take.

**The agreement check lives in `published-tree.py`.** That tool already parses
the served pages, already runs in both CI workflows, and already runs inside both
rigs, so the invariant costs no new workflow step and no new file. Alternative —
a `takes-in-step.py` beside the `stamp-*.py` family plus a step in each workflow
— rejected as two workflow edits and a new file for one assertion. Alternative —
a Go test — rejected: every served-tree invariant in this repository is a Python
check, and a Go test would put the site's consistency in a different loop.

**It asserts agreement among the takes, not equality with `VERSION`.** Every
plate's recorded label must name one `emb-top` version. The defect that shipped is
a take left behind by a change that re-recorded its sibling, and agreement catches
it with no ongoing cost. Requiring the plates to name `VERSION` would fail on
every version bump — including bumps with no dashboard change — and demand two
model-backed re-records before the bump could go green, with no way for CI to
produce them. What agreement cannot catch is *both* takes going stale at once;
that stays the obligation of `emb-top-capture`'s in-step requirement and of
review, because only the recorder can answer it.

**The version is read from the recorded-run label, not from the cast.** The label
is what `publish.py` writes from the run's own arguments, sits on the served page
in one attribute, and is already the site's statement of provenance. Reading the
cast would mean decoding a terminal recording for a value the caption states.
Alternative — parsing the trimmed cast — rejected as the more fragile of the two
readers for the same fact.

**A take whose mount carries no recorded label fails the check.** A rig that
stopped writing `data-topviz-run` would otherwise make the check vacuous, which is
the silent-success mode this whole change exists to remove. The check is scoped to
the mounts that carry a take (the single-node plate on both pages and the fleet
plate); the `emb-stress` figure is a drawing of a different tool's run, carries no
recording and is not one of them.

**The docs surface's two rows are edited by hand.** They are prose in an `<dl>`
whose source of truth is `docs/commands.md`; the pair is small enough that a gate
would cost more than the drift it prevents (see Non-Goals).

**The emb-stress plate is re-recorded only if its own claim breaks.** Its title
states the sizing lines `emb-top` rendered during that run ("none"), read from
the run's frames. This change retunes exactly those rules, though strictly
towards fewer claims: magnitude floors were added and a repair a node can make
now suppresses the fleet's capacity answer. So "none" most likely still holds —
but it is read from the frames during verification, not assumed.

## Risks / Trade-offs

- [Agreement does not prove currency] → the in-step requirement and the review
  trail carry that; the check is a floor, and its scenarios say so.
- [A rig stops writing the recorded label] → the check treats a take's mount
  without a recorded run label as a failure rather than skipping it.
- [The two plates now name different dates] → expected: each plate names its own
  run, and `product-site` requires only that they declare the same dashboard
  version and neither present the other's numbers.
- [The stressviz plate's "none" becomes wrong] → read the re-recorded fleet
  frames during verification; if they carry a sizing line, re-record the
  stressviz figure in this change, since its caption states what the frames
  carried.
- [Archive order] → `emb-top-course-of-action`'s deltas must be synced before
  this change's, or the fleet-view requirements this change assumes are absent
  from `openspec/specs/`.

## Migration Plan

Nothing deploys. The tree moves in one bundle: the fleet rig runs, then the
surface's two rows are edited, then the check is added — the check's own failure
on the pre-re-record tree is the proof it is falsifiable. Rollback is a revert;
the superseded artifacts are recoverable from git history, and nothing outside
this repository references them.
