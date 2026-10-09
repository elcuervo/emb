# script-eval

## Purpose

Lets clients run Redis-style scripts over config-mounted ONNX models, so non-embedding models (e.g. GLiNER extractors) can be served through the same RESP2 surface as embeddings, with the script and its parameters supplied per request.

## Requirements

### Requirement: EVAL evaluates a script against a model

The server SHALL support `EMB.EVAL <model> <script> <numtexts> <text...> <arg...>`: the script is compiled and executed against the named model, with the `numtexts` texts exposed to the script as `KEYS` and the remaining args as `ARGV` (positional, Redis-style). The model SHALL already be mounted in the server config (`models:`); scripts and labels never require config changes.

#### Scenario: Inline script with dynamic labels

- **WHEN** a client sends `EMB.EVAL` with a GLiNER script, `numtexts=1`, one text, and labels as args
- **THEN** the script executes with `KEYS={text}`, `ARGV={labels...}` and the reply is the script's converted return value

#### Scenario: Wrong numtexts

- **WHEN** `numtexts` does not match the actual number of text arguments
- **THEN** the server replies with a wrong-arity error and does not execute the script

#### Scenario: Unknown model

- **WHEN** `EMB.EVAL` names a model that is not mounted in config
- **THEN** the server replies with a model-not-found error and does not execute the script

### Requirement: Script identity and per-model caching

Scripts SHALL be cached per model keyed by SHA1 of the script source. `EMB.SCRIPT LOAD <model> <script>` SHALL compile and cache the script, replying with its SHA1. `EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>` SHALL execute the cached script by SHA1. `EMB.SCRIPT EXISTS <model> <sha...>` SHALL reply with an array of 1/0 flags, and `EMB.SCRIPT FLUSH [<model>]` SHALL clear cached scripts (all models when the model name is omitted).

#### Scenario: Load then evaluate by SHA

- **WHEN** a client loads a script with `EMB.SCRIPT LOAD` and then sends `EMB.EVSHA` with the returned SHA1
- **THEN** the cached script executes with the same KEYS/ARGV semantics as `EMB.EVAL`

#### Scenario: Unknown SHA

- **WHEN** `EMB.EVSHA` names a SHA1 that is not cached for the model
- **THEN** the server replies with a no-such-script error

#### Scenario: Exists reflects the per-model cache

- **WHEN** a script is loaded for model A but not for model B
- **THEN** `EMB.SCRIPT EXISTS A <sha>` replies `[1]` and `EMB.SCRIPT EXISTS B <sha>` replies `[0]`

#### Scenario: Load rejects an invalid script

- **WHEN** `EMB.SCRIPT LOAD` receives a script that fails to compile
- **THEN** the server replies with an error and caches nothing

#### Scenario: Boot-preloaded script is present without LOAD

- **WHEN** a script was preloaded at boot from a config file path
- **THEN** `EMB.SCRIPT EXISTS` reports it and `EMB.EVSHA` executes it without any client-side `EMB.SCRIPT LOAD`

### Requirement: Lua-to-RESP reply conversion

The server SHALL convert each script's return value to RESP2 using a Redis-faithful grammar: Lua string → bulk (byte-safe; UTF-8/JSON/binary all valid), integral Lua number → integer reply, non-integral Lua number → bulk string (RESP2 has no double, so the decimal is not truncated), list-form table (sequential integer keys from 1) → array reply, string-keyed table → hash reply as flat field/value pairs (HGETALL shape), `nil`/`false` → null, and a table with an `err` string field → error reply (an `err` value containing CR or LF SHALL be rejected so the error cannot splice extra RESP frames). Values SHALL nest recursively (a hash value may be an array, hash, bulk, integer, or null).

#### Scenario: Hash reply from an entities table

- **WHEN** a script returns `{PERSON={"Tim Cook"}, ORG={"Apple"}}`
- **THEN** the reply is a flat 4-element field/value array: `PERSON`, `["Tim Cook"]`, `ORG`, `["Apple"]`

#### Scenario: Array-of-hashes for multiple texts

- **WHEN** `numtexts=2` and each text yields an entities table
- **THEN** the reply is a 2-element array whose elements are the per-text converted values (hashes)

#### Scenario: Raw bytes from a string

- **WHEN** a script returns a Lua string containing raw bytes
- **THEN** the reply is a single bulk string containing exactly those bytes

#### Scenario: Script-reported error

- **WHEN** a script returns `{err="message"}`
- **THEN** the server replies with an error carrying that message

