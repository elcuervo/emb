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
- **THEN** the command shown to the reader is the command that was issued, including the model name, the reply form, and the arguments the demo actually sent

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

### Requirement: The demos run against the models the surface serving them reports

Every model a demo names SHALL be resident on the surface that answers its
queries and reported by that surface's metadata. Where a demo compares models it
MUST show that vectors from two different models are not interchangeable,
because each model defines its own space. A demo MUST NOT name a model the
surface it calls does not serve, and a plate served by a surface other than the
sandbox MUST state which surface answered it.

#### Scenario: Every named model is loaded

- **WHEN** a demo names a model
- **THEN** the surface the demo calls reports that model as loaded, and the page's model list is derived from the server rather than typed

#### Scenario: The comparison teaches incomparability

- **WHEN** a demo presents two models over one corpus
- **THEN** it states that the two models' vectors are not comparable, and demonstrates that the same text occupies different neighbourhoods under each

#### Scenario: A plate on another surface names it

- **WHEN** a plate's queries are answered by a surface other than the sandbox
- **THEN** the plate states which surface answered, and reports that surface's own model and index metadata rather than the sandbox's

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

### Requirement: A demo unfolds the commands its last run issued

Each demo that issues a command to the sandbox SHALL offer, once it has run, a
disclosure that unfolds the exact argv issued in that run: the reader's own text,
the digest the sandbox was given, and every argument in the order sent. The
disclosure MUST reflect the most recent run rather than accumulating earlier
ones, MUST be shown for a run that failed or was refused, and MUST carry nothing
but the commands — no timing, no reply shape, and no query the sandbox never
saw. The fold SHALL open without a script. The demo's static record of its call
MUST remain readable and MUST stay distinguishable from the run's own commands.

#### Scenario: The disclosure carries the run's own arguments

- **WHEN** a reader runs a demo and unfolds the commands
- **THEN** the argv shown carries the text the reader supplied, not a placeholder

#### Scenario: The disclosure is the most recent run's

- **WHEN** a reader runs a demo a second time
- **THEN** the commands shown are the second run's, with no command from the first run still listed

#### Scenario: A refused run still shows what was sent

- **WHEN** a demo's command is refused or fails
- **THEN** the commands issued before the failure are still shown as the commands that ran, and no reply or timing the demo did not receive is shown

#### Scenario: The commands and nothing else

- **WHEN** the disclosure is unfolded
- **THEN** it lists only the argv, with no elapsed time, no reply shape, and no query that ran in the browser

#### Scenario: A digest is the one that was sent

- **WHEN** a scripted demo unfolds its command
- **THEN** the digest shown is the one the sandbox ran, while the demo's static record keeps its placeholder shape

#### Scenario: A binary argument is named, not dumped

- **WHEN** a demo sends a binary argument, as the image preset does
- **THEN** the disclosure marks that argument as bytes of a stated length rather than the encoded payload

#### Scenario: Long text is clamped, not laid out in full

- **WHEN** a surface the page did not author carries more text than it can show — the run's own command, or a passage a plate quotes
- **THEN** that text is clamped to a few lines and marked as truncated, and the full text remains in the document rather than being cut from it

#### Scenario: The fold opens on the site's motion, or not at all

- **WHEN** a reader opens the commands
- **THEN** the disclosure unfolds on the site's own easing, and with reduced motion asked for the commands are shown in one frame without the unfold

#### Scenario: The fold needs no script

- **WHEN** a demo page is loaded with scripting disabled
- **THEN** the demo's static record of its call is still readable and the disclosure is absent rather than shown as an error

### Requirement: A demo offers the next plate at its top and its foot

A demo SHALL link to the next plate in the reading order from the top of the
page as well as from the rail at its foot, so a reader who has finished the
section in view can continue without scrolling the whole plate. The link MUST
name the plate it leads to and MUST be built from the page's existing type and
rule vocabulary. The last plate in the order, which has no next, is not required
to carry one.

#### Scenario: The next plate is reachable from the top

