---
version: 1
slug: "website-docs-index-html"
primary_target: "website/docs/index.html"
related_targets: []
---

## Scope and visitor mode

**Read.** A documentation surface at `website/docs/index.html` for `emb`, the
self-hosted text-embeddings server that speaks the Redis protocol. The visitor
has already decided the product is worth a look; this surface answers "how do I
run it, what exactly does it do, and what will bite me" without theatre and
without leaving the site.

The landing page is the poster; this is the reference. Detail that previously sat
on the landing (command tables, the YAML block, reply-format prose, the `emb-top`
figures, the version strings) has exactly one home, and it is this one.

## Audience, job, and action

Backend, platform, and application engineers who already operate Redis. They read
command documentation the way they read Redis's: they arrive with a specific
question, they want the exact command, key, or reply shape, and they judge the
project by whether the answer is precise.

Their job: get a vector out of a model, from inside the stack they already have.
The surface's success is that a reader gets from zero to `EMB minilm "hello
world"` returning bytes in one screen, and can then answer any later question
(config, scripting, ops, throughput) without leaving the page.

Action: copy an install command, then navigate by anchor.

## Proof and content

Every command, reply shape, configuration key, and number is sourced from
`README.md`, `BENCHMARK.md`, `examples/scripts/`, or the shipped code. The
repository README remains the source of truth; this surface is its hosted form
and must not contradict it.

Nine sections, in the order the questions arrive:

1. **Install → first vector** — `install.sh`, Docker (`elcuervo/emb`), the
   `emb-server` gem; start command; one `redis-cli` call and its reply.
2. **Commands** — the full set with arity and reply shape: `EMB`, `EMB.MULTI`,
   `EMB.MODELS`, `EMB.INFO`, `EMB.STATS`, `EMB.READY`, `MONITOR`,
   `EMB.CACHE.FLUSH`, `EMB.SAVE`, `EMB.SCRIPT *`, `EMB.HELP`, `INFO`,
   `CONFIG GET|SET`, `AUTH`, `HELLO`, `PING`.
3. **Replies and protocol** — raw little-endian float32 bytes by default, the
   self-describing `VALUES` envelope, RESP2 default with `HELLO 3` for RESP3, and
   how a client unpacks the bytes.
4. **Configuration** — the YAML keys, model options, batching (the 1 ms window,
   the token budget), caching and persistent snapshots.
5. **Lua API** — `EMB.SCRIPT LOAD`, `EMB.EVAL`, `EMB.EVSHA`, `emb.tokenize.encode`,
   `emb.run`, `emb.math.*`, `KEYS`/`ARGV`, the sandbox limits, the per-model SHA1
   cache, and the five shipped examples.
6. **Operations** — `EMB.READY` as a probe, connection lifecycle, `ERR busy`
   backpressure, `EMB.STATS`, `INFO` sections, `MONITOR`, and `emb-top`.
7. **Benchmarks** — the measured tables with their reproduction commands.
8. **Clients and framework integration** — redis-cli, redis-py, redis-rb, the
   Ruby gem, and what a pool or framework hook looks like.
9. **Status and version** — the version, pre-1.0, MIT, platforms.

**Honesty requirement.** Where a shipped script consumes a model branch a
request path does not populate, this surface says so. `examples/scripts/siglip2.lua`
states that its image branch is absent and `pixel_values` is fed as zeros; the
surface documents the text branch and states the limitation rather than
advertising image input.

## Chosen direction and memorable moment

**The reference is set in the poster's own hand.** The memorable moment is the
first command specimen: the same `redis-cli EMB minilm "hello world"` line, the
same `\x7c\x8e\x80\xbd…` bytes, and the same ruled ledger around it that the
landing uses — so the reader understands within one screen that this is the same
project, not a separate docs site.

The reading experience worth staying in comes from measure and rhythm, not
decoration: a narrow prose column for explanation, full-width ruled rows for
command reference, and the landing's own paper/dark block rhythm — a solid
section bar and a full-bleed ground — so the page opens and beats like the poster.

## Constraints

- **Inherit the world, do not fork it.** Same token set, same three self-hosted
  font families, same type floors, same contrast rules, same reduced-motion and
  print behaviour. No fourth font, no second accent, no new palette value.
