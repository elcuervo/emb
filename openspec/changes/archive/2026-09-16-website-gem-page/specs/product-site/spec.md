## ADDED Requirements

### Requirement: The client surface teaches the client's execution, not the server's

The site SHALL serve a client surface at `/gem/` that explains, to a developer,
what the `emb` Ruby gem does between a call site and the wire. It MUST cover the
request path from a proxy call to a pooled connection, the gem's three execution
modes (`lazy: false`, `:multi`, `:batch`) and the wire command each one produces,
the rule that a deferred value issues nothing until it is used, the batch's
fail-closed behaviour, the per-thread lifetime of the batch scope, and the
`emb-server` distribution gem. It MUST state the exact `EMB` and `EMB.MULTI`
argument shapes the gem emits for a single-model and a mixed-model scope.

Every claim, command and code sample on the surface MUST be traceable to the
shipped gem sources under `gems/`, `BENCHMARK.md`, or `README.md`, and MUST
describe behaviour the shipped code implements. The surface SHALL NOT state a
capability the gem does not have, and SHALL NOT restate the server's own
behaviour as if it were the client's.

#### Scenario: Each execution mode is explained with its own wire shape

- **WHEN** the client surface describes a `lazy` mode
- **THEN** it states what the mode defers, which command resolves the deferred work, and whether that command is sent once per scope, once per chunk, or concurrently

#### Scenario: The mode that sends nothing is stated as sending nothing

- **WHEN** the surface shows a deferred embed call
- **THEN** it states that the call issues no command, and that the command is issued by the first use of the returned value

#### Scenario: Code samples are the shipped code's behaviour

- **WHEN** a code sample on the surface is executed against the shipped gem
- **THEN** it produces the result, argument shape and reply shape the surface claims

#### Scenario: The distribution gem is distinguished from the client gem

- **WHEN** the surface describes `emb-server`
- **THEN** it states that `emb-server` installs a precompiled server binary and its executables rather than a client library, names the supported platforms, and does not present it as a variant of the client

#### Scenario: No server claim is presented as a client claim

- **WHEN** the surface describes coalescing or fan-out
- **THEN** it attributes the deferral, the chunking and the concurrency to the client, and any batching the server performs to the server

### Requirement: The client surface's visualisations are drawings, and say so

The client surface MUST NOT present a measurement it did not take. Its
visualisations of execution modes, round-trip counts and concurrency SHALL be
diagrams of the gem's dispatched command shapes, and the surface SHALL label
them as such. It MUST NOT display a timing, a rate, a throughput or a
comparative speed figure, because the sandbox runs the server rather than Ruby
and no reading of the gem is available to a page.

The round-trip counts a diagram shows SHALL be the counts the shipped code
produces, so a diagram can be checked against the gem rather than trusted. Any
other quantity the surface states SHALL be a value read from the shipped
configuration or the shipped gemspec.

#### Scenario: A diagram is captioned as a diagram

- **WHEN** a visualisation of an execution mode is rendered
- **THEN** it is captioned as the shape the client sends, with no implication that a page measured it

#### Scenario: No fabricated reading

- **WHEN** the surface is reviewed for numeric claims
- **THEN** it contains no elapsed time, no rate and no comparative speed figure, and every count is either a count of commands or a configured value with its source in `gems/`

#### Scenario: The page does not depend on the sandbox

- **WHEN** the page is loaded with scripting disabled, or with the sandbox unreachable
- **THEN** its explanation and its diagrams are complete and it makes no network request

### Requirement: The client surface's motion is the dispatch, and it is optional

The client surface SHALL carry at most one authored motion, at the instrument,
and that motion SHALL encode the execution modes rather than decorate them: a
mode whose texts travel in separate commands MUST show those commands arriving
separately, a mode whose texts travel in one command MUST show one stroke, and a
mode whose shares run concurrently MUST show them arriving at the same moment.
The mark that turns the accent SHALL be the single command carrying every text,
so the motion restates the same rule the static instrument encodes.

Motion SHALL be scroll-driven and SHALL NOT require a script. The surface's
content MUST be complete and correct in the state an engine without
view-progress timelines renders, so the animation can only add to a finished
page. A reader who prefers reduced motion MUST receive that same finished state.
A print pass MUST receive it too, because a print pass never scrolls and a
scroll-driven animation would otherwise print the instrument at the start of its
range.

#### Scenario: Motion is the mechanism, not a flourish

- **WHEN** the instrument's motion is reviewed
- **THEN** every element that moves encodes a fact about how that mode dispatches, and removing the motion would lose that encoding rather than merely lose polish

#### Scenario: The staggered and the simultaneous are distinguishable

- **WHEN** the motion is observed part-way through
- **THEN** the marks of a mode that sends one command per text are at different points in their arrival, and the marks of a mode whose shares run concurrently are at the same point as each other

#### Scenario: The accent marks the single call

- **WHEN** the motion completes
- **THEN** only the marks standing for a command that carries every text hold the accent, and the marks standing for separate commands are the rule's own colour

#### Scenario: No script is added for the motion

- **WHEN** the surface is reviewed for scripting
- **THEN** it loads no script of its own, and the motion is expressed entirely in the stylesheet it already links

#### Scenario: Motion is optional in three ways

- **WHEN** the page is rendered by an engine without view-progress timelines, with reduced motion preferred, or for print
- **THEN** each of the three renders the finished instrument with no element hidden and no state that only the animation could reach

### Requirement: The masthead carries one entry for every surface the site serves

Every page of the site SHALL offer, in its masthead, one navigation entry per
served surface — the documentation surface, the demos gallery, and the client
surface — in addition to its link to the project's source. The entries SHALL be
present in both the primary navigation and the narrow-width disclosure, SHALL
resolve to on-site paths relative to the page they sit on, and SHALL NOT point at
off-site anchors. The client surface's entry SHALL be distinguishable from the
documentation surface's, because the two teach different things.

#### Scenario: Every surface is reachable from every page

- **WHEN** any served page is rendered
- **THEN** its masthead links the documentation surface, the demos gallery and the client surface, and each link resolves to a served path

#### Scenario: The narrow-width navigation keeps the doorway

- **WHEN** a page is rendered below the masthead's wide breakpoint
- **THEN** the disclosure opens on the same three surface entries and the source link

#### Scenario: Entries are relative and on-site

- **WHEN** a masthead entry is reviewed
- **THEN** it is a site-relative path rather than a root-absolute or off-site URL, so it resolves under any mount point the tree is served from

#### Scenario: The client entry is not a second documentation entry

- **WHEN** the masthead is read
- **THEN** the client surface's entry names the client rather than repeating the documentation entry's label, so the two are not offered as the same destination

#### Scenario: The landing's install line is untouched

- **WHEN** the landing page is rendered after this change
- **THEN** its single copy-paste install command, its release facts and its calls to action are unchanged, because the masthead entry adds a destination rather than a second action
