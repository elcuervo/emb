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
--accent:     #FF5A1F;
--muted:      #77756F;
--rule:       #B8B5AC;
```

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
TEXT IN.
FLOATS OUT.

RESP/3
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
TEXT IN.
FLOATS OUT.
```

with a thin vertical divider between them.

Center navigation:

```text
DOCS
GITHUB
MODELS
COMMUNITY
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
TEXT IN.
FLOATS OUT.

top center:
small open-source description

top right:
RESP/3
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
TEXT IN.
FLOATS OUT.
```

Top center:

```text
OPEN SOURCE
EMBEDDING SERVER
FOR A MORE
VECTOR-NATIVE WORLD.
```

Top right:

```text
RESP/3
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
Drop-in compatible. Production ready.
Turn text, images, and more into vectors
at massive speed.
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

Raw text, documents,
or images.
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
optimized execution.
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
vectors, e.g. 384D.
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

Speak Redis (RESP/3).
Fast. Familiar. Powerful.
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

---

# Feature list

Under the main product copy, place a compact feature list.

Heading:

```text
BUILT FOR REAL SYSTEMS
────────
```

Then individual capabilities.

Example:

```text
✧ ONNX Runtime
  Fast, portable inference.

☻ Hugging Face
  Thousands of models, instantly.

▱ Smart Batching
  High throughput, low latency.

◉ LRU Cache
  Serve hot embeddings fast.

▦ Multi-Model
  Load and switch models on the fly.

▤ Lua Scripting
  Extend, compose, automate.
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
© 2026 emb. Open source, forever.
```

Right:

```text
TEXT IN. FLOATS OUT. / EMB
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
optimized execution
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
RESP/3
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
5. TEXT IN / FLOATS OUT
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

TEXT IN.
FLOATS OUT.

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

The orange line remains visible throughout.

Technical annotations should move underneath each layer.

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
RESP/3
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
BUILT FOR REAL SYSTEMS ─────────
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

Do not push them below roughly:

```text
12–13px desktop
14px mobile
```

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
TEXT IN.
FLOATS OUT.
```

reduces the entire product to the simplest possible explanation.

That should be the defining design principle of the site.

The final impression should be:

> **a small, opinionated systems project presented with the confidence of a piece of industrial infrastructure, not an AI startup.**