- **WHEN** a reader has finished a section near the top of a demo
- **THEN** the next plate in the reading order is linked from the top of the page, and the link names it

#### Scenario: The top link agrees with the foot rail

- **WHEN** a demo carries both a top link and a foot rail
- **THEN** both point at the same plate

#### Scenario: The last plate is honest about having no next

- **WHEN** the final plate in the order is read
- **THEN** it carries no top link to a plate that does not follow it

### Requirement: A demo shows one forward pass answering many typed questions

The gallery SHALL include a plate that demonstrates the decision surface: one scripted call to a decision model that answers several typed questions at once, and whose rendering makes the single-forward-pass fact legible (the answers SHALL appear together, as one reply, rather than one after another). The plate SHALL use the shared demo anatomy — plate, mechanism, rig, commands-that-unfold — and SHALL ship a preselected set of example states, each reusable from one button. The questions it runs SHALL be a reference implementation's own shipped preset (e.g. `Presets.email_questions`), verbatim, so the plate demonstrates the ONNX model's actual features — the typed head, the option markers, the per-bucket calibration, the action head, the single pass — rather than an invented question set.

#### Scenario: One run answers every question

- **WHEN** a visitor selects an example state and runs the plate
- **THEN** exactly one `EMB.EVSHA <model> <digest> 1 <state> <questions> <config>` evaluation issues, and every question's answer appears at once in the same rendering pass

#### Scenario: The questions are the reference preset, verbatim

- **WHEN** the plate sends its questions
- **THEN** they match the reference implementation's shipped preset for the example (same ids, instructions, and criteria, in order), and the plate says which preset it runs

#### Scenario: The answers differ in shape by question type

- **WHEN** the plate renders a `choice`, a `score`, and a `noul` answer
- **THEN** the `choice` shows a distribution with a winner, the `score` shows a position on a legend, and the `noul` shows a probability, and every answer carries its confidence (and the sheet carries the action probability)

### Requirement: A stand-in model demo states that its numbers are not judgments

A demo whose live model is a mechanism substitute — same architecture and export
pipeline, weights that answer nothing — SHALL say so in the plate's prose and figure,
SHALL NOT present its numbers as conclusions about the example states, and SHALL
show what the production-shaped call looks like. A plate that answers some examples
with a real checkpoint and others with a stand-in SHALL label each example's model,
so a reader can tell which numbers are judgments and which are only the mechanism.
The stand-in rule SHALL NOT be applied to an example answered by a real checkpoint.

#### Scenario: The substitute is labeled

- **WHEN** a plate runs an example against a stand-in model
- **THEN** the plate states that the model demonstrates the mechanism only and that its answers are shapes rather than judgments

#### Scenario: A real example is not labeled a substitute

- **WHEN** an example is answered by a real checkpoint
- **THEN** the plate does not present it as a mechanism substitute

#### Scenario: The production call is shown

- **WHEN** the plate's exact-commands section presents an example
- **THEN** it shows the invocation (model entry, state, questions, config envelope) that produced the displayed reply

### Requirement: A decision demo draws each example's typed questions as a tree

A demo plate for a decision model SHALL teach the model's typed-question
vocabulary before showing a call, and SHALL draw the call as a directed graph: the
state as the source, one node per question carrying its id and type, and one node
per answer the reply selected, with an edge from the state to each question and
from each question to its answer. An answer node SHALL state the option the reply
chose, by the checkpoint's own answer semantics, and SHALL carry that answer's
value; a question the reply does not answer SHALL be an empty node, never a
fabricated value. The figure MUST NOT be a hand-authored illustration of a result,
and every typed-question example MUST render through the same figure.

#### Scenario: The tree is the payload, filled by the reply

- **WHEN** an example runs
- **THEN** the drawn nodes and edges are the request's questions and the reply's chosen answers, and a changed reply changes the figure

#### Scenario: The answer node is the decision

- **WHEN** a question is drawn
- **THEN** its answer node names the option the reply selected and shows that answer's value, rather than a bar to compare across

