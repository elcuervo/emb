# Notes

Measurements and comparisons taken during implementation, kept so the claims in
`tasks.md` are checkable rather than remembered.

## Floors and contrast, measured (task 6.4)

Computed inside a same-origin iframe at two widths, reading `getComputedStyle`
and walking up for the effective background. Ratios are WCAG 2.x against the
element's own ground.

| Role | 1086px | 390px | Colour | Ratio |
|---|---|---|---|---|
| `.wire__t` (mode) | 13.575px | 15px | `--bg` on dark | 16.59:1 |
| `.wire__n` (count) | 12px | 15px | `--bg` on dark | 16.59:1 |
| `.wire__cmd` (command) | 12px | 14px | `--rule` on dark | 9.22:1 |
| `.wire__key dt` | 12px | 14px | `--bg` on dark | 16.59:1 |
| `.wire__key dd` | 12px | 14px | `--rule` on dark | 9.22:1 |
| `.cap__label` / `.cap__claim` | 15.5 / 13.575px | 15 / 15px | `--fg` on paper | 17.28:1 |
| `.block__deck` | 13.575px | 15px | `--fg` on paper | 17.28:1 |
| `.sec__h` | 12px | 12px | `--muted` on paper | 4.82:1 |
| `.fig` | 12px | 12px | `--muted` on paper | 4.82:1 |
| `.plate__teaches` | 12px | 12px | `--muted` on paper | 4.82:1 |
| `.code__body` | 12px | 14px | `--fg` on paper | 17.28:1 |
| `.t-str` | 12px | 14px | `--accent-ink` on paper | 4.66:1 |
| `.t-dim` | 12px | 14px | `--muted` on paper | 4.82:1 |

**Read of the numbers.** The smallest text on the page is **12px at 1086** and
**12px at 390**, and every 12px item at 390 is the site's own caption voice
(`.fig`, `.sec__h`, `.plate__teaches`) whose sizes this change does not touch.
The atoms this change introduces are **14–15px on a phone**, promoted in the
`.wire` family's own `@media (max-width: 1000px)` block — deliberately not in the
shared 1000px block, which the family is declared after, so equal specificity
would let the base `clamp()` win. That was caught by measurement, not by reading:
the first attempt put the promotion in the shared block and the ledger stayed at
12px on a phone.

The page carries **no rendered control** — no button, input, link-as-action, or
disclosure. Its only interactive elements are the masthead's own links and the
skip link, both pre-existing and already at the site's target floor.

## Instrument geometry, measured

At 1280px the track is 288px and the marks are:

| Row | Lanes | Marks | Reading |
|---|---|---|---|
| `lazy: false` | 1 | 45 × 6 | six commands of one text, end to end |
| `lazy: :multi` | 1 | 287 | one command carrying all six |
| `lazy: :batch` | 3 | 95 × 3 | three concurrent shares of two |
| `Emb.multi { }` | 1 | 287 | one command, any models |

At 390px the track is 310px and the same ratios hold (49 × 6 / 309 / 102 × 3 /
309). The encoding is exact at both widths, which is what makes the diagram
checkable rather than decorative.

## Detector (task 6.3)

`impeccable detect --json` over `website/gem/index.html` + `website/assets/css/styles.css`,
compared finding by finding against the same run in a worktree of `HEAD`
(`git worktree add --detach /tmp/emb-head HEAD`) over `website/demos/batch.html`
+ `HEAD`'s `styles.css`:

| Antipattern | New page | `HEAD` baseline |
|---|---|---|
| `cramped-padding` | 21 | 7 |
| `all-caps-body` | 6 | 1 |
| `wide-tracking` | 1 | 1 |
| `clipped-overflow-container` | 2 | 2 |
| `overused-font` | 2 | 2 |
| `em-dash-overuse` | 1 | 1 |
| **Totals** | **33** | **14** |

**No kind of finding is new.** Every category the page triggers is already
triggered by the gallery's own plates. The two that grow are functions of the
page being longer and carrying six `FIG.` captions:

- `cramped-padding` fires on the poster's ruled-ledger language — a hairline with
  its content flush against it — which `.cap`, `.code` and `.block` all carry at
  `HEAD`. `.wire` / `.wire__i` join that set by inheriting the same rule.
- `all-caps-body` fires on `text-transform: uppercase` in the `.fig` caption and
  `.wire__key` labels, which is the site's tracked-caps annotation voice and the
  reason `.fig` exists at `HEAD`.

