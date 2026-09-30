# Tasks

## 1. Sandbox mount

- [x]1.1 Add the `laya` model entry to `website/repl/sandbox.yaml` (onnx/tokenizer pointing at the shipped miniature `laya-model/`, `scripts: [presets/laya.lua]`), and extend `website/repl/Dockerfile` to copy `testdata/laya/` and the canonical `scripts/laya.lua` into the image beside the existing presets; verify an image build (`just sandbox-image` or the local dev derivation) boots with `EMB.SCRIPT EXISTS laya <digest>` → `[1]`.
- [x]1.2 Extend the `just website-dev` config derivation so the laya entry resolves to repo-relative `testdata/laya/` paths; verify `just website-dev` boots the local sandbox with the decision plate's model and preset.
- [x]1.3 Verify `just sandbox-stamp` (or its `--check` mode) bakes/stamps the shipped `laya.lua` digest and passes with the plate's stamped digest.

## 2. The plate — `website/demos/laya.html`

- [x]2.1 Build the page from the existing anatomy (masthead, plate XII "The decision" with teaches + figure, WHAT YOU ARE LOOKING AT, TRY IT with mechanism strip, ridge break, WHAT JUST HAPPENED, WHY IT MATTERS, THE EXACT COMMANDS, plate-nav, footer) using the shared `demos.js` harness (`g.preset`), `tldr.js`, and the world's tokens — following the impeccable craft floor; run `impeccable shape`/reference checks and a live browser pass before finishing.
- [x]2.2 Implement the rig: ticket queue (several curated states including the corpus's duplicate-charge, refund, and phishing states), one `EMB.EVSHA` per run, and the lock-step decision sheet — choice distribution bars with an orange winner, score legend row, noul probability split, `confidence` per answer, and `action.act_probability` + `input_tokens` in the sheet head; all rows render together.
- [x]2.3 Wire the honesty contract: "WHAT YOU ARE LOOKING AT" and the figure state the 32-hidden miniature stands in (mechanism only, numbers not judgments), and "THE EXACT COMMANDS" shows the production-shaped invocation with the real config envelope; the plate degrades honestly when the sandbox is unreachable (shared harness).
- [x]2.4 Wire `[data-cmds]` (commands this run sent) unfolding the exact `EMB.EVSHA laya <digest> 1 <state> <questions> <config>` call, and stamp the preset digest into `data-emb-preset-laya`.

## 3. Gallery integration

- [x]3.1 Update `website/demos/index.html`: twelve plates (intro copy, reading-order entry for the decision plate, `data-fig-*` line), and chain `plate-nav` from `graph.html` → `laya.html`.
- [x]3.2 Add the teaser link on `website/demos/function.html` ("the model is a function" → "…and decides"), keeping its prose honest.

## 4. Docs

- [x]4.1 Add section `10 · Laya decision models` to `website/docs/index.html` (TOC entry + body: mounting an export bundle, preloading the preset, the wire contract with state serialization/criteria order/config envelope, the question schema, calibration clamps, parity basis, and the sandbox stand-in note), cross-linked with the demo and the README section.

## 5. Validation

- [x]5.1 Run the full gate: `just format`, `just lint`, `go test ./...`, `openspec validate website-laya-demo`, and a live browser pass of the new plate on desktop and mobile (impeccable bounded QA — screenshots, defect scan, one fix round); verify the dev loop runs the plate end to end against the local sandbox.
- [x]5.2 Verify `just sandbox-stamp --check` passes with the plate in place, and confirm the static page (no scripts) reads as a complete explanation.