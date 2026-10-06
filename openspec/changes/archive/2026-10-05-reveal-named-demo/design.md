# Design

## Context

See proposal.md for motivation. The preceding change (`add-demo-deep-links`)
made each plate select a choice named by the fragment, and its design explicitly
chose **not** to scroll, calling the fragment "a state link, not a jump link".
Live validation showed the cost of that call: `#pacman` selects Pac-Man but the
reader still lands at the top of a long page.

Each demo page already carries a `TRY IT` heading with `id="try-it-h"`. The
stylesheet has a global `[id]{ scroll-margin-top: … }` rule that keeps a targeted
element clear of the sticky masthead. So the target, and the sticky-offset
handling, already exist.

## Goals / Non-Goals

**Goals:**
- A fragment that names a choice lands the reader on the demo region.
- The reveal is driven only by the fragment, never by a click.
- No new markup, token, or file.

**Non-Goals:**
- No smooth scrolling, no scroll animation, no reduced-motion branch — an instant
  scroll is the whole motion.
- No change to the selection behavior itself; the choice is still selected by the
  existing code.
- No scroll for the eight plates with no in-page choice, and none for the gallery
  index.

## Decisions

**Scroll `#try-it-h`, not the rig.** The heading already has an `id` and therefore
already inherits the site's `scroll-margin-top`, which clears the sticky masthead.
Targeting the existing heading reuses that rule instead of adding a scroll-margin
to the rig or an `id` to the controls. Rejected: giving the switch an `id` — it
would add markup and duplicate a rule the heading already carries.

**Instant scroll.** `scrollIntoView({ block: 'start' })` defaults to an instant
scroll, which is the least motion and needs no `prefers-reduced-motion` branch.
Rejected: `behavior: 'smooth'` — a long animated travel from the top of the page
is worse than landing, and it would need a reduced-motion guard.

**Reveal on load and on `hashchange`, never on click.** The click handlers select
a choice and write the fragment with `history.replaceState`, which does not fire
`hashchange`. So a click never triggers the reveal, while a shared link (load) and
an edited or arriving fragment (`hashchange`) do. That is exactly the distinction
the reader expects: a fragment takes you there, a click stays where you are.

## Risks / Trade-offs

- A fragment that names a choice but is also a section `id` would compete with
  native fragment scrolling. None of the chosen names collides with a section id
  (established by the prior change), so this cannot happen today.
- An instant scroll during font/image load can land a few pixels off if content
  above reflows. The heading sits high in its section and the shift is small;
  accepted rather than adding a layout observer.

## Migration Plan

Static-asset change: ship the four edited pages, no server or cache-key change.
Rollback is reverting the four inline scripts.
