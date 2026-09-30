# Design

## Context

See proposal.md — Why. The relevant facts:

- `emb.run` may be called repeatedly within one evaluation; the session persists
  (`examples/scripts/snippets/rerank.lua`), and each evaluation is bounded by a
  wall-clock deadline and a tensor-element budget rather than one run.
- The bridge rate-limits a client at **2 req/s** (burst 10) and serializes all
  traffic through one upstream connection (`website/repl/bridge.go`), so a
  per-frame round trip cannot be made fast from the page.
- The reference implementation's own Snake loop (`laya_coreml/snake/{game,policy}.py`)
  puts the planner and the game beside the model — the architecture this copies.
- `scripts/laya.lua` stays the generic decision preset (questions as data); the
  Snake task is a separate preset, so no model-specific code reaches the host.

## Goals / Non-Goals

**Goals**
- A run that looks realtime: frames animate at a stable cadence with no per-frame
  network.
- The model's own decisions and the shield remain visible, frame by frame.
- A preset that doubles as the readable example of "a task with a loop".
- Stay generic: nothing in the host learns about Snake or Laya.

**Non-Goals**
- No pub/sub, no streaming protocol, no new command.
- No server-side change at all (`emb.run` already loops; `script-config` is the
  only adjacent change and is optional).
- No removal of the per-tick Step path.

## Decisions

### D1. The loop moves into the preset

A dependent loop behind a 2 req/s, single-connection cap is the wrong shape. One
evaluation running N ticks removes the cap from the *animation* — only the
episode fetch is rate-limited, and one fetch covers ~96 frames.

### D2. Bounded episode, chained by the board

`snake.lua` takes the current board plus `ticks` and returns `{frames, board}`.
The page fetches the next episode from the returned board, so the game continues
seamlessly. `ticks` is bounded (≤ 200) so an episode cannot run away, and the
final board carries the RNG cursor so the food sequence is continuous across
chained episodes.

### D3. A task preset, not host code

`snake.lua` owns the planner, the rules, the prompt, the questions and the loop.
The host gains nothing Snake- or Laya-shaped. This is the same split as the
generic `laya.lua` versus the task that specializes it, and it is the working
answer to "make scripting easier" that stays general-purpose.

### D4. Prefetch keeps the network ahead of the animation

The page plays frames from a queue at a fixed cadence (~16 fps, ~63 ms/frame).
When the queue drops below a watermark it requests the next episode; the request
is issued while frames are still playing, so a fetch (a few tens of ms) never
lands in the visible timeline. A fetch failure stops the animation and shows the
sandbox state rather than fabricating frames — the gallery's existing rule.

### D5. Motion is a one-cell slide, and it is optional

Each frame is a board; the snake is drawn sliding one cell from its previous
position rather than teleporting, which is what makes the run read as motion
instead of a slideshow. Reduced-motion renders the frame directly, so no result
depends on the animation having run. `tween`/`motionAllowed` from the shared
harness are reused; no second animation language.

### D6. Step stays for inspection

`Step` = one tick = one `EMB.EVSHA` = one forward pass, with the tree and the
shield line for that single decision. `Play` = the episode loop. Keeping both
means the plate still shows the single-decision mechanism honestly, while the
loop shows what a realtime consumer looks like.

### D7. Honesty

Frames are computed live by the server in one evaluation — nothing is replayed or
precomputed by the page. The preset bounds the episode, the command disclosure
shows the single episode call, and the copy says one call per episode with one
forward pass per frame. The reply cache may serve a chained request whose board
repeats; that is the server's own answer and is labelled as such in the plate's
existing cache note.

## Risks / Trade-offs

- [An episode makes 3×N forward passes in one evaluation] → bounded `ticks`; the
  vendored miniature is sub-millisecond and a real checkpoint is ~5 ms, so 96
  ticks is well inside the evaluation deadline; the tensor budget is per small
  row and also bounded.
- [Chained episodes drift from a single continuous game] → the board carries the
  RNG cursor and the exact body/food, so the next episode resumes bit-identically.
- [A preset with game rules is more code than the browser version] → that is the
  point: the rules live with the model as a reviewable, testable script, and the
  page shrinks to a renderer.
- [`script-config` not landed yet] → the preset reads `emb.script.config` only if
  `emb.script` exists, so both orders land cleanly.