#### Scenario: The vocabulary is taught before the call

- **WHEN** the plate is read from the top
- **THEN** `choice`, `score`, and `noul` are each defined with the request they take and the reply shape they return before any example runs

#### Scenario: Multiple examples share one interaction

- **WHEN** a visitor switches between the plate's examples
- **THEN** each typed-question example runs through the same call, the same graph figure, and the same command disclosure

### Requirement: A dependent decision loop runs as one bounded episode

A demo whose decisions form a loop — each step depending on the previous one —
SHALL run that loop where the model runs, as a **bounded episode inside a single
call**, and SHALL animate the returned trace locally. It MUST NOT require a
network round trip per rendered frame, and it MUST NOT present a precomputed or
page-authored trace: every frame SHALL be a decision the server computed during
that call. The episode SHALL be bounded so a single request cannot run unbounded,
and the demo SHALL keep a single-step path so the per-decision mechanism remains
inspectable.

#### Scenario: One call returns the episode

- **WHEN** a visitor starts the looped demo
- **THEN** one command returns the episode's frames, and the commands disclosure shows that single call rather than one call per frame

#### Scenario: The animation survives a slow network

- **WHEN** frames are playing and the buffer runs low
- **THEN** the demo requests the next episode from the last frame's state before the buffer empties, so the visible animation does not stall on a round trip

#### Scenario: Frames are the server's

- **WHEN** a frame is drawn
- **THEN** its board and probabilities came from the server's reply for that episode, and a failed fetch shows the sandbox's state instead of any frame

#### Scenario: The single step remains

- **WHEN** a visitor steps once
- **THEN** exactly one tick is computed in one call and its typed questions and probabilities are shown for that single decision

#### Scenario: The episode is bounded

- **WHEN** a request asks for more ticks than the documented bound
- **THEN** the request is rejected or clamped rather than running an unbounded loop

### Requirement: A re-selected demo replaces its surface

When a visitor re-selects any control that changes which example or item a demo is
showing, the demo SHALL replace its surface rather than append to it. A single
selection MUST NOT leave two copies of a control in the DOM, and the number of
rendered controls MUST NOT grow with the number of selections.

#### Scenario: Switching an Inbox ticket

- **WHEN** a visitor selects a different Inbox ticket
- **THEN** the ticket list and its form appear exactly once, not once per selection

#### Scenario: Repeated switches stay single

- **WHEN** a visitor switches between tickets several times
- **THEN** each switch leaves exactly one list and one form, regardless of how many switches happened

### Requirement: A looped demo's readout shows the executed decision

A looped demo's readout SHALL lead with the move the preset actually executed, and
SHALL show the model's move probabilities over the **legal** moves only — a
direction the rules forbid MUST be shown as unavailable, not as a probability. The
executed move SHALL be marked in the probability table, and the model's own
proposal SHALL be named beside it whether or not the safety layer overrode it.

#### Scenario: Probabilities cover the legal moves only

- **WHEN** a looped demo displays a decision and some directions are blocked by the rules
- **THEN** only the legal directions carry a probability, and a blocked direction shows no value

#### Scenario: The executed move is the highlight

- **WHEN** a looped demo displays a decision
- **THEN** the executed move is named as the decision and marked in the table, and the model's proposal is shown separately

#### Scenario: A veto is visible

- **WHEN** the safety layer overrides the model's proposal
- **THEN** the readout names the executed move, names the proposal it replaced, and marks the intervention

#### Scenario: Only the decision is highlighted

- **WHEN** the readout draws a bar for a value that is not the decision
- **THEN** that bar carries the neutral tone, and only the executed move's bar carries the accent

### Requirement: The readout does not reflow as its values change

A demo readout SHALL keep its layout as its values change. A longest permitted
label or marker MUST NOT wrap to a second line or push a value out of its column,
and a row's height and alignment MUST NOT change when a different value is
displayed.

#### Scenario: A long direction is proposed

- **WHEN** the proposed or executed move is the longest direction name
- **THEN** the move's marker, name, bar and value stay on one aligned row

