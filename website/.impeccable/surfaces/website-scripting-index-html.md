---
version: 1
slug: "website-scripting-index-html"
primary_target: "website/scripting/index.html"
related_targets:
  - "website/docs/index.html"
  - "website/index.html"
---

## Scope and visitor mode

**Read.** A reference surface at `website/scripting/index.html` for `emb`, the
simple yet powerful self-hosted inference server that speaks the Redis protocol.
The visitor has already decided the product is worth a look and is now writing a
script; this surface answers "what exactly can this Lua call, what does it run
inside, what does it cost, and how is a reply cached" without theatre and without
leaving the site.

It is the complete home of the scripting fact. The landing's scripts block argues
the shift in one specimen (Persuade); the demos plate "the model is a function"
teaches the idea (Experience/Read); the docs surface's `05 · Lua scripting` is the
narrative introduction and links here. This surface is the full property set in
one place, so `product-docs`'s "a single fact has one home" rule holds.

## Audience, job, and action

Backend, platform, and application engineers who already operate Redis and are
writing a pre/post-processing script around a model call. They read the way they
read a language reference: they arrive with a call to write, they want the exact
signature, the tensor form, the reply shape, the budget, and what happens on a
miss, and they judge the surface by whether the answer is precise.

Their job: turn `model(input) → output` into `model(fn(input)) → output` and not
get bitten by the sandbox, the budgets, or the cache. Success is that a reader
finds the one function they need, understands what a script may and may not
touch, and knows that a reply is content-addressed and invalidated by an upgrade.

Action: copy the load-then-call pair, then navigate by anchor.

## Proof and content

Every signature, budget, command, and reply shape is sourced from
`internal/script/`, the `script-*` specs, `examples/scripts/`, and `README.md`.
The shipped code remains the source of truth; this surface is its complete form
and must not contradict it. The version is stamped from `VERSION` by
`tools/stamp-version.py`, never typed.

Eight sections, in the order the questions arrive:

1. **The script surface** — `KEYS`/`ARGV`, one evaluation per request, one return
   per text, and one worked example.
2. **Commands** — `EMB.EVAL`, `EMB.EVSHA`, `EMB.SCRIPT LOAD|EXISTS|FLUSH` with
   arity and reply shape.
3. **Preloading from config** — the `scripts:` entry forms, `emb.script.config`,
   and boot validation.
4. **The sandbox** — what is stripped, the fresh-state guarantee, determinism,
   and lazy resource allocation.
5. **The host API** — every `emb.*` and `json.*` function, grouped by job.
6. **Tensors and replies** — the input spec and dtypes, the packed form, and the
   Lua-to-RESP conversion grammar.
7. **Budgets** — the source, deadline, per-tensor, per-evaluation, output, and
   cache-key caps, each with its value.
8. **Caching and identity** — the per-model SHA1, the reply-cache key
   composition, and what a version bump does to a warm cache.

**Honesty requirement.** Where the surface states a cost or a limit, it states the
shipped value; where a host function is gated by model capability
(`emb.embed`, `emb.image.*`), it says so and describes the `type()` check rather
than implying the call always exists.

## Chosen direction and memorable moment

**The reference is set in the docs surface's own hand.** The memorable moment is
the host-API ledger: a ruled `dt`/`dd` ladder where the left column is the exact
call and the right column is what it does — the same ruled number-and-note entry
the landing and docs use, applied to a language surface. A reader recognizes the
project from the first row.

The reading experience worth staying in comes from measure and rhythm, not
decoration: a narrow prose column for explanation, full-width ruled rows for the
API, and the landing's paper/dark block rhythm — a solid section bar and a
full-bleed ground — so the page beats like the poster and the docs surface.

## Constraints

- **Inherit the world, do not fork it.** Same token set, same three self-hosted
  font families, same type floors, same contrast rules, same reduced-motion and
  print behaviour. No fourth font, no second accent, no new palette value, no new
  stylesheet — it links `styles.css` and `docs.css` and nothing else.
- **No axis.** The orange spine is the hero's own derived geometry. This surface
  has no hero, so it carries no spine, no `--fold`, and no second orange line.
- **Atoms only**: the shared `.block`/`.block--paper`/`.block--dark` ground and
  `.block__head` bar, the ruled `.doc-cmds` ledger, the `.doc-note` aside, the
  `.code` specimen treatment, the `.tldr` line. No cards, no gradients, no radii,
  no decorative shadows, no glass.
- **No third-party request, no build step, `file://` works.** Static HTML with
  relative paths; the only script is the shared `tldr.js`.
- Type floor 12px desktop / 14px mobile, every colour checked against its own
  ground; every heading level in order; every section anchored; every control
  keyboard-reachable and at or above the target floor.
- No coloured `border-left`/`border-right` above 1px as a code or callout
  treatment, and no kicker above a heading (craft floor).

## Unresolved decisions

- Whether the host API's long `dt` signatures read better as a `<dl>` ledger or a
  two-column table at 390px. Both are in the world; the ledger is used now.
- Whether to reconstruct the old 1.x→emb availability history as a column. The
  maintainer chose to drop it; the surface makes no per-feature availability
  claim against old versions.

Neither changes the section order, the inheritance rules, or the constraint set.

## Direction contract

THESIS: The scripting reference is the docs surface's own hand applied to a
language — one rule, one measure, one solid section bar per idea, and the
landing's black/cream block rhythm — rather than a second docs site in the
project's colours. It refuses the category default of a sidebar-plus-cards API
browser by using the landing's ruled-ledger and code-specimen atoms, so a reader
recognizes the project from the first `dt` row.

OWN-WORLD: Identical to the landing and the docs surface and nothing added —
`#F3F0E8` paper, `#0B0B0B` ink, `#FF5A1F` only as a surface, `#C23D00` as the
accent ink, the three self-hosted families, hairline rules, tracked mono labels,
and the `#111110` / `#292823` block inversion and code grounds. With all content
removed, what remains is a ruled ledger, a solid section bar, and the page's
black/cream block rhythm.

STORY: The visitor sees the shift, finds the exact call, reads what it runs
inside and what it may cost, and leaves knowing that a reply is content-addressed
and that a version bump invalidates a warm cache — without leaving the surface
and without reading prose that exists only to fill space.

FIRST VIEWPORT: The shared masthead with the **Scripting** nav item current; an
`h1` naming the surface; one paragraph stating the shift
(`model(fn(input)) → output`); the pre-1.0 note naming the single version; and
the contents. Nothing decorative above the fold.

SECTION ORDER: surface → commands → preload → sandbox → host API → tensors and
replies → budgets → cache. What a script is precedes how it is called; how it is
called precedes what runs inside; the environment precedes the API; the API
precedes the forms it produces; the forms precede what they cost; and the cost
precedes identity. Every section is a full-bleed block whose ground alternates
paper/dark, carries its own solid bar, and has a stable anchor.

FORM: Refinement of an established world, with a new surface inside it
(`new-work.md` §3, first case). The visual system is fixed by `DESIGN.md`, the
landing, and `docs.css`; the open work is the information architecture (settled
above) and the page-level composition.

FINISH: unreviewed and undocumented is unfinished. This build ends with the
detector run once over the changed files, the measured floors and contrast at
1086 and 390, and a `file://` load with no third-party request.
