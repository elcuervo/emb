# embedding-demos Specification

## Purpose

A gallery of small, honest demonstrations that let a developer who does not work
with vectors watch text become an embedding, watch that embedding retrieve by
meaning, and — across more than one model — see why the choice of embedding
model is a real decision rather than a detail.

## Requirements

### Requirement: Every demo teaches the same five things in the same order

Each demo SHALL present, in one fixed order: what the reader is looking at, the
interaction itself, what just happened (the path from text through a model to a
vector to a distance), why it matters (the real-world job the demo stands in
for), and the exact commands the demo issued. A demo MUST NOT offer an
interaction without the mechanism that produced it and the commands that ran it,
and the section headings MUST be the same across demos so the gallery reads as
one curriculum rather than a set of unrelated toys.

#### Scenario: A demo carries all five sections

- **WHEN** any demo page is rendered
- **THEN** it presents what it is, the interaction, the mechanism, the real-world use, and the commands, in that order

#### Scenario: The mechanism is named, not implied

- **WHEN** the mechanism section is read
- **THEN** it states that text is tokenized, encoded by a named model, pooled and normalized into a fixed-length vector, and compared by a distance, rather than describing the result as magic

#### Scenario: The commands are the ones that ran

- **WHEN** a demo issues a command to the sandbox
- **THEN** the command shown to the reader is the command that was issued, including the model name and the reply form

### Requirement: The corpus is fixed and the visitor's text is ephemeral

Every demo SHALL search a corpus committed to the repository and embedded before
deployment. Text a visitor submits MUST be used only to answer that request and
MUST NOT be stored, added to a shared corpus, or made visible to another
visitor. A demo MUST NOT introduce a server-side write, a visitor account, or
mutable shared state, and the sandbox's read-only posture MUST be preserved.

#### Scenario: Submitted text leaves no trace

- **WHEN** a visitor submits text to a demo
- **THEN** the text is used for the request and is not persisted, indexed into the gallery's corpus, or returned to any other visitor

#### Scenario: The corpus cannot change at runtime

- **WHEN** the gallery is running
- **THEN** no visitor action adds, removes, or modifies an item in a demo's corpus

#### Scenario: No demo requires an account

- **WHEN** a demo is used
- **THEN** it asks for no credential, no identity, and no stored preference

### Requirement: The search is a real vector search over the committed index

Each retrieval demo SHALL run an exact top-k search against a vector index over
the committed corpus, built from vectors the sandbox's own model produced. A
demo MUST NOT present a precomputed or hard-coded ranking as a live result. The
query MUST be embedded by the same model that embedded the corpus, and a
disagreement MUST fail legibly rather than return meaningless neighbours.

#### Scenario: The ranking is computed, not stored

- **WHEN** a visitor runs a query
- **THEN** the ranking is produced by searching the index with the query's vector, and a query the corpus was not built to anticipate still returns the nearest items

#### Scenario: Both sides use one model

- **WHEN** a corpus is embedded and later queried
- **THEN** the same model and the same normalization produce both sides, and the demo reports which model

#### Scenario: A model mismatch fails loudly

- **WHEN** a demo's index and its query model disagree, or the index is missing
- **THEN** the demo states the failure instead of rendering a ranking

### Requirement: A demo degrades honestly when the sandbox is unavailable

When the sandbox cannot be reached, a demo SHALL state that condition and MUST
NOT fabricate a reply, a vector, or a ranking. Every part of the demo that does
not depend on the live server — the corpus, the explanation, the mechanism, the
commands — SHALL remain readable.

#### Scenario: The unavailable state is stated

- **WHEN** a demo's live call fails or the sandbox reports itself unavailable
- **THEN** the page states that the sandbox cannot be reached and offers a retry, and no result is shown

#### Scenario: Static content survives the failure

- **WHEN** the sandbox is unreachable
- **THEN** the demo's explanation, corpus description, and commands are still present and readable

### Requirement: A demo's explanation survives without scripts

A demo page's prose, its mechanism, its corpus description, and its commands
SHALL render without JavaScript. The interaction is an enhancement: with
scripting unavailable the page MUST read as a complete explanation of the demo
with the interaction absent, and MUST NOT read as an error.

#### Scenario: The page is complete without scripts

- **WHEN** a demo page is loaded with scripting disabled
- **THEN** its explanation, mechanism, and commands are visible and it states that the interactive part requires scripting

### Requirement: The demos run against the models the sandbox reports

Every model a demo names SHALL be resident on the sandbox and reported by its
metadata surface. Where a demo compares models it MUST show that vectors from
two different models are not interchangeable, because each model defines its own
space. A demo MUST NOT name a model the sandbox does not serve.

