# emb — product site

The static marketing site for [`emb`](../README.md), designed as a
neo-brutalist technical poster that happens to function as a product
homepage. The brief lives in [`../DESIGN.md`](../DESIGN.md); the measured
corrections that took the page to the reference poster live in
[`CORRECTIONS.md`](CORRECTIONS.md) (implemented).

```
website/
├── index.html              the landing page: masthead → hero (prose + pipeline)
│                           → protocol → scripts → operations → terrain
│                           → footer
├── 404.html                the not-found page, root-absolute by necessity
├── docs/
│   └── index.html          the reference surface: install → commands → replies
│                           → configuration → scripting → operations
│                           → benchmarks → clients → status
├── _headers                caching policy; read by the platform, never served
├── .assetsignore           what must never be served. The only boundary
├── assets/
│   ├── css/styles.css      the shared world: tokens → primitives → sections
│   │                       → responsive (both surfaces link it)
│   ├── css/docs.css        documentation-only rules; declares no token
│   ├── js/main.js          entrance sequencing, reveals, pipeline, route
│   │                       (the landing only — docs ships no JavaScript)
│   ├── fonts/              self-hosted Archivo, Inter and JetBrains Mono
│   └── img/
│       ├── terrain-matte.png  the cut-out the page ships (2172×724, alpha)
│       ├── terrain-v2.png  generated rock formation, the matte's source
│       ├── terrain-v2.md   generation prompts and provenance
│       ├── speckle.svg     original photocopy speckle asset
│       └── og.png          1200×630 social card
├── tools/
│   ├── gen-isometric.py    regenerates the pipeline SVG
│   ├── gen-terrain-matte.py derives the terrain cut-out from terrain-v2.png
│   ├── png_lib.py          dependency-free PNG reader/writer for the above
│   ├── ink-probe.html      asserts no text ink crosses the viewport, 24 widths
│   ├── published-tree.py   what ships, the canonical origin, the cache rules
│   └── stamp-version.py    writes VERSION into every `data-emb-version` element
└── README.md
```

