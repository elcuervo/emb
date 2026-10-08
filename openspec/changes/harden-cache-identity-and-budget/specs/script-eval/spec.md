## ADDED Requirements

### Requirement: Script reply keys distinguish input representations

Script reply key derivation SHALL distinguish literal short inputs from digested long inputs, including when a literal input is byte-identical to the digest representation of another input. Inputs at or below `CacheKeyInlineLimit` SHALL retain an inline tail, larger inputs SHALL retain a bounded digest tail, and arbitrary binary input SHALL remain supported. A fixed derivation-version domain SHALL separate current script reply keys from legacy keys that lacked representation separation. The existing model/script prefix, per-text cache contract, argument framing, configuration identity, and host API identity SHALL be preserved. This key-only change SHALL NOT change the script host API version.

#### Scenario: Literal digest text cannot reuse a long-input reply

- **GIVEN** a long payload and a distinct short input equal to `#` followed by its hexadecimal SHA-256
- **WHEN** the same single-text script is evaluated for both inputs with identical model, arguments, and configuration
- **THEN** the inputs SHALL have distinct reply cache keys
- **AND** both replies SHALL match their respective uncached evaluations in either priming order

#### Scenario: Boundary and binary inputs remain distinct

- **WHEN** inputs at and above the inline limit or inputs containing NUL and high-bit bytes are keyed
- **THEN** their keys SHALL deterministically preserve input identity without representation aliasing
- **AND** long payloads SHALL NOT be retained verbatim in the key

#### Scenario: Legacy key is not a fallback

- **GIVEN** a cache contains a legacy script reply key for an otherwise identical request
- **WHEN** a current request performs its lookup
- **THEN** it SHALL miss that legacy identity and compute a fresh reply
- **AND** repeated current requests SHALL hit the new identity normally
