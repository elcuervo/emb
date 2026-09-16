## Context

See `proposal.md` — Why. The constraints that shape this design:

- **The sandbox cannot run the subject.** `website/repl/` forwards an allowlisted
  RESP surface to a real `emb` process. Every interactive plate in
  `website/demos/` is credible because its numbers come from that process. The
  Ruby gem is not the process. There is no reading of the gem available to a
  page, and pretending otherwise is the failure mode this design exists to
  prevent.
- **The gallery already owns the server's batcher.** `demos/batch.html` measures
  six `EMB` calls against one `EMB` carrying six texts, on the server's own
  `elapsed_us`. Repeating that on the client surface would spend a plate on a
  claim another page already made.
- **The poster's vocabulary is closed.** `product-site` forbids new visual
  language: no cards, gradients, radii, decorative shadows, or second accent.
  The existing atoms are `.plate` / `.fig` / `.block` / `.sec` / `.cap` / `.ord`
  / `.code` / `.ridge`, plus `.rig` / `.atlas__*` for the gallery's live
  instruments.
- **A new page is a check failure until it is registered.**
  `website/tools/published-tree.py` asserts the served set in both directions,
  `PAGES` requires an absolute origin on every page, and `check_internal_links`
  resolves every `href` and `src` against the served set.
- **`website-demos-gallery` is open at 89/95** and has already modified the same
  `site-deployment` requirement this change modifies.

## Goals / Non-Goals

**Goals:**

- A developer can read one page and know: what the gem sends, when it sends it,
  how many commands a scope costs, why the shape of their call site decides that
  number, and what happens when the batch fails.
- Every claim is checkable against `gems/emb/lib/emb/` in a few seconds.
- The page costs the sandbox nothing and works with scripting disabled.
- The surface is reachable from every page, and the site's own checks know it
  exists.

**Non-Goals:**

- Any performance figure. Not a benchmark table, not a measured `elapsed_us`,
  not a comparative ratio.
- Serving Ruby from the sandbox, or any new bridge command.
- Changing the gem, the server, or the bridge.
- A second page for `emb-server`. It is four facts and a one-liner; it is the
  closing plate, not a route.
- Re-litigating the poster's visual world.

## Decisions

### The page is static, and carries no numbers

The visualisations are **diagrams of the gem's dispatch rules**, drawn from
`pack_slices`, `dispatch_parallel`, `same_model_args`, `mixed_model_args` and
`MultiProxy#run`. No timing, no rate, no ratio.

Alternative considered and rejected: *measure-and-project* — take the three
shares' server-measured durations sequentially and draw them overlapping at the
overlap the client would produce. Honest if labelled, and it was the original
plan; dropped because the user asked for mechanism rather than performance, and
because the projected overlap is the only number on the page that no reader
could check against the repository without trusting the drawing.

The consequence is a rule worth freezing in the spec: **counts of commands are
allowed, durations are not.** A command count is the shipped code's output; a
duration would be the page's opinion.

### No JavaScript at all

The four wire shapes are rendered **side by side**, one ruled row each, rather
than behind a mode switcher.

A switcher hides three of the four shapes, and the whole teaching is the
comparison between them. Static markup also means the page has no sandbox
dependency, no `demos.js` import, no `noscript` fallback to write, and satisfies
`product-site`'s script-failure requirement by construction rather than by
handling.

The page links `assets/css/styles.css` and nothing else. The `data-sandbox`
module tag the gallery's plates carry is deliberately absent, and the motion
added later on this page did not change that: it is scroll-driven CSS.

Alternative considered and rejected: a small `IntersectionObserver` module, the
mechanism the landing uses for its reveals. It would work everywhere, and it
would put the page back in the business of shipping a script and a
script-failure story for one effect. See the motion decision below for what
replaced it.

### The instrument performs the dispatch, in CSS

The page carries one authored moment, at the instrument, rehearsed wherever a
ledger carries one. As a row comes into view its marks go onto the wire in the
shape that mode dispatches them: six ticks one after another for `lazy: false`,
one stroke that settles from the rule's grey into the accent for `:multi`, three
strokes at the same instant for `:batch`, and one stroke for `Emb.multi { }`.

Two properties make it the right moment rather than decoration:

- **The colour change is the message.** A mark that turns accent is a single
  call carrying every text — the same meaning the accent has everywhere else in
  the family — which is why the eager row's six marks and the batch row's three
  never turn. The animation restates the rule it sits under.
