## Context

See `proposal.md — Why`. The relevant current state, all of it already in the
tree when this change was authored:

- `website/index.html` carries the console as `<section class="console"
  id="console" data-console="transcript" … data-reveal>` inside the Protocol
  block, after `.block__grid`. It is the fourth plate, laid flat, and it sits
  above the spine.
- `website/assets/js/main.js` boots the console once, from
  `document.querySelector('[data-console="transcript"]')`, then owns
  everything: the transcript map, the mode tabs, the idle/running/result/error
  states, and the `window.embConsole = { exec(command, mode) }` seam. Nothing
  else on the page references the console.
- The section is one of the page's `[data-reveal]` elements and inherits
  `--i: 4` from `.block .console` for its entrance stagger.
- `website/assets/css/styles.css` gives the console its dark-ground block
  (`.console{ background:#111110; … }`) and sets no `display` on it.
- The site has no build step. `website/` is both the working tree and the
  publish root; `.assetsignore` is the only deploy-time filter and it does not
  need a change here, because no file is added, moved, or deleted.
- The deploy change (`deploy-site-to-cloudflare`) is in flight, so this is the
  window in which the first published revision is chosen.

## Goals / Non-Goals

**Goals:**

- The shipping page renders without the console panel while keeping every part
  of the console in the tree, so the live runtime is wired rather than
  re-authored.
- The hide is a property of the document, not of a script, so it holds over
  `file://`, with JavaScript blocked, and with the script throwing — the same
  failure modes `product-site`'s "survives a script failure" requirement
  already guards.
- The revert is one attribute, discoverable in the artifact it changes, with no
  build step, flag file, or comment elsewhere to consult.
- Nothing about the console's specified design — modes, states, keyboard
  operation, dark-ground contrast — is deleted or allowed to rot.

**Non-Goals:**

- Wiring or prototyping the RESP client. The seam is untouched; this change
  does not move toward it.
- A generic feature-flag system. There is exactly one switch, on one panel,
  for a known duration.
- Changing the console's copy, transcripts, styles, or position, or touching
  the rest of the page.

## Decisions

### D1 — Withhold with the native `hidden` attribute

The section gains one attribute: `<section class="console" … hidden>`. HTML's
`hidden` takes the element out of the rendered page and out of the
accessibility tree, and it is applied by the UA stylesheet rather than by this
site's CSS, so it survives JavaScript being blocked and CSS being unloaded.

*Alternatives:* **A `data-console="off"` token** (the JS selector would simply
miss and the panel would still render unless CSS also hid it — two places to
change, and it repurposes an attribute that selects the live console for a
different job). **A CSS-only `display:none`** (a script-less page with the
stylesheet missing would show the panel; the hide must not depend on the
stylesheet loading). **A `?console=1` query parameter** (turns the default into
the exception and complicates every measured check). **Deleting the markup and
the client** (the cost this whole change exists to avoid, and it puts the
placeholder at risk of drifting from the real server before it returns).

### D2 — Make `[hidden]` explicit in the stylesheet anyway

`website/assets/css/styles.css` gains `.console[hidden]{ display: none; }`
beside the `.console` block. The UA default already does this today, and
`.console` sets no `display`. The rule is added because the panel is a block
that is otherwise a candidate for `display:flex` in a future layout pass, at
which point `hidden` would silently stop hiding it — the same class of failure
`.assetsignore` and `published-tree.py` exist to prevent, one level down.

*Alternative:* rely on the UA default and add nothing. Rejected as a latent
trap: the failure is invisible until someone adds a layout property to
`.console`, and then it is a panel reappearing on the published page.

### D3 — The client does not boot while the panel is withheld

`main.js` changes one selector:
`[data-console="transcript"]` → `[data-console="transcript"]:not([hidden])`.
The whole console client lives inside the `if (root)` block, so while hidden
nothing boots: no timers are scheduled, no `aria-live` is armed, and
`window.embConsole` is never defined on the shipped page.

This matters beyond tidiness. A withheld panel that still booted would leave a
live region being armed on an element no reader can reach, and it would keep the
`window.embConsole` global on the page — a seam that claims a console is
available when none is. Guarding the selector keeps the hidden state honest at
runtime and not only in the visual layer.

