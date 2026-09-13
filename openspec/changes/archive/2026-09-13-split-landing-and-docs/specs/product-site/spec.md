## MODIFIED Requirements

### Requirement: Three ground-varied capability blocks before the terrain

The site SHALL carry the capability material as three distinct blocks — protocol,
script inference, and operations — placed between the hero and the terrain, and
the terrain SHALL be the last section before the footer. Each block MUST be
given a distinguishable ground or measure so the sequence reads as three
movements rather than one list. The wordmark, claim, six-row feature list, and
pipeline artwork MUST remain unchanged; the hero sub MAY state the product's
release status, and one install line carrying the command, version, licence, and
platforms MAY sit adjacent to the hero actions.

#### Scenario: The capability region covers all four themes

- **WHEN** the capability blocks are rendered
- **THEN** they present the Redis protocol, `EMB.EVAL`/`EMB.EVSHA` script inference, operational commands, and the `emb-top` dashboard

#### Scenario: The poster composition is preserved

- **WHEN** the page is rendered after this change
- **THEN** the masthead, wordmark, claim, six-row feature list and pipeline artwork are pixel-equivalent to their pre-change state

#### Scenario: Copy inside a fixed slot may be corrected

- **WHEN** the hero sub or the install line is changed
- **THEN** the change is confined to copy and to one install element, no composition atom is added or removed, and the claim, wordmark, feature list and pipeline artwork are untouched

#### Scenario: Document height is not a constraint

- **WHEN** the recomposition makes the page longer or changes its ratio
- **THEN** the change is accepted and the ratio is not treated as a regression

#### Scenario: The terrain closes the page

- **WHEN** the page is rendered
- **THEN** the terrain is the last section before the footer and no capability content follows it

#### Scenario: The grounds differ

- **WHEN** the three blocks are compared
- **THEN** at least one is a full-bleed dark ground, and the remaining grounds or measures differ from each other and from the hero

### Requirement: README is the source of truth for every claim

Every capability, command, reply shape, number, or output rendered on the site
MUST be traceable to `README.md`, `BENCHMARK.md`, `examples/scripts/`, or the
shipped code. The site MUST NOT introduce a claim the repository does not
support. A claim about an input path, model branch, or distribution channel MUST
be implemented by the shipped scripts or the server; a claim that a shipped
script explicitly documents as unavailable MUST NOT appear.

#### Scenario: A rendered command exists in the reference

- **WHEN** the site displays a Redis command and its reply
- **THEN** that command and that reply shape appear in `README.md`

#### Scenario: A number carries its provenance

- **WHEN** the site displays a measured number
- **THEN** the number appears in `BENCHMARK.md` with its machine and method, and the site either states that provenance or omits the number

#### Scenario: An absent commercial fact stays absent

- **WHEN** the site is reviewed for commercial claims
- **THEN** it contains no customers, logos, testimonials, pricing, hosted-service implication, or service-level commitment

#### Scenario: An input path is implemented before it is advertised

- **WHEN** the site names an input the product accepts
- **THEN** a shipped script or the server implements that input, and any input a shipped script documents as unavailable is either omitted or stated as unavailable

#### Scenario: A version string is generated, never typed

- **WHEN** the site displays a version
- **THEN** that value derives from `VERSION` rather than being transcribed by hand

#### Scenario: Pre-1.0 status is stated

- **WHEN** the site makes a readiness claim
- **THEN** the page states that the project is pre-1.0 and that interfaces may still move, or the readiness claim is removed

### Requirement: The `emb-top` panel shows real output

The `emb-top` presentation MUST be derived from the dashboard's actual output and
MUST NOT present invented models, metrics, or rates as if they were real. Its
version string MUST be generated from `VERSION`. Where the panel's figures are
illustrative rather than drawn from a `BENCHMARK.md` run, the panel MUST label
them as illustrative and MUST NOT be the site's only quantitative claim.