This list is two populations, and the difference is not visible in `ls`. The
pages and `assets/` ship; `tools/`, this README, `PRODUCT.md`, the
generation sources, and `.impeccable/` do not. [What ships](#what-ships) is
the contract and `tools/published-tree.py` is the check.

## Two surfaces, two modes

The landing is **Persuade**: a poster whose job is to earn a click. `/docs` is
**Read**: the hosted form of the reference that `../README.md` already carries.
`../README.md` stays the source of truth for both.

The landing therefore keeps exactly four things beyond its composition, each
one a decision a reader makes before clicking:

| Keeps | Where |
|---|---|
| one copy-paste install line | `.hero__start`, under the actions |
| the release, licence and builds | the same ledger |
| one measured number and how to reproduce it | the same ledger, from `BENCHMARK.md` |
| one REPL proof — `redis-cli EMB minilm "hello world"` and its bytes | the protocol block |

Everything else lives on `/docs` only: the command tables, the configuration
block, the reply-format prose beyond the one line that carries the argument,
the `emb-top` figures, and every version string. The landing's calls to action
and its `Docs`/`Models` navigation entries all resolve to `/docs`; the only
reference link that leaves the site is the benchmark's `reproduce`.

**Why `/docs` links `styles.css` rather than a shared subset.** The landing's
geometry is selector-scoped, not global — the only element-level rules in the
sheet are `:root`, the `*` reset, `html`, `body` and `body::after`, and every
`var(--fold)` use sits inside `.spine`, `.block__body`, `.topviz`,
`.terrain__art` or `.note`. A page with none of that markup inherits the world
and none of the composition, so no file had to be split. The cost is that
`/docs` downloads the hero's unused CSS (about 30 KB, far less gzipped); the
benefit is that the two surfaces cannot desynchronise.

## Render it

No build step, no dependencies, no CDN. Any static file server works:

```bash
just website            # http://localhost:8080
just website port=9000  # or pick a port
```

Or directly:

```bash
python3 -m http.server 8080 --directory website
```

Opening `index.html` over `file://` works too — nothing here needs a server.

`nix develop .#website` (or the combined `nix develop`) provides the tooling:

```bash
just website-browser                  # once: fetch Chrome for Testing
just website-shot                     # full-page screenshot of :8080
just website-shot http://localhost:8080 /tmp/phone.png 390x844
just website-version-check            # stamped versions still match VERSION
just website-published                # what ships, the origin, the cache rules
just website-ink                      # no text ink crosses the viewport
just website-ink http://localhost:8080 docs   # …on the docs surface
just website-ink http://localhost:8080 404    # …on the not-found page
```

### The ink check

`just website-ink` opens `tools/ink-probe.html`, which loads a surface in a
same-origin iframe at 24 widths — every supported width plus the measured
defect bands, the 1086px reference frame, the breakpoints, and the shell-cap
handoff — and fails if any text run's ink straddles a viewport edge.

The landing is the default. `?target=docs` and `?target=404` sweep the other two
surfaces, and the recipe takes the target positionally:

```bash
just website-ink                              # the landing
just website-ink http://localhost:8080 docs   # the docs surface
just website-ink http://localhost:8080 404    # the not-found page
```

Both surfaces are measured, not assumed safe because they are short. The recipe
waits for `window.__inkProbe` before asserting and calls
`window.__inkProbe.assert()`, which **throws** on failure; it used to evaluate
the instant the page opened and to print a verdict without checking it, so a
broken page reported `RUNNING…` and exited 0. The probe and its iframe are also
cache-busted per run, because a constant bust once measured the previous landing
and reported failures at 390px and 320px against HTML that had already changed.

The check exists because the obvious one cannot work. The page frame applies
`overflow-x: clip`, which creates no scroll container and removes the clipped
content from the scrollable overflow region, so
`documentElement.scrollWidth === clientWidth` stays **true whether or not glyphs
are being cut**. Twelve correction passes used that comparison and missed a real
defect: the stage notes sat 10px past the sheet edge, so three of the four
descriptions lost their last character between 1070px and 1150px, including at
the design's own 1086px reference frame. Only rects — or pixels — can see it.

The probe tests only ink that **straddles** an edge. A run parked entirely
off-screen (the focus-revealed skip link) and a run clipped to an inner box (the
`sr-only` pattern) are not this defect, and the probe skips both.

Two other measurement traps worth knowing before trusting a number:

- **Element-scoped screenshots lie about filtered SVG.** `screenshot .pipeline__stack`
  at a 900px-tall viewport paints only the first of the four plates and leaves the
  rest blank, because Chrome's `captureBeyondViewport` does not paint
  SVG-filtered or masked content outside the viewport. The diagram is correct; the
  capture is not. Use a tall viewport, or measure the DOM.
- **Stylesheets cache silently.** `index.html?cb=N` busts the HTML but not the CSS,
  and a reload may serve the old sheet. Verify CSS changes in a fresh browser
  session, or the numbers describe the previous build.

### The version check

`just website-version` writes the repo's `VERSION` into every element carrying
`data-emb-version`; `just website-version-check` fails when one has drifted. The
`emb-top` capture shipped a hand-typed `v0.4.0` against a `0.4.0.pre4` VERSION,
which is the drift this exists to stop. Run the stamper after bumping `VERSION`.

## What ships

The site is served at **https://emb.is** from a Cloudflare Worker with static
assets — `emb-site`, declared in [`../wrangler.jsonc`](../wrangler.jsonc). There
is no build step here and there is none there: the Worker's asset directory is
this folder, so what is published is these files.

That makes `.assetsignore` the only thing between a working file and a public
URL, which is a deliberate trade. One folder is easier to work in — `just website`
serves it at `/`, `tools/ink-probe.html` stays same-origin with `index.html`, and
nobody repoints a path — but the split it replaces prevented leaks structurally,
by putting authoring files in a directory that was never published. One folder
cannot. So the boundary is checked rather than trusted:

```bash
just website-published
```

`tools/published-tree.py` asserts three things, and `ci.yml`'s `site` job runs
the same two commands on every pull request:

| Set | Members |
|---|---|
| **Served** | `index.html`, `404.html`, `docs/index.html`, `styles.css`, `docs.css`, `main.js`, three WOFF2 subsets, `terrain-matte.png`, `og.png`, `speckle.svg` |
| **Ignored** | `tools/`, `.impeccable/`, `PRODUCT.md`, `README.md`, `terrain-v2.png`, `terrain-v2.md` |
| **Platform config** | `.assetsignore`, `_headers` — read, never served |

The check fails both ways: a file that would ship without being expected, and an
expected file that is missing or wrongly excluded. It also asserts that every
canonical, `og:url`, and social-image value is **absolute** and that every page
agrees on **one** origin, and that `_headers` never pins a `.html`, `.css`, or
`.js` path `immutable` — the one way the cache policy could serve a reader a
stale page. **Do not delete this check while tidying up:** without it the
boundary is a comment.

The origin is checked for shape but never pinned to `emb.is`. This one tree is
served from `localhost:8080`, from the Worker's `workers.dev` address, from
per-branch preview aliases and from production, so a check demanding the
production hostname would fail everywhere but production — and a check that
fails during the working loop is one somebody eventually disables. The origin
found is reported, and compared to `wrangler.jsonc`'s route when one is set; a
disagreement there is a warning, not a failure.

`404.html` is the one surface that uses root-absolute URLs (`/assets/...`), and
it has to: the platform serves its content with the requested path still in the
address bar, so a relative `assets/...` would resolve against `/foo/` and the
page would arrive unstyled.

### Publishing

`.github/workflows/site.yml` publishes, path-filtered to this folder:

| Event | What happens |
|---|---|
| push to `main` | `wrangler deploy` → https://emb.is |
| pull request | `wrangler versions upload --preview-alias pr-<number>` → a comment on the PR |

Both are gated on the checks above, which run in that workflow rather than being
depended on from `ci.yml`, so no publication can happen because another workflow
was skipped. `versions upload` creates a version and its preview URLs **without**
touching the production deployment, so a preview cannot disturb `emb.is`.

The alias is what makes a reviewer's link survive: each upload repoints
`pr-<number>-emb-site.<subdomain>.workers.dev`, so it always serves the newest
commit on that branch, while the versioned URL is unique per upload and dies on
the next one. Wrangler prints both addresses, and the workflow comments the
alias with the version beside it. The preview job invokes wrangler directly
pinned to the release the dev shell installs, because `--preview-alias` is a
wrangler 4 flag and `wrangler-action`'s default is 3.90.0.

A preview needs a Worker to upload a version *to*, so the first site pull
request — the one that merges the Worker into existence — gets no preview. The
job says so in the run's summary instead of failing; later pull requests get
one once `main` has deployed.

Pull requests **from forks** get no preview: fork runs receive no secrets, and a
`pull_request` run from a fork also gets a read-only `GITHUB_TOKEN`, so the job
cannot even comment to say so — it writes the explanation to the run's summary
instead. Review those locally with `just website`.

**Rollback.** Every deploy is one commit. Re-run the `Site` workflow on the
commit to return to (`workflow_dispatch` exists for exactly this), or redeploy
that commit with `wrangler deploy` by hand. Cloudflare also keeps previous
versions, so `wrangler rollback` restores the apex in seconds. Nothing here is
stateful, so no rollback can lose anything but a minute.

The site's dependency list is `websiteDeps` in [`../flake.nix`](../flake.nix) —
separate from the server's, and browser tooling lives there rather than in the
Go shell. `firefox` is deliberately not on it: nixpkgs builds it from source on
`aarch64-darwin`, which is hours. The check that catches that is in
[`../AGENTS.md`](../AGENTS.md).

## Design notes

**Palette** — a warm off-white ground (`#F3F0E8`), near-black ink
(`#0B0B0B`), one vivid orange (`#FF5A1F`) for surfaces, a deeper
`--accent-ink` (`#C23D00`) for the accent used as text or as a focus ring, a
muted grey (`#6B6963`) and a leader grey for the annotation steps. No
gradients, no rounded corners, no shadows except the 5px printed offset under
the hero button. `#FF5A1F` is 2.74:1 against the paper and `--accent-ink` is
4.66:1, which is why the accent never carries text or a ring at its surface
value.

**Type** — self-hosted Archivo for the claim, JetBrains Mono for technical
copy, buttons and labels, and Inter 900 for the masthead brand. The giant
wordmark uses native SVG outlines matched to the reference, so its silhouette
does not depend on font metrics. Fonts are preloaded; no CDN is needed.

Every technical label is clamped so it cannot compute below **12px on
`vw`-driven layouts**, and below 1000px the same labels are set at **14px**
explicitly. Both are floors in the stylesheet rather than wishes: the
smallest text on the 1086px frame is 12px and the smallest text on a phone is
14px, measured. That last claim was wrong until pass 11 — the masthead's
`Get Started` control and its mobile `Menu` disclosure were 13px below 1001px,
which is the two controls a phone reader touches first. They are 14px now, and
the masthead still fits at 320px.

**Composition** — the page is a signal travelling from the wordmark to the
massif, and it is organised around one line rather than stacked as bands.

The hero is *one* two-column spread: the left column carries the claim, the
sub, the actions, the kept-facts block and the feature list; the right column
carries the pipeline. At 1086px wide that hero spread now measures **1504px
tall — a 1 : 1.385 frame** against the poster's 1 : 1.333, up from 1464px
(1 : 1.348) before the kept-facts block added its 187px. The poster's ratio
governs the hero alone; the document is not one sheet and its total height is
not a constraint (4445px at 1086).

Because the wordmark is ~500px tall and the poster's ratio governs the spread,
**nothing in the hero's action area is above the fold on a desktop viewport** —
the CTA's own bottom edge sits at y 1205 in a 900px-tall 1440 viewport, and at
y 931 in a 900px-tall 1086 viewport. The kept-facts block is therefore placed
*adjacent* to the calls to action rather than pretending to a fold it cannot
reach: install line, release ledger and measured number are one ruled row
directly below the buttons.

Below the hero sit **three blocks**, each a movement rather than a section:

| Block | Ground | Carries |
|---|---|---|
| Protocol | paper | the ruled ledger, a shell specimen, the console plate |
| Scripts | full-bleed `#111110` | the `model(fn(input))` shift, the Lua specimen, the five shipped scripts |
| Operations | paper | the ops ledger and the `emb-top` capture |

Then the **terrain, last**, where the signal lands and the page stops. The
three grounds are the variety; the blocks themselves introduce nothing the
hero did not already use.

The composition's backbone is the orange signal axis:

```text
--fold: 65.31%          /* = --col-prose + 40.7% x (1 - --col-prose) */
--spine-w: calc(var(--plate) * 4 / 364)
```

`--fold` is the hero's own geometry, not a tuned number. `.pipeline` sits in
column 2 of `.hero__body` — which has no `gap`, so column 2 is
`100% - --col-prose` wide — and its spine is at 40.7% of that column
(`40.7% - --plate/2 + --plate/2`, the `--plate` terms cancelling). That makes
the axis width-independent, so every spine segment in the page and the route's
fork can be derived from the one value instead of defended at seven widths.

`--spine-w` is the line's one rendered weight. The SVG spine and the terrain
route both carry `vector-effect="non-scaling-stroke"` with that width, so they
render at exactly the CSS value however their viewBoxes are stretched to
cover their boxes. Above 1000px that value is `--plate`-derived; below it each
breakpoint redeclares `--spine-w` to hold the poster's weight as the stack
column goes to 50% of the content box and then the whole of it. It is not a
cosmetic number: measured with the plate-derived value at 600px, the CSS line
was **2.406px against a 6.066px SVG stroke** — a 2.5x weight step at the
handover — and the route rendered **~1.2px heavier** than the spine on a
desktop frame. With the single width the segments agree exactly (`CORRECTIONS.md`).

The hero keeps its own masked SVG spine (`.sig`), which is what hides the line
behind each plate's front edge; a CSS segment runs the hero's full height
behind the column at the same axis and weight, and the masthead carries one
from the very top of the page, so the line is unbroken from the viewport top
to the ridge. Every block and the terrain carry a `.spine` segment. Sections have **no
vertical margins, only padding**, so consecutive segments abut exactly — and
where a segment would cross body copy it is either placed in a lane the copy
clears, or hidden behind an opaque plate, which is the rule the hero's plates
already follow.
The composition's backbone is the orange signal axis: `.sig{x=182}` inside
the plate SVG, the plate centres from `.pipeline{margin-left}` , the ridge's
branch point and the axis the annotations hang off all sit on it. The terrain
route re-enters the artwork at that same axis (`x1124.3` of 2172), so the
spine, the fork and the ridge read as one line.

**The pipeline** uses a dimetric projection: half-width 182, top-face ratio
`.326`, plate pitch 119 and thickness 24. It carries 46 input marks and 48
small extruded blocks. The INFERENCE plate transitions to a dark lattice;
SERVE has black top and side faces. SVG grain adds a light print texture.
Lifted wire grids, faint surface grids, cube shadows and ruled slab edges
provide depth. The orange signal uses a mask so it enters each plate and
passes behind the front edge.

The plates are painted **back to front** — SERVE first, INPUT last — because
the camera sits about 19 degrees above the horizon and the highest plate is
therefore the nearest one. The other order leaves each lower plate's top face
cutting into the front skirt of the plate above it: a 73 x 24 unit wedge
around the spine, hidden today only because the signal line covers its centre.
`data-depth` on each `.slab` records the level (1 farthest, 4 nearest), and
the generator emits them in that order. The activation stagger is keyed by
plate name, so document order and animation order are independent. To
regenerate the SVG directly in the page:

```bash
nix develop --command python3 website/tools/gen-isometric.py --write
```

Without `--write`, the generator prints the SVG to standard output.

**The terrain** ships as `terrain-matte.png`: a real alpha cut-out derived
from the generated `terrain-v2.png` by `tools/gen-terrain-matte.py`, which
bakes the old `grayscale(1) brightness(1.08)` grade into the alpha channel
(`alpha = 1 - L/white` over black). Compositing it over the paper is
pixel-identical to the `mix-blend-mode: multiply` it replaced (mean error
0.33/255), without the failure mode that came with it: a blend with no
backdrop in its stacking context paints the artwork's own off-white ground as
an opaque rectangle until something forces a repaint. Regenerate with:

```bash
python3 website/tools/gen-terrain-matte.py --write
```

The cut-out and the ridge route share one box (`.terrain__art`), whose aspect
ratio is the artwork's, so neither can stretch relative to the other.

At **641px and up the box runs the full width of the shell**, so the massif is
complete and its base sits flush on the footer. The route's fork is authored at
`1124.3 / 2172 = 51.76%` of the image, while the spine sits at that
breakpoint's `--fold`, so the ridge is shifted inside its own SVG by the
distance between the two: `+269.6` units at `64.18%` on desktop, `-537.9` at
`27%` on tablets. Without the shift the spine would come down the page and
stop in mid-air, because the artwork only rides the ridge at its own fork.

On phones the box is `156vw` and anchored by its **left** edge
(`left: calc(var(--fold) - (1124.3 / 2172) * var(--art-w))`) so the fork lands
on the 90% rail by construction, with the massif's right slope deliberately
off-frame. `html{overflow-x:clip}` still holds any overflow, and
`documentElement.scrollWidth` equals `clientWidth` at every width measured.

`--band` is `max(--art-h, clamp(...))` where `--art-h` is
`--art-w x 770.2/2172`. The floor matters: it is what guarantees the art box's
top edge — where the route's trunk begins — is never above the band's top,
which is where the spine ends.

The route is the trunk only. It used to fork three **data branches** across
the sky over the massif — `BLOB OR VALUES`, `HELLO 3` and `1 MS WINDOW` — each
carrying a README fact the pipeline diagram cannot show, and each staged 14%
apart along the scroll so the facts arrived in sequence. They are gone: three
spurs crossing the peak turned the massif into a diagram of itself, and the
three facts already have a home in the blocks, where they sit beside the
command they describe. What is left is one line arriving somewhere, which is
the whole point of the metaphor.

Both slogans carry the paper with them (`background: var(--bg)`). The massif's
silhouette passes under a label at some width in the 320–1440 range, and a
knockout is both the fix and the honest one — no contrast check can certify
text over a photograph. It is the same device the spine gets from the content
above it.

**The blocks** — the poster argues; these prove. Each is built only from the
poster's atoms: a `.block__head` bar, ruled `.cap` entries (a hairline and a
mono ladder, never cards), and dark plates.