*Alternatives:* **Boot unconditionally and let `hidden` hide the panel**
(rejected — arms a live region and publishes the global while the panel is
withheld). **Guard inside the block with `if (root.hidden) return;`** (works,
but leaves a half-entered block as the first thing a future reader reads; the
selector states the contract at the point the console is found).

### D4 — The revert is one attribute, and the artifact says so

Removing `hidden` restores the placeholder exactly; the `:not([hidden])`
selector then matches again and the same client boots. The markup comment above
the section names the attribute, the selector, and the seam, and
`website/README.md`'s console section carries the same statement — so the person
who later wires the live runtime reads the instructions in the two files they
are already editing. Nothing about the revert is stored outside the diff that
would need it.

This is the reason the hide is an attribute rather than a deletion, and the
reason the spec change keeps the console's modes/states/keyboard/contrast
requirements rather than removing them: the artifact remains under contract
while it is withheld, so it cannot rot behind the flag.

### D5 — The specification scopes the console requirements; it does not delete them

`product-site`'s "The console is a placeholder with a live seam" is modified
from *the site SHALL render the panel* to *the site SHALL retain the panel and
withhold it until a live executor exists*, keeping every original scenario
under its original name so archive does not drop one:

- "The console works with no network" is retained and scoped to the enabled
  panel;
- "The placeholder is not mistaken for a live service" and "A real client can
  replace the transcript" are retained unchanged;
- three scenarios are added: the panel is withheld from the rendered page, the
  withholding is not a script's decision, and the artifact is retained.

The requirement text states that the mode, state, keyboard, and dark-ground
requirements apply to the retained panel and to the page whenever the panel is
enabled, so those requirements continue to govern the artifact without a
second, near-duplicate requirement.

The "capability region is responsive and accessible" requirement's
mobile-reflow scenario is modified to stop naming a console that is not
rendered, and to say instead that the console joins the reflow whenever it is
enabled.

### D6 — What does not change, and why that is the point

- **The published file set is untouched.** No file is added, moved, or deleted,
  so `.assetsignore` and `published-tree.py`'s expected set and served-path
  count do not move.
- **The page's measured floors hold.** The withheld panel occupies no layout
  space, so `just website-ink` measures the same rendered ink it would with the
  panel absent; the panel's own type and contrast values remain in the
  stylesheet and in `website/README.md` as the artifact's contract.
- **No build step is introduced.** The switch is one attribute in the markup
  and one selector in the script, both visible in a diff.

## Risks / Trade-offs

- **Someone removes `hidden` before the runtime exists.** The panel returns as
  the honest-but-unfinished placeholder. → Accepted: that is the designed
  revert, the placeholder states what it is, and the copy is not the failure
  this change is guarding against.
- **Someone keeps the panel hidden and deletes the client "because it is
  unused".** → The `.console` styles and the client remain reachable-looking
  dead code under a `hidden` section; the markup comment, `README.md`, and the
  spec's "artifact is retained" scenario all name it as load-bearing, so the
  deletion is a visible contradiction rather than a tidy-up.
- **A future layout pass gives `.console` a `display` value.** → D2's explicit
  `.console[hidden]` rule keeps the hide; this is the failure that rule exists
  for.
- **The hidden panel's styles drift from the real server** while the panel is
  not seen by a reader. → The transcripts remain repository-sourced and the
  spec keeps the panel under its modes/states/contrast requirements; the drift
  window is the untested-artifact window, and it is bounded by the live-runtime
  change that ends this one.

## Migration Plan

1. Withhold the panel: add `hidden` and the explaining comment to
   `website/index.html`.
2. Guard the boot: change the selector in `website/assets/js/main.js`.
3. Add the explicit hide rule in `website/assets/css/styles.css`.
4. Record the rule in `website/README.md` beside the console section.
5. Verify: the panel computes `display:none` and contributes no layout; the
   console controls are not focusable; `window.embConsole` is `undefined`; the
   page has no horizontal scroll; `just website-published` and
   `just website-version-check` still pass.

**Rollback:** remove the `hidden` attribute from the section. The selector,
the script, and the styles all still match the placeholder, so no other file
needs to change. This is the same one-attribute operation the spec records as
the restore path.
