## Context

See `proposal.md` — Why. The current state that shapes the approach:

- The console is one plate in the landing's **PROTOCOL** block; the sandbox's
  standalone terminal (`cli.emb.is/`) renders the same replies through the same
  module (`website/repl/terminal.js`), and the repository treats that as an
  invariant ("one client, two surfaces").
- `main.js` is landing-only and owns both the console chrome and the `emb-top`
  plate driver. It is 20 KB of hero entrance, reveals, pipeline sync, route
  drawing, and the plate.
- `website/docs/index.html` is a hand-written reference surface that links
  `styles.css` + `docs.css` and no script at all. That is documented in
  `website/README.md` and asserted nowhere.
- `website/tools/topviz/publish.py` is the only writer of the plate's content and
  currently stamps three places: the landing page, `published-tree.py`'s served
  set, and a marked block in `docs/operations.md`.
- `publish.py`'s GIF target stays: GitHub renders markdown and runs no
  JavaScript, so a picture is the honest form for `docs/operations.md`.
- Design constraints that bind every edit here are committed in `DESIGN.md` and
  restated as requirements in `product-site`: one accent (`#FF5A1F` is a surface
  colour only; text, hover and focus use `--accent-ink` on paper and `--accent`
  on the dark plate), a 12px desktop / 14px mobile type floor, no gradients,
  cards, rounded panels or shadows, no new colour, and `prefers-reduced-motion`
  honoured.
- `openspec/changes/deploy-sandbox-repl` is in progress and already ships the
  live console; its remaining tasks are deployment verification. Its deltas
  against `product-site` have not been archived.

## Goals / Non-Goals

**Goals:**

- A first-time visitor gets a reply from one click, and a second reply from one
  keypress, with no control to configure first.
- Fewer controls than today, not merely rearranged ones: the plate loses a mode
  tablist and its roving-`tabindex` keyboard handler entirely.
- The same console on both surfaces it drives.
- The plate's implementation has one home and two callers.
- The documentation surface shows the `emb-top` run without motion the reader did
  not ask for, and still shows it with scripting unavailable.

**Non-Goals:**

- No change to the sandbox bridge, the allowlist, the presets, or the recorded
  cast.
- No change to the reply renderer (`terminal.js`'s `format`).
- No new visual world. This is a refinement of the committed poster, not a
  redesign; `DESIGN.md` is not expected to change.
- No re-recording of the take. The plate plays the cast already published.
- Not retiring the GIF in `docs/operations.md`.

## Decisions

### The demonstration commands are a menu, not a state

The two tabs exist to demonstrate the two special functions: byte replies and a
script returning a structured reply. Both are *commands*. An example is the same
demonstration without a mode to select first, and it removes the tablist, its
roving-`tabindex` handler, and `setMode` plumbing on the landing page.

The rows are built at runtime from the two digests already stamped into the
section (`data-emb-preset-embed`, `data-emb-preset-classify`), so a preset whose
bytes change cannot leave the page suggesting a digest the sandbox refuses.
Seven rows: `EMB.HELP`, the raw `EMB` call, the same call with `VALUES`, an
`EMB.MULTI` call naming two models, the `embed` preset (dimension, norm,
similarity), the `classify` preset, and `EMB.MODELS`.

*Alternative considered:* keep the tabs and add examples inside each. Rejected —
it keeps the thing being removed and doubles the content in the panel.

### The menu sits above the output and stays there

The first version of this change painted the rows into the transcript as the idle
state and let the first command replace them, reasoning that they then cost their
tab stops only while they are the only thing to do. That is wrong for the panel's
actual job: the reader who has just run one command is the reader most likely to
run a second, and they were being sent back to a page reload to find one. The rows
are therefore part of the panel rather than of a state, they sit *above* the
transcript, and nothing replaces them.

Above rather than below because of what is on screen at rest: below the
transcript, an untouched console shows a tall empty field between the strip and
the menu; above it, the row under the strip *is* the menu and the empty space is
the output area directly over the prompt, which is where a terminal's empty space
belongs.

Because the rows are permanent, the offline state no longer has to re-offer them,
and `idleEl` / `showIdle` / `appendExamples` are deleted rather than adjusted.

*Alternative considered:* keep every row in the transcript and scroll to them.
Rejected — a menu that scrolls out of reach is the defect being fixed.

### The transcript is bounded, and the rows are sized to fit it

The panel is a window onto the sandbox's output, not a page that lengthens with
it. `.console__screen` gets a floor and a ceiling and scrolls between them, and
`main.js` follows the newest line — but only when the reader was already at the
end, so reading back through a long reply is not interrupted. Whether to follow is
decided *before* the transcript grows: decided after, a batch of lines arrives
with the panel already past its own threshold and the reader is left at the top of
output they never saw.

Two columns above 834px keep the seven commands to four ruled rows. Where a command
and its note cannot share a row, the row's *label* elides the digest
(`31ae7054…`) while the button still submits the command in full and carries it
in full in its accessible name — a 40-character SHA spent in a menu is a row that
wraps for no reader's benefit, and the digest is printed in full the moment the
command runs. Below 834px the rows are one column of one line: the note is
dropped from the drawn row and kept as the button's name, and the badge is dropped
from the strip, where the note under the panel says the same sentence in full.

Measured at 1440px: 538px for the panel at rest — the seven rows cost one more
ruled row than the six did — against 467px for the version this change started
from. The transcript is bounded at 216px and the commands are permanently
clickable, where the version it started from had a 262px unbounded transcript and
a menu that disappeared on first use.

### The protocol selector is not on the panel

The `RESP 2|3` selector was the last setting on the console, and it earned its
place while the panel was a demonstrator: it let a reader see the same command
answered as a flat reply and as a typed one. The menu already does that, with two
rows — `EMB …` and `EMB … VALUES` — that show the two forms without a control to
set first, and the selector's second effect (RESP3 versus RESP2 on the sandbox's
own connection) is a wire detail no visitor to a landing page is reading for.