**The block header is a solid bar, and it inverts with its ground.** The
heading is not a label with a hairline under it — it is the poster's loudest
typographic move, Archivo at 750 in uppercase at up to 34px, reversed out of
`--fg` on the paper blocks and out of `--bg` on the dark one. That gives the
sequence a beat at every block boundary, makes the dark block read as the page
turned over rather than as a paper page with a dark patch in it, and lets one
element carry a section without a rule. It is the existing display voice used
louder, not a new one.

The bar is opaque, so it covers the spine where it crosses — the same rule the
console plate and the hero's slabs follow. Above 1000px `.block__grid` is two
columns whose split *is* the spine's lane — body copy ends before the line and
the mono facts begin after it, so no text can ever sit on it. Below 1000px the
lane is no longer between two columns: at 641–1000px `--fold` is 25%, the
stacked pipeline's own plate centre, so it is the **left** gutter and both of
the block's parts take the content column, stacked (`426.7px` at 641,
`673.5px` at 1000, clearing the rail by `--fold-gap`). The emb-top band is a
sibling of `.block__grid` rather than a child of it, so it carries the same
left inset as a `margin-left` instead of a column placement — the lane is on
the opposite side of the content on a phone, which is the mirror-image rule.

**The dark block is the page's one inversion, made of material already on the
page** — the SERVE plate's own `#111110` and `#292823`, lit by the spine
crossing it in `--accent` at **6.06:1**. There is no new palette here, only
this one turned over. Its `--muted` is remapped to `--rule`, because
`#6B6963` measures only **3.40:1** on `#111110`.

