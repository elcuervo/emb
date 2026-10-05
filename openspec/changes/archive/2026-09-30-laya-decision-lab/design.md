# Design

## Context

See proposal.md — Why.

The incumbent plate (`website-laya-demo`, shipped) established the wire contract
and the honesty posture: one preloaded `scripts/laya.lua`, `EMB.EVSHA laya <sha>
1 <state> <questions> <config>`, a 32-hidden random-weight miniature in the
sandbox, production checkpoints named in prose. All of that is kept. What changes
is the *teaching order* and the *figure*.

The gallery's world is fixed (`DESIGN.md`: cream paper, black rules, one orange
accent, heavy grotesk display / mono labels). The shared harness (`demos.js`)
already exposes `g.preset`, `mechanism`, `tween`, `unfold`, `el`, `showState`.
Nothing new is needed at the harness level.

## Goals / Non-Goals

**Goals**
- Make the typed-question idea legible before any model call: `choice` / `score`
  / `noul` as vocabulary, with the request and reply each one produces.
- Make every example's question set visible as the same figure — a tree of typed
  branches over one state — with the reply's own probabilities on the leaves.
- Show the loop case (Snake) honestly: a forward pass per tick, a shield that
  can veto, a random-weight model whose confidence sits at the floor — and, once
  the loop runs server-side (`laya-live-loop`), a run that reads as realtime.
- Keep every number on the page a server number; no fabricated reply.

**Non-Goals**
- No wire change for the generic preset: `scripts/laya.lua` bytes and digest stay
  put for this rework. The Snake task preset (`scripts/snake.lua`) and the config
  envelope's move onto the model entry are the `laya-live-loop` and
  `script-config` changes.
- No new Redis command. The `EMB.PREDICT` tipping condition from `emb-laya`
  still does not hold for one page.
- No second animation language; reuse `tween` and the existing `.sheet` idiom.

## Decisions

### D1. The tree is generated, not drawn

`renderTree(state, questions, reply)` builds the DOM from the questions object
the plate is about to send, then fills the leaves with values from the reply the
server returned. The static figure ("the tree you don't walk") is prose plus the
same renderer's output, not a hand-authored SVG. This removes the failure mode
that produced the old figure: a hand-drawn token diagram that no call could
contradict. It also unifies "decision tree" and "typing": the branch label is the
question id, the leaf label is a criterion, and the leaf fill is a probability.

### D2. Three examples, one switch, one call shape

`EXAMPLES` is a small table (`snake`, `inbox`, `quickstart`). Each mounts its own
controls into a shared stage and calls `predict(state, questions)` — one
`g.preset` — then hands the reply to the shared tree. Inbox and Quickstart are
single rounds (the reference `Presets.email_questions` set; the reference repo's
smallest round). Snake is the looped example; this change first shipped it as a
browser-side loop of one call per tick, and `laya-live-loop` moves the loop into
a task preset so one call returns a whole episode.

### D3. Snake: the loop is a task preset, the page is a renderer

`SnakeGame`/`HamiltonianCycle`/`moves()` are a faithful port of
`laya_coreml/snake/{game,policy}.py` (collision/reverse/tail rules, food-skip
rule, reachability BFS, compact prompt, shielded argmax). The first form of this
change ran that port **in the page**, one `EMB.EVSHA` per tick. That cannot read
as realtime: the sandbox bridge caps a client at 2 requests/s and serializes all
visitors through one upstream connection, so the loop is transport-bound while
the model is sub-millisecond. `laya-live-loop` moves the port into
`scripts/snake.lua`, which runs a bounded episode of forward passes inside one
evaluation; the page fetches the frames, animates them locally and prefetches the
next episode. A `Step` control keeps one decision in one call for inspection.
The rules stay line-for-line with the reference; only their address changes.

### D4. The miniature is labelled on the figure

The Snake example states plainly that the probabilities are near-uniform because
the sandbox checkpoint is untrained, and that the shield (not the model) keeps
the snake alive — the reference implementation's own "Laya + cycle safety"
reading. The Inbox and Quickstart examples carry the same one-line note. The
production call is shown verbatim in THE API.

## Risks / Trade-offs

- [A live loop against the shared sandbox is rate-limited] → the loop does not
  live in the page: `laya-live-loop` runs it as one bounded episode per call, so
  the 2 req/s cap governs episode fetches, not frames.
- [Ported game logic drifts from upstream] → Kept structurally line-for-line with
  `game.py`, first in the page and then in `scripts/snake.lua`; the plate is a
  demo of the mechanism, and the reference remains the authority for the rules.
- [The renderer's leaves look empty before a run] → The tree renders tracks with
  an em-dash until a reply lands; the static section explains this is the shape.

## Migration Plan

Page-only. Delete the old figure and rig markup, keep the digest attribute and
`[data-cmds]` disclosure so `just sandbox-stamp --check` is unaffected. Rollback
is the previous `laya.html`.
