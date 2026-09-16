## RENAMED Requirements

- FROM: `### Requirement: The landing carries one install line and one door to the documentation`
- TO: `### Requirement: The landing carries one install line and doors to the documentation and the gallery`

## ADDED Requirements

### Requirement: The gallery is composed from the poster's atoms and reads as one instrument

The demos gallery MUST be built from the site's existing design vocabulary — the
poster's rule-head, ruled numbered entries, mono annotations, the dark plate, and
the orange signal — and MUST NOT introduce a new palette, a font, a texture, a
card grid, a gradient, a rounded panel, or a decorative shadow. It SHALL read as
one instrument rather than a set of unrelated pages: an engraved atlas, in which
the subject supplies the register and the decoration supplies nothing.

Each plate SHALL carry exactly one coloured element, spent on meaning — the query,
the highlighted neighbours, or the measured similarity — and SHALL caption itself
with the real figures of the index it draws. Passages SHALL be presented as
quotations attributed to their work and year in the site's annotation voice. The
gallery's interactivity MUST honour the site's committed motion contract, its
type floors, its contrast floor, and its reduced-motion behaviour, and any motion
it adds MUST be the reader's choice to keep, except where a demo's own result is
the moving thing.

#### Scenario: The gallery adds no new visual language

- **WHEN** a gallery page is reviewed
- **THEN** every colour, type role, texture, and rule it uses already exists in the site's stylesheet or is derived from an existing token without adding a custom property

#### Scenario: The gallery reads as one instrument

- **WHEN** the gallery's plates are reviewed together
- **THEN** they share one composition — ruled captions, numbered figures, mono annotations, paper for explanation and the dark plate for instrumentation — so a reader recognises them as one apparatus

#### Scenario: One signal per plate

- **WHEN** a plate is rendered
- **THEN** exactly one element carries the accent, and it is the element that carries meaning, with no second colour used for decoration

#### Scenario: A caption carries the real figures

- **WHEN** a plate is captioned
- **THEN** its figure carries the count, model, dimension, and size read from the index's manifest rather than transcribed

#### Scenario: Passages are quotations

- **WHEN** a plate displays a passage from the corpus

- **THEN** the passage is set as a quotation and attributed to its work and year in the site's mono annotation voice

#### Scenario: The gallery meets the committed floors

- **WHEN** a gallery page is rendered at the reference width and on a phone
- **THEN** no text computes below the site's floor, every control clears the target floor, and every text and control colour clears its ratio

#### Scenario: Motion is optional

- **WHEN** a reader prefers reduced motion
- **THEN** no demo animates without being asked, and no result depends on an animation having run

## MODIFIED Requirements

### Requirement: The landing carries one install line and doors to the documentation and the gallery

The landing page SHALL carry exactly one copy-paste install command — stating the
command, the version, the licence, and the supported platforms — placed in the
hero beside the calls to action, in the same block, so a reader who has decided
to try the product can act without leaving the site or the fold's neighbourhood.
Its calls to action and its navigation entries MUST point at the site's own
surfaces rather than at off-site anchors, and the landing MUST NOT link the
reference material from anywhere else. It MUST NOT carry the full command
tables, the configuration block, or the operational detail, which belong to the
documentation surface.

#### Scenario: A convinced reader can act

- **WHEN** a reader decides to try the product from the landing page
- **THEN** an install command is present in the hero in the same block as the calls to action, and the page's calls to action stay on the site

#### Scenario: The install line is adjacent, not merely present

- **WHEN** the install block is rendered
- **THEN** it sits directly below the hero actions as one ruled row, states the command and the release facts, and is not separated from the calls to action by any other content

#### Scenario: Every documentation link is a door, not a detour

- **WHEN** the landing page's links are reviewed
- **THEN** every call to action and documentation navigation entry resolves to a surface this repository serves, and no other link on the page points at reference material

#### Scenario: The gallery is reachable from the landing

- **WHEN** the landing page is rendered
- **THEN** it offers a route to the demos gallery that is distinguishable from the route to the documentation, so a reader who wants to see the product work is not sent to the reference

#### Scenario: Detail is not duplicated

- **WHEN** the landing page is reviewed for reference material
- **THEN** it contains no command table, no configuration block, and no more than four named commands, and the documentation surface is the only place the full set lives
