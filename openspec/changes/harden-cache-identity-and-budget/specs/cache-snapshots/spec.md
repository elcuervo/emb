## ADDED Requirements

### Requirement: Script reply key upgrades preserve snapshot readability

Changing script reply-key derivation SHALL NOT require a snapshot file-format change. Structurally valid old snapshots SHALL remain readable under existing integrity, model-fingerprint, and memory admission rules. Legacy ambiguous script keys SHALL NOT serve current requests, while compatible text/image entries SHALL remain addressable. Legacy script entries MAY remain within the bounded cache until normal eviction; the server SHALL NOT use legacy script keys as a lookup fallback.

#### Scenario: Mixed legacy snapshot is restored

- **GIVEN** a valid snapshot containing legacy script keys and compatible text/image embedding keys
- **WHEN** an upgraded server restores it and admits the corresponding models
- **THEN** requests for legacy scripted replies SHALL miss and recompute
- **AND** compatible text/image requests SHALL hit their restored embeddings
- **AND** existing restore and live-cache budgets SHALL remain enforced

#### Scenario: Current script keys survive restart

- **GIVEN** scripted replies cached with the new derivation
- **WHEN** they are saved and restored with matching model fingerprints
- **THEN** identical current scripted requests SHALL hit their restored replies