Resolved or accepted: all accepted as the established world, none silent.
`overused-font` is the site's three self-hosted families; this change adds none.

## Checks run

| Check | Result |
|---|---|
| `just website-published` | 52 served paths, one origin (`https://emb.is`) |
| `just website-version-check` | 6 stamped values, all `0.4.0` |
| `just website-presets-check` | 13 markers, all 6 digests current |
| `just website-ink … gem/index.html` | PASS at all 24 widths (1920 → 320) |
| `just website-ink` (landing) | PASS at all 24 widths |
| `just website-ink … demos/batch.html` | PASS at all 24 widths |
| `qa` on `/gem/` | passed; no console, page-error or network failures |
| `qa` on `/demos/` | passed; the new cross-surface link resolves |
| HTML nesting, all 14 served pages | balanced, nothing unclosed |
| `openspec validate website-gem-page --strict` | valid |

## Process defects worth recording

Two mistakes were made and caught by checks rather than by care, both in the same
mechanical step:

1. **A `</li>` sweep hit the wrong list.** Replacing the `.cap` ledger's
   `<li>` closers with `<div>` closers, by indentation, also rewrote the `.wire`
   `<li>` closers in both modes ledgers — because they share the same ten spaces
   of indentation. The result was not a rendering glitch but a **structural**
   one: every section after the first `.wire` list became a child of its `<li>`,
   which is why the body laid out at ~2400px inside a 1280px viewport while
   `documentElement.clientWidth` still reported 1280 and `overflow-x: clip` hid
   it. `scrollWidth === clientWidth` would not have caught it. The check that
   did was walking every element's `getBoundingClientRect().right` against the
   viewport, then an HTML nesting walk.
2. **A brace-free CSS guess.** The phone-size promotion was placed in the shared
   `@media (max-width: 1000px)` block at line ~1963; the `.wire` family is
   declared at the end of the file, so its equal-specificity `clamp()` overrode
   it and the ledger stayed at 12px on a phone. Only measuring
   `getComputedStyle().fontSize` inside a 390px frame showed it.

Both are the same lesson the site's own `PRODUCT.md` already records: the
measurement finds the defect, the reasoning does not.

## Screenshots

`website/.impeccable/surfaces/website-gem-index-html.md` is the direction
contract this build was audited against. Captures for this change:

- `/tmp/embreview/gem-final-1280.png` — full page at the browser's own width
- `/tmp/embreview/gem-390-full.png` — full page in a 390px frame
- `/tmp/embreview/modes-390b.png` — the modes ledger at 390, readable

They live outside the repository on purpose: nothing in `website/` may be an
authoring artifact, and `published-tree.py` would fail the build if they were.

## Deviations from `design.md`

Three, each decided while building and each recorded here rather than left in
the code:

1. **The two gems are a ruled ledger, not a `.wire` drawing.** `design.md` listed
   the distribution as one of the four uses of the instrument. Built as a
   two-column `.cap` ledger under a `FIG.` caption instead, because the claim is
   *two things with different jobs*, and the two-box-with-an-arrow drawing that
   would carry it is a composition this poster does not have. The `.cap` ledger
   already means "two of a kind, side by side" on the reference surface. The
   instrument keeps its three uses, all of which encode commands over texts.
2. **The request path is a numbered `.cap` ledger, not a diagram.** Same reason:
   the path is four stages in order, which is what `01 · YOUR CALL` … carries,
   and the stage order is the only information in it.
3. **Section grounds alternate strictly** (paper → dark → *(ridge)* → paper →
   dark → paper → dark → paper) where `design.md`'s table showed three paper
   sections in a row. The docs surface and the landing both alternate at every
   block, and `.block__head`'s job is to make the page read as turned over; three
   paper blocks in a row is the one arrangement the poster's own rhythm does not
   use.

## Checked against the shipped specs (task 6.5)

| Surface claim | Spec |
|---|---|
| three `lazy` values, defaults and their wire commands | `ruby-batch-loading` → Lazy mode configuration |
| deferred values issue nothing until used | `ruby-batch-loading` → Per-scope coalescing into `EMB.MULTI` |
| `:batch` chunk shares run concurrently | `ruby-batch-loading` → Batch mode parallel execution |
| a failed batch fails closed and raises `Emb::ServerError` | `ruby-batch-loading` → Batch failures fail closed |
| request- and job-scoped clearing, and the four registrations | `ruby-batch-loading` → Request-scoped / Job-scoped cache clearing |
| a null pair materializes as `nil` while siblings resolve | `ruby-batch-loading` → Failure handling follows MGET semantics |
| pool, rotation, pre-send retry across instances | `emb-ruby-client` → Connection pooling; `ruby-client-round-robin` |
| `VALUES` envelope shape | `emb-ruby-client` → VALUES envelopes decode to floats |
| pairs collected then one `EMB.MULTI` | `emb-ruby-client` → Multi-model batch |
| executables, platforms, binary wrapper, `onnxruntime` dependency | `emb-server-distribution` (all requirements) |

