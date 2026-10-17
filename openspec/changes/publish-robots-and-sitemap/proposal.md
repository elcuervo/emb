# Proposal

## Why

Three defects meet in the same surface, and they are the same defect read three
ways: the site states facts about itself by hand, in twenty places, and nothing
compares the copies.

1. **A crawler got nothing.** `https://emb.is/robots.txt` answered `404` — with
   the site's own HTML 404 page, so it was a soft 404 — and `/sitemap.xml`
   answered the same way. The readiness scan failed `discoverability.robotsTxt`
   and `discoverability.sitemap`, and two further checks could not be evaluated
   at all ("Cannot check AI rules without robots.txt").
2. **The page metadata is typed twenty times.** Every page carries its canonical,
   `og:url`, `og:image`, `theme-color`, title and description as literals in its
   own `<head>`: sixteen pages share one byte-identical tag sequence, and the
   same origin, the same social image and the same ground colour are written out
   again per page — including inside the favicon's data URI. `published-tree.py`
   can only assert that the copies *agree*; it cannot make them come from one
   place, so a twenty-first page is twenty-one new chances to disagree.
3. **The detector's verdict is 199 warnings.** `impeccable detect website`
   reports `cramped-padding` 154, `overused-font` 22, `undersized-ui-text` 12,
   `all-caps-body` 3, `tight-leading` 3, `flat-type-hierarchy` 2,
   `cream-palette` 2, `broken-image` 1 (plus 12 advisory `em-dash-overuse`
   notes, which the tool itself never counts as failures). The committed world
   is pinned in `DESIGN.md`, the `product-site` spec and seven
   `.impeccable/surfaces/*.md` briefs, and the site's own stated type floor is
   12px desktop — which the `undersized-ui-text` findings break on two demo
   plates. The audit cannot be used as a gate while it is arguing with the
   world it is measuring, and the world is not defensible where it contradicts
   the site's own floor.

## What Changes

- **`website/robots.txt` and `website/sitemap.xml` are served.** The policy is
  one explicit `User-agent: *` group with `Allow: /` and the sitemap reference,
  and no `Disallow`, because nothing this origin serves is withheld; the sitemap
  names every indexable served page at the address the page itself declares, and
  `404.html` is out by its own `noindex`.
- **One descriptor per page becomes the source of every repeated fact.** A
  committed `website/tools/pages.json` carries, per page: the file, the request
  path, the title, the description, whether it is indexable, and (once, for the
  whole site) the origin and the social image. `website/tools/stamp-pages.py`
  emits from it: the page's metadata fragment, spliced into its `<head>` between
  markers, and `website/sitemap.xml`. `--check` fails on any drift, the way
  `stamp-version.py` already does for `VERSION`, and both are wired into the
  same places (`just`, `ci.yml`'s `site` job, `site.yml`'s `checks` job).
  Generated output stays committed: the tree still renders with no build step.
- **The repeated `<head>` fragment is generated, the per-page extras stay
  authored.** Everything from `<meta charset>` through the favicon is emitted
  from the descriptor and the palette tokens; the font preloads, stylesheets and
  scripts that genuinely differ per surface stay where they are, after the
  fragment.
- **The detector's 199 warnings are repaired, not suppressed — except the ground
  the world keeps.** `Inter` leaves in favour of a self-hosted face that is not on
  the overused list (the display voice stays `Archivo`, which is not flagged),
  full-bleed blocks gain their insets, the sub-floor 10px and 10.5px labels rise
  to the site's own 12px floor, the all-caps body runs and the flat type steps
  are repaired, and the `<img>` with no `src` on `demos/image.html` is fixed. The
  paper ground **stays the beige** it has always been: the palette work mirrored
  it to a cool neutral at the same lightness band and that was overruled, so the
  two `cream-palette` notes the ground earns — on the two files that declare a
  page background — remain, deliberately, and every other rule is at zero.
- **The world's records move with it.** `DESIGN.md` carries the faces and
  measures; the seven surface briefs are updated where they pinned what changed;
  `website/README.md` describes the generator, the crawl surface and the
  detector's verdict; and `product-site` gains the requirement the detector made
  concrete — every surface meets the type floor and AA contrast on its own
  ground, not only the dark console.
