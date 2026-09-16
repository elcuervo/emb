## Why

The Ruby client's most consequential behaviour is invisible to a reader of the
repository. `ruby-batch-loading`, `emb-ruby-client` and `emb-server-distribution`
are specified and tested, but the site states none of it: `docs/index.html` §08
mentions the gem in one paragraph, and the demos gallery explains the *server's*
batcher, not the *client's* deferral. The one thing a developer gets wrong — that
`Emb[:model]["text"]` only sends a command when the returned value is *used*, so
loaders must be created before they are consumed — is documented nowhere a
visitor can see it.

## What Changes

- Adds a client surface at `website/gem/`, one page, explaining the Ruby client
  to a developer: the request path, the three execution modes (`lazy: false`,
  `:multi`, `:batch`) and the wire command each produces, the create-then-consume
  contract, the fail-closed batch, the per-thread scope, and the `emb-server`
  distribution gem.
- Explains the mechanism only. The page carries **no benchmark figures and no
  measured timings**: its visualisations are diagrams of the client's dispatch
  rules, drawn from `gems/emb/lib/emb/`, and they are labelled as drawings rather
  than presented as readings.
- Is static: no sandbox call, no `demos.js` import, no network dependency. The
  page renders completely with scripting disabled.
- Adds a **Gem** entry to the masthead navigation on every page, so the client
  surface is reachable from the landing, the reference and every gallery plate.
- Registers the page where the site's own checks expect it: the served set, the
  pages that must declare an absolute origin, the internal-link check, and the
  version stamper.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `product-site`: adds the client surface's content and honesty contract, and a
  requirement that the masthead carries one navigation entry for every surface
  the site serves (rather than documentation alone).
- `site-deployment`: the published tree's enumeration of surfaces gains the
  client surface, and the served-set check gains its page.

## Impact

- **New**: `website/gem/index.html`,
  `website/.impeccable/surfaces/website-gem-index-html.md`.
- **Modified**: `website/assets/css/styles.css` (the diagram atoms),
  `website/tools/published-tree.py` (`SERVED`, `PAGES`, the `/gem` pretty URL),
  `website/tools/stamp-version.py` (the new page as a stamp target),
  `website/docs/index.html` §08, `website/demos/index.html`, and the masthead
  navigation in all 14 pages.
- **No** change to the Go server, the sandbox bridge, either gem, or any reply
  shape. The page is documentation of shipped behaviour.
- **Sequencing**: this change's `site-deployment` delta and the version stamping
  overlap `website-demos-gallery`, which is open at 89/95. The deltas here are
  written against that change's post-state ("one demos gallery") and this change
  must be implemented and archived **after** it, or the two deltas to
  `The published tree is the site and nothing else` collide at archive time.
