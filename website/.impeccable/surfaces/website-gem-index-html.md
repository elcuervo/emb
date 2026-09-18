---
version: 1
slug: "website-gem-index-html"
primary_target: "website/gem/index.html"
related_targets: []
---

## Scope and visitor mode

**Read.** A client surface at `website/gem/index.html` for `emb`, the
simple yet powerful self-hosted inference server that speaks the Redis protocol. The visitor
already writes Ruby and has decided the product is worth a look; this surface
answers "what does the gem do between my call site and the wire, and how much
does a scope cost" without theatre.

The gallery explains the *server's* batcher and measures it. This surface
explains the *client's* deferral and measures nothing. The two must not read as
the same page in two places.

## Audience, job, and action

Ruby engineers who already operate Redis and are weighing the gem against
decoding float32 bytes by hand. They read the way they read a library's README:
they arrive with a call site in mind, they want to know what it sends and when,
and they judge the gem by whether it does something the raw client cannot.

Their job: get many vectors without writing a pipeline, and not get bitten by
laziness. The surface's success is that a reader leaves knowing one rule — a
deferred value issues nothing until it is used, so loaders are created before
they are consumed — and the four wire shapes that follow from it.

Action: copy the two setup lines, or leave for the gem's page on RubyGems.

## Proof and content

Every mode, default, argument shape and failure clause is sourced from
`gems/emb/lib/emb/`, `gems/emb-server/`, `BENCHMARK.md`, or `README.md`. The
shipped code remains the source of truth; this surface is its explanation and
must not contradict it.

Eight sections, in the order a reader asks:

1. **What it is** — a thin wrapper: the proxy registry, `unpack('e*')`, the pool.
2. **The request path** — call site → proxy → the instance's `pool` connections
   → the server, and what happens past the pool's parallelism.
3. **The four wire shapes** — `lazy: false`, `:multi`, `:batch`, and
   `Emb.multi { }`, each with its command and its count.
4. **The one rule** — create-then-consume, wrong and right side by side.
5. **When it fails** — fail-closed, `reconnect_attempts: 0`, why a read timeout
   is terminal, and the pre-send retry across instances.
6. **Scope is per thread** — the Rack, ActiveJob, Sidekiq and Shoryuken
   middleware, and what is cleared.
7. **Two gems, two jobs** — `emb` the client, `emb-server` the distribution.
8. **The exact commands** — the argument shapes and the reply shapes.

**Honesty requirement.** The sandbox runs the server, not Ruby, so no reading of
the gem is available to a page. Every count on this surface is a count of
commands the shipped code sends; the surface carries **no** duration, rate or
ratio, and every diagram is captioned as the shape the client sends rather than
as something observed.

## Chosen direction and memorable moment

**The ledger with an instrument.** The four execution modes are one ruled
ledger, one row each, each row carrying the same instrument column: an encoding
of the commands that row puts on the wire. The memorable moment is the column
read vertically — six narrow marks, one wide mark, three stacked marks, one wide
mark — because the whole page's argument is legible in that one glance, and the
geometry is the argument.

Each mark is one command. Its width is the texts that command carries. The
column is therefore not a time axis and cannot be misread as one: it is a
fingerprint of the client's dispatch. The accent marks the single call that
carries every text, which is what `:multi` and `Emb.multi { }` have and what the
eager and `:batch` rows do not.

The reading experience comes from the poster's own rhythm: paper for
explanation, the dark ground for the instrument, a massif band for the reading
break, and code typeset as code.

**The instrument performs the dispatch.** The page's one authored movement: as
a row comes into view its marks go onto the wire in the shape that mode sends
them — six ticks one after another, one stroke that settles from the rule's grey
into the accent, three strokes at the same instant, one stroke. The colour turn
is the message: a mark that turns accent is a single call carrying every text,
which is why the eager row's six and the batch row's three never turn. Each row
owns its timeline, so every row performs while the reader is looking at it
instead of playing once above the fold, and the same grammar rehearses on
create-then-consume. Everything is inside

```css
@supports (animation-timeline: view()) {
  @media screen and (prefers-reduced-motion: no-preference) { … }
}
```

so an engine without view timelines, a reader who prefers reduced motion, and a
print pass all render the finished ledger this page shipped before it moved.

## Constraints

- **Inherit the world, do not fork it.** Same tokens, same three self-hosted
  families, same type floors, same contrast rules, same reduced-motion and print
  behaviour. No fourth font, no second accent, no new palette value.
- **One new atom family.** `.wire` — the ledger row and its instrument. Anything
  else is an existing atom: `.plate`, `.block`, `.block__head`, `.sec__h`,
  `.block__deck`, `.cap`, `.code`, `.ridge`.
- **No axis.** No spine, no `--fold`, no route. The page is not the hero.
- **Static, and script-free after the motion.** No script, no sandbox call, no
  `demos.js`, no host or endpoint named. The page renders complete with scripting
  disabled and from `file://`, and the motion added to the instrument is
  scroll-driven CSS rather than a small observer module, so that stays true.
