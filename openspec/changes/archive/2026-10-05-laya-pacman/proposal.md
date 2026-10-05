# Proposal

## Why

The decision plate's Snake example is the page's only loop: it proves a dependent
loop can run where the model runs, as a bounded episode in one call. A second
loop in the same idiom — a Pac-Man clone — shows the mechanism is not
Snake-shaped: different rules, different typed questions, the same one-call
episode. The demo also needs a playback bound so an unattended tab does not
animate (and prefetch) forever, and a way for the gallery to say which plate is
new.

## What Changes

- Add a third game example to `website/demos/laya.html`: a Pac-Man clone running
  the same episode contract — one `EMB.EVSHA laya <pacman-sha> 1 <state>` returns
  a bounded trace of frames, and the plate animates it locally and prefetches the
  next episode from the returned game.
- Ship the rules as a task preset, `scripts/pacman.lua`, mirroring
  `scripts/snake.lua`: it owns the maze, pellets, ghost motion, collisions and the
  safety planner, and asks Laya a `choice` over the legal directions plus `noul`
  risk questions. The host learns nothing about Pac-Man.
- Bound the browser playback: after **3 minutes of active play** the animation
  pauses, and it pauses immediately when the page loses focus or is hidden. Play
  resumes; a paused demo does not prefetch.
- Mark the new example with a `new` label on its tab in the demo page, and name it
  in the gallery's decision-engine entry.
- Register the preset everywhere a preset must be registered so the stamped
  digest and the loaded bytes cannot disagree: the canonical `scripts/pacman.lua`,
  the `website/repl/presets/pacman.lua` symlink, both sandbox configs, and the
  `stamp-presets.py` manifest.

No **BREAKING** changes.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `embedding-demos`: the decision plate gains a second looped example; a looped demo
  gains a bounded active-play window and a pause on focus loss; the gallery gains a
  way to mark a newly added demo.

## Impact

- **Preset/runtime**: new `scripts/pacman.lua` (Lua, uses `emb.run`,
  `emb.tokenize`, `emb.math.softmax` — no host change and no new command).
- **Sandbox**: `website/repl/sandbox.yaml` and `.sandbox-dev.yaml` preload the new
  preset; `website/tools/stamp-presets.py` derives its digest; CI's
  `stamp-presets.py --check` is the drift guard.
- **Site**: `website/demos/laya.html` (rig attribute, example tab, controller,
  renderer, playback policy), `website/assets/css/styles.css` (Pac-Man board + HUD
  + the `new` chip), `website/demos/index.html` (entry 12 wording + label),
  `website/docs/index.html` §10 and `README.md` (name the second task preset).
- **Tests**: a `TestLayaPacmanEpisode` parity test mirroring
  `TestLayaSnakeEpisode`, pinning one bounded episode and the chained board.
