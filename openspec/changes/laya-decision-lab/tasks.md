# Tasks

## 1. The plate

- [x] 1.1 Rewrite `website/demos/laya.html`: masthead/anatomy kept; hero re-aimed at "state in, typed questions, one forward pass"; THE THREE TYPES vocabulary section; THE TREE teaching figure; THE API section with the per-example commands and the Python client.
- [x] 1.2 Implement the shared `renderTree(state, questions, reply)` and the `predict()` wrapper (one `g.preset`, digest from `data-emb-preset-laya`).
- [x] 1.3 Implement the example switch (`snake` / `inbox` / `quickstart`) with the shared stage, readout, and `[data-cmds]` disclosure.
- [x] 1.4 Implement the Snake example: port `SnakeGame` + planner from `laya_coreml/snake/{game,policy}.py`, the board drawing, Step/Auto/Reset, the compact prompt and questions, the shielded argmax, and the honesty note. (The browser-side loop this shipped is superseded by `laya-live-loop`, which runs the loop in `scripts/snake.lua` as a bounded episode and animates the trace; the port itself carries over.)
- [x] 1.5 Implement the Inbox example (four tickets, `Presets.email_questions`) and the Quickstart example (reference `examples/questions.json`).
- [x] 1.6 Styles in `website/assets/css/styles.css`: type cards, question tree, example switch, Snake board/results.

## 2. Gallery + docs

- [x] 2.1 Update `website/demos/index.html` plate 12 copy (three examples, the tree, the API) and the intro line if needed.
- [x] 2.2 Update `website/docs/index.html` section 10 to point at the three examples and the tree reading; keep the wire contract and clamps.

## 3. Validation

- [x] 3.1 Live browser pass (desktop + mobile) of every example against `just website-dev`; confirm the tree fills from real replies, the Snake shield reads correctly, and the static (JS-off) page is a complete explanation.
- [x] 3.2 `just sandbox-stamp --check` still passes (digest untouched); run the site's HTML/asset checks used by CI.

## 4. Follow-on

- [x] 4.1 `laya-live-loop`: move the Snake loop into the `scripts/snake.lua` task preset (bounded episode per call) and animate the returned trace in the plate, with prefetch. Tracked there; this change's browser-side loop is the interim form.
- [x] 4.2 `script-config`: move `scripts/laya.lua`'s envelope onto the model entry as `emb.script.config`, dropping the config argument from the call shown in THE API.
