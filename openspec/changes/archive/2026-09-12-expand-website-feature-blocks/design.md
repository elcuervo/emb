## Context

See `proposal.md` — Why. Technical context that shapes the approach:

- The site is a static, dependency-free single page (`website/index.html`,
  `assets/css/styles.css`, `assets/js/main.js`) rendered with no build step. Any
  new behaviour must remain plain HTML/CSS/JS with no CDN and no bundler.
- The composition is poster-derived and measurement-verified. `CORRECTIONS.md`
  and `website/README.md` record the atom vocabulary, the `--accent` /
  `--accent-ink` split (2.74:1 vs 4.66:1), and the 12px/14px type floor, all
  measured at a 1086px reference frame and a 390px phone.
- The page already contains one dark material — the INFERENCE plate's black
  ramp and `#p-fine` lattice, and the SERVE plate's `#111110` / `#292823` faces.
  There is no light-on-dark *text* surface yet, so no dark-surface type floor or
  contrast budget exists.
- The terrain is the last section; the footer follows immediately.
- `impeccable context` for `website/index.html` reports
  `surfaceBriefPath: "not-found"` — no Impeccable surface brief exists yet.

## Goals / Non-Goals

**Goals:**

- One merged capability region that argues with the product: protocol, scripts,
  ops, `emb-top`.
- A console placeholder that is honest, deterministic, accessible, and shaped so
  the real RESP client drops in later without a markup rewrite.
- Zero visual-language drift: every new atom must be traceable to the poster.

**Non-Goals:**

- No live RESP client, no WebSocket, no backend endpoint, no hosted demo.
- No new capability-block-per-feature, no cards, no tabbed feature grid.
- No changes to the hero, pipeline, terrain, masthead, or footer.
- No changes to `DESIGN.md`'s palette, type scale, or component language unless
  the user approves a durable system change.
- No new dependency, font, or asset pipeline.

## Decisions

### D1 — Placement: a second spread after the terrain

Place the merged region between `.landscape` and `.footer`, reusing the hero
spread's grid (`--col-prose`-equivalent two-column split) and the same signal
axis (`40.7% - var(--plate)/2`).

*Alternatives:* (a) between hero and terrain — rejected because it breaks the
pipeline→ridge handover, which is the page's one continuous idea; (b) inside the
hero prose column — rejected, the poster composition is locked and the hero is
already the tightest part of the frame.

### D2 — One merged block, not four

A single `rule-head` (`BUILT FOR REAL SYSTEMS` already exists in the hero; the
new heading is a distinct, more specific claim) followed by ruled entries and
two dark panels: the console and `emb-top`.

*Alternatives:* one block per capability — rejected as the exact drift
`DESIGN.md`'s "What the site should not become" names; four blocks read as a
feature grid in a trench coat.

### D3 — The console's material is the SERVE plate, flattened

The panel reuses the SERVE plate's black top face and side colours plus the
`#p-fine` lattice, with the orange signal entering at the established axis.

*Alternatives:* (a) a light panel with a hairline border — rejected as visually
inert and indistinguishable from the ruled list; (b) a literal terminal chrome
(title bar, traffic lights) — rejected as a costume note from another world.
Flattening the fourth plate makes the console the same object as the diagram,
turned to face the reader.

### D4 — Real form controls behind a transcript adapter

Markup ships `<form>` + labelled input + `<pre aria-live="polite">` output. The
default executor replays a static transcript map. A documented seam
(`window.embConsole = { exec(command) → Promise<lines> }`) lets a live client
replace it.

*Alternatives:* (a) a fake prompt drawn in a `<div>` — rejected: needs a rewrite
to become live, and is not keyboard or screen-reader operable; (b) a hidden real
input behind a painted prompt — rejected as two sources of truth.

### D5 — Two modes are the two special functions

`REDIS`: `EMB minilm "hello world"` → raw float32, and `HELLO 3` + `VALUES` →
the `dtype`/`shape`/`values` envelope. `SCRIPTS`: `EMB.SCRIPT LOAD` → SHA →
`EMB.EVSHA` → a labeled classifier reply.

*Alternatives:* a mode per README command — rejected as a documentation index
inside a marketing page. Two modes each prove one differentiator.

### D6 — Transcript content is copied from the repository

Every line replayed in the console is copied from `README.md` or
`examples/scripts/` output. No invented replies, no invented latencies.

### D7 — Dark-surface type and contrast are measured separately

The new panel gets its own measured minimum (≥12px desktop, ≥14px mobile) and a
contrast check of every text/indicator colour against its own background
(≥4.5:1 text, ≥3:1 non-text). `--accent-ink` carries any accent-as-text; the
surface accent stays a surface colour.