It is removed. `terminal.js` keeps `setProto` and still puts `proto` in the
request: that is the module's contract with the bridge, not the panel's
furniture, and the bridge's reply envelope is still chosen per request.

### The sandbox's own page is the terminal, and nothing else

`website/repl/index.html` is served at `cli.emb.is/`, and it was built like a
page: a masthead-sized heading, a paragraph, a bordered card with a status strip
and a footer. Everything in that frame restated what the terminal already said,
and the heading in particular restated it in a different voice from the replies
underneath it.

It is now the viewport. `100dvh`, one column, three bands — what the terminal
says about itself and the commands it takes, the transcript, the prompt — and no
card, no strip, no footer, no page scroll. The model is `redis.io/cli`: a
transcript that scrolls, a prompt that is the last line of the terminal, and the
disclosure written in the same monospace voice as everything else rather than in
a paragraph above it.

Two things did not move with the rest:

- **The samples stay pinned**, above the transcript. They are the terminal's
  opening message and the only clickable thing on it, and a reader who has just
  run one command should not have to find the list again. This is a deviation
  from the reference, which prints its banner once and lets it scroll; it is the
  deviation the panel's own requirement asks for.
- **The state strip is gone, and the state is not.** The client already prints
  `offline — …` into the transcript and the retry sits under it, so a second
  indicator in a bar said the same thing twice. The one thing the bar carried
  that the transcript does not — `sandbox · may reset` — moved into the line
  above the samples, where it is always in view.

### History lives in the client module

Arrow-key recall is behaviour of the console, not of either page, and the two
pages each own an input. Putting the history in `terminal.js` gives both surfaces
one implementation, which is the same reason the reply renderer lives there.

Recall stops at both ends (readline behaviour) rather than wrapping, and a draft
being typed is preserved when the reader walks away from it and back. That is the
smallest thing that behaves the way a REPL behaves.

*Alternative considered:* implement recall in each page's inline script.
Rejected — two implementations of identical behaviour in the two files the
repository works to keep identical.

The landing page feature-detects recall: `terminal.js` is served from the sandbox
origin, so the page must not depend on the sandbox having been redeployed.

### The submit control becomes `↵`, and is not deleted

`Enter` submits, which is what was asked for, but removing the button would make
the console submit-on-soft-keyboard only. The button stays as a quiet `↵` glyph
in the prompt row and keeps its 44px touch target below 834px.

### The plate driver becomes its own module

`main.js` does not belong on the documentation surface. Extracting the plate
block into `website/assets/js/topviz.js` costs one file and buys: the landing
page loads the console + plate, the docs page loads the plate alone, and the
plate has a single implementation with a single set of options passed through
`data-` attributes on its mount.

*Alternative considered:* make `main.js` safe to run on both surfaces. Rejected —
it means every landing-only initialiser needs a null guard, and it ships the
hero's entrance and route code to a reference page for one plate.

### The documentation plate is the recording, held — not its text frame

The landing plate autoplays and loops because it is the section's argument. On a
reference page, motion the reader did not ask for is a cost paid on every visit.

The first version of this change answered that by showing the run's *still
frame* — the `<pre>` that `asciinema convert -f txt` produces — and creating a
player only when the reader asked for one. That is wrong in a way that is easy to
miss: the text frame carries no ANSI codes at all, so the documentation surface
was presenting the dashboard in one flat colour while the GIF in
`docs/operations.md` showed the same run in its real palette, and while the page
itself claimed the plate was `emb-top`'s actual output. The frame is a *fallback
for a reader the player cannot reach*, not the surface's picture of the
dashboard.

