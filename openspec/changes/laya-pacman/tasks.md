# Tasks

## 1. The preset

- [x] 1.1 Write `scripts/pacman.lua` mirroring `scripts/snake.lua`'s shape: `load_game`/`game_of` chaining, `MAX_TICKS` clamp, per-tick `decide()` that builds typed rows with `emb.tokenize` and runs `emb.run`, and a deterministic maze + pellets + ghosts + collisions; verify `go test ./internal/server/ -run TestLayaPacmanEpisode -count=1` passes inside `nix develop`
- [x] 1.2 Add the safety planner: a `choice` over the legal directions, a `noul` ghost-proximity read and a `noul` reachability read, and a veto that replaces an unsafe model pick; add a `ponytail:` comment naming the greedy-ghost ceiling and its upgrade path; verify a scripted episode never returns an illegal `executed` direction
- [x] 1.3 Add `TestLayaPacmanEpisode` to `internal/server/laya_parity_test.go`, mirroring `TestLayaSnakeEpisode`: one bounded episode returns that many frames, four probabilities summing to 1 per frame, and the returned `game` chains into the next episode; verify it passes with `go test ./internal/server/ -run TestLaya -count=1`

## 2. The wiring

- [x] 2.1 Symlink `website/repl/presets/pacman.lua -> ../../../scripts/pacman.lua` and verify `git ls-files -s website/repl/presets/pacman.lua` reports mode `120000`
- [x] 2.2 Register the preset (path + the same `max_len`/`head_max_len`/`min_seq`/`min_markers`/`temperature`/`temperature_by_options` envelope) under `models.laya.scripts` in both `website/repl/sandbox.yaml` and `website/repl/.sandbox-dev.yaml`; verify both files list `presets/pacman.lua` exactly once
- [x] 2.3 Add `"pacman": ("laya", "scripts/pacman.lua")` to `PRESETS` in `website/tools/stamp-presets.py`; verify `python3 website/tools/stamp-presets.py --check` exits 0 after 3.1 stamps the page

## 3. The plate

- [x] 3.1 Add `data-emb-preset-pacman=""` to the `[data-rig="laya"]` element in `website/demos/laya.html` and run `just website-presets`; verify the attribute now carries the same 40-hex digest as `sha1(scripts/pacman.lua)`
- [x] 3.2 Add the `PAC-MAN` example to `EXAMPLES` (before Snake or after, but named), with a blurb that says the loop runs in the preset and spells out the model's scoped authority; add a `data-new` attribute to its tab; verify the tab renders with the mark and the blurb on click
- [x] 3.3 Implement `pacmanMount`/`pacmanFetch`/`pacmanPlay`/`pacmanStep`/`pacmanApplyFrame` and an SVG `pacmanSVG` renderer over the returned frames, reusing the episode prefetch loop; verify a full episode animates, Step runs exactly one tick in one call, and Reset starts a new game
- [x] 3.4 Implement the playback policy: pause after 3 minutes of active play and on `visibilitychange`/`blur`, stop prefetch while paused, resume from the Play button without resetting the window; verify with the tab backgrounded (pauses) and by shortening the budget locally (pauses and reports it)

## 4. The style

- [x] 4.1 Run the impeccable pass on the Pac-Man board and HUD: add styles in `website/assets/css/styles.css` beside `.snakegrid`/`.hud`, reusing the plate's tokens; distinguish ghosts by form and eye direction rather than four new hues; verify with a live screenshot at desktop and narrow widths (the `.snakegrid` breakpoint pattern)
- [x] 4.2 Style the `new` chip for the tab (`[data-new]::after` or equivalent) so the tab's text and accessible name remain the example's name; verify the chip is visible in both pressed and unpressed tab states and that the button's accessible name is unchanged
- [x] 4.3 Verify the board and HUD render legibly with JavaScript's live data only — no placeholder marks — using `just website-dev` and the sandbox preset; capture before/after screenshots as evidence

## 5. Docs and gallery

- [x] 5.1 Update `website/demos/index.html` entry 12 to name the Pac-Man example and add the `new` label to the decision-engine row; verify the label is visible and the link still targets `laya.html`
- [x] 5.2 Update the "Loops" paragraph in `website/docs/index.html` §10 and the Laya section of `README.md` to name `scripts/pacman.lua` as a second task preset; verify the commands named match the preset's actual `EMB.EVSHA` shape

## 6. Integration

- [x] 6.1 Run `just website-presets-check`, `just verify-harness`, and `go test ./internal/server/ -count=1` inside `nix develop`; verify all pass
- [x] 6.2 Drive `just website-dev` end to end: play the Pac-Man tab for a full episode, confirm the commands disclosure shows the episode calls (not one per frame), confirm prefetch keeps the animation from stalling, and confirm a sandbox failure shows the error state instead of frames
- [ ] 6.3 `just sandbox-deploy` (the sandbox is deployed by hand) and verify the live plate answers `EMB.EVSHA laya <pacman-sha> …` against `cli.emb.is`