**The console is the SERVE plate, laid flat.** It reuses that plate's own
faces plus the same 1px `--bg` stroke every other plate carries, and it sits
*above* the spine: the line passes behind it and re-emerges below, which is
the rule the hero's plates follow. There are no title bars and no traffic
lights — that would be a costume note from another world.

It introduces **no new colour**. The type lifts existing tokens onto the dark
field, and every value is measured (`:focus-visible` included): `--bg` ink at
**16.59:1**, `--rule` for the dim voice, the badge and the control boundaries
at **9.22:1**, `--accent` for the prompt, the error prefix and the focus ring
at **6.06:1**. `--accent-ink` is the one token that does *not* travel: it is
tuned for the paper (4.66:1) and measures only **3.52:1** here, so the console
overrides the global focus ring back to `--accent`.

**It is hidden for the first ship, and one attribute brings it back.** The
live runtime is not wired yet, so `<section class="console" … hidden>` keeps
the markup, the styles and the transcript client in the tree while taking the
panel out of the rendered page and the accessibility tree; `main.js` only
boots a console that is not `hidden`. Removing that single attribute restores
the placeholder exactly, and the `window.embConsole.exec` seam below is what a
live RESP client replaces. Everything the panel does is described here as the
artifact it is, not as what a reader sees today.

