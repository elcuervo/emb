# Spec Delta

## ADDED Requirements

### Requirement: The primary nav carries the scripting reference

The site's primary navigation SHALL include the scripting reference surface
beside Docs, Demos, and Gem, on every page that carries the nav (the landing,
the docs surface, the demos gallery, the gem surface, and the scripting surface
itself), in both the desktop nav and the mobile disclosure. The scripting
surface SHALL be an existing-atom composition, so it does not add a second
visual language to the site.

#### Scenario: The nav item is present on every page

- **WHEN** any page carrying the primary nav is rendered
- **THEN** it includes a **Scripting** entry in both the desktop nav and the
  mobile disclosure, with the correct relative destination

#### Scenario: The surface is published and tracked

- **WHEN** the published tree is verified
- **THEN** `website/scripting/index.html` is among the expected files, and the
  build fails if it drifts
