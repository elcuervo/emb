## Why

The `emb-top` plate on the landing page is a hand-built `<pre>` of spans whose own
caption reads "Illustrative render · not a measured run". It cannot show the thing
the panel exists to prove — a live dashboard moving under load — and it drifts
from the real render by hand (`stamp-version.py`'s docstring still cites the
capture shipped against a stale VERSION). The docs surface carries a second,
also-handwritten copy.

Meanwhile the repo has everything needed to capture the real thing: `asciinema`
records headlessly with no browser, `agg` renders a cast with a custom hex theme,
and `emb-top` already has a headless `-once` mode. There is no reason for the one
visual proof of the monitoring story to be a drawing.

The goal is a plate a reader can *read metrics off* — not a decorative loop. That
constrains the capture (legible type at the plate's width), the edit (dead time
cut, ramps visible), and the framing (the caption says when and at what version
the run was captured, instead of claiming to be current).

## What Changes

- Add `just website-topviz`: one command that builds the binaries, starts a
  single `emb` node with several models and the cache on, warms it, records
  `emb-top` headlessly under a scripted ~30 s traffic scenario, renders and edits
  the cast to an animated WebP plus a poster frame, and stamps the resulting
  content-hashed filenames into the page.
- **BREAKING (site markup)**: replace the illustrative `.topviz__screen` `<pre>`
  in `website/index.html` with the recording — a `<picture>` whose animated
  source is gated on `(prefers-reduced-motion: no-preference)` and whose fallback
  is the poster frame, plus an `.sr-only` copy of the metrics so nothing the
  `<pre>` carried to assistive technology is lost.
- Recolour the recording to the site's tokens at render time (the accent carries
  the `req/s` stream), rather than shipping a default terminal theme.
- The plate's provenance line states the run's date, the captured `emb-top`
  version, model count and node address. The captured version is frozen in
  pixels; the RELEASE ledger and docs keep using `data-emb-version`, so no
  version-bearing element is left un-stamped.
- Add the recording rig under `website/tools/topviz/` (unserved) with its traffic
  scenario, model config and the `.cast` as provenance, and add `asciinema` +
  `agg` to the website dev shell.
- Record no errors. `emb-top`'s per-model `err/s` comes from `EMB.INFO` pool
  counters, which only increment on a genuine tokenizer or inference failure;
  scripted-model failures land in a counter `EMB.INFO` does not report. The
  illustrative plate's `err 18 ↑` is not reproducible on purpose, so the
  recording drops the red rather than faking it.

## Capabilities

### New Capabilities

- `emb-top-capture`: a reproducible, scripted capture of the `emb-top` dashboard
  under load — one command, a deterministic traffic scenario, a themed and
  edited animation, a reduced-motion-safe plate, and an artifact whose name
  changes when its bytes do.

### Modified Capabilities

- `product-site`: the "The `emb-top` panel shows real output" requirement changes.
  The panel is no longer an illustrative text render carrying a `data-emb-version`
  string; it is a captured recording whose provenance is stated in text and whose
  version is frozen at capture time.

## Impact

- `website/index.html` — plate swap; `.topviz__screen` / `.tf` / `.tf--bar` markup
  and the `.topviz__src` "Illustrative render" caption go.
- `website/assets/css/styles.css` — the plate's rules are rewritten for the new
  element; the `.tf*` rules and the dead mobile `.tf--bar` rule are removed.
- `website/assets/img/` — two new content-hashed artifacts (animated WebP and
  poster PNG), immutable under the existing `/assets/img/*` cache rule.
- `website/tools/published-tree.py` — two lines in the explicit `SERVED` set; a
  re-record changes them, so the rig must delete the previous take.
- `website/tools/topviz/` (new), `justfile` (`website-topviz`), `flake.nix`
  (`websiteDeps`), `website/README.md` (the tools inventory).
- No server, protocol, client or `emb-top` behaviour changes. No `_headers`
  change: a hashed name makes the existing immutable rule honest.
- Design work runs through the Impeccable workflow, as `product-site` requires.
