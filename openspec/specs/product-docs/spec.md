# product-docs Specification

## Purpose
The `emb` reference documentation surface (`website/docs/`) is the hosted form
of the command, configuration, scripting, operations, and benchmark facts that
`README.md` already carries. It exists so the landing page can stay a poster:
a reader who has decided to try `emb` gets the specifics without leaving the
site, and a reader who is still deciding is not asked to read a manual.

## Requirements

### Requirement: The docs surface is the hosted form of the reference

The site SHALL carry a documentation surface at `website/docs/index.html`.
`README.md` remains the source of truth; the docs surface MUST NOT introduce a
capability, command, reply shape, configuration key, or number the repository
does not support, and it MUST NOT contradict `README.md`, `BENCHMARK.md`,
`examples/scripts/`, or the shipped code.

#### Scenario: A documented command exists in the reference

- **WHEN** the docs surface presents a Redis command, its arity, or its reply shape
- **THEN** that command, arity, and reply shape appear in `README.md`

#### Scenario: A documented number carries provenance

- **WHEN** the docs surface presents a measured number
- **THEN** that number appears in `BENCHMARK.md` together with the machine and method or the reproduction command that produced it

#### Scenario: A single fact has one home

- **WHEN** a fact appears on both the landing page and the docs surface
- **THEN** the docs surface carries the complete form of the fact, the landing carries at most one line of it, and the two cannot disagree about a value

### Requirement: The docs surface is ordered for an operator who already runs Redis

The docs surface SHALL present its sections in this order: **Install → first
vector**, **Commands**, **Replies and protocol**, **Configuration**, **Lua API**,
**Operations**, **Benchmarks**, **Clients and framework integration**, **Status
and version**. Each section MUST be reachable by an in-page anchor so the landing
page can link to the section it means.

#### Scenario: A reader gets a vector without leaving the page

- **WHEN** a reader opens the docs surface with no prior knowledge of the project
- **THEN** the first section provides the install command, the start command, and a Redis client command whose reply is an embedding

#### Scenario: The order follows the reader's questions

- **WHEN** the sections are reviewed
- **THEN** commands precede configuration, configuration precedes the Lua surface, and the Lua surface precedes operations

#### Scenario: The landing can deep-link

- **WHEN** the landing page links to the docs surface
- **THEN** it targets a stable anchor for a specific section or command rather than the surface's top

### Requirement: The docs surface inherits the poster's world instead of forking it

The docs surface SHALL reuse the landing page's design system: the same token
set, the same self-hosted font families, the same text-size floors, the same
contrast rules, and the same reduced-motion and print behaviour. It MUST NOT
introduce a second palette, a fourth font, a card grid, rounded panels,
gradients, drop shadows, or a second accent colour. Where the shared stylesheet
hides or replaces a control at a breakpoint, the docs surface MUST carry the same
replacement the landing uses rather than losing the affordance.

#### Scenario: Tokens are not duplicated

- **WHEN** a value is needed on both surfaces
- **THEN** it is defined once in the shared stylesheet and consumed by both, and the docs-only stylesheet carries page-specific rules only

#### Scenario: The axis does not leak

- **WHEN** the docs surface is rendered
- **THEN** it carries no signal axis or spine geometry, because that geometry is derived from the hero composition the docs surface does not have

#### Scenario: The floors hold on the second surface

- **WHEN** the docs surface is rendered at the 1086px reference frame and on a 390px viewport
- **THEN** no technical text computes below 12px and 14px respectively, and every text colour meets WCAG AA against its own ground

#### Scenario: A breakpoint does not remove a control

- **WHEN** the docs surface is rendered below the width at which the shared stylesheet hides the primary masthead navigation
- **THEN** the masthead carries the same disclosure the landing uses, its destinations are reachable with the keyboard alone, and each is at least the committed target size

#### Scenario: A control is not reduced below the pointer minimum

- **WHEN** the docs surface renders a link or control at a scale this page sets for itself rather than inheriting the landing's
- **THEN** its rendered box still meets the 24x24 minimum, so shrinking a wordmark for a reference masthead cannot silently shrink its target

### Requirement: The docs surface needs no build step, no network, and no CDN

The docs surface SHALL be static HTML with relative asset paths. It MUST render
completely when opened over `file://`, MUST NOT require a bundler or a package
installation to view or to edit, and MUST NOT load any script, style, or font
from a third-party origin.

#### Scenario: Opened from disk

- **WHEN** `website/docs/index.html` is opened directly from the filesystem
- **THEN** its text, layout, and code specimens render, and no request leaves the filesystem

