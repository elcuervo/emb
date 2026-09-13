---
target: website/ (emb product site)
total_score: 25
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:/Users/elcuervo/.herdr/worktrees/emb/website/website/index.html"
target_fingerprint: "sha256:79012da3ad6f4c7e6c47bac677ad2a402ce97f73498490e7298839c59d4bf8f5"
target_path: /Users/elcuervo/.herdr/worktrees/emb/website/website/index.html
timestamp: 2026-09-12T23-09-30Z
slug: website-index-html
---
# Critique — `website/index.html` (emb product site)

⚠️ DEGRADED: the parent held detector output before Assessment A completed. Dual-agent (A: reviewer · B: reviewer), both isolated from each other; the parent executed all detector and browser instrumentation because neither subagent surface exposed `exec` or `agent_browser`. Assessment A's design judgment was authored before any detector output reached it.

Target: `website/index.html` + `assets/css/styles.css`, `assets/js/main.js`. Server: `http://127.0.0.1:8080` (stopped). Browser: Chromium 1187 headless, DPR 1. Screenshots: `/tmp/emb-critique/shot-{390x844,390x844-motion,390-menu,1440x900,834x1112-tablet,834-top,320x568}.png`.

---

## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of system status | 2/4 | The only progress signal is the orange spine, and at 390×844 it draws at y1348 — off-screen. No active-section state, no loading state for the 0.8MB terrain. |
| 2 | Match system / real world | 3/4 | Redis vocabulary used faithfully; undercut by naming one stage `REDIS` on the slab and `SERVE` in the list, and never glossing `FLOATS OUT.` |
| 3 | User control and freedom | 3/4 | No scroll hijack, Escape closes the mobile menu, skip link present, reduced motion respected. |
| 4 | Consistency and standards | 3/4 | One accent, one button shape, one rule treatment — undercut by the same `SERVE`/`REDIS` split for one stage. |
| 5 | Error prevention | 2/4 | Nothing destructive; both CTAs leave the site with no on-page command sample to fall back on. |
| 6 | Recognition rather than recall | 2/4 | `01–04` + a hairline bracket is the only tether between the list and the diagram; on mobile the pairing spans a ~500px gap. |
| 7 | Flexibility and efficiency | 3/4 | Skip link, desktop nav, mobile menu, two CTAs. No on-page command. |
| 8 | Aesthetic and minimalist design | 3/4 | Genuinely restrained; the small end of the type scale compresses to 10–11px where restraint tips into strain. |
| 9 | Error recovery | 2/4 | No error surface exists; graceful degradation is real (all reveal CSS gated on `html.js`, so the no-JS view is complete) but there is no font/image failure affordance. |
| 10 | Help and documentation | 2/4 | The page explains the architecture and links out, but never shows `EMB minilm "hello world"` — the one line that proves the positioning. |
| **Total** | | **25/40** | **Acceptable — significant improvements needed** |

All ten heuristics applicable; maximum 40.

---

## Design Specificity Verdict

