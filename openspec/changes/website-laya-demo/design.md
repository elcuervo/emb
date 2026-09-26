# Design

## Context

See proposal.md — Why. The gallery (`website/demos/`) is eleven self-contained plates sharing one world (neobrutalist technical poster: cream paper, black rules, one orange accent; heavy grotesk display / mono labels / sparse body — repo `DESIGN.md`) and one harness (`website/assets/js/demos.js`: `gallery().preset(model, digest, texts, args)` compiles exactly to `EMB.EVSHA <model> <sha> <n> <texts...> <args...>`; `mechanism`, `playStages`, `unfold`, `tldr.js`). The sandbox (`website/repl/`) preloads per-model presets from `sandbox.yaml`, hashes them into a digest manifest the bridge accepts, and `just sandbox-stamp` bakes those digests into the plates. The `emb-laya` change shipped `scripts/laya.lua` (parity-pinned against the vendored corpus at `testdata/laya/`) — the exact wire contract `KEYS[1]=state, ARGV[1]=questions, ARGV[2]=config`.

The fourth gallery group's capstone is "The graph" (a script computing structure). The decision plate closes the arc one step further: the script doesn't reshape embeddings, it answers typed questions.

## Goals / Non-Goals

**Goals:**
- One plate that makes the single-forward-pass fact legible: all answers render together, from one `EMB.EVSHA`.
- A real-world frame (support-ticket triage) with a reproducible production-shaped command as the integration artifact.
- Honest mechanism demo: the shipped miniature is labeled as such; nothing is fabricated when the sandbox is down.
- Everything stays inside the incumbent world — no new tokens, no DESIGN.md change.

**Non-Goals:**
- No server-side change: `scripts/laya.lua`, the request-time config envelope, and the wire contract are already shipped and parity-pinned.
- No new commands or demo-harness features: `g.preset`, the digest stamping, and the per-model script preload all already exist.
- No real-checkpoint downloads in the sandbox (820 MB+ is out of a 1.92 GiB machine's story); the docs point at the published checkpoints for production.

## Decisions

### D1. Plate: "The decision", PLATE XII, after "The graph"

New `website/demos/laya.html`; the index becomes twelve plates with the intro's "the last is the scripting layer" extended to "…computing structure, then answering questions". Reading order: the decision plate is the capstone of the scripting story, so it follows `graph.html` and its `plate-nav` chain links back to the graph. Alternative considered: folding the demo into `function.html` — rejected: the plate deserves its own page and the gallery's anatomy guarantees consistency; function.html only gains a teaser link ("four answers → an answer").

### D2. Rig: ticket queue → one call → lock-step decision sheet

Three to four curated tickets (the corpus's own states — duplicate charge, refund, phishing — plus an account-locked variant) as a selectable queue. One button runs exactly one `g.preset('laya', digest, [state], [questionsJSON, configJSON])`; all answers render in the same pass (rows appear together — the deliberate contrast with multilingual's sequential bars). Choice rows draw a per-option distribution bar (orange winner, black rules, mono labels); score rows draw a position on a legend; noul rows draw a probability split; the sheet head carries `action.act_probability` and `input_tokens`. Rendering is HTML rows + CSS bars (rule-framed, no cards) rather than a bespoke SVG — the sheet is tabular data, and it keeps a11y simple; the existing `atlas__*` SVG conventions stay for plots. The questions payload is the corpus's six-question set and the config envelope merges the vendored `rl_agent_config.json`/`onnx_config.json` — the same bytes the parity test uses.

### D3. Sandbox mount: shipped, not downloaded

`sandbox.yaml` gains a `laya` model entry with `onnx`/`tokenizer` pointing at a shipped miniature and `scripts: [presets/laya.lua]`; the Dockerfile copies `testdata/laya/` and `scripts/laya.lua` into the image (the existing `presets/` copy pattern; `downloadModel` early-returns when the configured file exists, so no `model_repo` is needed). The local dev loop (`just website-dev`) rewrites the entry's paths to repo-relative `testdata/laya/` in its derived config. Digest stamping is unchanged: the bridge hashes the shipped preset bytes, `just sandbox-stamp` bakes the digest into `data-emb-preset-laya`. `sandbox-stamp --check` in CI keeps the plate's stamped digest equal to the shipped bytes.

### D4. Honesty contract for the stand-in

The miniature's weights are random; its answers are shapes, not judgments. The plate's "WHAT YOU ARE LOOKING AT" says so, the figure carries the label (`32-dim sandbox miniature · mechanism only`), and "THE EXACT COMMANDS" shows both the sandbox run and the production-shaped invocation (same command, real config envelope, notes on the English/multilingual checkpoints). The docs section repeats the note. This satisfies the gallery's existing honesty requirements (no fabricated replies; static content survives a sandbox outage) via the shared harness.

### D5. Docs: section 10 in the existing one-page reference

`docs/index.html` gains `10 · Laya decision models` (TOC + body): mount an export bundle, preload the preset, the wire contract (`KEYS`/`ARGV`, Python-style state serialization, criteria order, config envelope), the question schema, calibration clamps, and the parity basis. It inherits the docs page's existing structure and styling — no new pattern, cross-linked from the demo and the README section.

## Risks / Trade-offs

- [Stamped digest drifts from the shipped preset bytes] → `just sandbox-stamp --check` in CI fails the build; the plate and the ship are one artifact.
- [Local dev config mishandles the shipped-model paths] → `website-dev`'s config derivation rewrites the laya entry to repo-relative paths; verified by booting the dev loop before merging.
- [Deploy gap: the plate is live only after `just sandbox-deploy`] → stated in the proposal's Impact; the site's own CI never reaches `cli.emb.is`, matching the existing sandbox deploy convention.
- [A visitor reads the miniature's numbers as judgments] → the plate and docs label the stand-in explicitly (D4); the production call is shown verbatim.

## Migration Plan

Sandbox-only: `just sandbox-deploy` ships the new model + preset + plate together. Local dev flow unaffected (`website-dev` derives its own config). No server API change; nothing to roll back except the deploy.

## Open Questions

None that block the design. Page title ("The decision") and the exact fourth ticket's wording are taste calls the plate implementation can settle without changing the specs; the questions payload stays the parity-pinned six-question set either way.