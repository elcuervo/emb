## MODIFIED Requirements

### Requirement: Rails Railtie auto-installs the Rack middleware

The gem SHALL ship a Rails Railtie that, when the gem is loaded in a Rails application, inserts `Emb::Middleware` into the application's middleware stack during initialization, so request-scoped batch clearing is active without manual configuration. Insertion SHALL NOT depend on inspecting the application's middleware stack at initialization time, because the pre-build stack supports appending operations but not inspection. Insertion SHALL be additive and SHALL be safe when an operator also mounts `Emb::Middleware` manually: because scope clearing is idempotent, a duplicated middleware SHALL NOT change request-scoped clearing behavior. An operator SHALL be able to opt out via `config.emb.middleware = false` and mount the middleware themselves.

#### Scenario: Middleware auto-inserted in a Rails app

- **WHEN** a Rails application loads the gem without manual middleware configuration and boots
- **THEN** booting SHALL NOT raise
- **THEN** the built middleware stack SHALL include `Emb::Middleware` exactly once

#### Scenario: Manual insertion is not duplicated

- **WHEN** an application sets `config.emb.middleware = false` and mounts `Emb::Middleware` itself
- **THEN** the built middleware stack SHALL include `Emb::Middleware` exactly once

#### Scenario: Opt-out disables auto-insertion

- **WHEN** an application sets `config.emb.middleware = false`
- **THEN** the middleware stack SHALL NOT include `Emb::Middleware` from the Railtie

#### Scenario: Duplicate insertion is safe

- **WHEN** an application mounts `Emb::Middleware` manually while `config.emb.middleware` is left at its default
- **THEN** each request SHALL still start and end with a cleared batch scope
- **THEN** the presence of more than one copy SHALL NOT change request-scoped clearing behavior