**It is a placeholder, and it says so.** The bar reads `DEMO · NOT A LIVE
SERVER`, the note under the panel says the live client is not wired, and there
is no endpoint anywhere. The controls are real — a `<form>`, a labelled input
and a `<pre aria-live>` — so the live version is not a rewrite: replacing
`window.embConsole.exec` with a RESP client drives the same markup, modes and
states. Two modes are the two special functions: `REDIS` shows the bytes and
the `VALUES` envelope, `SCRIPTS` shows a script loaded once and called by SHA
to answer as a classifier. Every line is copied from `README.md` or
`examples/scripts/`, and the SHA1 in the scripts transcript is a real
`sha1()` of `examples/scripts/snippets/sst2.lua` — the same value the server's
`scriptSHA()` returns. The executor never touches the network, playback is
line-by-line rather than per character, and `prefers-reduced-motion` collapses
it to one frame. Without JavaScript the form is hidden and a `<noscript>`
transcript stands in.

The panel is a real `role="tablist"` with roving `tabindex` and arrow-key
navigation, and the live region is armed *after* the idle line is painted, so
loading the page does not announce a console hint. Every control clears the
44px target floor at 320–834px. The smallest text in the region is **12px at
1086** and **14px at 390**, which is the committed floor.

**Code is typeset as code.** Every specimen — the shell invocation, the Lua
source, the `model(fn(input))` shift, and the console's replayed commands and
replies — carries four token classes. The specs in the markup are marked by
hand; the console's plain-string transcripts get the identical classes from a
small pattern-based highlighter in `main.js` (four rules, no library), which
is why `sst2` is not mangled into `sst` + `2`: the numeric rule is
word-bounded. Emphasis is weight and colour-role, never a second hue — the
page has one accent and this does not spend it twice. Measured on both
grounds:

