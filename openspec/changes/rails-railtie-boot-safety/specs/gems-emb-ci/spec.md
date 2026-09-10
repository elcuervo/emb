## ADDED Requirements

### Requirement: Real-Rails boot validation

The `emb` gem's test suite SHALL exercise the Railtie against a real Rails application rather than only against hand-rolled Rails fakes. Rails SHALL be a development-only dependency, and the suite SHALL boot a minimal application and assert middleware behavior against the application's built middleware stack after initialization.

#### Scenario: Real Rails app boots without raising

- **WHEN** the integration suite boots a minimal Rails application with the gem loaded
- **THEN** initialization SHALL succeed
- **THEN** the built middleware stack SHALL include `Emb::Middleware` exactly once

#### Scenario: Opt-out is asserted on the built stack

- **WHEN** the integration suite boots the application with `config.emb.middleware = false`
- **THEN** the built middleware stack SHALL NOT include `Emb::Middleware`

#### Scenario: Duplicate mount remains functionally safe

- **WHEN** the application mounts `Emb::Middleware` manually and the Railtie also inserts it
- **THEN** every request through the application SHALL leave the batch scope cleared

#### Scenario: Test doubles cannot exceed the real API

- **WHEN** the suite defines a stand-in for a Rails internal used by the Railtie
- **THEN** a validation SHALL fail if the stand-in exposes a method the real Rails class does not define

#### Scenario: Supported Rails versions are exercised in CI

- **WHEN** CI runs the gem test job
- **THEN** it SHALL run the integration suite for each supported Ruby and Rails version
- **THEN** the suite SHALL run as part of the gem's default `rake` task