- **Each row owns its timeline.** A ledger-wide timeline was written first and
  discarded: it played the whole cascade as the ledger entered, so rows three
  and four performed below the fold and a reader found them already drawn. Per
  row, `view()` with a `cover` range is proportional to the row's own journey
  through the viewport, so every row performs while it is on screen, at every
  viewport height and row height.

Rejected alternatives: an `IntersectionObserver` + class toggle (a script for one
effect, and a state only the script can create); a time-based entrance on load
(it plays before the reader has scrolled to the ledger); a Web Animations API
sequence (interruption and sequencing for a scroll relationship that CSS already
expresses).

**The fallback is the shipped page.** The whole moment lives inside
`@supports (animation-timeline: view())` and `@media screen and
(prefers-reduced-motion: no-preference)`, so an engine without view-progress
timelines, a reader who prefers reduced motion, and a print pass all render the
finished static ledger — which is what this page was before the motion existed.
The `screen` guard is load-bearing rather than tidy: a print pass never scrolls,
so without it every mark would print at the start of its range and the
instrument would come out blank.

### One page, two subjects, in the plate rhythm

`website/gem/index.html`, opening with the gallery's `.plate` head and reading
as one instrument:

| Section | Ground | Carries |
|---|---|---|
| The Ruby client | plate head | `FIG. 1` — three modes, pool 5, batch 512, two gems |
| WHAT YOU ARE LOOKING AT | paper | what the gem is, and the request path diagram |
| THE THREE MODES | dark | the four wire shapes, one row each |
| *(ridge break)* | — | `terrain-shoulder.png`, the gallery's own reading break |
| THE ONE RULE | paper | create-then-consume, two lanes |
| WHEN IT FAILS | paper | fail-closed, why retries are 0, the 10s timeout |
| SCOPE IS PER THREAD | paper | the middleware ledger |
| TWO GEMS, TWO JOBS | dark | `gem install emb-server`, platforms |
| THE EXACT COMMANDS | paper | `.code` blocks and the reply shapes |

### One drawing primitive, four uses

A new `.wires` atom family — lanes, a rail, marks, a label, a count — drawn as
inline SVG. It renders:

1. the **request path** (thread → registry → pool → instance),
2. the **four modes**, one lane-set per row,
3. the **create-then-consume** pair,
4. the **two gems** (the client speaking RESP to the distribution).

One primitive, four diagrams, one place to get the geometry right. The accent is
spent **once per diagram**, on the command the mode exists to produce (the
coalesced `EMB`, the merged `EMB.MULTI`) — the rule the gallery already follows.

Every diagram is captioned **"the shape the client sends"** so a reader cannot
mistake it for a reading.

### Registration is part of the change, not an afterthought

```
website/gem/index.html
  → published-tree.py SERVED
  → published-tree.py PAGES            (canonical + og:url + og:image absolute)
  → published-tree.py served_urls()    the /gem pretty URL, beside /docs and /demos
  → published-tree.py check_internal_links  (passes once SERVED knows the page)
  → stamp-version.py TARGETS           both gems carry the repo's VERSION
```

### Navigation: one entry per surface, added mechanically

A `Gem` entry after `Demos` in the primary `.nav` and in the `.mobile-nav`
disclosure of all 14 pages: the landing, the reference, the gallery index, the
ten plates, and `404.html` (which uses root-absolute URLs by design). Relative
for every page but `404.html`, so the tree keeps working under any mount point —
which is what `check_internal_links` already asserts.

Alternative considered: one entry only in `docs/index.html` §08. Rejected by the
user — the client surface is a peer of the reference and the gallery, so the
masthead is where it belongs.

### Sequencing

`website-demos-gallery` has already modified
`site-deployment: The published tree is the site and nothing else`. This change's
delta to that requirement is written against the post-gallery text ("one demos
gallery") and **must be archived after it**. Applied first, the delta's header
still matches, but its body would erase the gallery's index-served scenario at
archive time.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| The page reads as marketing rather than mechanism | Every row states a command and a count; the create-then-consume rule is the payoff; nothing else is promised |
| A diagram drifts from the gem | Counts come from `bench.rb`'s own round-trip assertions (`eager = 5 EMB`, `multi = 1 EMB`, `batch = 3 shares at batch_size 2`, mixed = `1 EMB.MULTI`), so the diagram is checkable against a test that already runs |
| 14 masthead edits break a link | `just website-published` resolves every reference on every served page; `just website-ink` sweeps the new page at 24 widths |
| Two open changes touch one requirement | Prerequisite documented above, in `proposal.md` Impact, and in `tasks.md` |
| The page grows into a second reference surface | Its subject is narrower than the gem README: five mechanisms, not the API. `product-site`'s existing rule that the landing carries no reference material keeps the boundary |
