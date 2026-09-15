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
│   ├── css/asciinema-player-3.17.0.css
│   │                       the player's own sheet, verbatim (see below)
│   ├── js/main.js          entrance sequencing, reveals, pipeline, route,
│   │                       and the console's REPL (the landing only)
│   ├── js/topviz.js        the emb-top plate, written once and loaded by
│   │                       both surfaces: the landing plays it on its own,
│   │                       /docs holds the frame until the reader asks
│   ├── js/asciinema-player-3.17.0.min.js
│   │                       the player, verbatim: the landing plate replays
│   │                       the recorded take with it, from this origin — no
│   │                       CDN, and no build step to re-derive it. Both files
│   │                       come from the release tarball pinned in
│   │                       `flake.nix` and are written by `just website-player`
│   │                       (sha256 a13c3763… for the script, f619fe17… for
│   │                       the sheet); `just website-player-check` compares
│   │                       what is committed against that tarball
│   ├── cast/               the recorded take the plate plays, named from its
│   │                       own bytes by `just website-topviz`. ~850 KiB of
│   │                       asciicast that compresses to ~16 KiB, which is why
│   │                       `_headers` declares its content type
│   ├── fonts/              self-hosted Archivo, Inter and JetBrains Mono
│   └── img/
│       ├── terrain-matte.png  the cut-out the page ships (2172×724, alpha)
│       ├── terrain-v2.png  generated rock formation, the matte's source
│       ├── terrain-v2.md   generation prompts and provenance
│       ├── speckle.svg     original photocopy speckle asset
│       └── og.png          1200×630 social card (source: tools/og-card.html,
│                           a locked brand composition — never a page screenshot,
│                           so landing changes cannot rot it)
├── tools/
│   ├── og-card.html      the locked source of assets/img/og.png: the
│   │                     masthead, and the massif with the spine dripping
│   │                     into its fork (no middle band — the artwork takes
│   │                     the rest of the card). Regenerate at viewport
│   │                     1200x630 after `just website` (see the file
│   │                     header), capture exactly 1200×630
│   ├── gen-isometric.py    regenerates the pipeline SVG
│   ├── gen-terrain-matte.py derives the terrain cut-out from terrain-v2.png
│   ├── png_lib.py          dependency-free PNG reader/writer for the above
│   ├── ink-probe.html      asserts no text ink crosses the viewport, 24 widths
│   ├── published-tree.py   what ships, the canonical origin, the cache rules
│   ├── stamp-version.py    writes VERSION into every `data-emb-version` element
│   ├── stamp-presets.py    writes the preset SHA1s into `data-emb-preset-*`
│   ├── dev-server.py       serves this tree with the console's module origin
│   │                       pointed at a local bridge (`just website-dev`)
│   ├── topviz/             the rig behind the emb-top plate: one command
│   │                       records a real dashboard and publishes three
│   │                       things from that one take — the take itself into
│   │                       the plate, where the vendored player replays it;
│   │                       the dashboard's own text frame into the same
│   │                       plate, as the still state a reader with scripting
│   │                       off or a reduced-motion preference keeps; and an
│   │                       animated capture into `docs/assets/` for
│   │                       `docs/operations.md`
│   │                       (`just website-topviz`, see its README). Its
│   │                       `runs/` hold the last take's recordings, frame and
│   │                       logs — gitignored, and the only way to check what
│   │                       the plate shows
├── repl/                   the sandbox: NOT the site (see § the sandbox)
│   ├── *.go                the bridge — the sandbox's only public surface
│   ├── terminal.js         the one client module; both surfaces load it
│   ├── index.html          the standalone terminal served at cli.emb.is/
│   ├── presets/*.lua       the preloaded scripts, called by digest
│   ├── sandbox.yaml        the server config the bridge reads for its manifest
│   ├── Dockerfile, run.sh, fly.toml   the one-machine deployment
│   └── (all excluded by `.assetsignore`, asserted by published-tree.py)
└── README.md
```

This list is two populations, and the difference is not visible in `ls`. The
pages and `assets/` ship; `tools/`, `repl/`, this README, `PRODUCT.md`, the
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
and its `Docs` navigation entry resolve to `/docs`; `GitHub` and the benchmark's
`reproduce` are the only links that leave the site.

Both surfaces render the same masthead and carry the same black/cream block
rhythm — a full-bleed `.block` per section with a solid `.block__head` bar. The
docs surface has no hero, so it takes the ground and the bar and none of the
spine or `--fold` geometry.

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
just website-player-check             # the vendored player is the pinned release
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

The same idea covers the sandbox's preset digests: `just website-presets`
stamps `data-emb-preset-*` from the SHA1 of `website/repl/presets/*.lua`, and
`just website-presets-check` fails when a preset byte changes under a stamped
digest. `ci.yml` and `site.yml` both run the check.

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
| **Ignored** | `tools/`, `repl/`, `.impeccable/`, `PRODUCT.md`, `README.md`, `terrain-v2.png`, `terrain-v2.md` |
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

A deploy is not reported successful until the origin confirms it: the deploy job
requests the landing, `/docs/`, and a path the site does not serve, and diffs the
served bytes against the tree that was just deployed. The not-found probe is what
holds `assets.not_found_handling: "404-page"` in place — a 404 status alone would
pass against a platform default, but only the configured handler answers the miss
with the committed `404.html`.

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

## The sandbox

The console and the standalone terminal are driven by a real `emb` process — a
small Go bridge under `website/repl/` that forwards an allowlisted command
surface to a server bound to loopback on the same Fly machine. The bridge is
the only public surface, so no credential reaches a browser and a
misconfigured public route cannot reach the server.

```
browser ──HTTPS──▶ bridge (cli.emb.is) ──RESP on 127.0.0.1──▶ emb ──▶ /data/models
```

**The boundary.** `website/repl/` is a program that happens to live under the
site directory. It is excluded from the published tree by a `/repl/` entry in
`.assetsignore`, and `published-tree.py` asserts that every file under it was
excluded — so a deleted ignore line fails `just website-published` instead of
publishing service source or presets. CI classifies it as code: a change under
`website/repl/` runs the bridge's Go tests even though the rest of `website/`
would skip the server and gem jobs.

The local loop needs the site, the bridge and a server, and the bridge reads
the server's config to learn the preset digests it will accept. One command
starts all three and points the console at the local bridge:

```bash
just website-dev        # site :8080, bridge :8081, emb :6379 — Ctrl-C stops all three
```

Both the site and the bridge bind `0.0.0.0`, so a phone on the same network
reaches the same live console at `http://<your-lan-ip>:8080`. `emb` stays on
loopback. Pass `bind=127.0.0.1` to keep the loop to this machine.

`website/tools/dev-server.py` is what makes that possible: it serves this tree
with exactly one substitution in HTML responses, the console's module origin
(`https://cli.emb.is` → this machine, derived from the request's own Host). The
published page keeps a single source, and the page under test differs from
production in that one respect. `just website` still serves the tree
untouched — use it for the ink probe and anything else that must measure what
ships.

The pieces are also runnable on their own:

```bash
just website                                               # the static tree
just dev                                                   # the server alone
just sandbox-run config=website/repl/sandbox.yaml          # the bridge alone
just sandbox-test                                          # the bridge's tests
```

**One client, two surfaces.** `website/repl/terminal.js` is the canonical
client: a quote-aware tokenizer, the `POST /api/exec {args, proto}` request, the
reply renderer for every envelope kind, the recall over submitted commands, and
the idle / running / result / error / starting / offline states. The sandbox
serves it at `/terminal.js`, its standalone terminal page loads it, and the
landing page loads the same file from `cli.emb.is`. Nothing renders a reply
twice, so a change to the contract cannot land on one surface only. The landing
page presents no host or endpoint: the module captures the origin it was served
from.

**`cli.emb.is/` is the terminal and nothing else.** The standalone page is the
viewport: `100dvh`, one column, three bands — what the terminal says about
itself and the commands it takes, the scrolling transcript, the prompt as the
last line — with no heading, card, status strip, footer or page scroll. The
model is `redis.io/cli`: the disclosure is written in the same monospace voice as
the replies instead of in a paragraph above them, and the state is expressed by
the transcript itself (`offline — …` and its retry) rather than by a second
indicator in a bar.

Two things differ from that model on purpose. The **samples stay pinned** above
the transcript, because they are the only clickable thing on the page and a
reader who has just run one command should not have to find the list again. And
the **`SANDBOX · MAY RESET` line is permanent** — it sits above the samples
rather than scrolling away in a banner, because it is the one thing on the page
that has to stay true for as long as the page is open.

Above 760px the samples are two columns of one line; below it they are one
column, the note drops out of the drawn row and stays as the control's name, and
every control a finger has to hit keeps the 44px floor.

**The preset digests are checked.** A preset is called by the SHA1 of the bytes
the server preloaded, so the digest the site shows must be that value:

```bash
just website-presets          # stamp website/repl/presets/*.lua into index.html
just website-presets-check    # fail if a preset byte changed under a digest
```

`stamp-presets.py` also asserts that `website/repl/sandbox.yaml` preloads each
preset, so the site cannot name a digest the server was never told to load.
Both `ci.yml` and `site.yml` run the check.

**Nothing here is a hosted offering.** The machine stays running and the
sandbox may reset; it refuses `CONFIG`, `AUTH`, `MONITOR`, `EMB.SAVE`,
`EMB.CACHE.FLUSH`, `EMB.SCRIPT *`, and `EMB.IMG*` at the bridge, and bounds
what a visitor can spend with per-client and global rate limits, a concurrency
cap, request and text caps, and a rolling work ceiling. See
[`repl/`](repl/) and `PRODUCT.md`.

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
behind the column from just under the masthead rule to the first block.
The spine never enters the masthead. Every block and the terrain carry a
`.spine` segment. Sections have **no
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
mono ladder, never cards), and dark plates. The docs surface reuses the same
atoms — one `.block` per section, alternating paper and dark — so the two
surfaces beat alike. It carries no spine, so it takes the ground and the bar
without the lane.

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

**The dark block is the landing's one inversion, and the docs surface's
repeated one, made of material already on the page** — the SERVE plate's own
`#111110` and `#292823`, lit by the spine crossing it in `--accent` at
**6.06:1**. There is no new palette here, only
this one turned over. Its `--muted` is remapped to `--rule`, because
`#6B6963` measures only **3.40:1** on `#111110`.

**Direction contract (console REPL pass).** The console is a *terminal*, not a
*demonstrator*. One prompt, one transcript, one status strip, and a menu that
stays: no mode selector and no control whose only purpose is to change what the
transcript is about. The menu is seven commands the sandbox permits, rendered as
ruled numbered rows, each one click to run, always on screen — so the panel
teaches by being runnable instead of by carrying a hint sentence, and it stays
worth clicking after the first click. A
command that is run is a command that is in history, whether it was typed or
chosen, and `Enter` and `↑`/`↓` are the whole interaction. The one choice left
(`RESP 2|3`) is a real one the spec requires to stay observable, and it stays a
native `<select>` because a segmented control would cost new markup, new state
and new focus rules to look right in a two-item strip. Nothing here introduces a
colour, an atom or a motion that the plate did not already have: the status strip
is the bar's own bar, the example rows are the poster's ruled numbered entry, and
the `↵` is the `RUN` button with its label removed rather than a control deleted.
Target: a first-time reader holds a reply after one click and a second one after
one keypress, and a keyboard reader tabs into six operable commands rather than a
dead box.

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

**It runs a live executor.** The panel is rendered, not withheld: the executor
is `window.embTerminal`, the client module the sandbox serves at
`cli.emb.is/terminal.js`, and `main.js` drives the markup with it. The panel
never fabricates a reply — if the module cannot be loaded, or the sandbox
cannot be reached, the panel states that condition and offers a retry. There
is no transcript behind the seam.

**One client, two surfaces.** The same module drives the sandbox's own
standalone terminal at `cli.emb.is/`, so a command and its reply render the
same way on both — and so does the panel around them: the same prompt, the same
example rows, the same status strip, and the same recall over submitted
commands, because the history lives in the module rather than in either page. The module owns the tokenizer (quote-aware), the request
(`POST /api/exec {args, proto}`), the reply renderer for every envelope kind,
and the idle / running / result / error / starting / offline states; the page
owns only the DOM. Which host the module came from is captured from its own
`src`, so no page hardcodes the sandbox address in its copy — the console
presents no endpoint, host, or hosted-service affordance.

**The badge says what it is.** `SANDBOX · MAY RESET`, and the note under the
panel says the same in a sentence: a real `emb` process, a sandbox that may
reset, refusing anything that would change its configuration or shared state.
There is no pricing, account, uptime, or support affordance.

**The console is a REPL with a menu.** There is no mode selector: the two
special functions the panel exists to show are two rows in a list of six
commands, each one click to run, each one a real submission that lands in the
history the arrow keys walk. The rows are built from the two digests the section
carries, so the console cannot offer a command the sandbox refuses.

The menu is part of the panel, not of a state. It sits *above* the transcript and
nothing replaces it, so the reader who has just run one command can run the next
one without reloading the page — which is the only way a menu of commands
earns its place. Above rather than below because of what is on screen at rest:
under the transcript an untouched console shows a tall empty field between the
strip and the menu, and over it the row below the strip is the menu and the empty
space is the output area directly above the prompt.

**The reply forms are two rows, not a setting.** `EMB` shows the bytes and
`EMB … VALUES` shows the envelope, `EMB.MULTI` answers several models in one
call, and one row calls a preloaded preset by its digest for a labelled,
non-embedding reply. There is no protocol selector: the flat and the typed
forms are both reachable from the menu, so there is nothing to set before either
can be read. `terminal.js` still carries `proto` in its request — that is the
module's contract with the bridge, not the panel's furniture.

**The strip names the console's own condition**, and it is now only that: the
state on the left, `SANDBOX · MAY RESET` on the right. Its colour is read from
one `data-state` attribute rather than from a second list that can disagree with
the label — `--rule` while idle and `--bg` once there is something to read,
`--accent` while the sandbox is working, waking or gone, with the indicator
pulsing only in the two states that are actually in progress.

**The transcript is a window, not a page.** `.console__screen` has a floor and a
ceiling and scrolls between them, and `main.js` follows the newest line only when
the reader was already at the end — so a long reply neither grows the panel nor
interrupts a reader who has scrolled back through one. Whether to follow is read
*before* the transcript grows: read after, a batch of lines arrives with the
panel already past its own threshold and the reader is left at the top of output
they never saw. At 1440px the panel is **467px** at rest and never past **600px**,
against 467px and unbounded before this pass.

**A menu row is a label, not the command.** Where a command and its note cannot
share a row, the drawn label elides the digest — `EMB.EVSHA sst2 51ae48b3… 1 …` —
while the button submits the command in full and keeps it in full in its
accessible name. A 40-character SHA spent in a menu is a row that wraps for no
reader's benefit, and the digest is printed in full the moment the command runs.
Below 834px the rows are one column of one line: the note is dropped from the
drawn row and kept as the button's name, and the badge is dropped from the strip,
where the note under the panel says the same sentence in full.

**Enter submits, and the arrow keys recall.** Recall stops at both ends rather
than wrapping, holds the line being typed for the whole walk, and is
feature-detected, because the module is served from the sandbox's own origin
and a page can be newer than the client it loads. `RUN` survives as a `↵`
beside the prompt: it is not a second way to submit so much as the touch target
a soft keyboard needs.

**The digests are derived, not typed.** `just website-presets` stamps
`data-emb-preset-embed` and `data-emb-preset-classify` from the SHA1 of
`website/repl/presets/*.lua` — the same `sha1(bytes)` the server computes for
the script it preloaded — and `--check` fails when a preset byte changes under
a stamped digest. Without that, editing a preset would leave the site naming a
digest the sandbox answers `no such script` to.

The panel paints its examples before the live region is armed, so loading the
page does not announce a list of commands. Playback of replies is line-by-line
rather than per character, and `prefers-reduced-motion` collapses it to one
frame. Without JavaScript the form is hidden and a labelled `<noscript>`
specimen stands in, so the section is never empty.

Every control clears the 44px target floor at 320–834px. The smallest text in the region is
**12px at 1086** and **14px at 390**, which is the committed floor.

**Code is typeset as code.** Every specimen — the shell invocation, the Lua
source, the `model(fn(input))` shift, and the `<noscript>` console specimen —
carries four token classes, marked up by hand. Replies in the live console are
plain text, because they are data rather than a specimen: the transcript-era
pattern highlighter is gone with the transcript. Emphasis is weight and
colour-role, never a second hue — the page has one accent and this does not
spend it twice. Measured on both grounds:

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

**The same take is on `/docs`, and it does not move until asked.** The
documentation surface's plate is a figure in the reference measure rather than
a band, and it is written by the same `publish.py` from the same run: the frame,
the cast URL, the caption and the figures are stamped by data attribute on both
pages, so the two cannot describe different recordings.

The one real difference is declared on the mount. The landing page's plate
autoplays and loops because it is the section's argument; the reference page's
player is created at once and **held** — `autoplay: false` at the run's poster
frame — so the plate shows the recording drawn in the dashboard's own colours,
stopped at zero, and starts only when the reader asks. That is the difference
between a page arguing and a page explaining, and it is why the documentation
plate is the player rather than the `<pre>` beside it: the frame is
`asciinema convert -f txt` output and carries no ANSI at all, so resting on it
would draw the dashboard in one flat colour while the GIF in
`docs/operations.md` showed the same run in its real palette. The frame is what a
reader the player cannot reach keeps, not this surface's picture of the run.

**The held plate carries no control of its own.** The player draws a start
overlay over a poster frame, so the `PLAY THE RECORDING` button an earlier
revision added was a second control, in a second place, for the same action —
deleted with its markup and its rules. The plate's one control is the player's
own transport, which is also the pause the motion contract promises.

That retires this file's earlier rule that `/docs` ships no JavaScript. The page
now links the vendored player and one small module (`assets/js/topviz.js`, the
same file the landing loads), and its no-scripting state is the whole plate minus
the picture: the frame, the caption and the run's figures are all static markup,
and there is no control to be left inert, because the control is the player's
own. The plate's `restore()` path — a take that will not load — puts the frame
back and hides the emptied mount. At widths where the plate is not drawn the
player is never built and the recording is never fetched; the run's figures
stand in, as they do on the landing.

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
full height behind the column from just under the masthead rule to the first
block: confining it to a row of its own began it *below* the stage notes and left a
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
