The site should feel like a **neo-brutalist technical poster that happens to function as a product homepage**. It should look less like a conventional SaaS landing page and more like a hybrid of an engineering manual, an experimental editorial spread, and a systems-software interface.

The overall composition is deliberately oversized, asymmetric, and information-dense, but still highly structured. The visual hierarchy should come primarily from typography, scale, grid alignment, black-and-white contrast, and one strong orange accent.

# Overall visual direction

The page uses a warm off-white background rather than pure white. The surface should feel slightly tactile and printed, almost like archival paper or an engineering datasheet.

The dominant colors are:

```text
Background:     warm cream / off-white
Primary text:   near-black
Accent:         vivid orange
Secondary text: muted gray
```

A reasonable palette would be:

```text
--background: #F3F0E8;
--foreground: #0B0B0B;
--accent:     #FF5A1F;   /* surfaces: the button fill, the signal line */
--accent-ink: #C23D00;   /* the accent as text or as a ring: 4.66:1 on the paper */
--muted:      #6B6963;   /* 4.82:1 on the paper; #77756F is only 4.05:1 */
--rule:       #B8B5AC;
--rule-soft:  #D5D1C6;
--leader-c:   #A9A69D;
```

`#FF5A1F` is 2.74:1 against the paper. It is a surface colour: it may fill a
button or draw the signal, and it may never be the only thing that
distinguishes a focus ring, a hover state or a label. Those use
`--accent-ink`.

The page should avoid gradients, soft shadows, glass effects, rounded SaaS cards, and glossy illustration.

The feeling should be:

```text
industrial
editorial
technical
raw
precise
confident
slightly strange
```

The site should visually communicate:

> small software, serious engineering.

---

# Typography

Typography is the main visual element.

There should be three distinct typographic roles.

## Display type

The giant `emb` wordmark and major headlines use an extremely heavy grotesk sans-serif.

It should be:

* very bold
* wide or slightly condensed
* low contrast
* almost poster-like
* tightly kerned
* allowed to become part of the composition itself

Examples:

```text
emb

A fast
embedding server
that speaks the
Redis protocol.
```

The giant `emb` word should be dramatically oversized—large enough to almost leave the viewport horizontally.

The typography should feel closer to printed poster design than a normal website heading.

## Monospace type

Technical labels, metadata, navigation, diagrams, annotations, and system text use a monospace font.

Examples:

```text
BYTES IN.
VECTORS OUT.

RESP3
384D
ONNX

01 INPUT
02 INFERENCE
03 EMBEDDINGS
04 SERVE
```

The mono font should be relatively small and compact.

It gives the page the feeling of:

* command-line software
* engineering labels
* schematics
* technical documentation

## Body type

Descriptions can use either a clean sans-serif or mono depending on section.

Use conventional body typography sparingly.

The page should generally prioritize short statements over paragraphs.

---

# Page grid

The entire page should be based on a strong desktop grid.

Conceptually:

```text
12 columns
```

But the layout should not look like a conventional Bootstrap grid.

Instead, elements should intentionally span unusual proportions:

```text
logo             1–2 columns
hero typography  8–10 columns
annotations      2–3 columns
diagram           5–6 columns
feature list      3–4 columns
```

The page should have generous outer margins, roughly:

```text
desktop:
32–48px left/right

large desktop:
48–64px
```

Vertical rhythm should be spacious.

Thin rules are used frequently to structure areas without creating cards.

---

# Header

The header should be minimal and horizontal.

Left side:

```text
emb
```

Immediately beside the logo:

```text
BYTES IN.
VECTORS OUT.
```

with a thin vertical divider between them.

Center navigation:

```text
DOCS
GITHUB
```

Right side:

```text
Get Started →
```

inside a black rectangular button.

The button should be square-edged.

No border radius.

No gradient.

No icon except the arrow.

Something like:

```text
┌─────────────────┐
│ Get Started  →  │
└─────────────────┘
```

The header should feel like part of an engineering drawing rather than a floating navbar.

---

# Hero composition

The hero should occupy almost the entire first viewport.

It is intentionally asymmetrical.

The main components are:

