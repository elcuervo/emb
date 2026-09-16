## 0. Prerequisite and brief

- [x] 0.1 Confirm `website-demos-gallery` is archived, or that this change will be archived after it: run `openspec list` and verify the demos gallery no longer appears; then run `openspec validate website-gem-page` and verify it passes. The `site-deployment` delta here is written against that change's post-state, so archiving this one first would erase the gallery's index-served scenario
- [x] 0.2 Run `impeccable context` for `website/gem/index.html` and verify it reports the site's existing world rather than proposing a new one
- [x] 0.3 Write `website/.impeccable/surfaces/website-gem-index-html.md` with the direction contract — visitor mode (a Ruby developer deciding whether to use the gem), the job (know what a scope costs on the wire), the proof (the gem's own dispatch rules), the memorable moment (the four wire shapes on one page, contrasted), and the constraints (no new colour, font, asset or dependency; no benchmark figure; static) — and verify the brief exists before any markup is written

## 1. The drawing primitive

- [x] 1.1 Add the `.wires` atom family to `website/assets/css/styles.css` — a lane, a rail, a mark, a label and a count, drawn as inline SVG — and verify every colour it uses is an existing token (`--rule`, `--rule-dark`, `--accent`, `--muted`) with the accent reserved for the one command a diagram exists to show
- [x] 1.2 Verify the family survives the site's floors: render a specimen at 1440px and at 390px and confirm no label computes below 12px desktop / 14px mobile, measured on the text runs rather than the boxes

## 2. The diagrams

- [x] 2.1 Draw the **request path** — `Emb[:model]["text"]` → proxy registry → the instance's pool of `pool` connections → `emb` — and verify it shows that commands beyond the pool's parallelism wait rather than time out, because there is no checkout timeout
- [x] 2.2 Draw the **four wire shapes**, one ruled row each: `lazy: false` (one `EMB` per call), `lazy: :multi` (deferred, coalesced into one `EMB`, chunks serial), `lazy: :batch` (deferred, chunk shares concurrent), `Emb.multi { }` (pairs collected, one `EMB.MULTI`). Verify the counts against `gems/emb/bench/bench.rb`'s own assertions: eager = 5 `EMB`, `:multi` = 1 `EMB`, `:batch` at `batch_size: 2` over 5 texts = 3 shares, mixed-model scope = 1 `EMB.MULTI`
- [x] 2.3 Draw **create-then-consume** as two lanes — interleaved (six round trips, six passes) against hoisted (one round trip, one pass) — and verify the diagram's claim matches `batch.rb`'s `batch(default_value: [], key: BATCH_KEY, &BATCH_BLOCK)` firing on use rather than on creation
- [x] 2.4 Draw **two gems, two jobs** — the client speaking RESP, the distribution shipping executables — and verify it does not read as one product with two names
- [x] 2.5 Verify every diagram is captioned as the shape the client sends, and that no diagram, caption, comment or label states an elapsed time, a rate, or a ratio: grep the page for `ms`, `µs`, `req/s`, `×` and confirm every hit is either absent or a count of commands
- [x] 2.6 Verify each diagram's geometry matches the code it draws by reading the four dispatch functions (`pack_slices`, `dispatch_parallel`, `same_model_args`, `mixed_model_args`) and confirming no diagram shows a shape the code cannot produce — in particular that a single-model slice becomes `EMB`, never `EMB.MULTI`

## 3. The page

- [x] 3.1 Write `website/gem/index.html` with the gallery's plate rhythm: `.plate` head with `FIG.` caption, the paper explanation block, the dark modes block, a `ridge--band` break using `terrain-shoulder.png`, then the create-then-consume, failure, scope and distribution blocks, closed by `.code` blocks and the footer. Verify it carries the same masthead, footer and canonical/OG metadata shape as `website/demos/batch.html`, with every absolute URL on the site's origin
- [x] 3.2 State the three modes and their defaults (`false` eager, `pool` 5, `batch_size` 512, `read_timeout`/`write_timeout` 10s, `reconnect_attempts` 0), and verify each value against `gems/emb/lib/emb/configuration.rb`
- [x] 3.3 State the create-then-consume rule as the consequence of deferral, with the wrong and right call sites side by side, and verify the claim against `batch.rb` and the gem README's "create-then-consume contract" section
- [x] 3.4 State the fail-closed behaviour — `Emb::ServerError` carrying the cause, pending items cleared so a retry does not re-send, `reconnect_attempts: 0` by default, read timeouts never re-sent because `EMB.MULTI` is not idempotent, a refused connection retried on the next configured instance — and verify each clause against `fail_batch!`, `transient_error?` and `ConnectionRouter#call`
- [x] 3.5 State that the scope is per thread and cleared at the request and job boundary, and verify the middleware covered against `Railtie`'s registrations (Rack, ActiveJob `around_perform`, plain Sidekiq, plain Shoryuken, and the `Sidekiq::Testing` chain when it loads before boot)
- [x] 3.6 State the exact `EMB` and `EMB.MULTI` argument shapes, including the `VALUES` opt-in, and verify them character for character against `same_model_args` and `mixed_model_args`
- [x] 3.7 Write the `emb-server` plate — `gem install emb-server` as one copy-paste line, the executable names `emb` and `emb-top`, the platform table, and that it ships a precompiled binary with the `onnxruntime` dependency rather than a client library — and verify each fact against `gems/emb-server/emb-server.gemspec`, its README and `bin/emb`
- [x] 3.8 Verify both gems' versions are stamped rather than typed: carry `data-emb-version` on the version elements, then run `just website-version` and `just website-version-check`
- [x] 3.9 Verify the page carries no script: grep the file for `<script`, confirm it links `styles.css` only, and load it with scripting disabled and confirm the explanation and every diagram are complete

## 4. Registration

- [x] 4.1 Add `gem/index.html` to `SERVED` and to `PAGES` in `website/tools/published-tree.py`, and add the `/gem` and `/gem/` pretty URLs to `served_urls()` beside `/docs` and `/demos/`
- [x] 4.2 Add `website/gem/index.html` to `TARGETS` in `website/tools/stamp-version.py` and verify `just website-version-check` passes
- [x] 4.3 Run `just website-published` and verify it reports the new served path, resolves every reference on the new page, confirms one origin across all pages, and exits 0

## 5. The masthead entry

- [x] 5.1 Add the `Gem` entry after `Demos` in the primary `.nav` and the `.mobile-nav` disclosure on every served page: `website/index.html`, `website/docs/index.html`, `website/demos/index.html`, the ten `website/demos/*.html` plates, and `website/404.html` (root-absolute there, as that page's own rule requires)
- [x] 5.2 Verify the link depth per page: root pages reach `gem/index.html`, `docs/` and `demos/` pages reach `../gem/index.html`, and `404.html` uses `/gem/`
- [x] 5.3 Run `just website-published` and verify every new reference resolves and no page emits a root-absolute sibling link
- [x] 5.4 Link the surface from `website/docs/index.html` §08 Clients, replacing the paragraph's bare mention with a door to the page, and verify the reference surface still contains no more than its own detail
- [x] 5.5 Note the page in `website/demos/index.html`'s closing block as the gallery's companion for the client side, and verify the gallery's own ten-plate reading order is unchanged

## 6. Verification

- [x] 6.1 Run `just website-ink http://localhost:8080 gem` and verify every one of the 24 widths passes, with the new diagrams included in the measurement
- [x] 6.2 Run `just website-ink http://localhost:8080` and `just website-ink http://localhost:8080 docs` and verify the masthead change broke no existing surface
- [x] 6.3 Run `impeccable detect --json` once over `website/gem/index.html` and `website/assets/css/styles.css` and resolve or record every finding
- [x] 6.4 Measure the page's text and control floors at 1086px and 390px and record the numbers in the change's `notes.md`
- [x] 6.5 Verify the page describes only shipped behaviour by checking each mechanism against the gem's specs (`ruby-batch-loading`, `emb-ruby-client`, `emb-server-distribution`) and recording any place the specs and the page disagree
- [x] 6.6 Run `openspec validate website-gem-page --strict` and verify it passes

## 7. The motion

- [x] 7.1 Write the motion thesis before the code: one authored moment, at the instrument, rehearsed wherever a ledger carries one — the modes ledger and create-then-consume — with the accent marking the single call and no second effect on the page
- [x] 7.2 Implement the instrument's arrival in `website/assets/css/styles.css` as scroll-driven CSS: `view-timeline-name` on the row, a `cover` `animation-range` per mark with `--i` as the only stagger, and one keyframe carrying both the draw and the colour turn
- [x] 7.3 Verify each row performs while it is on screen by sampling the marks' `scaleX` at scroll offsets: at 1280×577 every row's marks move between y≈390 and y≈300 in the viewport, none at or below the fold
- [x] 7.4 Verify the encoding survives the motion: the eager row's six marks arrive at six different points, the batch row's three arrive at the same point as each other, and only `:multi` and `Emb.multi` hold the accent when complete — recorded in `notes.md` as a scroll-position table
- [x] 7.5 Verify the two rehearsals rhyme by sampling `create-then-consume`: `Interleaved` ticks six times one after another and `Hoisted` draws one stroke
- [x] 7.6 Verify the fallback is the shipped page: remove the `@supports` rule at runtime and confirm the instrument renders finished — marks at `scaleX(1)` with widths 45×6 / 287 / 95×3 / 287, the two coalesced rows in the accent, the rest in the rule's colour, and the scope's edge back on `border-right`
- [x] 7.7 Verify reduced motion on a fresh session: `animation-name: none`, no pseudo element, the finished instrument, and the scope's edge painted by the border
- [x] 7.8 Guard the motion with `@media screen and (prefers-reduced-motion: no-preference)` and verify the computed guard is exactly that, so a print pass — which never scrolls — cannot print the instrument at the start of its range
- [x] 7.9 Verify the scope's edge is not painted twice: give the transparent `border-right` a selector that out-specifies `.block--dark .wire__track`, then confirm the border computes to `rgba(0,0,0,0)` with the motion active and to `--rule-dark` without it
- [x] 7.10 Run `just website-ink … gem/index.html` with motion active and verify all 24 widths still pass
- [x] 7.11 Run `impeccable detect --json` over the changed files again and verify the motion adds no new antipattern kind and no new finding against the `HEAD` baseline
- [x] 7.12 Record the motion in the surface brief's direction contract and the change's `design.md`, and state the three fallbacks (no view timelines, reduced motion, print) as a spec requirement rather than as a comment
