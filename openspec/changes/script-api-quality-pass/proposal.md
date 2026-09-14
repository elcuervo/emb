# Scripted API consistency and correctness pass

## Why

`emb.API_VERSION` is `1.1.0`: the scripted surface grew a second wave
(`emb.embed`, `emb.image.embed`, `emb.similarity`/`emb.distance`, packed and
selective `emb.run` outputs, and the `emb.math` reduction set) on top of the
1.0.0 baseline. A read-only quality pass over `internal/script` (lint, deadcode,
tests and a live server probe) found no dead code and green performance, but it
did surface correctness and consistency defects that a growing surface will keep
multiplying:

1. **`json` loses `null` inside arrays.** `anyToLuaValue`'s `[]any` branch writes
   `LNil`, which deletes the key, so a decoded array never round-trips. Verified
   against a running server:
   ```
   json.encode(json.decode('[1,null,3]'))    -> {}            # keys 1,3 -> not a list -> empty map
   json.encode(json.decode('{"a":[1,null]}'))-> {"a":[1]}     # trailing null gone
   json.encode({1,json.null,3})              -> [1,null,3]    # user-built works; decode does not
   ```
2. **Array outputs omit `dtype`.** The packed form carries `dtype`; the default
   `{shape, data}` form does not, so feeding an array output back into `emb.run`
   re-infers the dtype from "is any element fractional?". An all-integral
   **float32** output (zeroed auxiliaries, id-like tensors) infers `i64` and
   mismatches the session — the exact hazard the `fill` path documents `dtype`
   to avoid.
3. **`softmax`/`argmax` reject scalars, contrary to `script-eval`.** The spec
   says the math module's `sigmoid`, `softmax`, and `argmax` accept "either a
   single number or an array of numbers"; only `sigmoid` implements the scalar
   case (`softmax(5)` and `argmax(5)` error with `expected an array of numbers
   ... got number`).
4. **Empty-operand behavior is split with no stated rule.** `sigmoid({})`,
   `norm({})`, `dot({},{})` error; `topk({},k)`, `gather(x,{})`, `scale({})`,
   `add({},{})` return `{}`. Neither the specs nor the README define which is
   correct.
5. **Numeric arguments are validated inconsistently.** `shapeOf` rejects
   fractional dimensions, but `gather`/`slice`/`topk` silently truncate
   (`gather({10,20},{1.9})` selects index 1; `slice` offset `1.5` becomes `1`;
   `topk(v, 2.9)` uses `k=2`).