#### Scenario: Every named model is loaded

- **WHEN** a demo names a model
- **THEN** the sandbox reports that model as loaded, and the page's model list is derived from the server rather than typed

#### Scenario: The comparison teaches incomparability

- **WHEN** a demo presents two models over one corpus
- **THEN** it states that the two models' vectors are not comparable, and demonstrates that the same text occupies different neighbourhoods under each

### Requirement: Demo claims derive from the index and the server

Every number a demo displays — a dimension, an index size, a model name, a
precision, a count — MUST be read from the committed index or from the server's
metadata rather than transcribed into the page, so a rebuilt index or a changed
model set cannot leave the page describing the previous build.

#### Scenario: A dimension is read, not typed

- **WHEN** a demo displays a vector's dimension or an index's size
- **THEN** the value comes from the index or the server's metadata and changing the index changes the page without a page edit

#### Scenario: A stale page is caught

- **WHEN** the committed index and the values a page carries disagree
- **THEN** the site's own check reports it before deployment

### Requirement: The gallery index lists what each demo teaches

The gallery SHALL present every demo with the one thing it teaches and the order
to read them in, so a reader who does not know what a vector is has a first
demo and a reader who does can skip. A demo MUST NOT be reachable only by
guessing its address.

#### Scenario: Every demo is reachable from the index

- **WHEN** the gallery index is rendered
- **THEN** it links every shipped demo and states what each one teaches

#### Scenario: The first demo is the foundation

- **WHEN** a reader arrives knowing nothing about vectors
- **THEN** the index identifies the demo that explains what a vector is as the place to start, and the order of the others is stated

### Requirement: The scripting surface is demonstrated through preloaded digests

A demo that shows the product's extensibility SHALL call a script the sandbox preloaded, by its digest, and SHALL show the reader the script's source, its digest, and the model it runs against. A demo MUST NOT send script source, and the sandbox's refusal of raw script evaluation MUST be preserved and stated as part of what is demonstrated.

#### Scenario: A scripted demo calls by digest

- **WHEN** a demo returns a structured reply computed next to the model
- **THEN** it does so by calling a preloaded preset with the model and digest that the sandbox was configured with, and the reply is the server's own

#### Scenario: The source and the digest are shown

- **WHEN** a scripted demo presents its result
- **THEN** the page shows the script's source and the digest it was called with, and the digest is derived from the shipped bytes rather than transcribed

#### Scenario: Raw evaluation stays refused

- **WHEN** the scripting surface is described or used
- **THEN** the page states that the sandbox runs only the scripts it preloaded, and no demo attempts to send Lua source or evaluate it

### Requirement: The corpus is public domain and attributed

Every corpus shipped with the gallery SHALL be public domain or openly licensed for redistribution, and the page that displays its passages SHALL carry the source attribution. A corpus MUST NOT be shipped without a recorded licence, and acquisition boilerplate MUST NOT reach the index or the page.

#### Scenario: A shipped corpus records its licence

- **WHEN** a corpus is added to the gallery
- **THEN** its licence and source are recorded with the corpus and stated on the page that displays it

#### Scenario: Attribution is displayed

- **WHEN** a demo presents passages from a corpus
- **THEN** the plate carries the corpus's attribution in the site's own annotation voice

#### Scenario: Boilerplate is not shipped

- **WHEN** a corpus is acquired from a public source
- **THEN** the acquisition headers and footers are stripped before embedding, and the check that builds the index fails if they are present

### Requirement: The atlas's order switch changes only the axis it names

A plate that offers an order switch SHALL draw each mark at the coordinate that
order defines, and MUST NOT draw an element at a coordinate the order cannot
supply. Under an order that places passages by an attribute they carry — a year —
an element that does not carry that attribute MUST be placed by an attribute it
does have, or omitted, rather than placed at the attribute's zero. Switching the
order SHALL NOT require re-embedding or re-searching: the plate redraws from the
data it already holds.

#### Scenario: The named regions belong to the meaning order

- **WHEN** the atlas is placed by year
- **THEN** the named cluster regions are not drawn, because a chronology has no clusters, and no ring or label is drawn at the origin of the year axis

#### Scenario: A query has no publication year

- **WHEN** the atlas is placed by year and a query has landed
- **THEN** the query's mark is placed at the mean year of the neighbours it retrieved, or is omitted, and is never placed at a non-finite coordinate

#### Scenario: Switching the order does not re-query

- **WHEN** a reader switches the atlas's order
- **THEN** the plate redraws from the projection it already holds without issuing another sandbox command

#### Scenario: An order without a coordinate is the year's

- **WHEN** a passage has no recorded year
- **THEN** its placement under the year order falls back to a defined value and does not compute a non-finite coordinate

