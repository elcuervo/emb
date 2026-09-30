# Proposal

## Why

The decision plate's Snake runs **one `EMB.EVSHA` per tick from the browser**.
Every tick therefore pays browser → bridge → emb, and the sandbox bridge caps a
client at **2 requests/s** while serializing every visitor through a single
mutex-guarded upstream RESP connection. The vendored miniature is sub-millisecond,
so the model is not the bottleneck — the loop is. Paced at ~1.5 ticks/s it reads
as a slideshow, and "the model is fast" is exactly the thing the plate fails to
show.

The Snake loop is **dependent**: tick *k+1* needs tick *k*'s move. A dependent
loop belongs where the model is, not behind a per-frame round trip. Moving it
into a task preset makes one call produce a whole episode, and the browser
becomes a renderer that can animate at a real frame rate.

This is the shape pub/sub was reaching for and would not have delivered: a broker
adds queueing and at-most-once loss to a loop that is already synchronous and
local.

## What Changes

- **New task preset `scripts/snake.lua`.** Self-contained: the Hamiltonian
  safety planner, the board rules, the compact prompt, the three typed questions
  (`move` choice, `risk` noul, `food` noul), and a bounded episode loop that
  calls `emb.run` once per tick inside **one** evaluation. Input is the board and
  a tick count; output is the frame trace plus the final board. It reads its
  constants from `emb.script.config` when present (change `script-config`) and
  falls back to defaults otherwise.
- **The plate animates the trace.** One call fetches an episode; the page draws
  frames at a fixed cadence (~16 fps) with a one-cell slide, the probability bars
  advancing per frame. When the buffer drops below a watermark it fetches the
  next episode from the last board, so the network stays ahead of the animation
  and the run never stalls.
- **`Step` stays.** A single step remains one tick in one call — the honest
  "one decision, one forward pass" view — with `Play` as the episode loop.
- **The honest copy changes** from "one call per tick" to "one call per episode;
  each frame is one forward pass", and the command disclosure shows the episode
  call rather than 96 of them.
- **Sandbox ships the preset** beside `laya.lua`, stamped by the existing digest
  mechanism.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `embedding-demos`: a demo of a dependent decision loop SHALL run the loop where
  the model runs — a bounded episode per call — and animate the returned trace
  locally, without a round trip per rendered frame.

## Impact

- `scripts/snake.lua` (new), `website/repl/sandbox.yaml`, `website/repl/Dockerfile`.
- `website/demos/laya.html` + `website/assets/css/styles.css` — the episode
  fetch, the animation/prefetch loop, the Step path, the copy.
- `website/demos/index.html`, `website/docs/index.html` — the plate's description.
- Depends on `script-config` for the preset's constants; tolerates its absence
  (`emb.script and emb.script.config or {}`), so it can land either order.
- No wire-format change, no new command, no host change: `emb.run` already runs
  many times per evaluation (the rerank snippet proves the pattern).