### D8 — Motion is opt-out and never load-bearing

Transcript playback is a typewriter that collapses to the final frame under
`prefers-reduced-motion`, matching the page's existing reduced-motion path. No
information depends on playback completing.

### D9 — The frame ratio is relaxed

The page is allowed to exceed the poster's 1:1.348 ratio. `website/README.md`
and `CORRECTIONS.md` must be updated so the measured-height claim is not left
stale.

### D10 — Impeccable governs the work (required workflow)

The change is executed as **`shape` on an extension of an established surface**
— not a new-work round. Concretely:

1. **Context.** Run `<skill-base-dir>/scripts/impeccable context --target
   website/index.html` once per session; keep cwd at the project. Already run
   for this change; it reported the missing surface brief.
2. **Shape.** No concept tournament and no `concept-seed` roll: `new-work.md` §3
   says a section inside an established surface inherits that world. Resolve
   only the new purpose, content, hierarchy, states, interaction, and how the
   addition joins.
3. **Record the decision before building.** Write the surface brief (job and
   audience; outcome and proof; selected direction; scope and boundaries;
   states and ranges; interaction and layout; constraints) with a
   `## Direction contract` carrying the six blocks — THESIS, OWN-WORLD, STORY,
   FIRST VIEWPORT, FORM, FINISH — via
   `impeccable surface-brief write website/index.html <body-file>`, and read it
   back to confirm every block is present. No markup edit happens before this.
4. **Craft floor.** Read `reference/craft-floor.md` immediately before the first
   UI edit, and again before any later refinement pass.
5. **Build.** One committed build of the new region; no placeholder styling left
   behind.
6. **Detect once.** After the build, run
   `impeccable detect --json website/index.html website/assets/css/styles.css`
   once (`MANUAL_DETECTOR_REQUIRED` — no hook is active), and resolve or record
   every finding.
7. **Finish.** Ship only with the finish review, its verdict, and
   `website/README.md` updated. `DESIGN.md` is touched only if the user approves
   a durable system change; an ordinary extension does not rewrite it.
8. **Do not** copy the direction contract into any browser-delivered artifact —
   not into HTML comments, `data-*` attributes, or shipped JS.

*Alternatives:* treating this as a new-work round would force a replacement
visual world onto a page whose identity is already settled and measured — the
opposite of the brief.

### D11 — Capability claims stay server-truthful

The region describes commands that exist in `README.md` with replies the server
actually produces. Nothing in it may imply a managed endpoint.

## Risks / Trade-offs

- **The page becomes a two-spread document rather than a poster.** → Mitigate by
  building the new region from the poster's atoms and by keeping the hero
  untouched; acceptance is explicitly the user's call (D9).
- **A dark panel breaks the page's one-dark-material rule by inventing a new
  chrome language.** → Mitigate by flattening an existing plate rather than
  designing a terminal, and by banning title bars and traffic-light dots (D3).
- **The console placeholder reads as a live service.** → Mitigate with an
  explicit demo-transcript statement and no endpoint affordance (spec: *The
  placeholder is not mistaken for a live service*).
- **Light-on-dark type falls below the committed floor** — the risk that already
  bit this page twice in `CORRECTIONS.md`. → Mitigate by measuring rather than
  assuming, at both reference widths (D7).
- **Accessibility regressions in an interactive widget.** → Mitigate with real
  form controls, an `aria-live` output region, keyboard-only operation, and a
  target-size check at 320–834px (D4, spec: *The console is usable by keyboard
  and screen reader*).
- **Stale documentation.** → `website/README.md` and the height claim in
  `CORRECTIONS.md` are updated in the same change (D9).
- **Transcript drift when commands change.** → Transcript entries cite their
  source (`README.md` section or example script) in a comment or sidecar so a
  future server change has a defined update path (D6).

## Migration Plan

Additive and static. There is no data migration and no deployment machinery:

1. Land the new markup, styles, and JS together; the page keeps working with JS
   disabled (the transcript falls back to a static rendering).
2. Verify at 1086px, 1440px, and 390px, and under `prefers-reduced-motion`.
3. Rollback is a single revert of the three site files plus the doc updates; no
   server, gem, or protocol surface is touched.

## Open Questions

- Exact wording of the merged block's `rule-head` — deferrable; the direction
  contract settles the register, and the copy is written against the README at
  build time.
- Whether the console sits beside the ruled entries or below them — deferrable;
  both use the same atoms, and the shape brief records the chosen arrangement.
- Whether `emb-top`'s panel is a sibling of the console or part of the same dark
  surface — deferrable; neither changes the spec, the approach, or the task
  breakdown.