| Class | Paper | Ratio | Dark | Ratio |
|---|---|---|---|---|
| plain | `--fg` | 17.28:1 | `--bg` | 16.59:1 |
| `.t-cmd` | `--fg` w700 | 17.28:1 | `--bg` w700 | 16.59:1 |
| `.t-str` | `--accent-ink` | 4.66:1 | `--accent` | 6.06:1 |
| `.t-num` | `--fg` tabular | 17.28:1 | `--bg` tabular | 16.59:1 |
| `.t-dim` | `--muted` | 4.82:1 | `--rule` | 9.22:1 |

A code block's only container is a rule above and below it. It never uses a
coloured side border, which the craft floor refuses and the poster's own
language does not use.

**`emb-top` is a full-width band.** Its capture's longest line is ~100
characters; no 640px column holds that without either cutting the output or
wrapping the heatmap bars mid-run. The panel is the dashboard's real render
from `README.md` with rows omitted, and it is labelled `Sample run`. Below
1001px the capture wraps (`pre-wrap`) rather than scrolling sideways, so the
region never introduces horizontal scroll — but it wraps **only between
fields**: each field is a `.tf` span, `white-space: nowrap`, so `280 r/s` can
never arrive as `280` / `r/s`, and the ASCII histograms — a second encoding of
the `r/s` number printed beside them — are dropped where they are what forces
the wrap. Every character of the render is unchanged at every width.

