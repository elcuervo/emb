## MODIFIED Requirements

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
