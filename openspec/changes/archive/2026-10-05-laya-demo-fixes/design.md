# Design

See proposal.md — Why.

## Context

The four examples share a mount/clear cycle (`mount()` → `clearStage()` →
`*Mount()`), a single `hudBuild` for the two loops, and one `showTree` for the two
typed-question examples. The defects come from two shortcuts in that shared code:
a `*Mount` that appends without clearing, and a readout row whose first column is a
fixed width that a marker plus a long direction overflows. Snake's preset also
diverges from Pac-Man's in how it populates the move question.

## Goals / Non-Goals

**Goals:**
- A re-selection is idempotent for every example.
- The readout's geometry is stable for every value it can show.
- Both loops show the same figure, over the same population (legal moves), with the
  executed decision as the headline.

**Non-Goals:**
- Changing what the presets ask the model or what the safety layers do.
- Making the random-weight probabilities informative; they stay shapes.
- A new example or a new command.

## Decisions

### D1 — Mounts clear their own stage

`inboxMount` clears `stage` before appending, so the ticket handler's direct call is
safe and the count cannot grow with selections. **Why:** the bug is a call path,
not a missing feature; making the mount idempotent fixes every present and future
direct caller. **Alternative:** route the ticket click through `mount()` (works, but
`mount` also resets `active`/copy and re-runs the switch bookkeeping for an in-place
change).

### D2 — Fixed columns, marker in its own cell

The move row becomes four columns — marker, direction, bar, value — with `nowrap`
on the direction, and the readout row's label column becomes `max-content` so a
label never wraps and the bar absorbs the slack. **Why:** the row's geometry must
not depend on the string it shows; a `› RIGHT` is not a special case but the worst
case. **Alternative:** widen the single direction column (a magic width that the
next longer label re-breaks).

### D3 — Legal moves only, and the decision leads

`snake.lua` builds its move criteria from the legal directions, exactly as
`pacman.lua` does (a blocked direction supplies no marker), and `build_row`'s
marker count then equals the legal count. The HUD renders one figure for both: a
`DECIDED` line with the executed move and the intervention badge, a small
`model proposed …` line, then the probability table with the executed move marked.
**Why:** over four markers including walls the softmax is flat and says nothing; over
the legal moves it at least describes the real choice, and the executed move is the
only thing the run actually decided. **Alternative:** keep highlighting the
proposal (rejected: it is the value the shield exists to correct).

### D4 — One figure, one builder

`snakeApplyFrame` and `pacmanApplyFrame` share the move/decision update; only the
two readout bars and the badge word differ, chosen by the same `hudBuild` call that
already takes the bar definitions. **Why:** the two loops drifting apart is what
produced the inconsistency in the first place.

### D5 — Copy is decisional

The example blurbs, the honesty caption and the readout labels are shortened to
what a reader decides with: what runs, where, and what the safety layer can do.
**Why:** the plate's job is to explain the mechanism at a glance; a paragraph is not
a decision. An `impeccable` pass owns the type, spacing and motion, not the palette.

## Risks / Trade-offs

- **Snake's `confidence` and answer shape change with fewer markers** → the parity
  corpus test still pins the generic preset; the loop preset is pinned by
  `TestLayaSnakeEpisode`, which reads counts and ranges, not the marker total.
- **A shorter label loses a verb** → labels keep their subject (`ghost near`,
  `pellets reachable`), and the readout bar is supporting detail.
- **Marker column width** → it is a one-glyph cell with `nowrap`; a two-glyph
  marker is a deliberate non-goal.
