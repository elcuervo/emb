## MODIFIED Requirements

### Requirement: Three ground-varied capability blocks before the terrain

The site SHALL carry the capability material as three distinct blocks — protocol,
script inference, and operations — placed between the hero and the terrain, and
the terrain SHALL be the last section before the footer. Each block MUST be
given a distinguishable ground or measure so the sequence reads as three
movements rather than one list. The hero, wordmark, claim, sub, actions, six-row
feature list, and pipeline artwork MUST remain unchanged.

#### Scenario: The capability region covers all four themes

- **WHEN** the capability blocks are rendered
- **THEN** they present the Redis protocol, `EMB.EVAL`/`EMB.EVSHA` script inference, operational commands, and the `emb-top` dashboard

#### Scenario: The poster composition is preserved

- **WHEN** the page is rendered after this change
- **THEN** the masthead, hero spread, wordmark, claim, sub, actions, six-row feature list and pipeline artwork are pixel-equivalent to their pre-change state, and the relocation of the terrain to the end of the page is the only structural change

#### Scenario: Document height is not a constraint

- **WHEN** the recomposition makes the page longer or changes its ratio
- **THEN** the change is accepted and the ratio is not treated as a regression

#### Scenario: The terrain closes the page

- **WHEN** the page is rendered
- **THEN** the terrain is the last section before the footer and no capability content follows it

#### Scenario: The grounds differ

- **WHEN** the three blocks are compared
- **THEN** at least one is a full-bleed dark ground, and the remaining grounds or measures differ from each other and from the hero

## RENAMED Requirements

- FROM: `### Requirement: One merged capability block below the terrain`
- TO: `### Requirement: Three ground-varied capability blocks before the terrain`

## ADDED Requirements

### Requirement: The spine is one continuous element from the wordmark to the massif

The spine SHALL be a page-level structural element: a single orange line running
from the `emb` wordmark to the terrain, present in every section between them,
with no visible gap or lateral step at any section boundary. Its horizontal
position MUST be derived from one named axis so the hero's SVG spine, the block
spines, and the terrain route cannot drift apart.

#### Scenario: No gap at a section boundary

- **WHEN** the rendered spine is measured at each section boundary
- **THEN** the segments join with no vertical gap and no lateral offset

#### Scenario: One axis, measured

- **WHEN** the spine's rendered x-position is compared with the terrain route's fork position
- **THEN** they agree within 1px at every tested width, including the 1086px reference frame

#### Scenario: The spine never carries text

- **WHEN** body copy is rendered
- **THEN** no text sits on the spine; the line either passes behind content or crosses a ground prepared for it

### Requirement: The spine is drawn from the page's own tokens

The spine MUST use the existing accent and the existing plate geometry; it MUST
NOT introduce a new colour, a new stroke-weight system, or a new asset.

#### Scenario: Spine and plate stroke agree

- **WHEN** the CSS spine's rendered width is compared with the SVG spine's rendered stroke width
- **THEN** they match, including when the plate width is at its minimum

### Requirement: Code is typeset and highlighted as code

Every specimen that is code — shell invocations, Lua source, and the console's
replayed commands and replies — SHALL be presented in the mono face with a token
treatment: command or keyword, string literal, numeric, and comment or meta.
Token colours MUST be defined for both the paper ground and the dark ground, and
every token colour MUST meet WCAG AA contrast against the ground it is used on.

#### Scenario: Tokens are distinguishable and legible

- **WHEN** a code specimen is rendered on paper
- **THEN** each token class has a distinct treatment and every token colour measures at least 4.5:1 against the paper

#### Scenario: The dark ground has its own tokens

- **WHEN** a code specimen is rendered inside the dark block
- **THEN** it uses the dark ground's token colours and every one measures at least 4.5:1 against that ground

#### Scenario: Highlighting is not decoration

- **WHEN** a specimen is examined
- **THEN** the highlighting marks what the token is (a command, a string, a number, a comment) and does not colour text for emphasis

### Requirement: The inverted block inverts the full type ladder

The dark block MUST carry its own ink, dim-voice, rule, and accent values, and
every text and control colour in it MUST meet WCAG AA against its own
background. The page's committed type floor MUST hold inside it.

#### Scenario: The dark ground's floor is measured

- **WHEN** the dark block is rendered at the 1086px reference frame and at 390px
- **THEN** its smallest computed text size is at least 12px and 14px respectively

#### Scenario: The dark ground's contrast is verified

- **WHEN** the dark block's text and control colours are measured
- **THEN** each meets at least 4.5:1 for text and 3:1 for non-text indicators

### Requirement: The terrain's route still starts where the spine ends

The terrain's route MUST begin at the spine's axis, and the spine MUST reach the
terrain's first drawable point, at every width where both are rendered.

#### Scenario: The handover holds

- **WHEN** the terrain is at the end of the page and the reader scrolls to it
- **THEN** the orange line is continuous from the last block into the ridge route, with no gap and no lateral step