- **No axis.** The orange spine is the hero's own derived geometry
  (`--fold: 65.31%`, computed from the landing's two-column spread). This surface
  has no such composition, so it carries no spine, no `--fold`, and no second
  orange line.
- **Atoms only**: the shared `.block`/`.block--paper`/`.block--dark` ground and
  `.block__head` bar, ruled ledgers, the code specimen treatment, the annotation
  form. No cards, no gradients, no radii, no decorative shadows, no glass.
- **No `--fold`, and no redefined token.** Anything both surfaces need is defined
  once in the shared stylesheet; this surface's own stylesheet carries
  page-specific rules only.
- **No build step, no dependency, no CDN, and `file://` works.** Static HTML with
  relative paths; opening the file from disk renders everything.
- Type floor 12px desktop / 14px mobile, with the smallest technical text
  measured rather than assumed, and every colour checked against its own ground.
- Every heading level in order; every command has a stable anchor so the landing
  can deep-link to it; every control keyboard-reachable and at or above the
  target floor.
- No coloured `border-left`/`border-right` above 1px as a code or callout
  treatment, and no kicker above a heading (craft floor).

## Unresolved decisions

- Whether the table of contents is a sticky rail or a single index block at the
  top. Both satisfy scannability; neither changes the tokens or the section order.
- Whether command reference rows are `<dl>` ledgers or a `<table>`. Both are in
  the world; the choice follows which one reads better at 390px.
- Whether the operations section embeds the `emb-top` capture or only describes
  it, given the landing already shows the render.

None of these change the section order, the inheritance rules, or the constraint
set.

## Direction contract

THESIS: The documentation is the poster's own hand applied to a reference — one
rule, one measure, one solid section bar per idea, and the landing's black/cream
block rhythm — rather than a separate docs site in the project's colours. It
refuses the category default of a sidebar-plus-cards docs layout by using the
landing's own ruled-ledger, code-specimen and inverted-block atoms, so a reader
recognizes the project from the first command block.

OWN-WORLD: Identical to the landing and nothing added — `#F3F0E8` paper, `#0B0B0B`
ink, `#FF5A1F` only as a surface, `#C23D00` as the accent ink, the three
self-hosted families, hairline rules, tracked mono labels, and the `#111110` /
`#292823` block inversion and code grounds. With all content removed, what remains
is a ruled ledger, a solid section bar, and the page's black/cream block rhythm.

STORY: The visitor gets a vector out of the model in the first screen, then finds
any later answer — a command's arity, a reply's shape, a configuration key, a
script's limits, a throughput figure with its reproduction command — by anchor,
without leaving the surface and without reading prose that exists only to fill
space. They leave knowing the version, the licence, the platforms, and that the
project is pre-1.0.

FIRST VIEWPORT: A tracked mono masthead carrying the project mark and one link
back to the landing; an `h1` naming the surface; a one-line statement of what the
page is; and the install → first vector block, whose code specimen shows the
`redis-cli` call and its float32 reply. Nothing decorative above the fold, and no
capture or illustration competing with the first command.

SECTION ORDER: install → first vector; commands; replies and protocol;
configuration; Lua API; operations; benchmarks; clients and framework
integration; status and version. Commands precede configuration, configuration
precedes the Lua surface, and the Lua surface precedes operations, because that
is the order a reader asks the questions. Every section is a full-bleed block
whose ground alternates paper/dark (install paper, commands dark, and so on to
status paper), carries its own solid bar, and has a stable anchor; the first one
is reachable without scrolling past anything.

FORM: Refinement of an established world, with a new surface inside it
(`new-work.md` §3, first case). The visual system is fixed by `DESIGN.md` and the
landing's implementation; the open work is the information architecture (settled
above) and the page-level composition, and it is resolved here and in `design.md`
rather than fanned out into a tournament.

FINISH: unreviewed and undocumented is unfinished. This build ends with the
detector run once over the changed files, the measured floors and contrast on
both surfaces, a `file://` load with no third-party request, and both surfaces
captured at 1086 and 390.

---
## Amendment — the terrain bands

**Scope: three `.ridge` sections and nothing else.** Every heading, ledger, code
specimen, and section order above is unchanged; the reading experience gains a
band of the landing's massif between the dense middle and a closing massif
before the footer.

**What changes.** The page carries the landing's own terrain as two `.ridge`
bands — `terrain-slope.png` between Replies and Configuration, `terrain-pass.png`
between Operations and Benchmarks — and the full `terrain-matte.png` as a
closing band before the footer. Each band is a different framing of the same
keyed matte, so the two do not repeat. The component is defined once in
`styles.css`, which both surfaces link, and carries neither `--fold` nor
`--band`, so it cannot pull the hero's geometry onto a page that has no hero.

**Constraints carried over, and how each is met**

- *Inherit the world, do not fork it.* Same cut-out, same paper ground, same
  engraving, same tokens. The bands are new framings of the one keyed matte,
  declared in `published-tree.py`; one new atom.
- *No axis, no second orange line.* The bands are monochrome. They carry no
  route, no spine, no `--fold`; the orange never appears on this surface.
- *Atoms only.* The band is a `.ridge` ground and an `object-fit: cover` crop of
  one shipped image — no card, gradient, radius, shadow, or glass. The two mid
  bands and the closing band are the same asset read through different crops.
- *Type floor and contrast.* The bands carry no text. `aria-hidden="true"` and
  `alt=""`; the massif is decoration and the sections around it keep their own
  headings and anchors.
- *No third-party request, `file://` works.* The images are same-origin files
  referenced by relative path; no script, no CDN, no build step.
- *Print.* `.ridge` is added to the print block's `break-inside: avoid` list, so
  a band is not split across pages.

**Detector.** `impeccable detect --json` over the changed files — the two
surfaces and `styles.css` — was run once after the build and compared finding by
finding against the same run in a worktree of `HEAD`: identical counts and
identical findings. The bands add nothing the detector flags.

**Why monochrome.** The route is the landing's spine; on a reference page it
would be the second orange line the brief forbids, and it would compete with the
one accent the ledgers already spend. The engraving carries the break on its
own.
