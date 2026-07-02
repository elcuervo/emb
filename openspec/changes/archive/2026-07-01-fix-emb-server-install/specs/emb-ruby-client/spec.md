## MODIFIED Requirements

### Requirement: Proxy-based API

The gem SHALL expose `Emb::VERSION` that resolves correctly regardless of install path.

#### Scenario: Version resolves from loaded spec

- **WHEN** `require "emb"` is called
- **THEN** `Emb::VERSION` SHALL be a semver string matching the gem's version
- **THEN** the version SHALL NOT depend on any file relative to the gem's install directory