```text
top left:
BYTES IN.
VECTORS OUT.

top center:
small open-source description

top right:
RESP3
384D
ONNX

center:
HUGE "emb"

bottom left:
main value proposition

bottom center/right:
exploded technical diagram
```

The hero should not follow the common:

```text
headline
paragraph
buttons
illustration
```

layout.

Instead, it should feel like several visual systems coexist on the page.

---

# Giant `emb` wordmark

The most important visual element is the massive lowercase:

```text
emb
```

It should occupy approximately:

```text
70–90% of viewport width
```

and roughly:

```text
300–450px height
```

on a large desktop.

It should visually dominate the page.

The letters should extend almost from the left edge to the right side.

The word is not merely a logo.

It is the layout.

Some annotations should partially overlap or sit inside the negative spaces of the letters.

For example, inside or near the `b`:

```text
SAME
PROTOCOL.
A MORE
SEMANTIC
WORLD.
```

This annotation should be small and monospace.

That scale contrast is important:

```text
giant grotesk typography
+
tiny technical annotations
```

---

# Hero top annotations

Above or around the giant wordmark, place small fragments of technical information.

Top left:

```text
BYTES IN.
VECTORS OUT.
```

Top center:

```text
OPEN SOURCE
EMBEDDING SERVER
FOR THE STACK
YOU ALREADY RUN.
```

Top right:

```text
RESP3
384D
ONNX
—
```

These should feel like labels placed on an industrial object.

They should not be styled as badges or chips.

Just text.

---

# Orange signal line

A thin vivid orange vertical line runs through the page.

This is an important recurring visual motif.

It begins around the top of the hero and passes through the architecture diagram.

Conceptually:

```text
             │
             │
             │
             ↓

            emb

             │
             │
             ↓

          pipeline

             │
             │
             ↓

          output
```

The orange line acts like:

* a signal path
* a data path
* an execution trace
* a visual spine

It should be approximately:

```text
2–4px wide
```

It may bend or route around objects later in the composition.

The line should always feel purposeful.

Not decorative.

---

# Main product statement

Below the giant `emb`, aligned to the left, place the main proposition:

```text
A fast
embedding server
that speaks the
Redis protocol.
```

The line breaks are important.

Do not put this into one normal heading.

The stacked typography should feel almost like editorial poster copy.

Below it, smaller text:

```text
Any Redis client works unchanged. Pre-1.0, MIT licensed.
Turn text — or any tensor a script builds —
into vectors.
```

Keep the paragraph narrow.

Approximately:

```text
360–420px max width
```

---

# Primary CTA area

Directly below the value proposition:

```text
[ Get Started → ]    View on GitHub  ◉
```

The primary button uses the orange accent.

It should have a small black offset/shadow underneath, almost like printed neo-brutalist UI:

```text
┌──────────────────────┐
│ GET STARTED       →  │
└──────────────────────┘
   █████████████████
```

But keep this subtle.

The GitHub action should not be another filled button.

It should be simple text with:

* underline or rule
* GitHub icon
* strong contrast

---

# Architecture diagram

The right side of the hero contains a vertically exploded technical diagram.

This is one of the signature visual elements.

It should look like a physical stack of computational layers.

Four primary layers:

```text
01 INPUT
02 INFERENCE
03 EMBEDDINGS
04 SERVE
```

The layers appear as floating square slabs.

Each slab is shown in slightly isometric perspective.

Something like:

```text
        ┌─────────────┐
        │    TEXT     │
        └─────────────┘

            gap

        ┌─────────────┐
        │    MODEL    │
        └─────────────┘

            gap

        ┌─────────────┐
        │   VECTORS   │
        └─────────────┘

            gap

        ┌─────────────┐
        │    REDIS    │
        └─────────────┘
```

The actual visual should be much more abstract and architectural.

---

# Pipeline layer 1: Input

The top plate should be light/off-white.

It represents input text.

Its surface contains scattered small points or marks.

Label:

```text
TEXT
```

Right-side annotation:

```text
01 INPUT

Raw text, or any tensor
a script builds.
```

The annotation should be small and mono.

---

# Pipeline layer 2: Inference

The second layer is darker.

It can show a grid or matrix pattern.

Label:

```text
MODEL
```

Right-side text:

```text
02 INFERENCE

ONNX Runtime,
batched runs.
```

The visual should imply computation.

