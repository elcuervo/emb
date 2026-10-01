# Tasks

## 1. Inference-time host surface

- [x] 1.1 In `internal/script/host.go`, time `h.Run(inputs)` with `time.Since` in `runHost` and the batch equivalent, and return the duration in milliseconds as a second Lua value alongside the tensor map; verify with `go test ./internal/script/ -count=1` (build + existing suites)
- [x] 1.2 Add a host test that `emb.run` returns a positive finite duration as its second value and that a single-assignment call (`local out = emb.run(...)`) still returns the same tensors with no error; verify `go test ./internal/script/ -run 'Run.*(Duration|Second)' -count=1`
- [x] 1.3 Audit every shipped preset for a bare `return emb.run(...)`/`emb.run_batch(...)` that would now leak two values; verify `grep -rn "return emb\.run" scripts website/repl/presets` reports nothing (or is fixed) and `just verify-harness` passes
- [x] 1.4 Document the second return value in the scripting API reference (`website/docs/index.html`); verify the new text renders on the docs page served by `just website`

## 2. Presets carry per-decision inference time

- [x] 2.1 In `scripts/laya.lua`, capture the duration from the single batched `emb.run` and return it as `usage.inference_ms`; verify a manual `EMB.EVSHA ... 1 <state> <questions>` reply carries a positive `usage.inference_ms`
- [x] 2.2 In `scripts/snake.lua` and `scripts/pacman.lua`, capture the duration per `decide` and set `frame.inference_ms` (summed into `usage.inference_ms` for the episode); verify a manual episode reply carries a positive `inference_ms` on every frame
- [x] 2.3 Extend `TestLayaParityCorpus`, `TestLayaSnakeEpisode` and `TestLayaPacmanEpisode` to require a finite, positive `inference_ms` on every reply/frame; verify `go test ./internal/server/ -run TestLaya -count=1` inside `nix develop`

## 3. The live readout

- [x] 3.1 Replace the Snake/Pac-Man `call` counter with the frame's `inference_ms` and add a `decisions/s` figure derived as `1000 / inference_ms`; verify with `just website-dev` that each advancing frame updates both numbers
- [x] 3.2 Show the single-pass replies' `usage.inference_ms` in the Inbox and Quickstart readouts beside the input-token count; verify both examples display a positive figure on a run
- [x] 3.3 Run the `impeccable` pass on the loop readout: recompose the HUD into the reference's order (status, big-digit counters, next-move probability table with the proposal marked, executing/shield, readout bars, inference, decisions/s, tokens) using only existing tokens; verify with desktop and narrow-width screenshots that no new colour/font/token was introduced
- [x] 3.4 Mark a repeated single-pass run as cached in the readout (client-side memory of the exact payload) so a stored inference figure is never shown as fresh; verify by running the same example twice and seeing the mark on the second run
- [x] 3.5 Make the typed-question tree fill question-by-question via the existing `tween` helper, with a single-frame draw under reduced motion; verify the leaves end at the reply's probabilities and the reduced-motion path draws the complete tree

## 4. An active Pac-Man policy

- [x] 4.1 Replace the `preferred` heuristic in `scripts/pacman.lua` with the first step of a BFS shortest path to the nearest pellet (mirroring the BFS already in `all_pellets_reachable`), keeping the Manhattan helper only where it is still correct; verify a manual episode's `preferred`/executed moves head toward the nearest pellet across walls
- [x] 4.2 Extend the planner's veto so it rejects an immediate reversal and a pellet-distance increase when a progressing safe move exists, in addition to the existing ghost-adjacency veto; verify a scripted episode never returns an `executed` direction that reverses the previous frame while another safe direction exists
- [x] 4.3 Add a documented stall watchdog: after a fixed number of decisions with no pellet collected, the planner executes the preferred progress move until one is collected, while a safe moving option exists; verify the constant and its guard in the preset and in a test
- [x] 4.4 Add a preset test that pins progress: a bounded episode from a fresh state collects at least one pellet and never locks into a reversal oscillation; verify `go test ./internal/server/ -run TestLayaPacman -count=1` inside `nix develop`, plus `TestLayaPacmanEpisode` still passing
- [x] 4.5 Update the `ponytail:` comment that names the greedy-ghost ceiling so it records the new planner's limits; verify the comment names what the planner does and does not solve

## 5. Docs, digests, integration

- [x] 5.1 Update `website/docs/index.html` §10 and the Laya section of `README.md` to name the per-decision inference reading and the Pac-Man progress policy; verify the named behavior matches the presets
- [x] 5.2 Restamp the changed preset digests with `python3 website/tools/stamp-presets.py`; verify `python3 website/tools/stamp-presets.py --check` and `python3 website/tools/published-tree.py` both exit 0
- [ ] 5.3 Run the full local gate inside `nix develop`: `just lint`, `just verify-harness`, and `go test ./internal/... ./website/repl/ -count=1`; verify all pass
- [ ] 5.4 After merge, `just sandbox-deploy` and verify the live plate answers an episode with positive `inference_ms` and a Pac-Man run that clears pellets