#### Scenario: The readout-bar labels fit

- **WHEN** the readout's bar labels are drawn
- **THEN** each label stays on one line and its bar and value remain aligned to it

### Requirement: A typed-question demo states each question's chosen answer

A demo that answers typed questions SHALL state the chosen answer for each
question alongside that question's distribution, leading with the decision the way
a looped demo leads with its executed move. The chosen answer SHALL follow the
checkpoint's own answer semantics — a `choice` is the highest-probability option, a
`score` is the rubric level nearest its rounded expected value, and a `noul` is
true only above 0.5 — and the marked leaf MUST be the one that answer selects. The
stated answer SHALL carry its value (the option's probability, the expected score,
or the noul probability), so a low-confidence answer reads as one. The example set
SHALL cover the plate's question vocabulary with distinct inputs rather than one
repeated shape.

#### Scenario: Each question names its decision

- **WHEN** a typed-question reply is shown
- **THEN** every question names the option the reply chose, and that option is the marked leaf

#### Scenario: The answer follows the checkpoint's semantics

- **WHEN** a typed-question reply is shown
- **THEN** the choice is the top option, the score's label is the rubric nearest its rounded expected value, and a noul is true only above 0.5

#### Scenario: A near-coin-flip reads as one

- **WHEN** an answer's probability is close to even
- **THEN** the stated answer carries that probability, so it is not presented as a confident decision

#### Scenario: The set covers the vocabulary

- **WHEN** the typed-question examples are listed
- **THEN** they exercise different subjects, so a reader sees the mechanism over varied input rather than one repeated ticket

### Requirement: A decision readout shows per-decision model inference time

A decision demo's readout SHALL show the model's own inference time for the
decision it is displaying, not the request round trip. The value SHALL come from
the server's measurement of the model call, and the plate MUST NOT substitute a
client clock around the request or the bridge's whole-command elapsed time. For a
looped demo the readout SHALL show the displayed frame's inference time; for a
single-pass demo it SHALL show the reply's inference time. The plate SHALL label
the value as model inference and, where the figure reads as a rate, derive that
rate from the inference time rather than from wall-clock playback.

#### Scenario: The looped readout shows the frame's inference time

- **WHEN** a looped decision demo displays a frame
- **THEN** the readout's inference figure is that frame's server-measured model time, and it changes as frames advance

#### Scenario: The single-pass readout shows the reply's inference time

- **WHEN** a typed-question demo shows a completed reply
- **THEN** the readout's inference figure is the reply's server-measured model time

#### Scenario: The figure is not the request time

- **WHEN** a visitor inspects the readout's inference value
- **THEN** it is the model call's duration, and the plate does not present the client-measured request duration as that figure

### Requirement: The typed-question tree fills as the reply arrives

The decision tree SHALL fill its leaves from the reply's probabilities as an
ordered reveal rather than a single instantaneous frame, so a visitor sees each
question's distribution populate in turn. The reveal SHALL be presentational only:
every leaf's final value MUST equal the reply's probability, the tree MUST be
complete in one frame under reduced motion, and no value may be shown before the
reply that carries it exists.

#### Scenario: Leaves populate in question order

- **WHEN** a typed-question reply arrives
- **THEN** each question's leaves fill in turn, and each leaf ends at the probability the reply carries for it

#### Scenario: Reduced motion keeps the whole tree

- **WHEN** a reader prefers reduced motion
- **THEN** the tree is drawn complete in one frame and no leaf depends on the reveal having run

#### Scenario: No fabricated value before the reply

- **WHEN** the plate draws the tree before the reply returns
- **THEN** every leaf is empty and no probability is shown

### Requirement: A looped demo's planner prevents stalls

A looped demo whose decisions move a position SHALL NOT allow the run to oscillate
without progress. Its planner SHALL veto an immediate reversal of the previous move
when another legal move exists, and SHALL veto a move that increases the distance
to the nearest objective when a move that decreases it exists. After a documented
number of consecutive decisions without collecting an objective, the planner SHALL
take the objective-seeking move until progress resumes.

