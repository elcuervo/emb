## MODIFIED Requirements

### Requirement: Math helper functions

The server SHALL provide an `emb.math` module with `sigmoid`, `softmax`, and `argmax`. Each SHALL accept a single number, an array of numbers, or a packed little-endian float32 string. `sigmoid(x)` SHALL compute 1/(1+exp(−x)) element-wise for arrays (vectorized, returning an array). `softmax` SHALL be numerically stable (subtract the max before exponentiating) and return probabilities summing to 1. `argmax` SHALL return the index (1-based) and value of the first maximum element. For a single-number argument the results SHALL be the degenerate scalar forms: `sigmoid(x)`, `softmax(x) == 1`, and `argmax(x)` returning `(1, x)`.

An operand with zero elements SHALL be accepted only where the operation has a defined empty result: `sigmoid` over an empty array SHALL return an empty array, while `softmax` and `argmax` over an empty array SHALL error. Non-numeric elements SHALL error.

#### Scenario: Vectorized sigmoid over a score array

- **WHEN** a script calls `emb.math.sigmoid({0.5, 1, -1})`
- **THEN** the result is an array of the three sigmoid values

#### Scenario: Stable softmax and argmax

- **WHEN** a script calls `emb.math.softmax({1000, 1001, 999})` and `emb.math.argmax({3, 7, 1})`
- **THEN** softmax returns finite probabilities (no overflow) and argmax returns index 2 with value 7

#### Scenario: Scalar arguments are accepted

- **WHEN** a script calls `emb.math.sigmoid(0)`, `emb.math.softmax(5)`, and `emb.math.argmax(5)`
- **THEN** the results are `0.5`, `1`, and the pair `(1, 5)` respectively

#### Scenario: Empty input errors

- **WHEN** a script passes an empty array to `emb.math.argmax` or `emb.math.softmax`
- **THEN** the evaluation fails with an error reply

#### Scenario: Empty sigmoid returns an empty array

- **WHEN** a script passes an empty array to `emb.math.sigmoid`
- **THEN** the result is an empty array

### Requirement: Content-addressed reply caching

When the server cache is enabled, scripted replies SHALL be cached under a content-addressed key derived from model name, script SHA1, the host API version, the args, and the text — distinct args (e.g. different label sets) SHALL be distinct cache entries, and the arg hash SHALL keep argument boundaries unambiguous (nil, empty, and NUL-containing arguments are distinct keys). Folding the host API version into the key SHALL ensure that changing a host function's semantics never serves a reply produced under the old surface. A cache hit (every text of a request) SHALL reply without re-running the script or model inference. When any text of a request misses, the server SHALL evaluate the script once with ALL the request texts as KEYS and merge the per-text results by their original indexes, so cache state never changes the inputs or the reply shape a script produces. Per-text caching assumes a script's reply for a text depends only on (model, script SHA1, host API version, args, text) — a script whose per-text output reads sibling texts in KEYS is outside the cache contract.

#### Scenario: Same script and labels hit the cache

- **WHEN** `EMB.EVSHA` is sent twice with the same model, SHA1, args, and text
- **THEN** the second reply is served from cache with identical bytes

#### Scenario: Different labels miss the cache

- **WHEN** the same script and text are sent with different label args
- **THEN** each distinct arg set is executed and cached separately

#### Scenario: A host-API change invalidates cached replies

- **WHEN** the server's `emb.API_VERSION` changes and a previously cached script is evaluated again
- **THEN** the evaluation runs rather than replaying a reply computed under the older host surface

## ADDED Requirements

### Requirement: JSON values round-trip through the sandbox

The server SHALL provide a `json` module with `encode`, `decode`, and a unique `null` sentinel. Decoding SHALL preserve `null` in both object values and array elements; because a Lua table cannot hold `nil`, the sentinel is stored in its place, and encoding that sentinel SHALL reproduce `null`. Encoding a table with contiguous integer keys `1..n` SHALL produce a JSON array, and a table with string keys SHALL produce a JSON object, so `json.encode(json.decode(s))` SHALL be semantically equal to `s` for any JSON value `s`.

#### Scenario: Object null round-trips

- **WHEN** a script returns `json.encode(json.decode('{"a":null,"b":1}'))`
- **THEN** the reply is `{"a":null,"b":1}`

#### Scenario: Array null round-trips

- **WHEN** a script returns `json.encode(json.decode('[1,null,3]'))`
- **THEN** the reply is `[1,null,3]`

#### Scenario: A user-constructed sentinel encodes as null

- **WHEN** a script calls `json.encode({1, json.null, 3})`
- **THEN** the reply is `[1,null,3]`
