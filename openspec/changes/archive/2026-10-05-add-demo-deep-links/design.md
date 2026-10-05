# Design

## Context

See proposal.md for motivation. Four of the twelve gallery plates carry an
in-page selection — `laya.html` (example tab), `atlas.html` (placement order),
`lens.html` (model lens), `image.html` (sample image). Each is a static page
whose inline module script keeps the selection in a variable (`active`, `order`,
the chosen model, the chosen sample) and repaints from a click handler. None
reads or writes `location.hash`; a repo-wide grep for `hashchange`, `pushState`
and `replaceState` under `website/assets/js/` and `website/demos/` is empty.

The controls are `role="group"` with `aria-pressed` toggle buttons, not
`role="tab"`/`tablist`. The pages have no build step and no shared router; each
inline script stands alone.

## Goals / Non-Goals

**Goals:**
- Each in-page choice has a stable fragment, and a link carrying it opens the
  plate on that choice.
- Selecting a choice is reflected in the address bar so the reader can copy it.
- No history entries are added, no control changes semantics, and the eight
  plates without a selection are untouched.

**Non-Goals:**
- No anchors on the gallery index or the twelve plate URLs — the plates are
  already separate, canonical addresses.
- No `role="tab"` rewrite, focus management, or arrow-key tab navigation.
- No scroll-to-rig behaviour on load; a fragment is a state link, not a jump
  link.
- No history that walks selections with Back.

## Decisions

**Native fragments, no router.** The mechanism is `location.hash` plus the
`hashchange` event — the platform's own router. A library or a shared `demos.js`
helper would be a new file for ~40 lines used by four pages; the inline script
already exists on each page, so the change lands there.

**`history.replaceState` over `location.hash =`.** Assigning `location.hash`
pushes a history entry, so Back would walk tab by tab instead of leaving the
plate; `replaceState` keeps the address in sync without a history entry.
Rejected: an `<a href="#pacman">` for each tab would give history and copy-paste
for free, but it changes the controls from buttons to links and alters the
`aria-pressed` toggle semantics the plates already use.

**Bare fragments, keyed by the plate's own value.** `#snake`, `#pacman`,
`#inbox`, `#quickstart` (laya); `#meaning`, `#year` (atlas); `#minilm`,
`#bge-small` (lens); `#raven`, `#storm`, `#portrait`, `#ship`, `#flowers`,
`#manuscript` (image). Three plates already use these values in `data-*`
attributes; `image.html` maps `data-sample="samples/storm.jpg"` to its basename
`#storm`. Rejected: a `#demo=pacman` namespace — none of these names collides
with the plates' section ids (`try-it-h`, `the-api-h`, `plate-h`, …), so the
namespace buys nothing.

**Read on load, listen for `hashchange`, write only on selection.** On load a
known fragment mounts; an unknown or absent one leaves the default. A
`hashchange` re-mounts, so a manually edited URL or an arriving link re-selects
on the loaded page. Only the click handler writes the fragment — internal
re-mounts (Laya's Reset increments a seed and calls `mount()` again) must not
rewrite the URL, and a default mount with no fragment must not stamp `#snake`
onto a plain visit.

**No element ids on the controls.** Browser-native fragment scrolling only
happens when an element carries the id; leaving the buttons id-less keeps a
shared link from landing the reader mid-rant. A future jump-link desire is a
different feature.

## Risks / Trade-offs

- `replaceState` does not fire `hashchange`, so writing on selection cannot
  re-trigger the listener; a loop is not possible. If a plate later assigns
  `location.hash` directly, the listener would fire — the current code does not.
- A fragment equal to an unrelated id would make the browser scroll; the chosen
  keys were checked against each plate's existing ids and collide with none.
- With scripting off, the default selection still renders and the fragment has
  no effect — the same posture as the rest of the gallery's non-script state.

## Migration Plan

Static-asset change only: ship the four edited pages, no server, index rebuild,
or cache-key change. Rollback is reverting the four inline scripts.
