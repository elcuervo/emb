## Why

The site can ship now; its live runtime cannot. The landing page's console is a
placeholder transcript client with a documented `window.embConsole.exec` seam —
it makes no network call and says so — but it is still **rendered**, so a
visitor meets a panel whose badge reads `DEMO · NOT A LIVE SERVER` and whose
note says the live client is not wired. That is an honest placeholder, and it is
also the wrong thing to lead a first public impression with when the deploy
pipeline (`deploy-site-to-cloudflare`) is landing in the same window.

The console cannot simply be deleted: the placeholder, its two modes, its states
and the seam are the expensive part, and re-authoring them later is the cost the
`product-site` capability exists to avoid paying. The one-attribute hide is
already made and verified; what is missing is the spec record. `product-site`
currently says the site **SHALL render** a realtime-console panel, so the
published page would contradict its own specification the moment it ships. This
change makes the intended state — *retained as an artifact, withheld from the
rendered page until a live executor exists* — the specified state.

## What Changes

- **The console panel ships hidden.** `<section class="console" … hidden>`
  takes the panel out of the rendered page and the accessibility tree. The
  markup, the styles, the two modes, the deterministic transcripts and the
  `window.embConsole.exec` seam all stay in the tree, byte-for-byte, so the live
  runtime is a wiring change rather than a re-authoring.
- **The client does not boot while hidden.** `main.js` finds the console with
  `[data-console="transcript"]:not([hidden])`, so no timers run, no live region
  is armed, and `window.embConsole` is not defined on the shipped page.
- **One attribute is the switch.** Removing `hidden` restores the placeholder
  exactly; replacing `exec` with a RESP client then makes it live. Both the
  markup comment and `website/README.md` say so, so the revert is discovered in
  the artifact rather than remembered.
- **The console's design contract survives the hide.** The requirements that
  describe the panel's modes, states, keyboard operation and dark-ground
  contrast continue to govern the retained artifact; they apply to the rendered
  page whenever the panel is enabled. The specification change scopes them
  rather than deleting them.
- **The capability block's reflow scenario stops assuming a rendered console.**
  The mobile-reflow check named "the console and the merged block"; while the
  panel is hidden the assertion is about the capability region alone.

## Capabilities

### New Capabilities

None. The console is an existing, specified behaviour of `product-site`; hiding
it is a change to that behaviour, not a new surface.

### Modified Capabilities

- `product-site`: the requirement "The console is a placeholder with a live
  seam" changes from *the site SHALL render the panel* to *the site SHALL retain
  the panel as an artifact and withhold it from the rendered page until a live
  executor is provided*, with scenarios for the hidden state and the
  one-attribute restore. The requirement "The capability region is responsive
  and accessible" drops the assumption that a console is always in the reflow.

## Impact

- **Edited:** `website/index.html` (the panel gains `hidden` plus the comment
  that explains the switch), `website/assets/js/main.js` (the boot selector and
  its comment), `website/assets/css/styles.css` (an explicit
  `.console[hidden] { display: none; }` so the hide does not depend on a user
  agent default), and `website/README.md` (the hidden-for-first-ship rule beside
  the console's own section).
- **Unchanged that a reader might expect to change:** the published file set.
  No file is added, moved or deleted, so `website/.assetsignore`,
  `published-tree.py`'s expected set and the served-tree count are untouched.
- **Unchanged:** every console transcript, mode, state, and the
  `window.embConsole.exec` seam; the poster's composition, tokens and type; the
  docs surface; all Go, gem, ONNX and Ruby code.
- **No build step** is introduced — the switch is one HTML attribute and one
  selector, which is what keeps the revert legible in a diff.
- **Deployment:** none specific to this change. It rides the
  `deploy-site-to-cloudflare` pipeline; the first published revision is the one
  that carries the hidden panel.
