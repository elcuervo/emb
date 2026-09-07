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

### Requirement: Lua-to-RESP reply conversion

The server SHALL convert each script's return value to RESP2 using a Redis-faithful grammar: Lua string → bulk (byte-safe; UTF-8/JSON/binary all valid), Lua number → integer reply, list-form table (sequential integer keys from 1) → array reply, string-keyed table → hash reply as flat field/value pairs (HGETALL shape), `nil`/`false` → null, and a table with an `err` string field → error reply. Values SHALL nest recursively (a hash value may be an array, hash, bulk, integer, or null).

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

Script execution SHALL be sandboxed and isolated: each evaluation runs in a fresh interpreter with only whitelisted host functions (`emb.run`, `emb.tokenize.pretokenized`, `emb.tokenize.words`, `emb.tokenize.encode`, `emb.tokenize.encode_pair`, `emb.math`, `json`) and a curated standard-library subset; `io`, `os`, file/network access, FFI, `math.random`, and all time functions SHALL be unavailable. Each evaluation SHALL be bounded by a wall-clock deadline, an execution-step budget, and a script-size cap; exceeding any bound replies with an error for that request only and never affects other in-flight requests. Scripted evaluation commands SHALL count toward the server's `max_concurrent_requests` gate.

#### Scenario: Infinite loop in one request

- **WHEN** a script runs past its execution budget
- **THEN** that request replies with an execution-time error while concurrent requests continue normally

#### Scenario: Oversized script

- **WHEN** a script exceeds the configured size cap
- **THEN** the server replies with an error and does not compile or cache it

#### Scenario: Forbidden library is absent

- **WHEN** a script attempts to use `os` or `io` functions
- **THEN** the script fails with an unknown-function error at runtime (or compile time), never touching the host

### Requirement: Content-addressed reply caching

When the server cache is enabled, scripted replies SHALL be cached under a content-addressed key derived from model name, script SHA1, the args, and the text — distinct args (e.g. different label sets) SHALL be distinct cache entries. A cache hit SHALL reply without re-running the script or model inference.

#### Scenario: Same script and labels hit the cache

- **WHEN** `EMB.EVSHA` is sent twice with the same model, SHA1, args, and text
- **THEN** the second reply is served from cache with identical bytes

#### Scenario: Different labels miss the cache

- **WHEN** the same script and text are sent with different label args
- **THEN** each distinct arg set is executed and cached separately

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