### Requirement: A failed interaction is stated, never rendered as an empty result

Every interactive instrument SHALL report the condition that stopped it. An
instrument MUST NOT swallow a failure and render its normal empty state, because
an empty result and a failed request are different facts and a reader cannot tell
them apart. A failed instrument SHALL offer the same retry the rest of the plate
offers and SHALL show no value it did not receive.

#### Scenario: A second instrument fails honestly

- **WHEN** any of a plate's instruments fails — the primary query or a secondary one such as a blend
- **THEN** that instrument states the condition and offers a retry, and does not present the failure as a result with no items

#### Scenario: No fabricated value on failure

- **WHEN** an instrument cannot complete its call
- **THEN** it shows no vector, no ranking, and no timing for the attempt

### Requirement: A cost demo measures the server's execution time, not the network

A demo whose subject is what a call costs SHALL measure the server's own
reported work — its per-command `elapsed_us`, or its cache counters — and SHALL
display the measured value rather than a claimed speed-up. The time a cost demo
shows MUST be execution time measured at or beside the server, and MUST NOT be a
client clock around the request, which folds in the network between the reader and
the sandbox and makes the figure track distance rather than work. A demo MUST NOT
present a comparison as measured when one side read the other side's cache, and
SHALL keep the two sides of a comparison from warming each other, or SHALL report
the cache state that makes them incomparable. The plate SHALL show whether a
repeated call was a hit or a miss, derived from the server's counters rather than
assumed.

#### Scenario: The measurement is the server's execution time

- **WHEN** a cost demo displays a time
- **THEN** the time is the server's reported `elapsed_us`, measured by the bridge around the upstream command, and the plate states that the network is not counted

#### Scenario: The page does not time the request

- **WHEN** a cost demo runs a command
- **THEN** it does not measure the request with a client clock, and no figure it draws is a wall-clock duration that includes the client's network

#### Scenario: A comparison is not made cheaper by the other side's cache

- **WHEN** a cost demo compares batched calls against single calls
- **THEN** the two sides do not share cache entries, and where they would, the plate separates them and says so

#### Scenario: A repeat is shown, not claimed

- **WHEN** a cost demo asks for the same text twice
- **THEN** it reads the model's cache counters around the pair and states whether the first call was a hit or a miss, so a warm cache is reported rather than presented as a cold cost

#### Scenario: The claim follows the numbers

- **WHEN** the measured speed-up is smaller than the copy implies, or is absent
- **THEN** the plate displays the measured value and makes no stronger claim, and a first call that was already cached is stated as such rather than shown as a slow miss

### Requirement: Every plate carries a figure built from the live reply

Each plate SHALL carry at least one visualization generated from the data it
actually received — the vector's values, the ranked neighbours, the label
probabilities, the measured times, the graph's edges — and the prose SHALL serve
the figure rather than replace it. A figure MUST NOT be a static image, a
screenshot, or a hand-drawn sketch of a result: it is drawn from the reply, so a
changed reply changes the figure. A plate that explains a result only in words
MUST NOT ship where the result has a shape.

#### Scenario: A figure is generated, not pasted

- **WHEN** a plate is reviewed
- **THEN** its visualization is constructed at runtime from the reply's own values, and no image file is presented as a result

#### Scenario: The prose is a caption

- **WHEN** a plate's sections are read
- **THEN** each section is short enough to read beside the figure, and no section restates in prose what the figure makes visible

#### Scenario: The figure reuses the instrument's atoms

- **WHEN** a new visualization is added
- **THEN** it is built from the site's existing SVG and type atoms, and it introduces no new colour, font, texture, card, gradient, panel, or stylesheet token

#### Scenario: A reduced-motion reader keeps the figure

- **WHEN** a reader prefers reduced motion
- **THEN** every figure is drawn in one frame, and no figure depends on an animation having run

### Requirement: The gallery shows a script computing structure, not only a value

At least one plate SHALL show the scripting surface computing over many vectors
at once — a nearest-neighbour graph, a ranking, or a reduction — and SHALL state
what crosses the wire and what does not. The plate MUST call a preloaded preset
by digest, and the work it demonstrates MUST share the server's own batcher and
cache rather than opening a second model.

#### Scenario: The structure is computed beside the model

- **WHEN** the graph plate runs
- **THEN** a preloaded preset embeds its whole batch in one call, reduces the pairwise comparison host-side, and returns edges rather than the full matrix

#### Scenario: The interactive structure is drawn

- **WHEN** the graph plate returns its edges
- **THEN** the plate draws them as a directed graph, with the strongest edge the single accent, and the nodes carry the passages they came from