No disagreement found. The one place the page is more specific than the specs is
`reconnect_attempts: 0` as a default with its rationale, which
`ruby-batch-loading`'s fail-closed requirement states in its scenario rather than
in its requirement text.

## The motion, measured (tasks 7.1–7.9)

Every number below is `getComputedStyle(el).transform` read after `scrollTo` and
two animation frames, with `scroll-behavior: auto` forced, on the shipped page.
`scaleX` for a mark; the row's own `cover` range is the clock, so a percentage
is a percentage of that row's journey through the viewport, not of the page.

**The four modes, row by row.** Viewport 577px, ledger row 108px, 1280px wide.

| mode | 25% | 32% | 36% | 40% | 46% | 50% |
|---|---|---|---|---|---|---|
| `lazy: false` | `0 0 0 0 0 0` | `.91 .59 0 0 0 0` | `.99 .96 .82 .23 0 0` | `1 1 .99 .93 .69 0` | `1 1 1 1 .99 .93` | `1 1 1 1 1 1` |
| `lazy: :multi` | `0` | `.91` | `.99` | `1` **accent** | `1` accent | `1` accent |
| `lazy: :batch` | `0 0 0` | `.90 .90 .90` | `.99 .99 .99` | `1 1 1` | `1 1 1` | `1 1 1` |
| `Emb.multi { }` | `0` | `.90` | `.99` | `1` **accent** | `1` accent | `1` accent |

