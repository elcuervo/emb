# Proposal

## Why

A gallery plate that offers more than one example hides its state in a script
variable, so a reader cannot link to the one they mean. Pointing a colleague at
the Pac-Man loop means naming the tab and hoping they find it; there is no
`#pacman` to send. Four of the twelve plates (laya, atlas, lens, image) have such
a selection, and none of them is addressable today.

## What Changes

- Four gallery plates gain a URL-hash deep link for their in-page selection:

  | Plate | Selection | Hashes |
  |---|---|---|
  | `laya.html` | example tab | `#snake` `#pacman` `#inbox` `#quickstart` |
  | `atlas.html` | placement order | `#meaning` `#year` |
  | `lens.html` | model lens | `#minilm` `#bge-small` |
  | `image.html` | sample image | `#raven` `#storm` `#portrait` `#ship` `#flowers` `#manuscript` |

- On load, a hash naming a known choice selects it; otherwise the plate keeps its
  current default. Selecting a choice updates the URL with `history.replaceState`,
  so tab clicks do not add history entries and Back still leaves the page.
- A `hashchange` (a manually edited URL or an arriving link) re-selects the
  matching choice on the already-loaded page.
- Nothing else changes: the twelve-page structure, the gallery index, the
  `plate-nav`, and the eight plates with no in-page selection are untouched.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `embedding-demos`: add a requirement that a plate's in-page selection is named
  by the URL, restored from it on load, and reflected back to it on selection.

## Impact

- Code: the inline module scripts of `website/demos/{laya,atlas,lens,image}.html`.
- No server, API, schema, dependency, or build-step change; no new file, no new
  asset, and no change to the index's manifest.
- Accessibility: the existing `role="group"` + `aria-pressed` controls keep their
  names and behaviour; the hash only adds a second way to reach a selection.
