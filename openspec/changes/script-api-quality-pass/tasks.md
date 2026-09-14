## 1. Correctness fixes

- [x] 1.1 Store the `json.null` sentinel for `null` elements in `anyToLuaValue`'s `[]any` branch (mirror the `map[string]any` branch) and confirm `json.encode(json.decode('[1,null,3]'))` returns `[1,null,3]` in `internal/script/host_test.go`
- [x] 1.2 Emit `dtype` from the default (non-packed) output path so every `emb.run`/`emb.run_batch` output is `{shape, data, dtype}`, and verify an all-integral float32 output round-trips back through `emb.run` without an inferred-`i64` mismatch in `internal/script/host_test.go`/`packed_test.go`
- [x] 1.3 Add a `json.null` round-trip regression test covering nested arrays and objects (`{"a":[1,null]}`), verifying byte-identical re-encoding
- [x] 1.4 Preserve empty-array identity through `anyToLuaValue`/`luaValueToAny`/`isListTable` (a decoded `[]` re-encodes as `[]`, not `{}`), with regression tests for direct, nested, and object-contained empty arrays

## 2. Operand decoder and math semantics

- [x] 2.1 Replace `mathOperand`/`vectorOperand` with one decoder returning `[]float64` and an explicit empty policy, and verify `emb.math.dot`/`cosine`/`l2`/`norm` still pass their existing tests in `math_ops_test.go`/`vector_test.go`
- [x] 2.2 Route scalar arguments through the shared scalar path for `softmax` and `argmax` (degenerate `1` and `(1, x)`), and verify `softmax(5)`/`argmax(5)` succeed in `internal/script/math_test.go`
- [x] 2.3 Enforce the "defined empty result" rule from design.md decision 4 across the math module, and verify each row of the table with unit tests (`sigmoid({})`→`{}`, `dot({},{})`→`0`, `cosine({},{})` errors, `topk({},k)`→`{}`, `float32_bytes({})` errors)
- [x] 2.4 Add `checkInteger` and apply it to `topk`'s `k`, `slice`'s `offset`/`length`, and `gather`'s index elements, and verify fractional arguments error instead of truncating in `math_ops_test.go`

## 3. Shared helpers

- [x] 3.1 Add `dtypeWidth(onnx.TensorType) int` and use it in the bytes decoder, `packTensor`, and `chargeOutputs`, verifying `just test` still passes
- [x] 3.2 Collapse `elementCountOf`/`product`/`innerInt` onto the checked `shapeElementCount`, verifying shape/overflow tests in `math_ops_test.go` and `packed_test.go` pass
- [x] 3.3 Add a single `renderTensor(ls, t, packed)` output-table builder used by `emb.run`, `emb.run_batch` slices, and the packed form (removing the inline shape-table duplication), verifying packed/array output tests pass

## 4. Cache identity and version

- [x] 4.1 Fold `emb.API_VERSION` into `script.CacheKey` and verify distinct keys across versions plus unchanged behavior for a fixed version in `internal/script/cache_test.go`
- [x] 4.2 Bump `APIVersion` to `1.2.0` and add the `1.2.0` history line in `internal/script/version.go`, verifying the `script-eval` version scenario still passes

## 5. Documentation and surface sync

- [x] 5.1 Refresh the `EMB.HELP` script-block line in `internal/server/server.go` to list the full 1.1.0/1.2.0 surface (`emb.embed`, `emb.image.embed`, `emb.similarity`/`distance`, `emb.API_VERSION`, all `emb.math` reductions, `json.null`), verifying `EMB.HELP` output includes each block
- [x] 5.2 Update the README script table and JSON grammar to document `json.null`, the `{shape, data, dtype}` array output form, and the empty/scalar semantics, verifying the README examples still run against a live server

## 6. Verification and regression gate

- [x] 6.1 Run `just lint` and `go vet ./...` and confirm zero issues
- [x] 6.2 Run `just deadcode` and confirm no new unreachable functions outside `deadcode-allow.txt`
- [x] 6.3 Run `just test` (or `go test ./...`) and confirm all packages pass, including the new script tests
- [x] 6.4 Run `just bench-script` and `just bench-budgets` and confirm every figure is within the recorded baseline gate, and run the maintained GLiNER2 reference script end-to-end (`EMB.EVSHA` with PERSON/ORG/PRODUCT) to confirm unchanged output