#### Scenario: An immediate reversal is vetoed

- **WHEN** the model proposes a reversal of the previous move and another legal move exists
- **THEN** the executed move is not that reversal

#### Scenario: A regressing move is vetoed

- **WHEN** the model proposes a move that increases the distance to the nearest objective while a move that decreases it is legal
- **THEN** the executed move does not increase that distance

#### Scenario: A stalled run recovers

- **WHEN** a documented number of decisions pass without an objective being collected
- **THEN** the next executed move reduces the distance to the nearest objective until one is collected

#### Scenario: A bounded episode makes progress

- **WHEN** a bounded episode runs from a fresh state
- **THEN** it collects at least one objective, or the objective set was already empty

### Requirement: A looped demo pauses on a bounded play window and on focus loss

A demo that animates a server-computed trace locally SHALL bound its playback to
a documented active-play window, and SHALL pause — not merely hide — when that
window is spent. It SHALL also pause when the page loses focus or the document
becomes hidden. A paused demo MUST NOT advance frames and MUST NOT request the
next episode, and resuming MUST NOT silently restart or overrun the window.

#### Scenario: The play window is spent

- **WHEN** a looped demo has played for its documented active-play window
- **THEN** it stops advancing frames and reports the pause rather than continuing to animate

#### Scenario: The page loses focus

- **WHEN** the document becomes hidden or the window loses focus while a looped demo is playing
- **THEN** the demo pauses immediately, and no further episode is requested while it is paused

#### Scenario: Resuming continues the window

- **WHEN** the visitor resumes a demo paused by the window rather than by focus
- **THEN** playback continues from where it stopped and the window is not reset

### Requirement: The gallery marks a newly added demo

The gallery SHALL mark a demo that is new in the current release, so a returning
reader can find what changed. The mark SHALL be visible beside the demo's own
name and MUST NOT alter the demo's link or its accessible name.

#### Scenario: A new demo is marked

- **WHEN** a demo is added and the gallery lists it
- **THEN** the demo carries a visible `new` mark, and the marked element still links to and names the demo

#### Scenario: The mark is not permanent

- **WHEN** a later release adds another demo
- **THEN** the mark is removed from the previously new one, so the mark identifies the current addition

### Requirement: A demo's in-page selection is named by the URL

A plate that offers a choice among examples or items SHALL name the current
choice in the URL fragment, SHALL restore the choice the fragment names when the
page loads, and SHALL reflect a selection back to the fragment. A fragment the
plate does not know MUST leave its default selection unchanged and MUST NOT be
treated as an error. Reflecting a selection MUST NOT add a history entry, so a
Back gesture leaves the plate rather than walking its selections.

#### Scenario: A shared link opens the named choice

- **WHEN** a plate that offers an in-page choice is opened with a fragment naming one of its choices
- **THEN** that choice is the one selected and rendered

#### Scenario: Selecting a choice names it in the URL

- **WHEN** a reader selects a choice on a plate
- **THEN** the URL fragment names the choice that was selected

#### Scenario: An arriving fragment re-selects on the loaded plate

- **WHEN** the fragment changes to a name the plate knows while the plate is loaded
- **THEN** the plate selects that choice

#### Scenario: An unknown fragment leaves the default

- **WHEN** a plate loads with a fragment naming none of its choices, or with no fragment
- **THEN** it renders its default selection and reports no error

#### Scenario: Selection does not consume the Back gesture

- **WHEN** a reader selects a choice and then navigates Back
- **THEN** the plate is left rather than the previous selection restored

### Requirement: A plate may query the zone without a resolver

The gallery SHALL include a plate that runs the DNS zone's queries over the
zone's own read-only HTTP surface, so a reader needs no resolver and no
terminal, and SHALL show the `dig` invocation that would issue the same query —
the same name, the same record type — beside it. The plate MUST refuse a query
the DNS transport would refuse, so it never demonstrates a name the zone would
not answer, and it MUST state that the two routes run the same query against the
same service.

