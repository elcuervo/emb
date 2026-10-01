# Proposal

## Why

Testing the shipped decision plate surfaced three defects. Selecting a different
Inbox ticket calls `inboxMount()` without clearing the stage, so every switch
stacks another ticket list and form (4 switches → 4 lists, 16 buttons). Pac-Man's
proposed-motion row wraps — the `› RIGHT` marker overflows the 48px direction
column — so the probability table breaks whenever a long direction is proposed.
Snake builds its move question from all four directions including walls, so the
softmax is flat 0.25 forever and its `›` marks the model's pick rather than the
move that was actually played. The four examples also disagree on which controls
they show and how they name the decision.

## What Changes

- **Inbox re-selection replaces the panel.** A demo's mount clears its own stage,
  so switching a ticket (or any re-selection) never stacks duplicate controls.
- **The readout holds its layout.** The move row becomes fixed columns — marker,
  direction, bar, value — with no wrapping, and the readout-bar labels stop
  wrapping, so a long proposed direction cannot break the table.
- **The loops show the executed decision.** Both loop presets mark the move
  probabilities over the **legal** moves only (Snake joins Pac-Man), the HUD leads
  with the move that was actually executed, and the model's proposal is named
  beside it rather than being the highlight.
- **The typed-question demos show their decisions.** Inbox and Quickstart lead with
  each question's chosen answer (the argmax of its distribution, matching the
  marked leaf), and the Inbox set covers the question vocabulary with varied
  tickets instead of one repeated shape.
- **Consistent readout and copy.** Both loops render the same figure through one
  builder; the blurbs, caption and labels are shortened to decisional copy.

No **BREAKING** changes.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `embedding-demos`: a re-selected demo must replace its surface; a looped demo's
  readout must show the executed decision over the legal moves; the readout must
  not reflow as its values change.

## Impact

- **Site**: `website/demos/laya.html` (mount clearing, unified loop readout, copy),
  `website/assets/css/styles.css` (HUD columns + decision styles).
- **Presets**: `scripts/snake.lua` (move criteria limited to legal directions, like
  `pacman.lua`); digests restamped via `website/tools/stamp-presets.py`.
- **Tests**: a parity test catches the empty questions set already; the layout and
  duplicate-control fixes are verified in the browser and by the detector.
