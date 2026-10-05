# Proposal

## Why

`https://emb.is/demos/laya#pacman` now selects the Pac-Man example, but it still
opens the page at the top: the TRY IT rig sits below several sections, so a
reader who follows the link has to scroll to find the demo the link named. A
link that names a choice should land on that choice, not merely preselect it.

## What Changes

- Four plates that already read a choice from the URL fragment
  (`laya`, `atlas`, `lens`, `image`) also bring the demo region into view when
  the fragment names one of their choices:
  - on load, when the page's own fragment names a choice;
  - on `hashchange`, when an arriving or edited fragment names a choice.
- The reveal targets the plate's existing `TRY IT` heading (`#try-it-h`), which
  already carries the site's `scroll-margin-top`, so the sticky masthead does
  not cover it.
- The scroll is instant and runs after the choice is selected. Choosing a control
  by clicking still does not scroll — only a fragment does.
- No fragment, or one that names no choice, leaves the page where it opened.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `embedding-demos`: extend the URL-selection requirement so a fragment that
  names a choice also reveals that demo, not only selects it.

## Impact

- Code: the inline module scripts of `website/demos/{laya,atlas,lens,image}.html`
  (same four files the URL-selection change touched).
- No markup, stylesheet, server, dependency, or build-step change: the heading,
  its `id`, and the scroll-margin rule already exist.
- Accessibility: the reveal is an instant scroll (no motion), so no
  reduced-motion branch is needed.