#### Scenario: The panel is sourced

- **WHEN** the `emb-top` panel is rendered
- **THEN** its rows and figures correspond to a real run or are clearly labelled as illustrative

#### Scenario: The panel's version cannot drift

- **WHEN** `VERSION` changes
- **THEN** the version shown in the panel changes with it and no hand-typed version remains

#### Scenario: The panel is not the only number

- **WHEN** the site makes a performance claim
- **THEN** at least one figure on the site is traceable to a `BENCHMARK.md` run with its reproduction command, independently of the illustrative panel

## ADDED Requirements

### Requirement: The landing carries one install line and one door to the documentation

The landing page SHALL carry exactly one copy-paste install command — stating the
command, the version, the licence, and the supported platforms — placed in the
hero beside the calls to action, in the same block, so a reader who has decided
to try the product can act without leaving the site or the fold's neighbourhood.
Its calls to action and its documentation navigation entries MUST point at the
documentation surface rather than at off-site anchors, and the landing MUST NOT
link the reference material from anywhere else. It MUST NOT carry the full
command tables, the configuration block, or the operational detail, which belong
to the documentation surface.

#### Scenario: A convinced reader can act

- **WHEN** a reader decides to try the product from the landing page
- **THEN** an install command is present in the hero in the same block as the calls to action, and the page's calls to action stay on the site

#### Scenario: The install line is adjacent, not merely present

- **WHEN** the install block is rendered
- **THEN** it sits directly below the hero actions as one ruled row, states the command and the release facts, and is not separated from the calls to action by any other content

#### Scenario: Every documentation link is a door, not a detour

- **WHEN** the landing page's links are reviewed
- **THEN** every call to action and documentation navigation entry resolves to the documentation surface, and no other link on the page points at reference material

#### Scenario: Detail is not duplicated

- **WHEN** the landing page is reviewed for reference material
- **THEN** it contains no command table, no configuration block, and no more than four named commands, and the documentation surface is the only place the full set lives

### Requirement: Text ink stays inside the sheet at every width

Every text element on the site MUST render with its glyph ink inside the
viewport at every supported width. A container may bleed past the sheet edge only
where it carries no text. Where the page frame clips horizontal overflow, that
clip MUST NOT remove any part of a character.

#### Scenario: Ink is measured, not the box

- **WHEN** the page is rendered at any supported width
- **THEN** the rightmost and leftmost ink rect of every text element lies within the viewport, measured on the text runs themselves

#### Scenario: The frame's clip cannot hide a defect

- **WHEN** a horizontal clip is applied to the page frame
- **THEN** the check for overflowing text is performed against text rects, because the clip removes the overflow from the scrollable region and makes a scroll-width comparison report success

#### Scenario: A deliberate bleed carries no text

- **WHEN** a container is positioned past the sheet edge
- **THEN** any text inside it is inset far enough that no glyph reaches the clip boundary

### Requirement: The composed page survives a script failure

The site SHALL render its complete content when its JavaScript fails to load or
throws. Motion and progressive enhancement MAY be gated on a script-provided
class, but no content, artwork, or text MUST be permanently hidden by a class
that only the script can remove.

#### Scenario: The script never arrives

- **WHEN** the site's script fails to load, is blocked, or throws before completing
- **THEN** the hero artwork, the annotations, and every section are visible without interaction

#### Scenario: The enhancement class is reversible

- **WHEN** the script cannot run
- **THEN** the enhancement class is not left applied, so the styles that hide content for animation do not take effect

### Requirement: The printed page keeps its dark surfaces legible

The site's print stylesheet SHALL keep every dark-ground surface readable on
paper: text that is light-on-dark on screen MUST either keep its ground when
printed or be re-coloured for a white ground.

#### Scenario: Dark surfaces print readable

- **WHEN** the page is printed with background graphics disabled
- **THEN** no text is rendered light-on-white, and every dark-ground section remains legible
