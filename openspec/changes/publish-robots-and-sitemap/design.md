# Design

## Context

- `website/` is both the working tree and the publish source: `wrangler.jsonc`
  deploys it as an assets-only Worker at `emb.is`, and `.assetsignore` is the
  only boundary between an authoring file and a public URL.
- There is **no build step**, and `site-deployment` requires the served tree to
  render without the deploy tooling. Anything generated is therefore generated
  *into the tree* and committed, and the gate is a `--check` mode — the pattern
  `stamp-version.py`, `stamp-presets.py` and `stamp-examples.py` already use.
- The repeated facts, measured: twenty served pages each carry their own
  canonical, `og:url`, `og:image`, `theme-color`, title, description and favicon
  data URI; sixteen share one byte-identical tag sequence (22 tags), `docs` is
  25, `index` and `scripting` 24, `gem` 22, `404` 11. The favicon's data URI
  repeats the paper colour `#F3F0E8`, which appears 61 times in the tree.
- `published-tree.py` owns the served set, the canonical origin, the cache
  rules, the internal links and the takes' agreement. It can assert that the
  copies agree; nothing makes them come from one place.
- `impeccable` (v4.5.2) is available at
  `~/.pi/agent/skills/impeccable/scripts/impeccable`: `detect` scans for UI
  anti-patterns, `ignores` manages per-project suppressions, and the skill's
  `reference/craft-floor.md` states the controlling rule — *a pinned brief or
  the committed visual world overrides the floor*, which is why the audit is
  currently unusable as a gate.
- `product-site` pins the 12px desktop / 14px mobile floor, the dark console
  ground, ground variation across the three capability blocks, and the banned
  devices. It does **not** name the paper colour or the body face; `DESIGN.md`
  does, and seven `.impeccable/surfaces/*.md` briefs repeat them.

## Goals / Non-Goals

**Goals:**

- Every fact the site states about itself exists once and is emitted everywhere
  it is needed: page metadata, the sitemap, the crawl directives' reference, the
  palette tokens, the favicon.
- Both a missing page and a drifted one fail a check before a reader or a
  crawler can see them, with no build step and no tooling at request time.
- `impeccable detect website` reports zero warnings, so it becomes a gate that
  can be trusted instead of a report that argues with the world.
- The world the detector measures and the world the repository documents are the
  same world — `DESIGN.md`, the briefs and the spec agree with the CSS.

**Non-Goals:**

- The advisory `em-dash-overuse` notes. The tool's own contract excludes them
  from the failure count, and the house voice is deliberate.
- Any server, protocol, gem, ONNX, or benchmark surface.
- A build step, a bundler, a CSS preprocessor, or a new runtime dependency.
- Replacing the composition: the atoms, the banned devices, the wordmark, the
  claim, the feature list and the pipeline artwork stay as they are.

## Decisions

**D1 — One descriptor file, `website/tools/pages.json`, is the site's identity.**
It carries the site block (origin, social image, theme colour, favicon geometry
and colours) and one entry per page (file, request path, title, description,
indexable). Its palette tokens are the same values the CSS uses.

The descriptor is JSON rather than YAML deliberately: the `site` jobs in
`ci.yml` and `site.yml` install nothing — they run these tools with the runner's
own `python3` and its standard library — so a commented YAML file would buy
readability at the price of a dependency those jobs deliberately do not have.
What the schema means is documented in `stamp-pages.py` and
`website/README.md` instead of in comments beside the values.