- **No performance claim.** No `ms`, no `µs`, no `req/s`, no ratio, at any
  heading, caption, comment or code sample.
- **One movement, and it explains.** No second effect on the page, no time-based
  entrance, no hover decoration, no reveal on every block. Every animated
  element encodes a fact about how its mode dispatches, and the reduced-motion,
  no-support and print states are the finished page rather than a degraded one.
- **Atoms only**: no cards, gradients, radii, decorative shadows, glass, kicker
  above a heading, or coloured side border.
- Type floor 12px desktop / 14px mobile measured on the text runs; every colour
  checked against its own ground; every heading level in order; the diagrams
  reachable as text through a caption rather than by colour alone.
- Print: the instrument column must survive `@media print` as its end state.

## Unresolved decisions

- Whether the create-then-consume pair reuses the same instrument column or gets
  its own two-lane drawing. Both keep the encoding rule; the reuse is cheaper and
  the reader already knows how to read it.
- Whether `emb-server` closes the page or sits beside the client in section 1.
  The closing plate is the current plan; the reader who wants the binary has to
  scroll, which is the cost.
- Whether the modes ledger is a `<ol>` or a definition list. Both are in the
  world; the choice follows which one reads better at 390px.

None of these change the section order, the inheritance rules, or the encoding
rule.

## Direction contract

THESIS: The client's execution is taught as a **ledger with an instrument
column** — one row per mode, the same column of command marks down the page —
rather than as four code samples with prose between them. It refuses the category
default of a README restated as a webpage, and it refuses the gallery's own idiom
of a measured bar, because there is nothing here to measure.

OWN-WORLD: Identical to the rest of the site and nothing added — `#F3F0E8` paper,
`#0B0B0B` ink, `#FF5A1F` only as a surface and only on the call that carries
every text, `#C23D00` as accent ink, the three self-hosted families, hairline
rules, tracked mono labels, and the `#111110` / `#292823` inversion. With all
content removed what remains is a ruled ledger with a left label column and a
right instrument column, and the page's paper/dark block rhythm.

STORY: The visitor learns that `Emb[:model]["text"]` in a lazy mode sends
nothing, that the first use of the value sends one command for everything in the
scope, that the shape of the call site therefore decides the count, and that a
failed batch fails closed rather than retrying work the server may have done.
They leave able to read their own code and predict its round trips.

FIRST VIEWPORT: The tracked mono masthead carrying the project mark, the tagline
rule, the four surface entries and the get-started action; then the plate head —
`FIG. 1`, the surface title, one sentence naming what the page answers in the
visitor's terms, and the figure line carrying the three modes, the pool, the
batch size and the two gems. Nothing decorative above the fold and no diagram
competing with the first sentence.

SECTION ORDER: what it is → the request path → the four wire shapes → the one
rule → when it fails → scope is per thread → two gems, two jobs → the exact
commands. The modes precede the rule, because the rule is the consequence of the
deferral the modes establish; the failure section follows the rule, because
fail-closed is what makes the deferral safe; the distribution closes, because a
reader who wants a server rather than a client has by then read what the client
is. Every section is a full-bleed block whose ground alternates paper/dark,
carries its own bar where it needs one, and sits on the page's one measure.

FORM: Refinement of an established world with a new surface inside it
(`new-work.md` §3, first case). The visual system is fixed by `DESIGN.md` and the
gallery's implementation; the open work is the section order and the instrument
encoding, both resolved here and in `design.md` rather than fanned out into a
tournament.

FINISH: unreviewed and undocumented is unfinished. This build ends with the
detector run once over the changed files, the measured floors and contrast on
both grounds, a load with scripting disabled, the instrument's motion sampled as
scroll positions, its three fallbacks rendered, and captures at 1086 and 390.

---
## Amendment — the read-mode switch (change `website-demos-tldr`)

The gallery's `/demos` switch also loads on `/gem`. A ruled bar inside `<main>`
carries one button that sets `data-tldr` on `<html>`, remembers it in
`localStorage` (`emb.tldr`), and is revealed only when scripting can honour it.
On this surface the gist keeps the plate, the `.tldr` one-paragraph claim,
`THE FOUR WIRE SHAPES` (the instrument) and `THE EXACT COMMANDS`, and hides the
argument sections and the terrain break. The button is the page's own material —
hairline rule, mono label, square bordered control that inverts on press — and
adds no colour, font or claim. Detector parity with `HEAD` is recorded in the
demos brief.

---
## Amendment — three mechanism strips (2026-09)

The client page now opens three of its sections with the gallery's mechanism
strip, drawn from `assets/js/mechanism.js` (the module the demos share, which
does not pull in the sandbox client): *your call site → a scope, per thread →
the command it sends* in `WHAT YOU ARE LOOKING AT`; *five connections → the
scope's commands → the wire* in `THE SCOPE IS PER THREAD`; and *emb, the client
→ emb-server, the binary → one version* in `TWO GEMS, TWO JOBS`. Each is drawn
complete and animates once as it scrolls into view, and stands still under
`prefers-reduced-motion`. No command is sent: the page has no sandbox, so these
are diagrams of the page's own account of the gem.