A specimen's only container is a rule above and below it, and the specimen is
a `<figure>`: `figure{ margin: 0 }` is in the base reset, because the UA's
40px side margin silently inset every code block on the page (see pass 11).

**The wordmark** is an inline SVG with three optical outlines in a
1086 × 480 viewBox. At the reference width, the `b` tower starts at y87,
the x-height at y218, and the bowls finish at y567. The `e` terminal, `m`
arches and `b` counter follow the supplied poster. A restrained SVG noise
filter provides the ink texture. The counter caption
is decorative and is omitted below 1000px when it becomes too small.

**Motion** — things move because data is moving. The hero entrance is one
authored sequence: the masthead, the annotations, the wordmark and then the
prose and actions land 90ms apart, settling at 780ms while the spine finishes
its 850ms draw at 1000ms. The four slabs (INPUT → INFERENCE → EMBEDDINGS →
SERVE) then activate in name-keyed order when the pipeline crosses 0.85
viewport heights, and the ridge route is drawn from `stroke-dashoffset` as the
terrain rises through the viewport. The
terrain is now the last section before the footer, so the route's progress
denominator (`rect.height * 0.86`) is reached by the page's own end rather
than mid-scroll: at maximum scroll `vh - rect.top` is the band's height plus
the footer's, which clamps the trunk to fully drawn. Reveals are driven by
element position rather than `IntersectionObserver`
intersection, so an anchor jump or a fast flick can never leave content
invisible; a `ResizeObserver` on the root covers the rest of what can move the
trigger line under an element — zoom, rotation, a late font or image — and a
`@media print` block collapses the whole cascade to its end state, because a
print pass never scrolls.

**The one case that is not covered is a capture that neither scrolls nor
resizes.** A `captureBeyondViewport` full-page screenshot lays the document
out without moving the trigger line, so every reveal below the fold is still
at `opacity: 0` and the capture silently omits it — measured: 10 of 20
`[data-reveal]` elements hidden on a fresh load at 390px. A print pass is
safe (`@media print` collapses the cascade); a screenshot is not. `just
website-shot` therefore scrolls the page to the end and back before it
captures, and any hand-rolled capture must do the same. This is the accurate
version of the claim the earlier passes made too broadly.

Groups ripple instead of flipping as a block: each feature, stage and block
entry carries its list position in `--i`, and the slab stagger is keyed by
plate name, so a shared property can never make a hover wait out another
element's delay. Each block ripples on its own, so one block's entrance never
waits out another's.
`prefers-reduced-motion` is honoured throughout.

