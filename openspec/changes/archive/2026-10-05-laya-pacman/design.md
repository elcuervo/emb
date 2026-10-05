# Design

## Context

The decision plate (`website/demos/laya.html`) runs three examples through one
`EMB.EVSHA` call. The Snake example is a *task preset*: `scripts/snake.lua` owns
the board, the safety planner and the three typed questions, and one call returns
a bounded episode of frames plus the board to resume from. The plate animates the
frames locally and prefetches the next episode. The preset's digest is stamped
into the page from the preset bytes (`website/tools/stamp-presets.py`), and the
bridge only accepts digests the sandbox config preloaded — so the shown digest and
the loaded bytes cannot disagree.

The constraints that shape this design:

- The sandbox bridge rate-limits and serializes clients, so a round trip per frame
  cannot look realtime; the loop must stay inside the preset (proposal's episode
  requirement in `embedding-demos`).
- The served checkpoint is a random-weight miniature. Its answers are shapes, not
  judgments, so game rules may not depend on the model being right.
- The preset is called by digest and cannot be edited per request: it must clamp
  its own tick bound and validate its own state.

## Goals / Non-Goals

**Goals:**
- A second looped example whose rules are visibly not Snake's, sharing the one-call
  episode contract and the local-animation model.
- A playback bound that survives an abandoned tab: bounded active play, pause on
  focus loss, no prefetch while paused.
- A derived, CI-checked digest — never a typed one.
- A `new` affordance on the tab and the gallery entry.

**Non-Goals:**
- A faithful Pac-Man port: no authentic maze, no per-ghost scatter/chase timers, no
  cutscenes, no sound, no keyboard play.
- A new demo plate or a thirteenth reading-order entry.
- A host or protocol change: no new command, no new preset surface.
- Ghost colour fidelity to the arcade original.

## Decisions

### D1 — The example lives inside the decision plate, as a tab

A new `PAC-MAN` tab beside Snake / Inbox / Quickstart, in the existing
`EXAMPLES` switch. **Why:** the plate is already the "decision loop" plate; a
separate page would duplicate the rig, the mechanism strip, the tree figure and
the command disclosure, and would break the gallery's twelve-plate reading order
and the docs' fixed numbering. **Alternative:** a standalone `pacman.html` plate
(rejected: four copies of shared machinery, and every "twelve plates" figure
would drift).

### D2 — `scripts/pacman.lua` mirrors `snake.lua`'s episode shape

```
EMB.EVSHA laya <pacman-sha> 1 '{"ticks":N,"game":null|{…},"seed":7}'
  -> {"frames":[{board, probs, proposed, executed, intervened, shield, …} × N],
      "game":{…}, "usage":{"input_tokens":…,"output_tokens":0}}
```

Same `MAX_TICKS` clamp, same `load_game`/`game_of` chaining, same per-tick
`decide()` that builds rows with `emb.tokenize` and runs `emb.run`. **Why:** one
contract, tested once; the plate's prefetch loop is reused almost verbatim.
**Alternative:** a per-tick preset called from JS (rejected: the bridge's per-frame
round trip stalls the animation and violates the episode requirement).

### D3 — The model picks a direction; the rules own everything else