A technical grid works well here.

---

# Pipeline layer 3: Embeddings

The third plate should visually communicate high-dimensional vectors.

Use:

* small vertical bars
* dots
* columns
* matrix-like structures
* little 3D blocks

Label:

```text
VECTORS
```

Right-side annotation:

```text
03 EMBEDDINGS

High-dimensional
vectors (e.g. 384D).
```

This section should feel slightly more complex than the previous two.

---

# Pipeline layer 4: Serve

The bottom plate should be almost black.

Label:

```text
REDIS
```

Right-side text:

```text
04 SERVE

Speak Redis.
RESP2 or RESP3.
```

This creates a visual progression:

```text
input
↓
compute
↓
representation
↓
protocol
```

---

# Pipeline signal

The orange line runs directly through all four slabs.

It visually connects:

```text
TEXT
│
MODEL
│
VECTORS
│
REDIS
```

This is a critical part of the composition.

Without the line, the diagram becomes a generic exploded view.

With the line, it looks like a signal moving through a system.

## Stacking order

The four plates are painted back to front: SERVE first, INPUT last. The camera
sits about 19 degrees above the horizon, so the highest plate is the nearest
one and has to occlude the plate below it. Painted the other way round, each
lower plate's top face eats the front skirt of the one above it in a 73 x 24
unit wedge around the spine — a seam that only stays invisible because the
signal line covers its centre. `data-depth` records the level (1 farthest,
4 nearest) and `tools/gen-isometric.py` emits the plates in that order, so a
regeneration cannot silently revert it.

The activation stagger is keyed by plate name, never by position, so document
order and animation order stay independent.

Note that the plate and its note name the same stage differently on purpose:
the plate carries the protocol (`REDIS`), the note carries the stage
(`04 SERVE`). That split is the brief's, not an inconsistency.

---

# Feature list

Under the main product copy, place a compact feature list.

Heading:

```text
EVERYTHING SERVER-SIDE
────────
```

Then individual capabilities.

Example:

```text
✧ ONNX Runtime
  Fast, portable inference.

☻ Hugging Face
  Download models by repo id.

▱ Smart Batching
  High throughput, low latency.

◉ LRU Cache
  Serve hot embeddings fast.

▦ Multi-Model
  Load and switch models on the fly.

▤ Lua Scripting
  Your code around the model call.
```

Each item contains:

```text
icon
feature name
one-line description
```

But these should not be cards.

They should simply be stacked vertically.

Spacing between them:

```text
18–24px
```

Icons should be simple black line art.

No colored product logos unless absolutely necessary.

---

# Icon style

Icons should look like diagrams from a manual.

They should be:

* simple
* monochrome
* geometric
* approximately 24–32px
* line-based
* slightly playful but not cartoonish

Examples:

```text
ONNX       star / network node
HF         simplified face / model
Batch      stacked layers
Cache      cylinder
Multi      four squares
Lua        document/script
```

Keep stroke weight consistent.

---

# Secondary slogan

Around the central/lower part of the composition, place:

```text
EMBED
EVERYTHING
FURTHER
—
```

This is not the primary headline.

It acts like a small editorial annotation.

---

# Mountain image

The bottom third contains a large black-and-white mountain photograph.

The image should:

* span almost the full width
* have very strong black/white contrast
* feel like a halftone editorial photograph
* partially bleed into the bottom edge
* avoid glossy photographic treatment

A mountain works because it communicates:

* scale
* infrastructure
* elevation
* technical ambition

The image should appear almost screen-printed.

You can achieve this with:

```css
filter:
  grayscale(1)
  contrast(1.4);
```

Optionally add:

* noise texture
* halftone overlay
* very light dithering

Avoid smooth photographic color.

---

# Orange route across mountain

The same orange signal line from the architecture section continues into the mountain.

It becomes a path moving across the terrain.

For example:

```text
architecture
     │
     │
     ▼
    /\
   /  \____
  /        \____
```

This is visually important because it connects abstract infrastructure with the physical metaphor of scale.

It should feel like:

```text
execution path
+
topographic route
```

## Data branches — removed