Alternatives considered: a Python literal inside the tool (rejected: the values
are content, and the tool would then be the only place a maintainer could read
them); keeping the literals in the pages and asserting agreement (rejected: that
is today's arrangement, and it is what produced twenty copies).

**D2 — `website/tools/stamp-pages.py` emits, in place, and `--check` guards.**
From the descriptor it writes: (a) each page's metadata fragment, spliced
between `<!-- meta:start -->` / `<!-- meta:end -->` markers immediately inside
`<head>`, leaving the per-surface preloads, stylesheets and scripts after the
fragment untouched; (b) `website/sitemap.xml`; (c) the `:root` token block in
`assets/css/styles.css`, between markers. `--check` rewrites nothing and exits 1
naming every file that drifted.

Alternatives considered: a `just`-invoked build that renders pages at deploy
time (rejected: it breaks "renderable without the deploy tooling" and would put
the tool between a reader and a byte); a runtime-injected head via JavaScript
(rejected: crawlers, `og:` scrapers and no-JS readers must see the metadata in
the HTML).

**D3 — The same descriptor decides the sitemap, so the crawl surface cannot
drift from the pages.** `sitemap.xml` lists the canonical URL of every page
marked indexable, and `published-tree.py` keeps asserting that the emitted
`<loc>` set equals the canonical set of the served pages — now checking a
generated file against generated heads, which is the strongest form of the
assertion the change already had. The `robots.txt` reference and the served-set
membership are asserted as they are today.

**D4 — The paper ground moves to a cool neutral, and the tokens carry the
contrast.** `#F3F0E8` (warm cream, hue ≈41°) becomes a cool paper at the same
lightness band, and the warm cast leaves the favicon data URI, the terrain matte
and the social card, all of which are regenerated from the tools that own them
(`tools/gen-terrain-matte.py`, `tools/og-card.html` → `assets/img/og.png`).
Because the tokens now have one source, `stamp-pages.py` also computes WCAG
contrast for every text/ground pair and fails below AA (4.5:1 body, 3:1 large) —
which is what makes the new `product-site` requirement checkable rather than
declared.

Alternative considered: inverting the whole site to the dark ground (rejected:
the poster's paper/dark alternation is compositional, and the spec's dark ground
belongs to the console block).

**D5 — `Inter` leaves; the body face is a self-hosted, not-overused grotesque,
and the display voice stays `Archivo`.** `Inter` is the one face the detector
names (22 findings, one per page plus the stylesheet) and it currently leads
`--display` as well as `--font`; `Archivo` is already vendored as a variable
subset and is not flagged, so `--display` becomes `Archivo` and the 900-weight
`inter-900-latin.woff2` leaves the tree, its preloads and its `SERVED` entry.
The replacement body face is vendored as one latin WOFF2 from a pinned,
versioned source (the precedent is the asciinema player, pinned in `flake.nix`
and copied in by a tool), committed, and recorded in `website/README.md` with
its source and version — a font subset is versioned by its file, which is why
`_headers` may pin it immutable.

Alternative considered: subsetting a system font with `fonttools` (rejected:
`fonttools` is not in the dev shell, and the repository's own rule is that font
subsets ship as files whose names carry their version).

`Inter` leaves entirely, including from the masthead wordmark, and that is a
measurement rather than a preference: the detector fires `overused-font` on any
element whose first `font-family` is an overused face, whatever it is called and
however little of the page it covers (a 9px `.tiny` rule is enough; an unused
`@font-face` is not). Keeping Inter for the mark therefore puts the change's
target out of reach — a faithful copy of the tree with the mark on Inter reports
23 findings against 22 before it. Renaming the family to hide the face would
pass the detector while shipping the same bytes, which is the evasion the rule
exists to catch, so it is refused along with suppression.

The wordmark keeps its shape, size, place and 900 weight and is re-set in
`Archivo` 900, the display voice the rest of the page already uses. `product-site`
pins the wordmark, claim, feature list and pipeline artwork as unchanged, and
this is read as composition — nothing about the mark's form or position moves;
its face follows the world's display face, which is what "the face leaves the
tree" costs. It is a four-reference revert if that reading is ever rejected, and
the mark is captured at both widths so the change is visible rather than argued.

**D6 — The 154 `cramped-padding` findings are repaired in CSS, not by
suppressing the rule.** The rule has two shapes: text too close to its own
container's box, and children landing flush against a visible boundary. Both are
repaired by insetting the component (block grounds, ruled ledgers, `code`
panels, `plate__head`, `rig__pick`) in `assets/css/styles.css`, `docs.css` and
the plates' inline styles, keeping the composition full-bleed *between* sections
while the content inside a ground gains its margin.

**D7 — The type findings are repaired against the site's own floor.** The 10px
and 10.5px functional labels in `demos/dns.html` and `demos/laya.html` rise to
12px (the committed desktop floor); the three `all-caps-body` runs lose
`text-transform` or lose their length; the three `tight-leading` elements reach
`line-height: 1.3`; and the two `flat-type-hierarchy` surfaces gain real steps
(`index`: body 14 / h3 15 / h2 16 becomes a scale with ≥1.15 adjacent ratios;
`medium.html`: every role at 12px becomes a ladder). The `<img>` with no `src`
on `demos/image.html` is fixed or removed.

**D8 — `DESIGN.md`, the seven briefs and `website/README.md` are updated in the
same change as the CSS.** They are the world's records: a brief that says "no
new colour, font" would forbid this change, and a `DESIGN.md` that names cream
and Inter would document a world that no longer ships. Leaving them stale would
make the repository disagree with itself — which is the defect this change
exists to remove.

**D9 — The detector becomes a gate, not a report.** Verification is `impeccable
detect website` reporting zero warnings (advisories excepted), run by hand in
this change and recorded in it: the detector is not a CI dependency, because it
lives outside the repository and would put a network-installed tool on the path
of every site change. `just website-ink` remains the repository's own geometry
gate and must still PASS at all 24 widths after every inset and type change.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Generated heads make a page unreadable in raw form | The fragment sits between visible markers, values are literal text, and the page still renders with no tooling; only edits inside the markers are overwritten, and `--check` says so before a deploy |
| A palette change ripples into assets that are drawn from it (favicon, terrain matte, `og.png`) | All three are regenerated from the tools that own them in the same change; `published-tree.py` already refuses a served file that is not named, so a forgotten asset cannot ship silently |
| New font file changes metrics and can reintroduce ink overflow | `just website-ink` at all 24 widths is the check, and the surfaces are captured at desktop and mobile before the change is called done |
| Insets and a wider type scale can break the poster's composition | The atoms and bans are unchanged; captures at desktop and mobile are part of verification, and the terrain section ordering and grounds stay as the spec requires |
| A vendored face is a new asset whose provenance is a URL | Pinned, versioned source recorded in `website/README.md` beside the tool that vendors it, with the file's bytes committed |
| The detector is not in CI, so a future change can drift back into warnings | The change records the command and its verdict in the repository (`website/README.md`), and the repairs it makes are structural (one source per fact) rather than cosmetic |

## Migration Plan

1. Land the generation first: `pages.json`, `stamp-pages.py`, the markers in the
   twenty heads, the generated `sitemap.xml`, and the `just`/CI wiring, with
   `published-tree.py`'s crawl assertions pointed at the emitted files.
   `--check` must be clean and `just website-published` must exit 0 before any
   visual work starts, because every later step edits what this emits.
2. Move the world: tokens (paper, type scale, leading), the favicon and the
   regenerated assets, then the body face and the removal of `Inter`; re-run ink
   at 24 widths after the type work, not before.
3. Repair the detector findings in CSS order — insets, sub-floor type, caps,
   leading, hierarchy — and the `<img>` defect.
4. Update the records: `DESIGN.md`, the seven briefs, `website/README.md`,
   `product-site`'s new floor requirement; run the detector and record zero
   warnings. The two `emb-top` captures in `docs/assets/` are re-rendered from
   their committed casts with the pinned `agg` (deterministic: two renders of a
   cast are byte-identical), and the file names carry the new content digest with
   their references in `docs/operations.md` moved with them. Two deltas ride on
   that re-render, and both are recorded rather than hidden: the theme entries,
   and a width change 881→879px, which is **not** the palette — `agg` resolves
   its default font-family through the host's fontconfig, so a render's glyph
   advances depend on the machine's fonts, and the same committed cast rendered
   with the *old* theme here is also 879 wide and not byte-identical to the
   committed file. The committed takes therefore predate this font environment;
   what ships is what the rig would write today, not a byte-reproduction of the
   old bytes. The `stressviz` capture is not re-rendered: it has no committed
   cast, and refreshing it is a re-measurement (see Open Questions).
5. Publish: merge to `main`; `site.yml` runs the checks and deploys. Confirm
   against the origin: `/robots.txt` `200 text/plain`, `/sitemap.xml` `200` XML,
   and the readiness scan reporting `robotsTxt` and `sitemap` `pass`.
6. Rollback: revert the merge. The tree reverts as a whole — generated files are
   committed, so the previous revision is internally consistent; nothing else
   depends on the new files.

## Open Questions

- **The `stressviz` capture still shows the old palette.**
  `docs/assets/stressviz-1261d8dd.gif` is referenced by `docs/operations.md:574`
  and `BENCHMARK.md:725`, and no cast is committed for it: refreshing it means a
  full re-measurement (`just website-stressviz` — dev shell, a two-node fleet, a
  real `emb-stress` run) and rewriting the record in `BENCHMARK.md`. That is a
  measured bundle, not a re-render, so it belongs in its own change and is left
  alone here rather than half-done.
- **`bench-cpu-budget.{png,svg}` are drawn in the old palette and owned by
  nobody.** They sit at the repository root, nothing references them (the
  similarly named `bench-cpu-budget-off.yaml` and `bench-cpu-budget-width4.yaml`
  are different files, and are referenced), and no generator produces them.
  Deleting or redrawing an unreferenced artifact is the maintainer's call, not a
  side effect of a site change, so they are reported rather than touched.
- **`demos/dns.html` fails the ink check at 390 and 320, before and after this
  change.** One JS-printed reply line inside its console (`span.t-dim`, a long
  escaped-TXT string) reaches ~490px at those widths. It reproduces on the tree at
  `24e9f7b`, with that tree's own probe and its own CSS, so it is not a
  regression from the palette, the type work or the insets — which is why it is
  reported rather than repaired here: the fix is a decision about that line
  (wrap it, or let the console scroll horizontally), not a palette or floor
  repair, and it belongs with whoever owns the plate's console. Every other
  surface passes at all 24 widths.
- Whether the detector should ever join CI. It is a tool outside the repository
  with its own release cadence, so this change runs it by hand and records the
  verdict; wiring it into `ci.yml` would need a pinned, substitutable install.
- Whether the marketing copy's em-dash density should come down. Advisory by the
  tool's own contract; left alone here, and visible if the team decides
  otherwise.