6. **The documented surface drifted.** `EMB.HELP` (which the README says "lists
   the full surface") omits every 1.1.0 addition — `emb.embed`,
   `emb.image.embed`, `emb.similarity`, `emb.distance`, `emb.API_VERSION`, and
   the math reductions — and `json.null` is absent from the README and all
   specs. `emb.math.scale`/`add` are implemented and documented in the README
   but appear in no spec.

The same pass showed the module carries two operand decoders (`mathOperand` as
`[]float64`, `vectorOperand` as `[]float32`, with opposite empty policies) plus
duplicated dtype-width, element-count, and shape-table helpers. That duplication
is what let the defects above go unnoticed; tightening it is part of the fix.

## What Changes

- **Fix `json` array nulls.** Decoding an array stores the `json.null` sentinel
  for `null` elements (mirroring the object branch), so
  `json.decode`/`json.encode` round-trips arrays containing `null`. Document
  `json.null`.
- **Emit `dtype` on array outputs.** `emb.run`/`emb.run_batch` default-form
  outputs become `{shape, data, dtype}`, matching the packed form and making
  every output a valid input spec regardless of element values.
- **Settle scalar math semantics.** `softmax` and `argmax` SHALL accept a single
  number exactly as `sigmoid` does and the `script-eval` spec already requires
  (degenerate results: `softmax(x)==1`, `argmax(x)==(1, x)`), OR the requirement
  text is narrowed to `sigmoid` only. Recommended: implement the spec.
- **One empty-operand contract.** An operand with zero elements is valid iff the
  operation has a defined empty result: element-wise maps (`sigmoid`, `scale`,
  `add`) and linear pairwise ops (`dot`, `l2`, `norm`) yield the empty
  array / `0`; selections (`gather`, `topk`, `slice`) yield the empty array;
  operations needing an element or a non-zero denominator (`softmax`, `argmax`,
  `cosine`) error; `float32_bytes({})` stays an error per `script-tensor-utils`.
- **Validate numeric arguments.** `gather`/`slice`/`topk` SHALL reject
  non-integer indices, offsets, lengths, and `k` instead of truncating, matching
  the existing shape validation.
- **Version the reply-cache key.** `script.CacheKey` SHALL fold
  `emb.API_VERSION` into its digest, so replies produced under an older host
  surface are never served after one of these semantics changes. (Today the key
  is `model:scriptSHA:argHash:text`; the host version is absent, so a
  host-function change under an unchanged script SHA can replay a stale reply.)
- **Deduplicate internals.** Collapse to one operand decoder with an explicit
  empty policy, one dtype-width helper, one checked element-count helper, and
  one tensor-rendering path (array + packed, both carrying `dtype`). `emb.run`'s
  array outputs and `emb.run_batch`'s per-item slices share the renderer.
- **Sync the documented surface.** Refresh `EMB.HELP` and the README script
  table to the 1.1.0 surface (including `json.null`), and add the missing
  `emb.math.scale`/`add` and empty-operand requirements to the specs.
- Non-`**BREAKING**` in the protocol sense (no command grammar or reply-shape
  change beyond the additive `dtype` field), but the empty-operand and
  scalar-math behavior changes and the array-output `dtype` addition are
  script-visible; `emb.API_VERSION` SHALL be bumped to `1.2.0`.

## Capabilities

### New Capabilities

<!-- None: every change refines behavior owned by an existing capability. -->

### Modified Capabilities

- `script-eval`: the math-helper requirement is clarified for scalar
  `sigmoid`/`softmax`/`argmax`, a single empty-operand rule is stated for those
  helpers, the script surface gains explicit `json.null` round-trip semantics,
  and the reply-cache key gains the host API version.
- `script-tensor-io`: default-form (array) outputs SHALL carry `dtype`; the
  empty-operand rule is stated for the vector/reduction operations
  (`dot`/`cosine`/`l2`/`norm`/`topk`/`gather`/`slice`); non-integer arguments
  SHALL be rejected.
- `script-tensor-utils`: `emb.math.scale` and `emb.math.add` gain a requirement
  (currently unspecified); the empty-array rule for element-wise ops is stated
  consistently with `sigmoid` and `float32_bytes`.

## Impact

- **Code:** `internal/script/host.go` (array-output rendering, `json.null`,
  preprocess budget ordering), `internal/script/math.go` +
  `math_ops.go` + `vector.go` + `embed.go` (single operand decoder, scalar
  math, empty policy, numeric validation), `internal/script/packed.go` +
  `batch.go` (shared dtype-width/element-count/shape-table helpers),
  `internal/script/cache.go` (version in the cache key), `internal/script/version.go`
  (`1.2.0`), `internal/server/server.go`
  (`EMB.HELP`).
- **Docs/specs:** `README.md` (script table + JSON grammar), `EMB.HELP`,
  `openspec/specs/script-eval|script-tensor-io|script-tensor-utils`.
- **Behavior:** script-visible changes are the additive `dtype` on array outputs
  and the corrected empty/scalar/argument semantics; no RESP command grammar or
  reply-shape change. The cache-key format changes once, so pre-upgrade cached
  scripted replies are not reused (a deliberate invalidation, not a bug).
  Existing reference/snippet scripts (packed reads, `emb.embed`,
  `emb.math.gather`/`sigmoid`) keep their current outputs.
- **Tests/bench:** new unit coverage for JSON null round-trip, array-output
  dtype, scalar/empty math, and fractional-argument rejection; the existing
  scripted benchmark and budget suite is the regression gate and is expected
  unchanged.
