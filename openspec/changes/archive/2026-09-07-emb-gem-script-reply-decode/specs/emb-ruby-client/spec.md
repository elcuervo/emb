# emb-ruby-client — Script reply decoding

## Purpose

Adds explicit, opt-in decoding to the gem's script evaluation surface
(`EMB.EVAL` / `EMB.EVSHA`), so scripts that reply with raw little-endian
float32 bulks (`emb.math.float32_bytes`) — or numeric arrays — can yield float
arrays exactly like the embed path, while every existing scripted reply shape
parses unchanged when no decode is requested.

## MODIFIED Requirements

### Requirement: Script reply decoding (`decode:`)

The gem SHALL accept an optional `decode:` keyword on `eval`, `evalsha` (client
and module level). With no `decode` (or `nil`), existing reply parsing SHALL be
unchanged. The following decode modes SHALL be supported:

- `decode: :f32` — the top-level reply for a single text, or each element of a
  multi-text reply, is treated as a vector: a string bulk SHALL be decoded via
  little-endian float32 `unpack('e*')` into an array of floats; a numeric array
  SHALL pass through as floats.
- `decode: {field => :f32}` — after hash parsing, the named field SHALL be
  decoded as `:f32`; applies recursively for multi-text replies (arrays of
  hashes). Fields absent from a reply SHALL be left untouched.

Unknown `decode:` values SHALL raise an error at call time. A value in a
decodable position that is neither a string bulk nor a numeric array SHALL
raise an error, as SHALL a string whose byte length is not a multiple of 4.

#### Scenario: Single vector bulk reply decodes to floats

- **WHEN** a script returns `emb.math.float32_bytes(vec)` for a 768-dim vector
  and `client.evalsha(model, sha, ["text"], ["normalize"], decode: :f32)` is called
- **THEN** the reply SHALL be an array of 768 floats
- **AND** the values SHALL equal a manual `unpack('e*')` of the raw bulk

#### Scenario: Multi-text vector bulk replies

- **WHEN** `client.evalsha(model, sha, ["a", "b"], ["normalize"], decode: :f32)`
  is called with a script replying one packed bulk per text
- **THEN** the reply SHALL be an array of two float arrays, one per text

#### Scenario: Hash field with a packed vector

- **WHEN** a script replies `{dim = 768, embedding = emb.math.float32_bytes(vec)}`
  and `decode: {embedding: :f32}` is passed
- **THEN** the reply SHALL be a hash with numeric `dim` and an array-of-floats
  `embedding` field
- **AND** for multi-text calls the same decode SHALL apply to each per-text hash

#### Scenario: Legacy numeric-array reply passes through

- **WHEN** a script replies with a plain Lua numeric array (the pre-`float32_bytes`
  style) and `decode: :f32` is passed
- **THEN** the reply SHALL pass through as the same array of floats, unchanged

#### Scenario: No decode keeps existing behavior

- **WHEN** `eval`/`evalsha` are called without `decode:`
- **THEN** every reply shape (string bulk, hash, array, nested) SHALL parse
  exactly as before the change

#### Scenario: Decode errors are explicit

- **WHEN** `decode: :bidirectional` (unknown mode) is passed
- **THEN** the call SHALL raise an error before any command is sent
- **WHEN** `decode: :f32` is passed and the reply is a hash (not a vector)
- **THEN** the call SHALL raise a descriptive error (not silently return
  garbage)
- **WHEN** `decode: :f32` is passed and the bulk byte length is not a multiple
  of 4
- **THEN** the call SHALL raise a descriptive error