The route used to fork three branches above the fork point, each carrying a
README fact the pipeline diagram cannot show (`BLOB OR VALUES` / `BYTES STAY`
`BYTES`, `HELLO 3` / `RESP2 RISES TO RESP3`, `1 MS WINDOW` / `SHARED ONNX
RUNS`), staged 14% apart so the facts arrived in sequence. They are gone: three
spurs crossing the peak turned the massif into a diagram of itself. The route
is the trunk alone — one line arriving somewhere — and the three facts now sit
in the blocks beside the command they describe. The two slogans,
`EMBED EVERYTHING FURTHER` and `HIGHER DIMENSIONS`, stay as the only margin
labels on the terrain.

---

# Supporting mountain annotation

On the right side of the mountain:

```text
HIGHER
DIMENSIONS
BRIGHTER
APPLICATIONS
—
```

Again, small mono typography.

Do not turn this into a headline.

`BLOB OR VALUES`, `HELLO 3` and `1 MS WINDOW` were the three branch
annotations (see **Data branches — removed** above); `EMBED EVERYTHING
FURTHER` and `HIGHER DIMENSIONS` stay as they are. Both are margin labels, not
callouts: they sit in the paper beside the massif and never point at anything.

---

# The terrain as a reading break

The massif is not only the landing's full stop. The reference and every gallery
plate reuse it — the same cut-out, the same paper ground, the same engraved
black-and-cream — as a band of rock between text blocks. The world is not forked
and no second asset enters it:

```html
<section class="ridge ridge--band" aria-hidden="true">
  <img class="ridge__art" src="../assets/img/terrain-slope.png"
       width="1200" height="150" alt="" decoding="async" fetchpriority="low">
</section>
```

`ridge--band` is an 8:1 bottom-anchored stretch of the ridge. There are
thirteen of them — `slope`, `crag`, `saddle`, `foothill`, `pass`, `scarp`,
`shoulder`, `west`, `cragw`, and the mirrored `range`, `descent`, `outcrop`,
`bluff` — so every divider on the reference and the gallery is its own
framing of the same massif: no two pages show the same picture, and the
reference's two bands are `slope` and `pass`. `ridge--full` is the closing
statement, the whole massif from `terrain-matte.png`, exactly as the landing
closes on it — taller on a phone, where the cut-out is cropped to the peak
rather than shrunk to a 49px strip. The component lives in `styles.css`, both
surfaces link it, and it carries neither `--fold` nor `--band`: the hero's
geometry does not follow it onto a page that has no hero.

**No orange route here, and that is the constraint, not an omission.** The
reference brief carries no second orange line, and a gallery plate spends its
one signal on meaning. A band is a photograph of rock on paper: the eye gets a
rest and the argument is unchanged. The route stays the landing's, where it is
the page's spine.

**Its placement is the reading order.** On the documentation, one band before
Configuration and one before Benchmarks bracket the dense middle, and the full
massif closes the page. On a gallery plate, one band falls after the dark `TRY
IT` plate and before the explanation, so the reader leaves the instrument, sees
the ground, and comes back to prose. `.ridge` is listed with `.terrain` in the
print block's `break-inside: avoid`, so a band never splits across pages.

---

# Decorative crosshair

There should be a simple technical cross symbol near the lower-right portion of the architecture area:

```text
   │
───┼───
   │
```

This acts as visual punctuation.

It reinforces the engineering/drawing-board aesthetic.

Use similar schematic marks sparingly.

---

# Footer

The footer should be extremely thin.

Left:

```text
© 2026 emb. made with ☠️ by elcuervo
```

Right:

```text
BYTES IN. VECTORS OUT. / EMB
```

No giant multi-column footer.

No social-link wall.

No newsletter signup.

It should feel like the lower margin of a technical drawing.

---

# Interaction design

The site should not be static, but animation must feel mechanical.

The overall rule is:

> things should move because data is moving.

Not because the website wants to show off animation.

---

# Hero load animation

On initial load:

1. header fades or snaps into position
2. tiny annotations appear
3. giant `emb` wordmark reveals rapidly
4. orange signal line grows vertically
5. pipeline slabs appear one at a time
6. CTA and secondary content appear last

Timing should be fast.

Something around:

```text
total hero entrance:
800–1200ms
```

Avoid slow luxury-site fades.

## As built