Each tick asks one `choice` over the legal directions (UP/DOWN/LEFT/RIGHT, with
wall and reverse rules), plus `noul` reads ("is a ghost within two tiles?", "are
all pellets still reachable?"). A deterministic planner computes the *safe* set —
moves that do not walk into a non-frightened ghost and do not strand the board —
and vetoes the model's pick that is unsafe, exactly as Snake's shield does.
Maze, pellets, power pellets, ghost motion, collisions, lives and score are pure
rules. **Why:** the miniature's weights are random; without the veto it plays
garbage and the demo teaches nothing. It also keeps the honesty line true: the
preset runs the real mechanism, the game stays playable. **Alternative:** let the
model drive everything (rejected: random weights make it unplayable and the
"mechanism only" caption would be misleading); let a scripted AI play and show no
model (rejected: then it is not a decision demo).

Ghost motion is a deterministic greedy step toward the player's tile, reversing
only when blocked, with frightened ghosts fleeing. **Why:** correct-looking and
cheap in Lua; the point is the decision, not the ghost AI. Marked with a
`ponytail:` comment naming the ceiling (no BFS/targeting) and the upgrade path.

### D4 — Playback policy in the browser: 3 active minutes, pause on blur/hide

The controller tracks *active* play time and stops advancing frames at
`PAC_BUDGET_MS = 180_000`, setting a `PAUSED · 3 min` status. It pauses on
`document.visibilitychange` (hidden) and `window.blur`, and does **not** auto-
resume — Play resumes and the budget continues from where it stopped. While
paused, neither the frame timer nor the prefetch runs. **Why:** an unattended tab
should not animate or spend sandbox work; a pause the user did not ask for is
less surprising if it needs a click to leave. **Alternative:** wall-clock deadline
from first play including hidden time (rejected: a backgrounded tab would burn the
budget); auto-resume on focus (rejected: surprising, and it resumes work the user
did not ask for).

### D5 — Visual language stays the plate's; the `new` label is CSS

The board reuses the `.snake`/`.hud` tokens on the dark panel (paper, ink,
`--accent`, the muted green). Ghosts are distinguished by form and eye direction,
not by four new hues. The `new` label is a generated chip on the tab — a
`data-new` attribute with an `::after` — so the button's text and its accessible
name stay the example's name. **Why:** the plate's two-colour world is a stated
rule on the site; the label is decoration, not markup a script must maintain.
**Alternative:** four arcade ghost colours (rejected: new palette the design
system forbids without a reason); a hand-written `<span>new</span>` per tab
(rejected: a second place to forget).

### D6 — The digest is derived, never typed

Add `"pacman": ("laya", "scripts/pacman.lua")` to `PRESETS` in
`website/tools/stamp-presets.py`; symlink `website/repl/presets/pacman.lua` to it;
add the preset (with the same envelope config) to both `sandbox.yaml` and
`.sandbox-dev.yaml`; put `data-emb-preset-pacman=""` on the rig; run
`just website-presets`. CI's `stamp-presets.py --check` fails the moment the
page's digest and the preset bytes diverge. **Why:** this is the existing
mechanism; the only new thing is one row in the manifest.

### D7 — Bounded episode, bounded maze

The maze is a fixed hand-authored grid small enough to render legibly and cheap
enough to run in Lua, with 1-tile corridors. The preset rejects a request whose
`ticks` is not a finite number and clamps it to `MAX_TICKS`. **Why:** the episode
requirement says one request cannot run unbounded; a fixed maze keeps a sealed
board. **Alternative:** a generated maze (rejected: more Lua, and no readability
gain).

## Risks / Trade-offs

- **Ghost colour vs the no-new-colour rule** → distinguish by shape/fill and eye
  direction; revisit only if the board reads muddy (impeccable pass decides with
  a live screenshot).
- **A maze with a dead end makes the legal move set empty** → the preset keeps a
  fallback move (reverse is always legal unless the ghost sits on it) and the
  board is authored with no dead ends; the parity test asserts four probabilities
  per frame, which a dead end would break.
- **Pause leaks sandbox work** → pause stops prefetch as well as the frame timer;
  the plate's commands disclosure still shows the calls that were made.
- **Digest drift on a preset edit** → `just website-presets` + CI's `--check`.
- **Doc/gallery figures drift** ("twelve plates", the §10 prose) → the same change
  edits entry 12, §10 and the README; the plate count is unchanged, so no figure
  moves.
- **Preset size growth** → the maze and ghosts are a few hundred lines of Lua in
  the same style as `snake.lua`; no new dependency.

## Migration Plan

Ship as one commit with the preset, both sandbox configs, the stamp, the page,
the CSS, the docs, and the parity test. The sandbox is deployed by hand
(`just sandbox-deploy`); a stale sandbox simply refuses the new digest and the
plate shows the sandbox's own error state (the demos requirement), so rollback is
reverting the page — the preset and config rows are inert until referenced.

## Open Questions

- Whether the playback bound should also apply to the Snake example (same
  unattended-tab argument). Deferred: the change is scoped to the new example, and
  the shared controller can adopt the policy later without a spec change.
