## Why

The site's whole argument is *"the interface is the integration"* — but the page
never shows an interface. It asserts drop-in Redis compatibility, ops-readiness,
and scripted inference in prose and then sends every visitor to GitHub. A
developer evaluates a tool by evidence, and the page currently carries none: no
command, no reply, no running server.

At the same time the README documents five genuinely marketable capabilities
that have no surface on the page at all: **ops-readiness** (`INFO`, `CONFIG`,
`EMB.READY`, backpressure), **`emb-top`** (visible operating evidence),
**`EMB.MULTI`** (many models, one round trip), **script inference**
(`EMB.EVAL`/`EMB.EVSHA` — the model as a function), and the **distribution
story** (single binary, one-line install, Docker, gems). The page's six-row
feature list is an index with no body.

This change adds that body — one merged, design-aligned capability block plus a
console placeholder — so the page argues with the product instead of about it.

## What Changes

- **A new second spread below the terrain**, built from the poster's own atoms
  (`rule-head` + ruled entries, no cards, no gradients, no rounded corners). The
  existing hero and the poster composition are untouched; the document is
  allowed to grow past the 1:1.33 poster ratio.
- **One merged capability block**, not a block per feature. It carries: the
  Redis protocol as the SDK, script inference as the differentiator,
  ops-readiness, and `emb-top`.
- **A realtime-console placeholder** — a dark panel reusing the SERVE plate's
  material (`#111010`/`#292823` top face, `p-fine` lattice) and the orange signal
  axis, so it reads as the fourth plate turned to face the reader rather than as
  a bolted-on widget. It ships **placeholder-only**: real form controls
  (`<form>`, `<input>`, `<pre aria-live="polite">`) driven by a deterministic,
  network-free transcript adapter, behind a documented seam so the real RESP
  client can be written later without restructuring the markup.
- **Two console modes = the two special functions**: `REDIS` (`EMB minilm "hello
  world"` → float32 bytes; `HELLO 3` + `VALUES` → envelope) and `SCRIPTS`
  (`EMB.SCRIPT LOAD` → `EMB.EVSHA` → a labeled classifier reply), both replayed
  from real outputs in `README.md` and `examples/scripts/`.
- **`emb-top` visibility**: its terminal render appears as a second dark panel in
  the merged block, sourced from the real `emb-top` output, not invented.
- **An Impeccable surface brief + direction contract** recorded before any markup
  edit, and a mechanical detector pass after (see `design.md`).

Explicitly **not** changing: the masthead, wordmark, claim, sub, CTAs, the
six-row hero feature list, the pipeline artwork, the terrain, the footer, the
palette, the fonts, or the copy voice.

## Capabilities

### New Capabilities

- `product-site`: the content, composition, and interaction contract of the
  static marketing site (`website/`), including its relationship to the README
  as the source of truth, its use of the Impeccable design workflow, and the
  console placeholder's states and seams.

### Modified Capabilities

(none)

## Impact

- `website/index.html` — new section markup below `.landscape`; `data-console`
  root, mode tabs, form controls, output region.
- `website/assets/css/styles.css` — new section styles composed from existing
  tokens and primitives; the page's first large light-on-dark surface, so the
  12px/14px type floor and contrast must be re-measured for it.
- `website/assets/js/main.js` — transcript adapter, mode switching, typewriter
  playback honoring `prefers-reduced-motion`; no network code.
- `website/README.md` — document the new section, the console seam, and the
  relaxed frame-ratio constraint.
- `openspec/specs/product-site/spec.md` — the new capability.
- No changes to server code, commands, protocol, or the Go/Ruby trees.
- No hosted service, endpoint, or managed offering is implied or introduced.