**Responsive** — the two-column spread holds down to 1000px; below that the
prose, the pipeline and the terrain stack. From 641px to 1000px the four stage
notes stay beside the plates they describe (each note's top is a percentage of
the stack's own height, so it is centred on its plate at every width), and the
blocks put their copy in the content column with the lane as the left gutter.
Below 640px they become a ruled list directly under the stack, bound to the
diagram by the same `01–04` numbers the plates carry: four annotated layers
cannot share a 350px measure at the 14px floor, so the list is the honest shape
for that width. Phones keep the isometric stack — it is the page's whole
argument.

Two rows are re-shaped at the same breakpoint, because at 236–299px their
wide form does not fit at the 14px floor: `.shift__row` puts
`model(fn(input))` on its own line with the arrow, output and tag below it
(the arrow used to sit 0px off the closing paren at 390px), and the `.uses`
ledger — `gliner2  span extraction` needs 23 characters where two 150px
columns hold 17 — goes to one entry per row.

**The spine is responsive in three states, and `--fold-n` and `--art-w` move
together in each one** — change one without the others and the line and the
route come apart:

| Range | `--fold` | What the spine does |
|---|---|---|
| ≥1001px | 65.31% | an explicit lane: `.block__grid`'s split *is* the lane, so copy clears it; the console and emb-top plates cover it |
| 641–1000px | 25% | the lane is the **left** gutter — the stacked pipeline's plate centre — so the blocks and the emb-top band take the column right of it; the massif runs the full shell with the ridge shifted `-537.9` units onto that lane |
| ≤640px | 90% | the rail moves to the right margin, because a 390px box cannot give both a centre line and a readable measure; everything is held to its left, the plates' own spine is hidden, and the rail threads the plates |

Three measured corrections are baked in. The hero's CSS spine runs the shell's
full height behind the column from the masthead's rule to the first block:
confining it to a row of its own began it *below* the stage notes and left a
390px hole after the last plate (phones) or whenever the notes outgrew the
plates (tablets). The stack's paper covers the line behind the plates, so the
masked SVG spine still draws the gaps rather than the CSS one filling them in.
The masthead carries the same line from the very top of the page, and every
spine segment, the SVG spine and the route render at exactly `--spine-w` — a
`%` width resolved against different boxes (and, for the non-scaling SVG
strokes, against the viewport diagonal), so the breakpoint values are lengths
derived from the content box.

And **on phones the rail moves to the right margin** (`--fold: 90%`), with the
stage notes, the block grids, the header bars and the emb-top panel all held to
its left:

```css
width: calc(var(--fold) - var(--spine-w) / 2 - var(--fold-gap));
```

At the plate centre a 390px content box gives 175px per side, which left the
notes about **16 characters** — readable in theory and not in the hand. On a
right rail the same content measures **299px** (≈38 characters) and the rail is
still one line. Measured at 390px: notes and block grids 299px, all of them
ending **16px** short of the rail, **zero** text boxes on a visible rail, and no
horizontal scroll; at 320px the rail is at 272 and the notes are 236px.

Two things follow, and both are deliberate:

- **The diagrams's own spine is hidden on phones** (`.pipeline__svg .sig`). There
  is only room for one line: two orange lines 40% apart, both visible in the
  gaps between plates, is not a composition. The rail passes *behind* the plates
  instead — the stack's paper knockout is dropped at this width so the line
  threads them exactly as it does on desktop, visible in the gaps and hidden
  where a diamond covers it.
- **The massif is cropped.** The route's fork is fixed at 51.76% of the artwork,
  so putting it on a right rail necessarily pushes the massif's right slope
  off-frame (`--art-w` grows to `156vw` to keep the left edge at the content
  edge). The summit now sits under the rail, which is where the line arrives —
  the handover is exact (rail 334.98, fork 334.99 at 390px).

Gutters survive a notch: `--pad-l` / `--pad-r` fold in
`env(safe-area-inset-*)`, and everything that cancels a gutter to reach the
sheet's edge cancels those instead.

**The footer is dark** — the same `#111110` as the console, the emb-top
capture and the scripts block, so the page closes on the plate's own material
rather than putting a paper strip back after the photograph. `--muted` is
remapped to `--rule` there for the same reason it is in the dark block
(`#6B6963` is only 3.40:1 on `#111110`); measured, the copy is **9.22:1** and
the `BYTES IN. VECTORS OUT. / EMB` mark is **16.59:1**.

**Accessibility** — real `<header>`, `<nav>`, `<main>`, `<section>`, `<h1>`,
`<h2>`, `<h3>`, `<footer>`; the giant `emb` is `aria-hidden` decoration and
the semantic `<h1>` is the value proposition. The pipeline is announced by a
visually hidden `<h2>` that the four stage `<h3>`s hang off (the notes carry
`role="list"` so VoiceOver and Safari keep the list semantics that
`list-style: none` removes), and the terrain is decorative. Focus is a 2px
`--accent-ink` ring at 2px offset — 4.66:1 against the paper, where the
surface accent is only 2.74:1. The note ⇄ plate emphasis is decoration that
no information depends on: the stage number and label are always visible, and
it answers to hover, to tap and to a second tap elsewhere, so touch is not
left out. Every rendered control is at least 44px tall at 320–834px.

## Assets

- Fonts are self-hosted WOFF2 subsets (latin) from Google Fonts.
- `terrain-v2.png` was made with the built-in image generation tool using the
  supplied poster as a visual reference. Prompts and provenance are recorded
  in [`assets/img/terrain-v2.md`](assets/img/terrain-v2.md). It is kept as the
  source for `terrain-matte.png`, which is what the page loads.
- `terrain-v2.png` and `terrain-v2.md` are authoring sources, not assets: they
  are excluded from the deployed tree by `.assetsignore`. See
  [What ships](#what-ships).
