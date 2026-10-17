## ADDED Requirements

### Requirement: Every surface meets the type floor and AA contrast on its own ground

The site's committed type floor — 12px on desktop, 14px on mobile — SHALL hold on
every ground, not only on the dark console, and every text and control colour
MUST meet WCAG AA contrast against the ground it actually sits on: 4.5:1 for
body text and 3:1 for large text. Functional text below the floor MUST NOT ship,
including the labels inside a plate's own diagram, and the world's records —
`DESIGN.md` and each surface brief — SHALL name the ground and the type the site
actually uses, so the description and the CSS cannot drift apart.

The composition's atoms and its bans are unchanged by this requirement: grounds
may still be full-bleed, the ruled ledgers, plates and signal axis stay, and no
card, gradient, radius, glass effect or second accent is introduced to satisfy a
floor.

#### Scenario: The floor holds on paper

- **WHEN** the smallest functional text on a paper ground is measured
- **THEN** it is at or above the committed floor for the width it renders at

#### Scenario: Contrast is met per ground

- **WHEN** text and control colours are measured against their own ground
- **THEN** every pair meets AA for its size, on the paper grounds and on the dark ground alike

#### Scenario: A floor repair keeps the composition

- **WHEN** type is raised to meet the floor, or a ground gains its inset
- **THEN** the section's atoms, its ground variation and its place in the sequence are unchanged

#### Scenario: A world change updates its records

- **WHEN** the ground, the type scale or the body face changes
- **THEN** `DESIGN.md` and the affected surface briefs name the new value in the same change