**Authored for `emb`, with one generic quadrant.** The composition *is* the product's data path: `.slab[data-slab=input|inference|embedding|serve]` labelled `TEXT / MODEL / VECTORS / REDIS`, threaded by one orange line, with `01 INPUT … 04 SERVE` notes. That diagram only makes sense for a server that takes text and emits vectors. The copy is the product's own vocabulary (`RESP/3`, `384D`, `ONNX`, `that speaks the Redis protocol`, Smart Batching / LRU Cache / Multi-Model / Lua Scripting mapping 1:1 onto PRODUCT.md's capability list). The bottom plate being a black **REDIS** slab is a joke only a Redis server can make.

Category-interchangeable: the six-row `icon + name + one-line description` feature list could belong to any "fast X server"; the terrain is a generic scale metaphor doing its work through the annotation `EMBED EVERYTHING FURTHER` rather than the image; and the genre itself (huge wordmark, mono metadata, one accent, rules not cards) is a known editorial-brutalist register. The pipeline diagram is the one element an imitator could not swap strings into.

**Deterministic scan** — `impeccable detect --json website/index.html`: 7 findings, all `warning`. `wide-tracking` ×2 (0.05em, 0.12em), `clipped-overflow-container` ×3 (`html`, `body`, `section.hero`), `overused-font` ("Primary font: inter"), `cream-palette` (rgb(243,240,232)). Adding `styles.css` surfaced one more: `overused-font` at `assets/css/styles.css:17`. **Zero true positives** — see false positives in Minor Observations.

**Visual overlays** — injection succeeded and the detector ran in the page: 7 overlay nodes rendered (cream-palette banner, page-frame outline, clipped-child strip, wide-tracking callout pinned over the landscape annotations) and the console reported `[impeccable] 4 anti-patterns found`: `clipped-overflow-container` (section.hero), `wide-tracking` (0.12em), `text-occlusion` ("a GitHub is 100% covered by overlapping text (p.hero__meta-b)"), `cream-palette`. The overlay ran in the headless session I control, **not in a browser window on your display** — there is no user-visible overlay to look at. `text-occlusion` appears only in the in-page run and is a false positive: at 834 the alleged occluder `p.hero__meta-b` occupies y101–186 while `.ghlink` sits at y977–1025, and `elementFromPoint` at the link's centre returns the link.

---

## Overall Impression

A finished, unusually disciplined poster that has one real hole in it: **the phone page is a reflow of the desktop page, not the mobile composition the brief specifies**, and the type scale quietly drops below the accessibility floor the project committed to in writing. Nothing here is broken; everything here is under-built at exactly the widths the brief cares most about. The desktop spread is the strongest thing in the repository — the orange axis holding to 0.09px across seven widths is engineering, not decoration — and the reveal/route/reduced-motion layer is genuinely correct. The gap between those two facts is the whole critique.

---

## What's Working

1. **The signal axis is an invariant, not a vibe.** Spine `x=182/364` of the plate SVG vs terrain fork `x=1124.3/2172` of the artwork agree to **0.09px or better at 2400, 1920, 1720, 1440, 1280, 1086 and 1001** (worst |error| 0.09 at 1920/2400; 0.01–0.02 elsewhere). The README's claim is true and survives past the stated 1720 cap.
2. **Motion is mechanical and property-disciplined.** No `@keyframes` anywhere; everything is `opacity`/`transform`/`translate`/`stroke-dashoffset`. The slab entrance rides `transform` while hover rides `translate` with its own delay slot (`transition-delay: .17s, .17s, 0s`), so a hover never waits out the 0.51s stagger — measured delays input 0s / inference .17s / embedding .34s / serve .51s. The reveal uses `translate` while note centring uses `transform: translateY(-50%)`, so the two never collide.
3. **Reduced motion is a real path.** Verified: all 12 `[data-reveal]` plus `.pipeline.is-live` land in one 11ms tick, `.route--signal` computes `stroke-dashoffset: 0` at every scroll position, no transitions run, hover still functions. Reveal CSS is gated on `html.js`, so the no-JS view is complete content.

---

## Priority Issues

### [P1] The committed type floor is violated — worst at the reference frame
- **What:** Measured computed sizes. Desktop 1086: footer **10.0px**, `hero__meta-b` **10.5**, `rule-head` **10.5**, `note__desc` **10.53**, `feat__desc` **11.08**, `anno` **11.08**, `note__num` **11.95**; 10.0–12.3px across the whole 1001–1240 band. Phones: `hero__meta-b` **11**, `rule-head`/`anno`/`footer` **12**, `note__num`/`hero__tech` **13**.
- **Why it matters:** `DESIGN.md` and `PRODUCT.md` both commit to "roughly 12–13px desktop / 14px mobile". These are the labels carrying the technical argument (`OPEN SOURCE EMBEDDING SERVER…`, `Raw text, documents, or images.`, `High-dimensional vectors`), in muted grey at 4.82:1 on a phone in daylight. The floor was never written as a floor: every annotation is `clamp(<10–11px min>, Xvw, <12–13px max)` (`styles.css:470, 587, 602, 677, 728, 859, 883, 888, 895`), so between 1001 and ~1240 the *minimum* governs.
- **Fix:** Raise every annotation clamp minimum to 12px and set explicit floors in the `max-width:1000px` block (`.hero__meta-b`, `.rule-head`, `.anno`, `.note__num`, `.hero__tech` → 14px; `.footer__copy` → 13px). Recover density by trimming tracking, not size.
- **Suggested command:** `/impeccable typeset`

### [P1] The mobile page is a reflow, not the briefed composition
- **What:** Two measured violations of `DESIGN.md`'s mobile paragraph. (a) At 390 the isometric stack occupies y1348–1835 and the four stage notes sit in one ruled list at y1863–2268 — the brief says "Technical annotations should move underneath each layer". (b) At ≤640 `.claim span{display:inline}` (`styles.css:899`) collapses the authored four-line break into **3 rendered lines** (measured: h1 104px tall, line-height 34.61px, all four spans `display:inline`; the render reads "A fast embedding / server that speaks / the Redis protocol."), against "The line breaks are important. Do not put this into one normal heading."
- **Why it matters:** The relational claim (`01` belongs to the INPUT slab) is the page's entire argument, and on the platform where a reader has most attention it is carried by four numbers across half a screen. The collapsed claim removes the editorial stacked-copy rhythm the brief calls out by name.
- **Fix:** Keep the ladder instead of replacing it: at `max-width:640px` give `.pipeline` `grid-template-columns: 55% minmax(0,1fr)`, drop the `.note{position:relative}` override so notes keep their `top: 11.8% / 32.1% / 56% / 78.6%` of the stack's height, and restore `.claim span{display:block}` with the mobile font-size tuned so four lines still fit 350px.
- **Suggested command:** `/impeccable layout`

### [P2] The orange spine breaks by 208px at tablet
- **What:** Measured spine vs route fork: 1001→2400 agree to ≤0.09px, then **834 → 225.18 vs 433.46 (−208.28px)**, 390 → 195 vs 203.53 (−8.53), 320 → 160 vs 166.99 (−6.99). Below 1000 the art offsets are literals (`right:-6vw; width:112vw`; `right:-12vw; width:124vw`) while the stacked pipeline spine follows the grid.
- **Why it matters:** The spine-becomes-ridge handover is the page's single organising idea and the README states the axis without a width qualifier. A 208px lateral jump at the most common tablet width reads as a bug, not a metaphor — and the 7–8.5px kink on phones is visible in the screenshot.
- **Fix:** Derive the offset from the spine instead of hard-coding it: one token `--spine: 25%` (tablet two-column) / `50%` (phone one-column) with `.landscape__art{ left: calc(var(--spine) - 0.5176*var(--art-w)) }`. The route SVG needs no change — it lives in the artwork's own pixel space.
- **Suggested command:** `/impeccable layout`

### [P2] The accent fails contrast wherever it becomes text or an indicator
- **What:** `#FF5A1F` on `#F3F0E8` = **2.74:1**. It is used for `:focus-visible{outline:2px solid var(--accent)}` (`styles.css:124`, needing 3:1 for non-text contrast, WCAG 1.4.11), for `.ghlink:hover{color:var(--accent)}` and the hot stage number at 13px (`styles.css:220`, `612`, needing 4.5:1, WCAG 1.4.3). Measured directly: with the note hot, `.note__num` computes to `rgb(255,90,31)` at 13px.
- **Why it matters:** The keyboard focus ring is the only way a keyboard user can see where they are, and it is the lowest-contrast element on the page. Hovering the one secondary CTA lowers its contrast from 17.28:1 to 2.74:1 — the interaction makes the text harder to read.
- **Fix:** Darken the focus ring and the accent-as-text cases (`#B03300`-ish clears 4.5:1 on paper) or pair the accent with a black 1px inner ring for the focus indicator. Keep `#FF5A1F` for surfaces, where it is not carrying text.
- **Suggested command:** `/impeccable audit`

### [P2] The pipeline interaction is one-way, mouse-only, and invisible
- **What:** Measured: `mouseenter` on `.note[data-note=input]` adds `is-hot` to the note and the slab and lifts the plate `0px -6px`; dispatching the same event on `.slab[data-slab=input]` changes nothing. `DESIGN.md` specifies "Hovering a pipeline layer should reveal or emphasize its annotation". The `focusin`/`focusout` handlers (`main.js:131–132`) can never fire — measured `tabindex: null` and **0 focusable descendants** in the note. `.note` has no cursor, underline or icon marking it as interactive.
- **Why it matters:** Either the diagram is a control or it is an illustration; today it is a control-shaped object with a one-way, mouse-only link to a panel that gives no sign of being interactive, plus a keyboard path that exists only in the source. On touch there is no equivalent at all.
- **Fix:** Bind the pairing both ways (`.slab[data-slab]` → its note), give `.note__label` a hairline underline or dotted decoration so the affordance is visible, and either give notes `tabindex="0"` with a visible focus style or delete the dead listeners.
- **Suggested command:** `/impeccable harden`

---

## Persona Red Flags

**Jordan (first-timer, engineering-adjacent):** The page never expands the name — the wordmark is `aria-hidden` decoration and no sentence says what emb is; the closest thing is `OPEN SOURCE EMBEDDING SERVER…` at **10.5–11px**. `FLOATS OUT.` is never glossed. Both CTAs read `Get Started` and both leave for `github.com/elcuervo/emb#quick-start`, so "how do I run it" is unanswered by the page he is on. The list heading says `SERVE` while the slab it points at says `REDIS` — read as "requires Redis" vs "is a server", which is the exact ambiguity the product must not create. `COMMUNITY` lands on the issue tracker.

**Casey (distracted mobile user):** At 390×844 the first screen is masthead → 11px meta block → 172px `emb` → a 3-line claim → sub → two stacked actions → the start of a six-row feature list. The diagram starts ~y1246 and the stack at y1348: she scrolls ~1.6 screens before the page's argument appears. **Nothing on the first screen moves** — the only load animation (`.sig`) draws at y1348, and no above-fold element carries `data-reveal`. Tap targets are all ≥44px, but nothing tappable reveals anything: the note↔slab pairing is mouse-only. `scroll-behavior` is `auto` at 390 (measured), so anchor jumps are fine. Tapping `Menu` overlays the wordmark. `.mobile-nav a:hover{color:var(--muted)}` (`styles.css:830`) means a tapped menu row sticks in its *lighter* hover colour — tapping reduces contrast from 17.28:1 to 4.82:1.

**Riley (stress tester):** Resizes across 1000/1001 and the plate jumps 335px → 460px wide in one pixel while the axis error goes 0.00px → −208.28px at 834. Toggles reduced motion mid-session: `main.js:11` caches `matchMedia` with no `change` listener, so the route keeps being scroll-drawn until reload while CSS now shows it statically. Tabs the page: skip link works, pipeline is `aria-hidden`, terrain is `alt=""` — but the four `.note` elements are unreachable and their `focusin` handlers inert, so the "keyboard focus" claim is not reproducible by tabbing. Zooms: every annotation clamp caps in `px`, so at ≤1000 `hero__meta-b` is a hard 12px (11px ≤640) and does not follow the reader's zoom preference the way the `vw`-based desktop value implies. Measures the orange line: **5.16px spine vs 6.18px ridge at 1920/2400**, a 20% weight step exactly where the README says they match (true where measured, false above ~1443px). Disables JS: content is complete — a genuine pass.

---

## Minor Observations

- **Hero entrance is one step of six.** `document.getAnimations()` at 120/300/600/1000/1600ms returns exactly one animation — `.sig | stroke-dashoffset | delay 150 dur 850`, complete ≈1000ms (inside the brief's 800–1200ms). The masthead, both annotation blocks, the giant wordmark, the CTA row have no load-time animation; `main.js`'s comment says they are "painted from the start", and there is no `@keyframes` in the stylesheet. `DESIGN.md`'s six-step hero entrance is not implemented. (`/impeccable animate`)
- **Doc drift, three items, all in the docs not the code.** `PRODUCT.md` says the page uses `terrain-v2.png`; it loads `terrain-matte.png` (the matte is derived from v2). `DESIGN.md` specifies `--muted: #77756F` = **4.05:1**, which would fail AA; `styles.css:43` ships `#6B6963` = **4.82:1** — the implementation is the compliant one. `README.md` says the counter caption is "omitted below 980px"; the code hides it at `max-width:1000px`.
- **The 1:1.33 frame is 13px over.** Measured at 1086×1448: document height **1461**, ratio **1.345** against the claimed 1.333. `CORRECTIONS.md`'s own arithmetic said "≈1460" and admitted it was never browser-verified.
- **Reduce-motion removal is not complete.** The `transition-duration: .001ms !important` wildcard (`styles.css:763–766`) does not zero `transition-delay`, so the .17/.34/.51s and `55ms × --i` delays survive with ~0 duration. Invisible only because the same block sets end states directly — any future rule relying on the transition to reach its end state would still wait out its delay.
- **False positives, with the proof.** `cream-palette` and `overused-font` are the committed world: DESIGN.md pins `#F3F0E8`, and Inter 900 is the masthead brand only — the working faces are Archivo and JetBrains Mono. All three `clipped-overflow-container` hits are the deliberate page frame (`html`/`body`/`.hero` `overflow:clip`); `scrollWidth − clientWidth = 0` at all 11 widths and the clipped regions carry no text. Both `wide-tracking` hits sit on authored-uppercase mono labels, not body copy. The four closed-menu anchors measuring 8×52 are Chrome's `content-visibility: hidden` layout for a closed `<details>`, not dead tap targets — 316×52 each when open, and hit-testing resolves correctly.
- **Deliberate bleeds measure correctly.** `.landscape__art` reaches +38px past the 320 viewport and +25.9px at 1440; the wordmark's SVG ink reaches x=321 at 320 (+1px); `.note{right:-10px}` puts the note boxes 10px past the right edge at 1001–1720. Interior text never overflows (`.note__desc` scrollWidth−clientWidth = 0), so nothing is clipped.
- **Decorative contrast is very low where it is not exempt.** Footer rule `--rule-soft` **1.34:1**, hairline leaders **2.14:1**, `--rule` **1.80:1**. Exempt as decoration, but 1.34:1 means the footer rule is effectively invisible on many displays.
- **Desktop masthead has no rule.** `.masthead{border-bottom:1px solid transparent}` (line 228); only ≤1000 sets a visible rule. With `position: sticky` and an opaque background, content's top line vanishes under an invisible edge.
- **`<ol class="pipeline__notes">` with `list-style:none` and no `role="list"`** — Safari/VoiceOver drop list semantics while the markup claims order, and the visible `01–04` are decorative spans.
- **Wordmark is not cropped on mobile.** 390×172 at 390 (right edge exactly 390), 320×141 at 320. `DESIGN.md` allows the final letter to crop; the implementation fits exactly. Full-bleed but safe.
- **`wordmark-note` is `display:none` at ≤1000**, so `SAME PROTOCOL. A MORE SEMANTIC WORLD.` — the annotation DESIGN.md puts inside the `b` counter — is desktop-only.
- **`main.js:55`** re-adds `is-ready` at 1236ms after it already landed at 21ms (measured). Idempotent, one wasted timer.
- Desktop nav anchors (38–85 × 32) and the skip link (158 × 32) pass WCAG 2.2 AA target size (24×24) but not 44px guidance; both are mouse/keyboard-only.
- **No page errors and no console messages** at any viewport, on any run, including the instrumentation runs.

**Verification gaps:** WebKit/Safari untested; no real touch emulation; `env(safe-area-inset-*)` untested by construction; no print stylesheet exists to test; `terrain-matte.png` alpha not sampled under the annotation boxes; the route-on-ridge fit is unverifiable because the polyline was traced from the poster's mountain while the shipped artwork is a generated cut-out; the external GitHub anchors (`#quick-start`, `#commands`, `#configuration`) were not resolved.

---

## Questions to Consider

1. If the desktop page is deliberately *one* two-column spread with exact poster measurements, why does ≤1000 become a conventional stack? The brief's mobile paragraph describes a vertical pipeline with the line running through it — what would the phone page look like if the mobile pipeline were *authored* rather than reflowed?
2. The giant `emb` bleeds 5.4vw on desktop and 1px on a phone: is a wordmark that fits exactly the "oversized, almost leaving the viewport" mandate, or a poster element the responsive layout quietly rescued into safety?
3. Is animating only the orange spine an intentional editorial statement ("things move because data is moving") or the residue of a six-step sequence that got harder to maintain? `DESIGN.md` lists six entrance steps; the render ships one. Which is the design?
4. `PRODUCT.md` says "The interface is the integration." Should the homepage contain `EMB minilm "hello world"` — the one line that proves the entire positioning — instead of two buttons that both leave for GitHub?
5. If hovering a pipeline layer is the briefed interaction, why is the only wired direction note→slab? Is the diagram a control or an illustration — and if illustration, why is half of it listening to the mouse?
