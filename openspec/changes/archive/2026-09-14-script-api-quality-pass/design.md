## Context

See `proposal.md` for the defects. This document records how they are fixed and
the API decisions behind each fix.

The scripted surface is a sandboxed Lua host (`internal/script`) with three
constraints that shape every decision:

- **Determinism.** Replies are content-addressed and cached, so every host
  function must be a pure function of (model, script SHA, args, text). A
  behavior change under a stable identity silently corrupts cached replies.
- **Budgeting.** Tensors are charged against a per-evaluation element budget;
  the packed path exists so large graph outputs never round-trip element-wise.
- **Versioning.** `emb.API_VERSION` is the contract scripts use to detect the
  host surface, and it must change when semantics change.

Two decoders coexist today: `mathOperand` (`[]float64`, accepts empty) and
`vectorOperand` (`[]float32`, rejects empty). dtype width `4/8` is repeated in
four places, and element counts are computed by four helpers
(`shapeElementCount`, `elementCountOf`, `product`, `innerInt`) with different
overflow behavior. These are the seam where the correctness defects live.

## Goals / Non-Goals

**Goals:**

- Make every documented output a valid input (dtype round-trip) and every
  documented JSON value round-trip (array nulls).
- Give the math surface one predictable rule for scalars, empties, and argument
  types, and make the code enforce that rule in one place.
- Make behavior changes safe with respect to the reply cache.
- Bring the specs, `EMB.HELP`, and README back in line with the implementation.

**Non-Goals:**

- No new host blocks or metrics; no change to command grammar or RESP reply
  shapes; no change to the embedding/inference path.
- Not a rewrite of the sandbox or the tensor budget model. The dedup refactor is
  scoped to the operand/render/count helpers.

## Decisions

### 1. JSON `null` is stored as the sentinel in arrays too

`anyToLuaValue`'s `[]any` branch writes `LNil` (deleting the key); the object
branch already stores `json.null`. Mirror the object branch for array elements,
and let `isListTable`/`luaValueToAny` treat the sentinel as a present element
(they already do). **Alternative considered:** reject `null` in arrays at decode
time — rejected, it makes a valid JSON value an error and still fails to
round-trip.

### 2. Default-form outputs carry `dtype`

Emit `dtype` alongside `shape` and `data` from the single tensor renderer, so
the default form equals the packed form minus the `bytes` payload. This removes
the "infer dtype from element values" hazard that already forced `dtype` onto
`fill`. **Alternative considered:** keep inference and document the hazard —
rejected; it leaves a silent dtype mismatch for all-integral float tensors.

### 3. `softmax` and `argmax` accept scalars

The `script-eval` spec already requires it. Implement the degenerate forms
(`softmax(x)==1`, `argmax(x)==(1, x)`) in the shared scalar path used by
`sigmoid`. **Alternative considered:** narrow the requirement to `sigmoid` only
— a legitimate reading, but it changes the contract in the direction of less
capability and the scalar forms are cheap and well-defined.

### 4. One empty-operand rule: "defined empty result"

An operand with zero elements is valid iff the operation has a well-defined
empty result; otherwise it errors.

| Operation | Empty operand |
|---|---|
| `sigmoid`, `scale`, `add` (element-wise) | empty array |
| `dot`, `l2`, `norm` (linear reduction) | `0` |
| `topk`, `gather`, `slice` (selection) | empty array |
| `softmax`, `argmax`, `cosine` (need an element / non-zero denominator) | error |
| `float32_bytes` | error (existing `script-tensor-utils` requirement) |
| `mean_pool`, `cls` (need a positive shape) | error (existing) |

**Alternatives considered:** strict-all-error (simpler but breaks the useful
`gather(x, {})` "no candidates" case and contradicts `topk`); permissive-all-
empty (mathematically wrong for `softmax`/`cosine`). The table rule is the
smallest change that is defensible in each case.

### 5. One operand decoder returning `[]float64`

Replace `mathOperand`/`vectorOperand` with a single decoder returning
`[]float64` plus an explicit empty policy argument. This removes the f32
narrowing in `dot`/`cosine`/`l2`/`norm` (they accumulate in f64 already) and
gives one error vocabulary. Consequence: those four become f64-precise, a
last-ulp behavior change — captured by the `1.2.0` version bump and the
cache-key versioning below.

### 6. Validate numeric arguments

Add a small `checkInteger(ls, idx, name)` helper used by `topk`/`slice`/`gather`
for `k`, `offset`, `length`, and index elements, so fractional arguments error
instead of truncating. `shapeOf` keeps its own dimension validation.

### 7. Shared helpers

- `dtypeWidth(onnx.TensorType) int` (4/8) used by the bytes decoder, the
  renderer, `packTensor`, and `chargeOutputs`.
- `shapeElementCount(shape []int64) (int64, error)` (checked) becomes the only
  element-count primitive; `elementCountOf`, `product`, and `innerInt` become
  thin wrappers or are removed.
- `renderTensor(ls, t, packed) *lua.LTable` is the single output-table builder,
  used by `emb.run`, `emb.run_batch` slices, and the packed form.

### 8. Reply cache folds `emb.API_VERSION`

`script.CacheKey` prepends the host API version to its digest, so a semantics
change never replays a reply from the previous surface. This changes the key
format once, intentionally invalidating existing entries. **Alternative
considered:** document "flush the cache on upgrade" — rejected; it relies on the
operator remembering, and the cache is otherwise content-addressed precisely so
this is automatic.

### 9. Version bump to `1.2.0`

`emb.API_VERSION` moves to `1.2.0` (additive `dtype`, corrected empty/scalar/
argument semantics, plus the cache-key input). The `version.go` history gets a
`1.2.0` line.

## Risks / Trade-offs

- **[Stale replies across upgrade]** → Mitigated by decision 8; the version is
  now part of the cache identity.
- **[Reference-script output drift]** → The maintained `gliner2.lua` uses
  `emb.math.gather`/`sigmoid` with non-empty selections and packed reads, so it
  is unaffected; the benchmark suite and `just verify-emb` are the regression
  gate.
- **[f64 vs f32 rounding in `dot`/`cosine`/`l2`/`norm`]** → Results can move by
  a last ulp. Acceptable and more precise; no client decodes these into exact
  bytes (unlike `float32_bytes`, which is untouched).
- **[`dtype` field breaks a script that iterates output keys]** → Any script
  doing `for k,v in pairs(out)` already sees `shape`/`data`; a third key is
  additive. No reply shape changes.
- **[Empty-rule change alters existing scripts]** → Only scripts passing empty
  operands to `sigmoid`/`norm`/`dot`/`l2` are affected, and they move from error
  to a defined result.

## Migration Plan

Single server release. No config or data migration. On upgrade the first script
cache miss recomputes and stores the entry under the versioned key; old entries
become unreachable and age out with the normal LRU/eviction.

## Open Questions

None.