#### Scenario: The browser and the resolver ask the same question

- **WHEN** a reader runs a query on the plate
- **THEN** the answer is the service's reply over HTTP, and the plate shows the `dig` command carrying the same name and record type

#### Scenario: A refused name is refused on the plate

- **WHEN** a reader enters a name the zone refuses — unparseable, empty, or over the transport's limit
- **THEN** the plate states the refusal and shows no result, exactly as the resolver route would

#### Scenario: The plate names the surface that answered

- **WHEN** the plate renders a result
- **THEN** it names the service that answered and the model that embedded the query

### Requirement: The plate shows the working the record omits

Because each record the zone answers with carries only its glyph, the plate SHALL
show every ranked result's vocabulary name and its similarity score, in the order
the records were ranked.

#### Scenario: Names and scores are shown in rank order

- **WHEN** the plate renders a result
- **THEN** each ranked result shows its vocabulary name and its score, in the order the DNS records carry them

#### Scenario: The plate shows the working the record omits

- **WHEN** the plate renders a result
- **THEN** each ranked result shows its vocabulary name and its score, in the order the DNS records carry them

### Requirement: The plate shows the answer as the wire carries it

Because every resolver escapes non-ASCII `TXT` data, the plate SHALL show both
the escaped record data the DNS transport produces and the glyph it stands for,
side by side, and SHALL state why the two differ. The escaped form SHALL be
derived from the reply the service returned rather than transcribed into the
page.

#### Scenario: The escaped bytes and the glyph are shown together

- **WHEN** a query returns an emoji
- **THEN** the plate shows the escaped `TXT` data beside the glyph, and both come from the same reply

#### Scenario: The reason is stated

- **WHEN** the escaped form is shown
- **THEN** the plate states that the escaping happens in the resolver and in the reader's tool, not in the zone

### Requirement: The plate's examples are a fixed, verified set

The plate SHALL ship a fixed set of example queries covering each mode the zone
answers — a sentence, an emoji, and a composition — each reachable from one
control and each named by the URL fragment. Every shipped example MUST be
verified against the zone before deployment, and the plate SHALL state which
mode each example demonstrates.

#### Scenario: One control per mode

- **WHEN** the plate is rendered
- **THEN** a reader can run a sentence example, an emoji example, and a composition example from one control each

#### Scenario: An example is named by the URL

- **WHEN** a reader selects an example
- **THEN** the fragment names it, and opening that fragment selects it again

#### Scenario: A shipped example is verified, not asserted

- **WHEN** the examples are checked before deployment
- **THEN** each one returns the result the plate's own copy claims for it, and a drifting example fails the check rather than shipping

### Requirement: A fragment that names a choice reveals the demo

When a plate is opened, or its fragment changes, to a name that selects one of
its choices, the plate SHALL bring the demo region into view rather than leaving
the reader at the top of the page. The reveal SHALL run after the choice is
selected, SHALL NOT run when the fragment names no choice, and SHALL scroll
instantly rather than animating. Selecting a choice by clicking its control MUST
NOT scroll the page.

#### Scenario: A naming fragment lands on the demo

- **WHEN** a plate is opened with a fragment that names one of its choices
- **THEN** the page is scrolled so that the demo region is in view, with the sticky masthead not covering its heading

#### Scenario: No naming fragment does not move the page

- **WHEN** a plate is opened with no fragment, or a fragment that names none of its choices
- **THEN** the plate does not scroll the page and renders its default as before

#### Scenario: A changed fragment reveals the choice

- **WHEN** the fragment changes to a name the plate knows while the plate is open
- **THEN** the plate selects that choice and brings the demo region into view

#### Scenario: The reveal is instant

- **WHEN** the reveal runs
- **THEN** the scroll is immediate and does not animate, so it needs no reduced-motion branch

#### Scenario: Selecting by click does not jump

- **WHEN** a reader selects a choice by clicking its control
- **THEN** the plate selects the choice without scrolling the page itself
