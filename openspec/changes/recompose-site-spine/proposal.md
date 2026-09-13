## Why

The first pass added one merged capability region and left it as a single paper
block. Three things are wrong with the result. The page ends on a metaphor rather
than on the product: the terrain — the strongest image on the page — sits in the
middle, and the page then trails off. The spine is decoration rather than
structure: the orange line exists only inside the pipeline SVG and on the terrain
route, so the page's own organising idea stops two-thirds of the way down and
never arrives anywhere. And the capability region is one rhythm repeated — three
ruled entries, a dark console, a dark capture band — carrying four different
capabilities at one weight.

## What Changes

- **The terrain moves to the end.** The massif becomes the last section before
  the footer, so the spine arrives where it has been travelling. Its horizontal
  alignment to the spine is re-derived and must keep the fork on the axis.
- **A continuous spine, `emb` → massif.** The line becomes a reusable
  structural primitive: a `--fold` axis derived from the pipeline's own
  geometry (`--col-prose + 40.7% × (1 − --col-prose)`), plus spine segments in
  every section between the wordmark and the terrain. Sections join exactly
  (each segment is a grid track that abuts its neighbour) and the hero's
  existing masked SVG spine is kept rather than duplicated.
- **Three blocks with three grounds**, replacing the single merged region:
  1. **Protocol** — paper; a ruled ledger and the console plate.
  2. **Scripts** — a full-bleed dark band; the page's one inversion, carrying
     the `model(fn(input)) → output` material and the working Lua examples.
  3. **Operations** — paper; a dense ledger and the `emb-top` capture.
- **Code highlighting.** A small token set (command, string, numeric, comment)
  applied to every code specimen and console transcript, defined twice — once
  for paper and once for the dark band — and contrast-checked on both.
- **Amended prior decisions.** The archived change's requirement that the
  capability region sit *after* the terrain and that the terrain stay
  pixel-fixed is superseded. The hero, wordmark, claim, sub, actions, six-row
  feature list, and pipeline artwork remain unchanged.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `product-site`: the capability region is split into three ground-varied blocks
  placed before the terrain; the terrain moves to the end; the spine becomes a
  continuous page-level element with a named axis; code specimens gain a
  contrast-checked token treatment.

## Impact

- `website/index.html` — terrain section moved after the blocks; the single
  `section.capability` replaced by three `section` blocks; spine elements added;
  code specimens marked with token spans.
- `website/assets/css/styles.css` — a `--fold` axis token, the `.spine`
  primitive, three block grounds, the dark band's inverted type scale, and the
  code token styles for both grounds.
- `website/assets/js/main.js` — the pipeline/terrain reveal and route logic
  must follow the terrain to its new position; the terrain route's scroll
  window changes because the band is no longer mid-page.
- `website/README.md`, `website/CORRECTIONS.md` — composition, spine geometry,
  block grounds, and the re-measured numbers.
- `openspec/specs/product-site/spec.md` — via this change's delta.
- No server, protocol, gem, or Go/Ruby changes. No new asset, font, or
  dependency: the spine and the code surfaces are drawn from existing tokens.
