# Spec Delta

## ADDED Requirements

### Requirement: A deferral is resolved by identity, not by content

The deferred-call registry SHALL identify each pending call by a stable identity that payload mutation cannot change, so a call that resolves always finds and closes its own entry, and a batch scope SHALL drain its registry to empty when every deferred call has resolved or been flushed.

#### Scenario: A mutated payload still closes its span

- **WHEN** a deferred call's payload is mutated after collection and the call is then forced
- **THEN** its registry entry SHALL be removed and its embed span SHALL be ended, leaving no entry behind

#### Scenario: A resolved scope drains to empty

- **WHEN** every deferred call in a scope has resolved
- **THEN** the thread's deferral registry SHALL be empty
