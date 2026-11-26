# Spec Delta

## ADDED Requirements

### Requirement: Ruby client supports cross-model image batches

The Ruby client SHALL expose module-level and instance-level `image_multi(pairs, format:)` methods that send ordered `[model, image_bytes]` pairs through `EMB.IMGMULTI`. `format:` SHALL accept `:binary` (default) and `:values`; results SHALL preserve pair order, decode each pair using its configured output names, and map a failed or truncated pair to `nil`.

#### Scenario: Cross-model image results preserve order
- **WHEN** `image_multi` receives pairs for two models and both succeed
- **THEN** it returns one output-name-to-value map per pair in the original order
- **AND** each map uses the reply decoder for the selected format

#### Scenario: One failed image pair does not fail its siblings
- **WHEN** the server returns a null for one pair in an `EMB.IMGMULTI` reply
- **THEN** the corresponding result is `nil` and other pair results are decoded normally

#### Scenario: Binary image bytes are preserved
- **WHEN** `image_multi` receives image bytes containing NUL or high-bit bytes
- **THEN** the client sends the exact bytes as binary-safe RESP bulk arguments without text transcoding or URL fetching

#### Scenario: VALUES format is decoded per pair
- **WHEN** `image_multi` is called with `format: :values`
- **THEN** it sends the leading `VALUES` keyword and returns each output's self-describing VALUES envelope

#### Scenario: Invalid format is rejected before sending
- **WHEN** `image_multi` receives a format other than `:binary` or `:values`
- **THEN** it raises an argument error and sends no command
