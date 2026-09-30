# Tasks

## 1. The task preset — `scripts/snake.lua`

- [x] 1.1 Port the board and the planner from `laya_coreml/snake/{game,policy}.py`: Hamiltonian cycle, collision/reverse/tail/food-skip rules, reachability, the compact prompt, and the shielded argmax. Kept line-for-line with the reference so the rules stay auditable.
- [x] 1.2 Build the three typed questions (`move` choice of four, `risk` noul, `food` noul) and the Laya sequence/batch for them; request `logits` + `act_logits`.
- [x] 1.3 Add the bounded episode loop: read `{board, ticks}` from `KEYS[1]`, run ≤ 200 ticks of `emb.run` + shield + `game:step`, and return `{frames = {...}, board = <final>}` where each frame carries the board, the move probabilities, `risk`, `food`, `proposed`, `executed`, `intervened`, and `input_tokens`.
- [x] 1.4 Read constants from `emb.script.config` when present (falling back to defaults), so the preset works with or without the `script-config` change.
- [x] 1.5 Determinism: carry the LCG cursor in the board so a chained episode resumes the same food sequence. (Verified by chaining calls against the tiny model; the automated equivalence test is task 2.3.)

## 2. Shipping the preset

- [x] 2.1 Add the `snake` preset to `website/repl/sandbox.yaml` (mounted beside `laya.lua`) and copy it into the image in `website/repl/Dockerfile`; the `just website-dev` derivation carries it through unchanged.
- [x] 2.2 Stamp its digest into the plate's `data-emb-preset-snake` attribute via the existing `website-presets` mechanism; `just website-presets-check` passes (8 digests current).
- [ ] 2.3 Add a preset test (the `internal/server` scripted-model pattern) proving one evaluation returns `ticks` frames, that the shield intervenes on an unsafe top-1, and that the episode is bounded. **Not done** — the episode was verified live against the vendored model (8- and 96-tick calls, shield veto observed) and by the plate's browser pass, but there is no Go test pinning it.

## 3. The plate

- [x] 3.1 Replace the per-tick browser loop with the episode fetch: `Play` requests `{board, ticks}` once and enqueues the frames; `Step` keeps one tick in one call.
- [x] 3.2 Implement the animation: a fixed-cadence frame player (~16 fps, one frame per 62 ms) advancing the board, the move-probability bars, and the counters together.
- [x] 3.3 Implement prefetch: fetch the next episode from the last board when the queue drops below the watermark, and stop the animation with the sandbox state on failure (never a fabricated frame).
- [x] 3.4 Update the readout, the mechanism strip, and the honesty copy to "one call per episode, one forward pass per frame"; unfold the single episode command.
- [x] 3.5 Render the reference implementation's own left/right dashboard: the board on the left; `laya` + `LIVE`, the four move probabilities with the model's pick marked `›`, `executing` + the `SHIELD` badge, dead-end risk and food reachability, and the score/length/best/ticks/input-token/call counters on the right.

## 4. Docs and copy

- [x] 4.1 Update `website/demos/index.html` plate 12 and `website/docs/index.html` section 10 to describe the loop running as an episode and the preset as the example of a task with a loop.

## 5. Validation

- [x] 5.1 Live pass against `just website-dev`: a played run animated to 322 ticks without stalling (score 5, length 11), `Step` showed one decision in one call (proposed `UP`, executed `DOWN`, `SHIELD`), and the command disclosure showed one episode call.
- [ ] 5.2 Confirm the plate degrades honestly with the sandbox down and with JS off; run `just lint`, `just test`, `openspec validate laya-live-loop`. Lint/tests/validate pass; the JS-off and sandbox-down passes are still owed.
