## MODIFIED Requirements

### Requirement: Content-addressed reply caching

When the server cache is enabled, scripted replies SHALL be cached under a content-addressed key derived from model name, script SHA1, the args, the number of KEYS in the evaluation, and the text — distinct args (e.g. different label sets) SHALL be distinct cache entries, and the arg hash SHALL keep argument boundaries unambiguous (nil, empty, and NUL-containing arguments are distinct keys). Including the KEYS count SHALL keep a single-text evaluation and a multi-text evaluation of the same script in separate namespaces, because the server interprets their return values differently: a single-text call replies with the script's whole return value, while a multi-text call replies with one element per text. An evaluation whose KEYS contain a repeated text SHALL bypass the reply cache entirely (no lookup, no store), because one per-text key cannot represent two element replies for the same text. A cache hit (every text of a request) SHALL reply without re-running the script or model inference. When any text of a request misses, the server SHALL evaluate the script once with ALL the request texts as KEYS and merge the per-text results by their original indexes, so cache state never changes the inputs or the reply shape a script produces. Per-text caching assumes a script's reply for a text depends only on (model, script SHA1, args, KEYS count, text) — a script whose per-text output reads sibling texts in KEYS is outside the cache contract.

#### Scenario: Same script and labels hit the cache

- **WHEN** `EMB.EVSHA` is sent twice with the same model, SHA1, args, KEYS count, and text
- **THEN** the second reply is served from cache with identical bytes

#### Scenario: Different labels miss the cache

- **WHEN** the script is evaluated with a different arg set
- **THEN** the evaluation runs the script again and stores under a different key

#### Scenario: Repeated KEYS bypass the reply cache

- **GIVEN** the server cache is enabled and a position-dependent script returning `[1, 2]` for `KEYS=[x, x]`
- **WHEN** that exact request is sent twice
- **THEN** the second reply SHALL equal the first (`[1, 2]`), because a repeated text makes the evaluation uncacheable
- **AND** neither request SHALL leave a per-text cache entry for `x`

### Requirement: Bounded script reply-cache keys

The content-addressed script reply-cache key SHALL NOT inline large text payloads. When a KEYS element exceeds a small documented threshold, the key SHALL incorporate a digest of that element instead of its raw bytes, so image-sized KEYS do not retain megabytes per cache entry. Short text elements SHALL be inlined unchanged in the key. The key SHALL be deterministic and content-addressed: identical inputs (model, script SHA1, args, KEYS count, text) always map to the same key, and distinct inputs map to distinct keys. Because the key encodes the evaluation's call shape, its bytes MAY change when the key derivation is extended (for example to add the KEYS count); a changed key is a cache miss, never a wrong hit.

#### Scenario: Image KEYS do not bloat the key

- **WHEN** a scripted image request is cached
- **THEN** the cache key size is bounded (a digest, not the full image bytes) and the reply is identical to an uncached run

#### Scenario: Text keys are unchanged

- **WHEN** a short text scripted request is cached
- **THEN** the text payload SHALL be inlined in the key exactly as provided (not digested)
- **AND** the same inputs SHALL always produce the same key

#### Scenario: Distinct images remain distinct entries

- **WHEN** two different images are embedded through the same script
- **THEN** they occupy distinct cache entries and each returns its own embedding

#### Scenario: Arity separates entries

- **WHEN** the same script is evaluated with one text and again with two texts that include that text
- **THEN** the two evaluations SHALL use different cache keys

#### Scenario: Arity change does not replay a stale reply

- **GIVEN** the server cache is enabled and a script whose reply depends on `#KEYS`
- **WHEN** the script is evaluated with one text, then with two texts, then with that first text again
- **THEN** the third evaluation SHALL return the same value a cold single-text evaluation returns
- **AND** it SHALL NOT return the value cached during the two-text evaluation
