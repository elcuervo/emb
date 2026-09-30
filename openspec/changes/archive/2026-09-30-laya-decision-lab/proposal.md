# Proposal

## Why

The first Laya plate (`website-laya-demo`) hid its own thesis. It opened on a
1 900-pixel token-sequence SVG, buried the rig in a dark band, and closed on
three full-width JSON dumps. A reader could not answer the three questions that
matter — *what is a typed question, what does one call actually send, what comes
back* — without decoding the figure first. The lesson was in the page, not on it.

The reference implementation's own demos point the other way: a tiny typed-question
example (`examples/quickstart.py`, `examples/questions.json`) and a real game loop
(`laya_coreml/snake/`) where the model answers `choice`/`noul` questions every
tick with visible probabilities and a safety shield. This change rebuilds the
plate from scratch around that shape: **state in, a tree of typed questions, one
forward pass, typed answers out** — taught once as vocabulary, then shown three
times: a game loop, a triage sheet, and the smallest round.

## What Changes

- **Rewrite `website/demos/laya.html`.** The plate is reorganized around one
  idea and one shared figure:
  - **THE THREE TYPES** — `choice` / `score` / `noul`, each with the request it
    takes and the reply shape it returns. The vocabulary comes before any call.
  - **THE TREE** — the plate's one figure: a state is the root, each typed
    question is a branch, each option is a leaf, and one forward pass scores
    every leaf. A caption contrasts it with a walked decision tree (breadth, not
    depth) — which is why the answers arrive together.
  - **THREE EXAMPLES, one interaction** — a switch selects the example and the
    same tree renderer draws its actual question set, with the model's own
    probabilities filling the leaves:
    - **Snake** (flagship, ported from `laya_coreml/snake/game.py` + `policy.py`):
      a live board asking `move` (choice of four), `risk` (noul), `food` (noul)
      each tick, and the cycle safety shield drawn as an explicit veto on the
      move set. The loop runs as a **bounded episode inside one call** — the
      preset owns the board, the planner and the questions — and the plate
      animates the returned frames locally; a single `Step` keeps one decision in
      one call (see the `laya-live-loop` change, which replaces the browser-side
      per-tick loop this change first shipped).
    - **Inbox**: the reference `Presets.email_questions` five-question triage.
    - **Quickstart**: the reference repo's own `examples/questions.json` round.
- **One tree renderer for every example.** `renderTree(state, questions, reply)`
  builds the state root and one branch per question from the payload actually
  sent, and fills each leaf with the reply's probability — so the figure is never
  a hand-drawn illustration and cannot drift from the wire.
- **API section made concrete.** The exact `EMB.EVSHA` call for each example, the
  envelope (`KEYS[1]` state, `ARGV[1]` questions, `ARGV[2]` config), the
  Python client from the reference repo, and the honest note that the sandbox
  serves a random-weight miniature.
- **Gallery index + docs.** `demos/index.html` plate 12 copy updated;
  `docs/index.html` section 10 re-pointed at the three examples and the tree
  reading.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `embedding-demos`: a decision-model plate SHALL draw each example's question
  set as a tree whose leaves carry the model's own reply, and SHALL show the
  typed-question vocabulary and the API before the examples.

## Impact

- `website/demos/laya.html` — rewritten.
- `website/assets/css/styles.css` — styles for the type cards, the question tree,
  the example switch, and the Snake board.
- `website/demos/index.html` — plate 12 entry rewritten.
- `website/docs/index.html` — section 10 re-pointed at the examples/tree.
- No server or wire-format change: `scripts/laya.lua` and the shipped miniature
  are unchanged by this plate's own rework. The loop's server-side form is the
  `laya-live-loop` change (`scripts/snake.lua`), whose preset digest does move the
  plate's stamped attribute; the config envelope moving onto the model entry is
  the `script-config` change.