### Requirement: Sandboxed execution with budgets

Script execution SHALL be sandboxed and isolated: each evaluation runs in a fresh interpreter with only whitelisted host functions (`emb.run`, `emb.run_batch`, `emb.tokenize.pretokenized`, `emb.tokenize.words`, `emb.tokenize.encode`, `emb.tokenize.encode_pair`, `emb.math`, `emb.image.preprocess`, `emb.image.info`, `json`) and a curated standard-library subset; `io`, `os`, `module`, file/network access, FFI, `math.random`, and all time functions SHALL be unavailable. Text arguments (KEYS) SHALL be binary-safe: arbitrary bytes SHALL be delivered to the script unmodified, so image content can be passed as a KEYS element without encoding. Each evaluation SHALL be bounded by a wall-clock deadline (enforced at VM instruction granularity), a call-stack depth limit, and a script-size cap; exceeding any bound replies with an error for that request only and never affects other in-flight requests. Scripted evaluation commands SHALL count toward the server's `max_concurrent_requests` gate.

#### Scenario: Infinite loop in one request

- **WHEN** a script runs past its wall-clock deadline
- **THEN** that request replies with an execution-time error while concurrent requests continue normally

#### Scenario: Oversized script

- **WHEN** a script exceeds the configured size cap
- **THEN** the server replies with an error and does not compile or cache it

#### Scenario: Forbidden library is absent

- **WHEN** a script attempts to use `os` or `io` functions
- **THEN** the script fails with an unknown-function error at runtime (or compile time), never touching the host

#### Scenario: Binary KEYS are delivered unmodified

- **WHEN** an `EMB.EVAL`/`EMB.EVSHA` text argument contains arbitrary bytes (for example image content with NUL and high-bit bytes)
- **THEN** the script receives those exact bytes in `KEYS`, and the sandbox remains network-free

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

### Requirement: Math helper functions

The server SHALL provide an `emb.math` module with `sigmoid`, `softmax`, and `argmax`, accepting either a single number or an array of numbers. `sigmoid(x)` SHALL compute 1/(1+exp(−x)) element-wise for arrays (vectorized, returning an array). `softmax` SHALL be numerically stable (subtract the max before exponentiating) and return probabilities summing to 1. `argmax` SHALL return the index (1-based) and value of the first maximum element. Empty arrays SHALL error; non-numeric elements SHALL error.

#### Scenario: Vectorized sigmoid over a score array

- **WHEN** a script calls `emb.math.sigmoid({0.5, 1, -1})`
- **THEN** the result is an array of the three sigmoid values

#### Scenario: Stable softmax and argmax

- **WHEN** a script calls `emb.math.softmax({1000, 1001, 999})` and `emb.math.argmax({3, 7, 1})`
- **THEN** softmax returns finite probabilities (no overflow) and argmax returns index 2 with value 7

#### Scenario: Empty input errors

- **WHEN** a script passes an empty array to `emb.math.argmax` or `emb.math.softmax`
- **THEN** the evaluation fails with an error reply

### Requirement: Plain and pair tokenization

The server SHALL provide `emb.tokenize.encode(text, maxLength)` and `emb.tokenize.encode_pair(first, second, maxLength)` using the model tokenizer's own pretokenization pipeline (not the word-splitting block). `encode` SHALL return `{ids, mask, offsets}` where offsets are per-token byte ranges into `text`; `encode_pair` SHALL compose the BERT-family pair template `[CLS] first [SEP] second [SEP]` and additionally return the `sep` token position (the separator between the two parts) and per-part offsets (tokens of `first` map into `first`, tokens of `second` map into `second`). Models with non-BERT pair templates SHALL remain script-composable via `encode` plus explicit separator token ids.

#### Scenario: Plain encode matches the embedding path

- **WHEN** a script encodes a text with `emb.tokenize.encode`
- **THEN** the ids and mask equal the values the embedding pipeline's tokenizer produces for the same text

#### Scenario: Pair encode composes the pair template

- **WHEN** a script calls `emb.tokenize.encode_pair("who founded Apple", "Apple was founded in 1976.", 512)`
- **THEN** ids equal `[CLS]` + encode(first) + `[SEP]` + encode(second) + `[SEP]`, `sep` points at the separator token, and offsets of `second`-owned tokens slice the `second` string directly

#### Scenario: QA answer slicing via offsets

