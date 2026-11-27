# Spec Delta

## ADDED Requirements

### Requirement: A deferred or block call snapshots its payloads

A deferred call (`lazy: :multi` / `:batch`) and an explicit `client.multi` block SHALL retain a copy of every payload they collect — text strings and encoded image bytes — so a caller mutating or re-tagging those strings after the call cannot change what is sent. The copy SHALL happen at collection time, not at dispatch.

#### Scenario: Mutating deferred text changes nothing

- **WHEN** a caller defers a text embed and then mutates the same String before forcing the batch
- **THEN** the server SHALL receive the text as it was at the call, unchanged

#### Scenario: Mutating multi-block text changes nothing

- **WHEN** a caller adds a text to a `client.multi` block and then mutates the String before the block returns
- **THEN** the sent `EMB.MULTI` pair SHALL carry the original text

#### Scenario: Mutating multi-block image bytes changes nothing

- **WHEN** a caller adds image bytes to a `client.multi` block and then appends to the same String before the block returns
- **THEN** the sent `EMB.IMGMULTI` pair SHALL carry the original bytes

### Requirement: A failed share is isolated to the calls it carries

When one share of a serial batch fails, only the calls in that share SHALL fail; calls from a different client in the same batch window SHALL be dispatched on their own or fail with their own error, never with the other share's error.

#### Scenario: A failing client does not poison another

- **WHEN** two clients defer text in one batch window and the first client's command fails
- **THEN** forcing the second client's call SHALL NOT raise the first client's error, and SHALL send the second client's command

### Requirement: Image coercion leaves the caller's object untouched

Coercing an image argument to binary bytes SHALL return a new String and SHALL NOT change the encoding, contents, or identity of the caller's String.

#### Scenario: Caller string keeps its encoding

- **WHEN** a UTF-8 String is passed as image bytes
- **THEN** after the call the caller's String SHALL still be UTF-8 and unchanged
