# Design

See proposal.md — Why. The constraints that shape this design come from
`openspec/specs/embedding-demos/spec.md` (the tree is the reply's payload; a plate
introduces no new colour/font/token; a loop runs as one bounded episode) and
`openspec/specs/script-tensor-io/spec.md` (the `emb.run` return contract).

## Context

`scripts/laya.lua`, `scripts/snake.lua` and `scripts/pacman.lua` build one
`emb.run(batch)` per `decide` and read its outputs. The server measures only the
whole scripted evaluation (`scriptLatencyUs`) and the bridge exposes only
whole-command `elapsed_us`; the Lua sandbox has no clock. The plate's only time is
`performance.now()` around the fetch. Pac-Man's `decide` sets `preferred` to the
first safe adjacent pellet or to `safe[1]` otherwise, and never forbids reversal,
so a random-weight model oscillates.

## Goals / Non-Goals

**Goals:**
- A real per-decision model-inference number on every example, sourced server-side.
- The loops' readout composed and styled like the reference's live decision view,
  using only the site's existing tokens.
- The typed-question tree filling as an ordered reveal over the real reply.
- A Pac-Man run that clears pellets and cannot lock into a two-cell loop.

**Non-Goals:**
- A new command, protocol, or preset surface.
- Importing the reference's own palette; the plate stays in the site's tokens.
- Reducing network or queueing time; only the model call is reported.
- Multiple returns from `emb.embed`, `emb.math`, or any other host function.

## Decisions

### D1 — `emb.run` duration as a second Lua return value

`runHost`/`runBatchHost` measure `time.Since` around `h.Run(inputs)` and push the
duration (ms) as a second Lua return value; presets read
`local out, inference_ms = emb.run(batch, opts)`. **Why:** a second value cannot
collide with a graph output tensor name, needs no new function or global state, and
leaves `local out = emb.run(…)` byte-for-byte unchanged. **Alternatives:**
`out.inference_ms` (collides with a tensor named `inference_ms`, and pollutes the
tensor map a script may feed back); `emb.last_inference_ms()` (stateful and
order-dependent); a reply-header (the scripted reply is one opaque bulk string).

### D2 — Time the session run, not the host wrapper

The clock starts and stops immediately around `h.Run`, excluding tensor
marshalling, output selection, and the script's own softmax. **Why:** that is what
"inference" means and what the reference's `inference_ms` measures; timing the
whole `runHost` would fold in Lua↔Go conversion. **Alternative:** time the whole
evaluation and divide by frames — rejected, it reports tokenization and loop
overhead as inference.

### D3 — The plate reads the reply, never its own clock

Each loop frame carries `inference_ms`; a single-pass reply carries its inference
time in `usage`. The plate's counters show that value and derive `decisions/s` as
`1000 / inference_ms` (model throughput), not the animation rate. The client
`performance.now()` around the fetch is removed from the readout. **Why:** the
proposal's requirement and the existing "measures the server's execution time, not
the network" principle.

### D4 — Cache honesty

A scripted reply is cached by content, so a repeated single-pass request would
replay the first run's `inference_ms`. Loop requests resume from a new board/game
each episode and are effectively never repeated identically, so the loops always
show a live number. For the single-pass examples the plate keeps a client-side
memory of the exact payload it sent and, on a repeat, marks the figure as a cached
run rather than presenting the stored number as fresh. **Why:** an inference time
that silently decays to a stale constant is worse than no figure. **Alternative:**
a per-request nonce to force misses (rejected: it lies about the cache and wastes
the one shared vCPU).

### D5 — Readout composition in site tokens

Restructure the loop HUD into the reference's composition — status line, big-digit
counters (score/length/best), a "next move · model probabilities" table with the
proposal marked and two-decimal values, executing + shield badge, two readout bars,
inference ms, decisions/s, tokens — built only from the existing `.hud` atoms and
CSS custom properties. No new colour, font, gradient, or panel token. An
`impeccable` pass owns typography, motion, and responsive behavior, not palette.
**Why:** the existing figure requirement forbids new atoms, and the site's paper +
accent identity is the brief. **Alternative:** importing the reference's cyan/green
palette (rejected by the palette decision).

### D6 — Tree fills as an ordered reveal

`showTree` keeps drawing the whole tree from the payload, then fills the leaves
question by question using the existing `tween` helper; the reply's data is never
mutated and the final widths equal the reply's probabilities. Reduced motion draws
one frame. **Why:** the reference reads as realtime because decisions update their
own readout; a single forward pass cannot stream, so the honest analogue is an
ordered presentation of the one reply.

### D7 — Pac-Man: BFS preference, reversal/regression veto, stall watchdog

`preferred` becomes the first step of a BFS shortest path to the nearest pellet
(reusing the BFS pattern in `all_pellets_reachable`), because Manhattan distance is
wrong across walls. The shield vetoes, in order: ghost-adjacency (existing), an
immediate reversal when another legal move exists, a move that increases
pellet-distance when a decreasing move exists. After a documented run of decisions
with no pellet collected, the planner executes the preferred move until a pellet
lands. Criteria text names the preferred route so a real checkpoint can follow it.
**Why:** it removes the left↔right attractor while keeping "model proposes, planner
guards". **Alternatives:** pure planner (kills the demo's premise); a Hamiltonian
tour (the maze is irregular; a full tour is not worth it here).

### D8 — Tests pin the new behavior

A host test asserts `emb.run` returns a positive duration and that a
single-assignment call still works; a preset test drives a Pac-Man episode and
asserts the pellet count decreases and no frame's `executed` reverses the previous
one while alternatives exist; the existing parity tests are extended to require
`inference_ms` on every frame.

## Risks / Trade-offs

- **Multiple Lua returns leak through `return emb.run(...)`** → audit every preset
  for a bare `return emb.run` before merging; the spec scenario "ignoring the
  second value is unchanged" covers the single-assignment form.
- **Sub-millisecond values look broken beside the reference's 4.98 ms** → the plate
  labels the value as the sandbox's miniature on CPU; the figure is real, not
  comparable to ANE hardware.
- **Cache staleness (D4)** → the repeat is marked cached; loops are stateful and
  unaffected.
- **Watchdog threshold too eager** → it only fires while a safe progressing move
  exists, and it is a documented constant a test pins.
- **Shared-vCPU timing noise** → the figure is the model call's own duration, so it
  excludes queueing; a miss is stated, not smoothed.

## Migration Plan

Host change is additive and backward compatible; presets and the plate ship
together. Rollback is reverting the host return value and the preset field. The
sandbox is hand-deployed: after merge, restamp digests and `just sandbox-deploy`;
the host change requires the image rebuild that deploy performs.