Read the two columns that matter: at 32% the batch row's three marks are at
**identical** scale (`0.90 0.90 0.90`) while the eager row's are at three
different points (`0.91 0.59 0`) — the concurrency and the sequence are
distinguishable part-way through, which is the whole encoding. And the accent
appears on exactly the two rows whose mark is one call, mid-draw (at 32% the
`:multi` mark is `rgb(253, 92, 34)` — between the rule's grey and the accent).

**Where each row performs.** Sampled across all four rows: a row's marks move
between y≈390 and y≈300 in a 577px viewport — mid-screen — for every row. The
first implementation put the timeline on the ledger instead of the row and the
cascade finished while rows three and four were still below the fold; that is
the defect the per-row timeline exists to remove, and it was found by measuring
where the marks were moving, not by looking.

**Create-then-consume** (the second rehearsal): `Interleaved` reads
`0.97 0.87 0.43 0 0 0` at 34% and `1 1 .99 .93 .69 0` at 40%; `Hoisted` reads
`0.97` at 34% and `1` at 40%. Six ticks, then one stroke.

**The scope's edge** draws at `cover 53% → 64%` of the row, after that row's last
mark (which ends at 52.5%), and the element's own `border-right` computes to
`rgba(0, 0, 0, 0)` while the motion is active, so the edge is painted once.

**The three fallbacks, each measured rather than reasoned about**

| Condition | How it was produced | Result |
|---|---|---|
| No view-progress timelines | deleted the `@supports` rule from `document.styleSheets` at runtime | `animation-name: none`; marks `1,1,1…`; widths `45×6 / 287 / 95×3 / 287`; accent on the two coalesced rows only; `border-right` painted by the border |
| `prefers-reduced-motion: reduce` | a fresh session (headless Chrome reports `reduce`) | identical to the row above — `animation-name: none`, no pseudo element, finished instrument |
| Print | the guard is `@media screen and …`, read back from the CSSOM as `"screen and (prefers-reduced-motion: no-preference)"` | the block cannot match print, so print gets the base styles — the same state the first row proves is finished |

The `screen` keyword is the fix for a real defect: a print pass never scrolls,
so without it every mark would print at the start of its range and the instrument
would come out blank on paper. The page's existing `@media print` block — which
collapses the landing's reveal cascade to its end state for exactly this reason —
needed no entry, because the motion is screen-only by construction.

**A specificity defect the measurement caught.** The scope's edge stayed visible
under the motion because `.block--dark .wire__track` (0,2,0) out-specified the
transparent `.wire__track` (0,1,0) declared later, so on the dark ground the edge
was painted twice — once still, once drawing. Fixed by giving the transparent
rule the extra class (`.wire .wire__track`), and verified both ways: transparent
with motion active, `--rule-dark` without it.

**Detector.** Re-run over `website/gem/index.html` + `website/assets/css/styles.css`
against the same `HEAD` worktree baseline as before the motion: 33 findings to 14,
**no new kind, and no count changed** (21 `cramped-padding`, 6 `all-caps-body`,
1 `wide-tracking`, 2 `clipped-overflow-container`, 2 `overused-font`,
1 `em-dash-overuse`). The motion adds no static finding, and its own rules are
all inside conditional group rules the detector does not read.

**Ink probe with motion active** (`prefers-reduced-motion: no-preference`): PASS
at all 24 widths, 1920 → 320.

## Cost of the motion

| | |
|---|---|
| Scripts added | 0 |
| Animated elements | 12 marks + 6 hairlines, all inside one section |
| Properties animated | `transform` and `background-color` only — no layout property |
| Elements repainted per frame | bounded to one row's marks (≤ 6 small fills) |
| Runs | only while a row is between 28% and 64% of its own viewport journey |

## The capture trap this page adds

The site's `just website-shot` scrolls to the end and back before capturing,
because a `captureBeyondViewport` full-page screenshot lays the document out
without moving the trigger line, and every `[data-reveal]` below the fold would
otherwise capture at `opacity: 0`.

**On this page that recipe now produces a blank instrument**, and the reason is
the same one stated in the other direction: a scroll-driven animation's state is
a function of the scroll position *at capture time*. After scrolling back to the
top the ledger sits below the fold again, so its marks are back at the start of
their ranges; and under `captureBeyondViewport` the viewport is expanded to the
document's height, which leaves each row a per-cent or two into its `cover`
range. Either way the ledger photographs empty.

Two consequences, both worth knowing before someone concludes the page is
broken:

- **A full-page capture of this page is not evidence of the instrument.** Capture
  the section with the row in view (`.wire__i` at `cover ≈ 32%`), or capture with
  `prefers-reduced-motion: reduce` / print emulation, where the finished ledger
  is the base state.
- **Nothing is hidden from a reader by this.** A reader scrolling to the ledger
  has it in view, which is exactly the condition the animation is keyed to. The
  failure mode is confined to tools that capture without scrolling a row into
  view, and to `captureBeyondViewport`, which no reader uses.

## What happens on load, measured rather than assumed

A scroll-driven animation has a state at scroll position zero, so the honest
question is what a reader sees before touching the wheel. Measured on the shipped
page at two viewport heights, with no scrolling at all:

| Viewport | Ledger top | What the marks read at load |
|---|---|---|
| 1280 × 577 (the desktop the page was built against) | y 1634, below the fold | nothing visible; every mark at `0`, nothing plays above the fold |
| 1280 × 2500 (an unusually tall window) | y 1635, on screen | row 0 `0.96 0.81 0.19 0 0 0`, row 1 `0.63`, rows 2–3 at `0` |

So the tall-window case is not a blank instrument: the rows near the top of the
journey perform on load and the deeper ones follow on the first scroll. The
reason is arithmetic rather than luck — a mark starts at 28% of its row's `cover`
range, and a row sitting at y 1832 in a 2500px viewport is only 25.6% of the way
through its own journey, so it is *entering* rather than entered. A row begins
drawing when it reaches the lower third of the viewport, which is the moment a
reader is arriving at it.

Two properties of the scroll-driven choice are worth keeping in view:

- **A reload restores the correct visual state**, because the state is a function
  of scroll position rather than of elapsed time since load.
- **Nothing plays above the fold on load**, so the page's first paint is the same
  page it was before it had any motion, and no reader waits through a choreography
  to read the prose.

## The QA preset's selector wait and a zero-width mark

`qa` with `expectedSelector: ".wire__lamp.is-one"` fails on this page: the preset
waits for the element to be *visible*, and a mark at the start of its range is
`scaleX(0)` — correctly, because nothing has been sent yet. This is not a defect
and not script-hidden content; it is a mark that has not been drawn because the
row has not been scrolled to. `qa` with a selector that exists independent of the
motion (`".wire__key dd"`) passes, and that is the check that was run.

Worth stating as a rule for whoever checks this page next: **do not assert
visibility on an element the motion owns.** Assert the text, or scroll the row
into view first.