- **WHEN** a script selects start/end logits positions and slices `second` at the corresponding offsets
- **THEN** the sliced text is the answer surface string (no token-decode block required)

### Requirement: Structured extraction reference behavior

The server SHALL serve GLiNER-style extraction end-to-end: with the `gliner2-multi-v1` ONNX mounted and a reference script loaded, `EMB.EVSHA` with a text and entity labels in ARGV SHALL return a hash whose fields are the labels and whose values are arrays of extracted entity strings. Extraction SHALL match the reference decoder of the model repository (span search over word-start positions, sigmoid thresholding, best-span selection, overlap suppression).

#### Scenario: Extract entities from a sentence

- **WHEN** `EMB.EVSHA` runs the reference script against `"Apple CEO Tim Cook announced iPhone 15."` with labels `PERSON`, `ORG`, `PRODUCT`
- **THEN** the reply is a hash with `PERSON: ["Tim Cook"]`, `ORG: ["Apple"]`, `PRODUCT: ["iPhone 15"]`

#### Scenario: Different labels, same script

- **WHEN** the same script and text are run with a different label set
- **THEN** the reply is a hash whose fields are exactly the requested labels, with array values of extracted entity strings (matching the reference decoder's output for those labels)

### Requirement: Image preprocessing host block

For a model configured with an `image:` block, the server SHALL provide `emb.image.preprocess(bytes)` to scripts: it SHALL decode the raw image bytes and apply the model's configured image preprocessing (size, crop, resample, rescale, mean, std), returning a tensor spec `{shape = {1, 3, H, W}, bytes = <little-endian float32>, dtype = "f32", input = <configured input tensor name>}` that can be passed directly to `emb.run` or `emb.run_batch`. The server SHALL also provide `emb.image.info()` returning the model's configured preprocessing parameters. Preprocessing SHALL be deterministic (identical bytes and config produce identical output), SHALL be bounded by the same per-image byte and decoded-pixel caps as `EMB.IMG`, and SHALL be charged against the evaluation's tensor budget. Calling `emb.image.preprocess` for a model without an `image:` block, or on undecodable bytes, SHALL raise an error. This block SHALL add no network capability.

#### Scenario: Image bytes become a runnable tensor

- **WHEN** a script calls `emb.image.preprocess(KEYS[1])` with image bytes for a model with `image: {input: pixel_values, size: 224}`
- **THEN** it receives `{shape = {1, 3, 224, 224}, bytes = <4*3*224*224 bytes>, dtype = "f32", input = "pixel_values"}` that `emb.run` accepts without a per-element Lua table

#### Scenario: Deterministic and cacheable

- **WHEN** the same script runs twice with identical binary KEYS and args
- **THEN** the preprocessed tensor is byte-identical and the reply is served from cache on the second request

#### Scenario: No image config errors

- **WHEN** a script calls `emb.image.preprocess` for a model without an `image:` block
- **THEN** the evaluation fails with an error naming the model

#### Scenario: Image block adds no network

- **WHEN** a script uses `emb.image.preprocess`
- **THEN** the sandbox remains network-free and no fetch is performed

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

### Requirement: Script API version is the server version

The server SHALL expose `emb.API_VERSION` as a string carrying the server's own
version — the repository `VERSION` value injected into the binary — so `emb` has
a single version and the scripted surface reports the server a script is talking
to rather than a second numbering scheme. It SHALL fall back to `dev` when no
build version was injected, matching `INFO`'s `emb_version`. The value SHALL be
folded into reply-cache identity so an upgrade can never serve a reply computed
under an older surface.

#### Scenario: Version is readable

- **WHEN** a script returns `emb.API_VERSION`
- **THEN** the reply is a non-empty string

#### Scenario: Version tracks the server

- **WHEN** the server was built with a version injected from `VERSION`
- **THEN** `emb.API_VERSION` equals that value and equals the `emb_version`
  reported by `INFO`

#### Scenario: Unset build version falls back

- **WHEN** the server runs without an injected build version (`go test`,
  `go run`, an unlabelled build)
- **THEN** `emb.API_VERSION` is `dev`

#### Scenario: A version change invalidates cached replies

- **WHEN** a script reply is cached under one version and the server restarts on
  a different version with the same cache snapshot
- **THEN** the request misses and recomputes

#### Scenario: The server never refuses on version grounds

- **WHEN** a script asserts a capability against `emb.API_VERSION` and the
  capability is absent
- **THEN** the script can report its own error instead of the server refusing to
  evaluate it
