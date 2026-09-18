## Context

Every command a plate sends already funnels through one private function in
`website/assets/js/demos.js`:

```
plate ──▶ gallery() ──▶ exec(args, bin) ──▶ POST /api/exec ──▶ bridge ──▶ emb
                          ▲
                          records nothing today
```

`exec` is called by `embed`, `search`, `preset`, `imagePreset`, `models` and
`raw`; the plates never fetch the bridge themselves. `argvLine(args)` already
exists in this module — it renders one argv in the site's `code` / `t-cmd`
atoms — and is imported but unused by `vector.html`. `similarity.html` instead
rebuilds its command by hand (`source.textContent = 'EMB.EVSHA minilm ' + …`)
and `function.html` prints each preset's digest by hand.

The gallery's own state machine (`set()`) only runs for the calls that go
through `run(work)` — `search`, `preset`, `imagePreset`. A plate that only
calls `g.raw` (batch, cache, function) never moves the gallery's state, so
"the run started" cannot be inferred from it.

Constraints from `embedding-demos` and the surface brief: atoms only, no new
component language, no-JS reading preserved, dynamic content hidden in print.

## Goals / Non-Goals

**Goals:**
- The commands a plate unfolds are the argv it actually sent, in that run.
- One implementation in `demos.js`; the eleven plates differ only in markup and
  one call.
- The fold needs no script and no new visual language.

**Non-Goals:**
- Reply shape, server timing, cache hit/miss, and browser-side sqlite-vec SQL.
  The disclosure is the argv, per proposal.md.
- Changing the static `THE EXACT COMMANDS` section, the bridge, or the server.
- A site-wide console or drawer.

## Decisions

### Record at one choke point, replay at the plate, not a second request path

The trace is an array of `{args, bin}` pushed by `send`, the thin wrapper every
`gallery()` method already funnels through before `exec` hands the argv to the
transport. Nothing about the trace is sent over the wire: the argv is already in
the client's hand, and the bridge's validation means a call that succeeds is
exactly the argv the server ran. A server-side echo was considered and rejected
— it adds an envelope field and a bridge test to reproduce bytes the client
already has. The record sits in `send` rather than in `exec` so each gallery
*instance* keeps its own trace: the atlas drives two instruments on one page,
and recording at the transport would interleave their commands.

### An explicit run boundary, because the gallery's state machine misses `raw`

`gallery()` gains:

```
g.begin()   // starts a fresh run: clears the trace
g.trace()   // the commands recorded since begin()
```

Each plate calls `g.begin()` at the top of its own `run()` and renders
`g.trace()` when it renders its result. Alternatives: clearing on the gallery's
`set('starting')` (unreliable — `g.raw`-only plates never reach it) and a
DOM-scanning auto-render on every `exec` (magical, and it cannot know when a
run begins). One line per plate is the smaller, honest thing.

### Native `<details>`, filled by one shared renderer, opened on the site's easing

The disclosure is `<details><summary>…</summary><div data-cmds></div></details>`,
hidden until the plate renders. The fold is the browser's, so it opens with
scripting off, is keyboard-reachable, and adds no ARIA. A shared helper —
`unfold(node, entries)` beside `argvLine` — fills the body and unhides it, and
is the one place `argvLine` finally gets its caller. Opening it grows the
platform's own `::details-content` box where the engine animates that, with a
short keyframe on the body as the fallback every engine runs; the
`prefers-reduced-motion` branch shows the commands in one frame, which is the
same rule the plates' own tweens follow.

### Arguments are shown as sent, with binary named rather than dumped

`argvLine` currently quotes an argument containing whitespace and joins the
rest. It grows one rule: an argument index in the `bin` set renders as
`[N bytes]`, because the image preset's argv carries base64 of the uploaded
file and printing it would be a wall of text. The digest is rendered verbatim —
it is what the sandbox ran; the static section keeps its `<embed.lua sha1>`
shape, and the two are visually distinct because one sits in the plates' static
specimen blocks and the other inside the rig.

### Reuse the existing rule atoms

`.code`, `.code__body`, `.t-cmd` and `.rig__out` already style a command block
and the rig's output. The disclosure adds a summary row and a border, using the
existing rule variables — no new colour, font, or panel.

### The clamp belongs to the surface, not to the command

Long text arrives from two places — the argv a run issued, and the corpus
passage a plate quotes — so one rule lists both selectors and each names its own
`-webkit-line-clamp`. `.shape__body` is deliberately absent: the function plate
prints a fixed four-line reply, and clamping that would hide a field rather than
clip prose. Nothing is cut from the document, so a reader who selects the text
gets all of it, and the ellipsis is the platform's own.

### The top link is the foot rail, moved to the head

The next plate's link takes the first row of the plate head, spanning both
columns and right-aligned, and wears the rail's own vocabulary — a mono label
and the plate's name in the display face. The head's grid needed `column-gap`
where it had `gap`, so the new row is separated by its own margin rather than by
the column rhythm. The last plate omits the link because it has no next; that
matches the foot rail, which does not invent one either.

## Risks / Trade-offs

- [The disclosure duplicates the static section on some plates] → The static
  section states the call's shape with a placeholder digest; the disclosure is
  the reader's own run. The summary names the run ("the commands this run
  sent") and `similarity.html`'s hand-built line — the one that reads like the
  disclosure — is deleted, so no plate shows its own run twice.
- [A long input makes a very long line] → The run's own command is clamped to
  two lines with the platform's ellipsis, so one corpus passage cannot turn the
  disclosure into a page; the full argv stays in the document, which is what a
  reader selecting it gets.
- [A trace left over from a page-load run is shown before the reader acts] →
  Intended: several plates run once on load, and the disclosure then describes
  that run rather than appearing empty.
- [Print and TL;DR views gain a dynamic block] → The disclosure lives inside
  `[data-live]` with the rest of the rig, so it is absent from the no-JS
  reading; a print rule drops `.cmds`, since the plate's static command section
  is what paper carries.

## Migration Plan

No migration: the site is static and ships with the page. Rollback is a revert
of the module, the eleven plates, and the stylesheet — the static sections were
not touched.

## Open Questions

None.
