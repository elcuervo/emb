# Proposal

## Why

The gallery's arc ends on scripting ("The graph": a Lua preset computing structure beside the model). emb now also serves Laya — a System 1 decision engine whose checkpoints answer typed questions in one forward pass — via the same scripted-model path (`scripts/laya.lua`, parity-pinned). A twelfth plate turns that into the gallery's capstone: a visitor watches one `EMB.EVSHA` answer six questions at once, and reads how to point the same command at a production checkpoint for a real support-triage pipeline. The demo page, the sandbox mount, and a docs section together make the integration legible to the operator audience the site already serves.

## What Changes

- **New demo plate — `demos/laya.html` ("The decision").** A support-ticket triage rig: four curated emails (duplicate charge, refund, phishing, and a Spanish ticket badged for the `multilingual` checkpoint a deployment would route it to), one button, one `EMB.EVSHA laya <digest> 1 <state> <questions> <config>` call per run, and a decision sheet rendering every answer at once (all rows materialize in the same lock-step — the plate's thesis is one forward pass, deliberately contrasted with the multilingual plate's sequential bars). A figure above the rig draws the input as a decision tree (five sequences, every option a `[MASK]` marker, the state at each tail) and teaches the correct reading: breadth, not depth — every marker scored in the same pass. `choice` answers draw a distribution bar, `score` a legend row, `noul` a probability split; every row carries `confidence` and the sheet header carries `action.act_probability` (the act/escalate head) and `input_tokens`.
- **Honest stand-in model.** The rig runs the vendored 32-hidden miniature (random weights, real mechanism) shipped inside the sandbox image. The plate states plainly that the miniature decides nothing and that the same command against the real English/multilingual checkpoints is the integration; "THE EXACT COMMANDS" shows the production-shaped call with the config envelope, plus a `redis-cli`/Ruby client snippet.
- **Gallery integration.** `demos/index.html` grows to twelve plates with a rewritten intro and reading-order entry; `plate-nav` chains after "The graph"; the `function.html` "four answers" plate gains a forward teaser to the decision plate.
- **Sandbox mount.** `website/repl/sandbox.yaml` adds a `laya` model entry (onnx/tokenizer from the shipped miniature, `scripts: [presets/laya.lua]`); the Dockerfile copies `testdata/laya/` and `scripts/laya.lua` into the image so the preset digest is stamped by the existing `just sandbox-stamp` and — **BREAKING** for the sandbox only — the next `just sandbox-deploy` ships the new model+preset (no visitor-visible API change; `EMB.EVAL` stays refused).
- **Docs section.** `docs/index.html` gains `10 · Laya decision models`: mounting, the wire contract (`KEYS`/`ARGV`, state serialization, criteria order, config envelope), the question schema, calibration clamps, the parity corpus, and the honest note that the sandbox miniature is a mechanism stand-in.
- **Dev loop.** `just website-dev` picks up the new model entry through the existing sandbox-config derivation; `test-laya.yaml` (repo root) already documents the standalone mount.

## Capabilities

### New Capabilities
<!-- none: every change lands in an existing capability -->

### Modified Capabilities
- `embedding-demos`: a twelfth plate demonstrates the scripting surface answering typed questions in one forward pass, and a stand-in model demo states that its model is a mechanism substitute whose numbers are not judgments.
- `product-docs`: the docs surface documents the Laya decision-model integration (mount, wire contract, question schema, calibration).
- `sandbox-service`: the showcase mounts a shipped miniature decision model with its preset by digest, alongside the downloaded retrieval models.

## Impact

- `website/demos/laya.html` (new), `website/demos/index.html`, `website/demos/function.html` — plates and gallery intro.
- `website/repl/sandbox.yaml`, `website/repl/Dockerfile` — laya model entry + in-image model/preset copies.
- `website/docs/index.html` — section 10 (TOC + body).
- `website/repl/presets/` deployment (the canonical `scripts/laya.lua` shipped as a preset; digest stamped by `just sandbox-stamp`).
- `testdata/laya/` reused as the shipped miniature (already committed by the `emb-laya` change).
- No server API change; no wire-format change; sandbox deploy is hand-driven (site CI never reaches `cli.emb.is`), so the new plate is live only after `just sandbox-deploy`.