- **Not changed:** the composition and its atoms (rules, ledgers, plates, the
  signal axis, the annotation form), the banned devices (no cards, gradients,
  radii, glass, second accent), the paper ground and the palette that sits on it,
  the wordmark's shape, scale and place, the claim, the six-row feature list, the
  pipeline artwork, the dark console ground, and the twelve advisory em-dash
  notes — advisory findings are not failures, and the house voice is not a
  defect. The wordmark's *face* follows the display voice, because the detector
  cannot clear while `Inter` ships in any element.
- No **BREAKING** changes to the server, the protocol, the client gems, or any
  on-disk format, and no new runtime dependency for the site.

## Capabilities

### New Capabilities

None. The generated metadata is the deployment surface's own bookkeeping, and
the world change lands in the capability that already owns the poster's
composition and its floors.

### Modified Capabilities

- `site-deployment`: the origin additionally serves `robots.txt` and
  `sitemap.xml`, both members of the asserted served set; every page's
  canonical, social and discovery metadata is emitted from one descriptor per
  page and asserted against that descriptor, so the pages, the sitemap and the
  directives cannot disagree; and the sitemap's `<loc>` set is exactly the
  canonical set of the served pages.
- `product-site`: every surface meets the committed type floor and AA contrast on
  its own ground — paper included — rather than only the dark console; the paper
  ground and the body face are the ones `DESIGN.md` names after this change, and
  the composition's atoms and bans are unchanged.

## Impact

- **New files**: `website/tools/pages.json` (the descriptors),
  `website/tools/stamp-pages.py` (the emitter and its `--check`),
  `website/robots.txt`, `website/sitemap.xml` (generated, committed),
  one vendored body-face WOFF2.
- **Edited**: `website/tools/published-tree.py` (`SERVED`, the crawl-surface
  assertions, and the fragment-vs-descriptor drift check), every served page's
  `<head>` fragment, `website/assets/css/styles.css` and `docs.css` (palette,
  insets, type scale, leading, caps), `website/404.html` and the demos plates'
  inline styles and script literals, `website/assets/img/terrain-matte.png`,
  `terrain-v2.md` and `og.png` (regenerated from the tools that own them),
  `website/assets/fonts/*`, the two tools that draw the ground
  (`tools/gen-terrain-matte.py`, `tools/gen-isometric.py`) so the paper is read
  from the descriptor instead of typed, the two authoring instruments that must
  measure on the shipped ground (`tools/ink-probe.html`, `tools/og-card.html`),
  `website/repl/{index,stats}.html` and `website/PRODUCT.md` (their palette
  copies), `DESIGN.md`, `website/README.md`, the seven
  `.impeccable/surfaces/*.md` briefs, `justfile` (the three `agg --theme` strings
  and `website-shot`'s viewport handling), `ci.yml`, `site.yml`.
- **Two latent defects the change had to fix to be verifiable.** `gen-isometric.py`
  was stale against the page — `--write` would have reverted four hand-added
  per-plate signal bands and a `vector-effect` — so the tool was brought up to the
  page before regenerating; and `just website-shot`'s `viewport=` was a silent
  no-op on agent-browser 0.38.1 (`open --viewport` is accepted and ignored), which
  made every documented mobile capture a 1440-wide desktop one. The recipe now
  applies `set viewport`, which is what makes this change's mobile captures
  evidence rather than decoration.
- **Verification**: `just website-published` exits 0 and fails on each planted
  regression; `stamp-pages.py --check` and `stamp-version.py --check` are clean;
  `impeccable detect website` reports zero warnings (advisories excepted);
  `just website-ink` PASSes at all 24 widths; the changed surfaces are captured
  at desktop and mobile widths; and after publication the live origin answers
  `/robots.txt` as `text/plain` and `/sitemap.xml` as XML, with the readiness
  scan reporting both checks `pass`.
- **Delivery**: static files plus two generated ones; `wrangler.jsonc` and the
  Worker's configuration are untouched, and no path becomes HTML that was not
  HTML, so `html_handling` does not change behaviour.
