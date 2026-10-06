# Spec Delta

## ADDED Requirements

### Requirement: Snapshot header preserves a negative model dimension

The snapshot header SHALL store each model's dimension so that a negative
dimension reads back equal to its configured value, because a model with no
pooled output reports `-1` and would otherwise fail its fingerprint comparison
before any entry is considered. Non-negative dimensions SHALL remain
byte-compatible with snapshots written before this change.

#### Scenario: A dimension-less model's entries are considered

- **GIVEN** a snapshot whose model header was written with dimension `-1`
- **WHEN** the restore compares it against the configured model, also `-1`
- **THEN** the dimensions SHALL compare equal
- **AND** its entries SHALL be admitted or quarantined rather than skipped

#### Scenario: Positive dimensions are unchanged

- **WHEN** a snapshot written with a positive dimension is read
- **THEN** the stored dimension SHALL equal the configured dimension

### Requirement: Restored entries are validated per key family

Entry validation SHALL branch on the cached key's family. A key addressing a
float32 embedding (`txt:` or `img:`) SHALL be admitted only when its value length
is the model's dimension times four. An opaque key — a scripted reply — SHALL be
validated by model fingerprint alone, because it has no dimension. The
whole-file checksum, the bounded per-entry length, and the shared restore budget
SHALL apply to both families.

#### Scenario: Script replies restore for a dimension-less model

- **GIVEN** a snapshot holds a scripted reply for a model whose dimension is `-1`
- **WHEN** that model's fingerprint matches on restore
- **THEN** the reply SHALL be admitted or quarantined for that model
- **AND** it SHALL NOT be discarded for having no embedding dimension

#### Scenario: Embedding length is still enforced

- **WHEN** a `txt:` or `img:` entry's value length is not the model's dimension
  times four
- **THEN** that entry SHALL be skipped as incompatible

#### Scenario: Opaque value lengths are not dimension-checked

- **WHEN** an opaque entry's value is any length that fits the restore bound and
  its model's fingerprint matches
- **THEN** the entry SHALL be retained rather than skipped for its length

### Requirement: Script evaluation admits quarantined entries

A quarantined entry SHALL be admitted when its model is first resolved for a
scripted evaluation, before that evaluation's cache lookup, so a restored reply
is served instead of recomputed. This SHALL hold for a model whose embedding pool
never loads.

#### Scenario: First script call serves a restored reply

- **GIVEN** a restored reply is quarantined for a model that has not loaded
- **WHEN** a client calls `EMB.EVSHA` for that model and the script's key
- **THEN** the reply SHALL be served from the cache without running the script

#### Scenario: A script-only model still admits

- **GIVEN** a model configured only for scripted use, whose embedding pool is
  never loaded
- **WHEN** its first scripted evaluation runs
- **THEN** its compatible quarantined entries SHALL be admitted