The sequence is driven by one `--e` ordinal per element (90ms apart), gated on
the `is-ready` flag the page sets once the display face is in place:

```text
masthead          --e 0   delay 0ms
hero annotations  --e 1   delay 90ms
hero tech block   --e 2   delay 180ms
giant emb         --e 3   delay 270ms
prose + CTAs      --e 4   delay 360ms   (settles at 780ms)
spine draw        150ms delay, 850ms   (settles at 1000ms)
```

Each element moves 10px and fades over 280/420ms on an exponential ease-out,
from an already-visible default. The slab stagger is not part of the load
sequence: it runs when the pipeline crosses 0.85 viewport heights, which on a
desktop is immediately and on a phone is after the reader has scrolled to it.

---

# Scroll behavior

As the user scrolls:

* the orange line continues downward
* diagram layers slightly separate
* labels become visible
* small technical annotations appear
* the mountain route draws progressively

Animation should be tied to scroll position but not aggressively scrubbed.

Avoid full-page scroll hijacking.

---

# Architecture animation

When the pipeline enters the viewport:

```text
TEXT
   ↓
MODEL
   ↓
VECTORS
   ↓
REDIS
```

Each layer activates in sequence.

Suggested behavior:

```text
1. INPUT slab highlights
2. orange signal passes downward
3. INFERENCE slab activates
4. vector layer animates
5. final Redis layer activates
```

Total duration:

```text
1000–1500ms
```

The animations can repeat only when explicitly triggered, or run once.

---

# Hover states

Hovering a pipeline layer should reveal or emphasize its annotation.

For example:

```text
[MODEL]
```

might increase contrast while:

```text
ONNX Runtime
batched runs
```

becomes fully opaque.

Do not make layers rotate or float around.

Motion should remain constrained.

---

# Button behavior

Primary buttons should feel physical.

Default:

```text
orange surface
black text
small black offset
```

Hover:

```text
translate: 2px 2px
offset shadow reduces
```

Pressed:

```text
translate: 4px 4px
shadow disappears
```

This gives a neo-brutalist tactile response.

---

# Texture

The site should not be perfectly digital.

Use a tiny amount of visual imperfection.

Possible techniques:

```text
subtle paper grain
very faint noise
slight print texture on huge letters
halftone imagery
imperfect diagram linework
```

The large `emb` letters can contain subtle speckling.

Keep it restrained.

The site should still feel modern.

---

# Borders and rules

Instead of cards, use rules.

Example:

```text
────────────────────────
```

Use 1px lines in:

```text
black
or muted gray
```

Rules can divide:

* navigation
* feature sections
* labels
* metadata
* footer
* diagrams

This gives the page the feeling of a printed technical document.

---

# Content density

The composition should alternate between:

```text
VERY LARGE
```

and:

```text
very small technical detail
```

Avoid medium-sized everything.

The design becomes interesting through contrast.

Examples:

```text
EMB
```

could be 300px tall, while:

```text
RESP3
384D
ONNX
```

could be 14px.

Likewise:

```text
A fast
embedding server...
```

should be very large, while the supporting copy is compact and understated.

---

# Visual hierarchy

The user should perceive the page approximately in this order:

```text
1. emb
2. embedding server + Redis
3. exploded architecture
4. Get Started
5. BYTES IN / VECTORS OUT
6. implementation details
7. mountain / scale metaphor
```

The details should reward exploration without competing with the main idea.

---

# Desktop behavior

The desktop version should feel intentionally oversized.

Do not constrain everything inside a 1200px SaaS container.

The primary composition should comfortably use:

```text
1440–1800px
```

wide screens.

Maximum page width could be:

```text
1600–1800px
```

with some elements extending beyond conventional content boundaries.

The giant `emb` word should scale using something like:

```css
font-size: clamp(12rem, 31vw, 32rem);
```

or equivalent.

---

# Tablet

Tablet should retain the experimental hierarchy.

Do not simply scale everything down proportionally.

Possible structure:

```text
header

giant emb

value proposition

architecture

feature list

mountain
```

The giant word can still partially exceed the viewport.

The architecture becomes centered underneath it.

---

# Mobile

Mobile should remain dramatic.

Do not turn the site into a generic stacked product page.

Hero:

```text
emb

BYTES IN.
VECTORS OUT.

A fast
embedding server
that speaks Redis.

[ GET STARTED → ]
```

The giant `emb` could remain oversized enough that part of the final letter is cropped.

The architecture becomes vertical.

Example:

```text
INPUT
  │
  ▼
INFERENCE
  │
  ▼
EMBEDDINGS
  │
  ▼
REDIS
```

The orange line remains visible throughout: it runs through the isometric
stack, and on phones it appears again in the gap between the last plate and
the annotation list, which it then ends behind.

Technical annotations sit beside their layer, not in a list at the end of the
page. From 641px up the four notes share a row with the plate they describe
(their tops are 11.8%, 32.1%, 56% and 78.6% of the stack's own height, so each
one is centred on its own top face). Below 640px four annotated layers cannot
share a 350px measure at the 14px floor — the bands are 22% of a stack that is
only 249px tall — so the annotations become a ruled list directly under the
stack, bound to the diagram by the `01–04` numbers the plates also carry. That
is the one place the phone layout diverges from this section, and it is a
measured constraint rather than an unexamined reflow.

---

# Implementation philosophy

The site should be built primarily using:

```text
HTML
CSS Grid
SVG
CSS transitions
lightweight JavaScript
```

Avoid unnecessary canvas or WebGL.

The distinctive look should come from:

```text
typography
layout
linework
motion
contrast
```

not rendering technology.

SVG is ideal for:

* pipeline lines
* signal path
* exploded architecture
* crosshairs
* icons
* mountain path overlay

---

# Component structure

A useful component hierarchy could be:

```text
<HomePage>

  <Header />

  <Hero>
    <TechnicalTagline />
    <HeroWordmark />
    <HeroAnnotations />
    <ValueProposition />
    <HeroActions />
    <PipelineDiagram />
  </Hero>

  <FeatureSummary />

  <LandscapeSection>
    <SignalPath />
    <MountainImage />
    <LandscapeAnnotations />
  </LandscapeSection>

  <Footer />

</HomePage>
```

The architecture diagram can internally use:

```text
<Pipeline>
  <PipelineLayer type="input" />
  <PipelineLayer type="inference" />
  <PipelineLayer type="embedding" />
  <PipelineLayer type="serve" />
  <SignalLine />
</Pipeline>
```

---

# Key reusable design primitives

Create reusable components for the visual language.

## TechnicalLabel

```text
RESP3
384D
ONNX
```

## Annotation

```text
SAME
PROTOCOL.
A MORE
SEMANTIC
WORLD.
```

## SectionRule

```text
EVERYTHING SERVER-SIDE ─────────
```

## PipelineNumber

```text
01
02
03
04
```

## FeatureRow

```text
[icon] Smart Batching
       High throughput, low latency.
```

## SignalLine

The orange line connecting sections.

## BrutalistButton

Square button with physical offset shadow.

## TerrainBand

The landing's massif reused as a reading break on the reference and the
gallery: a full-bleed `.ridge` section whose `.ridge__art` is one framing of
the one keyed matte. `ridge--band` is an 8:1 bottom-anchored stretch of ridge
(`terrain-slope.png`, `terrain-crag.png`, `terrain-saddle.png`, … — thirteen
of them, one per divider, named in the markup); `ridge--full` is the whole
massif (`terrain-matte.png`) as the landing closes on it. It carries no
`--fold` and no route, so a page without a hero gets the ground without the
spine.

```html
<section class="ridge ridge--band" aria-hidden="true">
  <img class="ridge__art" src="assets/img/terrain-slope.png"
       width="1200" height="150" alt="" decoding="async" fetchpriority="low">
</section>
```

---

# Accessibility

Despite the experimental layout, the semantic document structure should remain straightforward.

Use:

```html
<header>
<nav>
<main>
<section>
<h1>
<h2>
<footer>
```

The giant `emb` wordmark should not replace the real `h1`.

The real semantic heading should describe the product.

Animations must honor:

```css
prefers-reduced-motion
```

When reduced motion is enabled:

* signal line appears immediately
* diagram layers do not animate
* hover functionality remains available
* no critical information depends on motion

Maintain proper contrast.

Small mono annotations must remain readable.

Do not push them below:

```text
12px desktop
14px mobile
```

These are floors, not averages: every technical label — the hero annotations,
the plate labels, the stage numbers and descriptions, the feature descriptions,
the landscape annotations, the section rules and the footer — is clamped so it
can never compute below the floor for its breakpoint. Below 1000px they are
set explicitly. A poster scaling with `vw` must not drag its smallest text
under the floor on the way.

---

# What the site should not become

Do not introduce:

```text
purple gradients
3D glowing spheres
AI particles
feature card grids
rounded SaaS panels
giant testimonial sections
pricing tables
customer logos
enterprise sales CTAs
animated blobs
glassmorphism
```

The site should not look like a general-purpose AI company.

It should look like `emb`.

---

# The demos gallery

`emb.is/demos` is the same world used for instrumentation instead of persuasion.
The concept is **an engraved atlas of a mind**: Poe's century is the century of
the steel engraving, the phrenological plate and the star chart, and the site's
poster vocabulary is already that. The gothic is in the *subject* — the
passages, the work titles, the years — and never in the decoration.

## The instrument rules

1. **Paper explains, the dark plate instruments.** Each plate mirrors the
   landing: the five teaching sections on paper, the interactive apparatus on
   `#111110`.
2. **One signal, spent on meaning.** Every plate has exactly one coloured
   thing — the query, the highlighted neighbours, the similarity, the morphed
   point set. On paper the signal is `--accent-ink`; on the plate it is
   `--accent`. No second hue is ever spent on decoration.
3. **The atlas is drawn, not rendered.** Marks are small filled circles — a star
   chart, not glowing bubbles — so density reads as stipple. The graticule is a
   ruled grid, the cluster regions are hand-named mono caps placed on their own
   ring, and nothing animates without being asked.
4. **Every plate is captioned with its real figures.** `FIG. 3 — THE SEARCH ·
   2 782 PASSAGES INDEXED · all-MiniLM-L6-v2 · 384 DIMENSIONS · INT8 VECTORS ·
   RECALL@10 0.9541` is read from the index's manifest, never typed. The
   caption's labels are caps and its values keep their own case, because a model
   name is a proper noun. The caption is the requirement — claims derive from
   the index — worn as the aesthetic.
5. **Passages are quotations.** The display face for the text, the mono voice
   for `THE TELL-TALE HEART · 1843`. Emphasis is a rule and the accent, never a
   second colour and never italics-as-gothic.
6. **Numbering is part of the title.** A plate is numbered (`II. The atlas`)
   rather than carrying a tracked-caps eyebrow above its headline: the number
   belongs to the plate, and a kicker over a hero headline is a shape this site
   does not use.

## The teaching sections

Every plate presents the same five sections in the same order, and they are the
whole curriculum:

```text
WHAT YOU ARE LOOKING AT   — what the plate shows and why it is here
TRY IT                    — the instrument, on the dark plate
WHAT JUST HAPPENED        — the path from text to vector to distance
WHY IT MATTERS            — the real-world job this stands in for
THE EXACT COMMANDS        — the argv that ran, including the reply form
```

The section headings are identical across plates, so the gallery reads as one
apparatus rather than six pages, and the commands shown are the commands that
were issued — built at run time from the argv the plate sent.

## Motion

The site's motion contract holds unchanged: **things move because data is
moving.** The gallery adds exactly one movement that is the argument — the model
lens re-placing the same passages under a second model — and one short precise
move where a query lands on the atlas. `prefers-reduced-motion` collapses both
to a cut, no result depends on an animation having run, and no plate animates
without being asked.

# Core visual metaphor

The entire site is built around one conceptual diagram:

```text
TEXT
 │
 ▼
INFERENCE
 │
 ▼
VECTOR
 │
 ▼
REDIS
```

The orange signal line represents that transformation.

The giant `emb` word represents the system responsible for the transformation.

The architecture slabs make the process tangible.

The mountain at the bottom transforms the same signal into a metaphor for scale.

The repeated statement:

```text
BYTES IN.
VECTORS OUT.
```

reduces the entire product to the simplest possible explanation.

That should be the defining design principle of the site.

The final impression should be:

> **a small, opinionated systems project presented with the confidence of a piece of industrial infrastructure, not an AI startup.**