The plate therefore creates the player at once and **holds** it: `autoplay:
false` with the run's poster frame (`npt:0:20`, the same busy poll the landing
holds), so the reader sees the recording drawn in the dashboard's own colours,
stopped, at zero elapsed time. Nothing plays until it is asked to.

That also removed a control. The player draws its own start overlay over a held
poster, so the `PLAY THE RECORDING` button this change had added was a second
control for the same action in a second place — deleted, with its markup and its
rules, rather than kept beside the player's own.

The `<pre>` frame stays in the markup and stays the no-scripting state. It is
also what a take that will not play falls back to, and reaching that path now
costs nothing: the player is created at load either way, so the error handler
that swaps the plate back is the same one the landing already had.

*Alternatives considered:* autoplay on the documentation surface (rejected: Read
surface, and the motion contract this change wrote); a second GIF (rejected: an
image of the dashboard is what `emb-top-capture` forbids at the plate, and a GIF
is heavier than the cast it would replace); the uncoloured frame as the resting
state (rejected — the defect this revision fixes).

### `publish.py` gains a third stamp target

The documentation plate's frame, cast URL, caption and figures are content of the
same run, and the project's rule is that the two surfaces cannot disagree about
it. `publish.py` writes them into `website/docs/index.html` by the same marker
mechanism it already uses, and fails loudly when a marker is missing rather than
writing half the plate.

*Alternative considered:* hand-writing the frame into the docs page. Rejected —
it is exactly the drift the existing rig exists to prevent.

## Detector findings from this pass

`impeccable detect --json` was run once over the five changed HTML and CSS files
after the build. Against the same files at `HEAD` the run reports 34 findings and
this revision reports 35: the protocol selector this change removed took one with
it, and the two below arrived with the plate the change added to
`website/docs/index.html`. Both are properties of the design rather than defects:

- **`cramped-padding` on `.topviz__play`.** The landing page's identical element
  already carries this finding at `HEAD`. The heuristic reads the border-to-first-child
  gap; the plate's padding is `clamp(12px, 1.4vw, 18px)` and the frame inside it
  has `margin: 0`, so the padding is real and the measurement is of the margin box.
  Left as is: giving the frame a margin to satisfy the check would inset it
  relative to the player, which is fitted to the padding box.
- **`tight-leading` at 1.05 on the terminal frame.** The rule targets body prose.
  The frame is box-drawing art: its line height is what makes the vertical rules,
  the heatmap cells and the gauge bars join, and 1.5 would break the dashboard
  into disconnected rows. The landing's frame has carried 1.05 since the recording
  shipped.

Neither is suppressed. Suppressing them would need a project-scoped ignore rule
in the detector's own config, which is a durable change to the repository's
tooling and belongs to whoever decides the rule does not apply here — not to this
change as a side effect.

## Risks / Trade-offs

- **`deploy-sandbox-repl` has not archived, and both changes touch the same
  `product-site` requirements.** → This change's deltas are written against the
  *live* console text that change introduced, and it must archive first. If it
  does not, the deltas apply against the placeholder-era text and the resulting
  main spec is wrong. Checked before archiving, not before implementing.
- **Tab order in the panel.** → Examples sit before the prompt in both DOM and
  visual order, and exist only in the idle state, so the prompt is never more
  than six stops behind a control that is still useful.
- **`terminal.js` is served by the sandbox, so the landing page's history depends
  on a sandbox redeploy.** → The page feature-detects recall and simply has no
  history until the module has it.
- **The documentation surface now ships JavaScript**, which retires a documented
  invariant. → The frame is static markup and the player is created on demand, so
  the page's no-scripting state is the same page minus a button. `website/README.md`
  is updated in the same change so the invariant cannot be re-asserted by a
  reader who only read the README.
- **Contrast and type floor on the rebuilt bar.** → The status strip and the
  example rows use existing tokens only: `--bg` ink, `--rule` for the dim voice
  and control boundaries, `--accent` for the prompt, the busy indicator and the
  focus ring. `--accent-ink` must not appear on the plate (3.52:1 there). Every
  value is measured at 1086px and at 390px, as the existing console was.
- **The example rows are new focusable elements.** → They are real `<button>`s
  with accessible names carrying the command text, styled from the existing ruled
  row geometry, so they announce as commands rather than as decoration.
- **The two surfaces could still drift** if only one of them is changed. → Both
  are changed in the same task group, and the landing page's examples are derived
  from the same stamped digests the standalone page fetches at runtime.

## Migration Plan

No data, deployment, or compatibility surface moves. The steps:

1. Console on the landing page and the standalone terminal, in one pass.
2. The extracted plate module, with the landing page as its first caller.
3. The documentation plate, and `publish.py`'s third target.
4. `published-tree.py`'s served set and `website/README.md` in the same commit as
   the file that requires them.

Rollback is a revert: nothing outside `website/` changes, and the recorded cast
is untouched by every step.