#### Scenario: Edited by hand

- **WHEN** a section is added or corrected
- **THEN** the change is complete without running a build, a generator, or a dependency install

### Requirement: The docs surface is scannable and accessible

The docs surface SHALL be structured for comprehension: a single `<h1>`, a
document outline that never skips a level, a table of contents or equivalent
in-page navigation, one heading per command or configuration topic, and code
specimens rendered with the site's existing code treatment. Every control MUST
be keyboard-operable, MUST have an accessible name, and MUST meet the site's
committed target size.

#### Scenario: A command is findable

- **WHEN** a reader looks for a specific command
- **THEN** that command has its own heading or definition row with a stable anchor, and its reply shape is shown as a code specimen

#### Scenario: Keyboard and screen reader

- **WHEN** a reader uses only the keyboard or a screen reader
- **THEN** in-page navigation, command entries, and code specimens are reachable and announced in document order

#### Scenario: No information depends on motion

- **WHEN** the reader prefers reduced motion
- **THEN** every section renders immediately and completely, with no content gated behind an animation

### Requirement: The docs surface states what it documents

The docs surface SHALL state the version it documents, generated from
`VERSION` rather than typed by hand, and SHALL state that the project is pre-1.0
with interfaces that may still move.

#### Scenario: The version cannot drift

- **WHEN** `VERSION` changes and the docs surface is regenerated
- **THEN** every version string on the docs surface reflects the new value and no hand-typed version remains

#### Scenario: The reader is warned

- **WHEN** a reader reaches the install section
- **THEN** on the same page they can learn that the project is pre-1.0 and that interfaces may change

### Requirement: The docs surface documents the Lua image story honestly

The docs surface SHALL document what the shipped scripts actually do. Where a
script consumes a model branch that a request path does not populate — the
`examples/scripts/siglip2.lua` image branch — the docs surface MUST say so
explicitly rather than presenting the feature as available.

#### Scenario: The zeroed branch is stated

- **WHEN** the docs surface describes the `siglip2` example
- **THEN** it states that the text branch is used and that `pixel_values` is fed as a constant zero tensor, so image input is not offered by that path

#### Scenario: An unimplemented input path is not advertised

- **WHEN** the docs surface describes supported inputs
- **THEN** it names only the input paths the shipped scripts and server implement

### Requirement: Script API reference and production guide

The documentation surface SHALL carry a script API reference covering every host function available to scripts, and a production scripting guide. The reference SHALL present functions grouped by module (`emb.embed`, `emb.run`/`emb.run_batch`, `emb.tokenize`, `emb.math`, `emb.similarity`/`emb.distance`, `emb.image`), and for each function SHALL state its signature, accepted operand forms (per-element array vs packed `bytes`), return shape, error conditions, and whether it is available only for models with an embedding or image configuration.

The production guide SHALL state, at minimum:

- the determinism and reply-cache contract, including that a script's reply must depend only on (model, script SHA1, args, text), and the one-KEY-plus-ARGV idiom for pairwise scoring;
- how to obtain embeddings (`emb.embed` / `emb.image.embed`) versus raw graph tensors (`emb.run`), and when each is appropriate;
- the packed-buffer workflow for large outputs, and that per-element materialization is the dominant cost when avoided;
- the configured limits (deadline, script size, tensor elements) and how they fail;
- the memory and observability characteristics of scripted models (session count, `script_workers`, how to see them in `EMB.STATS`/`MONITOR`).

Example scripts SHALL be classified as **reference** (maintained, exercised by CI against a testbed model) or **snippet** (illustrative, not maintained), and the docs SHALL say which is which.

#### Scenario: Every host function is documented

- **WHEN** the script API reference is read
- **THEN** each function exposed under `emb.*` by the sandbox is described with its signature, operand forms, and errors

#### Scenario: Similarity and distance semantics are explicit

- **WHEN** the reference documents `emb.similarity` and `emb.distance`
- **THEN** it states that `similarity` returns higher-is-more-similar and `distance` returns lower-is-closer, and lists each supported metric with its formula

#### Scenario: Maintained scripts are distinguishable

- **WHEN** a reader opens the examples directory through the docs
- **THEN** the maintained reference scripts and the illustrative snippets are separated, and the GLiNER2 extractor is listed as maintained

#### Scenario: Production constraints are stated

- **WHEN** the production guide is read
- **THEN** it states the determinism/caching contract, the limits and their failure modes, and how scripted traffic appears in `EMB.STATS` and `MONITOR`
