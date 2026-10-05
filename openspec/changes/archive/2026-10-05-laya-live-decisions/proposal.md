# Proposal

## Why

The decision plate shows the right mechanism but the wrong telemetry. Its only
clock is the client's wall time around the whole request (`callMs`), so the number
on screen tracks the sandbox's network and queueing rather than the model; the
Snake/Pac-Man readout is a generic dark HUD that does not match the reference's
live decision view; and the Pac-Man preset's planner only vetoes ghost-adjacent
moves, so its random-weight player oscillates left↔right instead of clearing
pellets. A visitor seeing "0.31 ms" cannot tell whether that is inference or the
round trip, and a Pac-Man game that never eats teaches the opposite of the point.

## What Changes

- **Per-decision inference time.** `emb.run` and `emb.run_batch` return the model
  call's own duration (milliseconds) as a second Lua value, measured in the host
  around the session run. Presets attach it to each decision (`frame.inference_ms`,
  and one figure per non-loop reply), so the plate shows model time, never a client
  clock.
- **A live decision readout across all four examples.** The loops keep the move
  probability table; the typed-question examples keep the tree, and its leaves fill
  live, question by question, instead of one frozen tween. Every example surfaces
  the per-decision inference time beside its probabilities.
- **The readout restyled in the site's own palette.** Snake's board + readout adopt
  the reference's composition (status line, big-digit counters, "next move · model
  probabilities" table with a marked proposal, executing/shield, readout bars,
  inference and decisions/s) built from the site's existing tokens — no new colour,
  font, or panel vocabulary.
- **An active Pac-Man policy.** The planner computes a BFS first step toward the
  nearest pellet as `preferred`, vetoes immediate reversal and pellet-distance
  regression when a progressing move exists, and a stall watchdog promotes the
  planner's move after a documented run of ticks without eating. The model still
  proposes; the planner still guards.
- Restamp the changed preset digests and update the plate's docs to name the
  inference-time reading and the loop policy.

No **BREAKING** changes.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `script-tensor-io`: `emb.run` / `emb.run_batch` gain a documented second return
  value — the model call's inference duration in milliseconds, measured host-side.
- `embedding-demos`: the decision plate's readout SHALL show per-decision model
  inference time (not request time); the typed-question tree SHALL fill live; and
  the looped demo's planner SHALL pursue pellets and resist oscillation.

## Impact

- **Server/script host**: `internal/script/host.go` (`runHost`, `runBatchHost`)
  times `hosts.Run` and returns the duration; new host tests. No new command, no
  protocol change.
- **Specs**: delta on `script-tensor-io` (the `emb.run` return contract) and on
  `embedding-demos` (readout timing, live tree, loop policy).
- **Presets**: `scripts/snake.lua`, `scripts/pacman.lua` (`frame.inference_ms`),
  `scripts/pacman.lua` policy rework. Digests restamped via
  `website/tools/stamp-presets.py`.
- **Site**: `website/demos/laya.html` (readout composition, live tree reveal,
  inference-time wiring, Pac-Man controller), `website/assets/css/styles.css`
  (restyled readout in site tokens), `website/docs/index.html` §10.
- **Tests**: a host test pinning the `emb.run` duration return; a Pac-Man policy
  test pinning progress (pellet count decreases, no reversal oscillation) and the
  bounded episode.
- **Deploy**: the sandbox is hand-deployed; the preset and host changes require
  `just sandbox-deploy` after merge. CI's `stamp-presets.py --check` guards drift.
