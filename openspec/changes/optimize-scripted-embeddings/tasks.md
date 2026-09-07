## 1. Constant-filled tensor specs

- [ ] 1.1 `fill` support in `namedTensorFromLua` (used by `emb.run` and `emb.run_batch`): `{shape, fill, dtype?}` constructs the tensor host-side with every element equal to `fill`; `fill`+`data` together error; non-integer `fill` infers float32 (explicit `dtype` wins). Verify: unit tests — zero-filled float32 from `{shape={1,3,224,224}, fill=0, dtype="f32"}` has no Lua data table and correct element count; ones-filled int64 mask; fill+data conflict errors; run_batch accepts fill items
- [ ] 1.2 `emb.math.float32_bytes(vals)` host fn: little-endian IEEE 754 pack to a Lua string; empty array errors. Verify: unit tests — 768 floats round-trip via Go float32 LE decode; a 2-element pack has exactly 8 bytes; empty errors

## 2. Example adoption + measurement

- [ ] 2.1 Rewrite `examples/scripts/siglip2.lua`: `pixel_values` via `fill = 0, dtype = "f32"`, L2-normalize, `return emb.math.float32_bytes(vec)`. Verify: wire smoke — `EMB.EVSHA siglip2 <sha> 1 <text> normalize` replies a single 3072-byte bulk (not an array of floats)
- [ ] 2.2 `gems/emb/bench/bench_models.rb`: siglip2 scenario decodes `.unpack('e*')` (single-string reply passes through `parse_script_reply`). Verify: harness runs; siglip2 row's dim matches 768 in the decode

## 3. Sweep and docs

- [ ] 3.1 `EMB.HELP` note for `fill`/`float32_bytes`; spec/design parity; `openspec validate` passes
- [ ] 3.2 Full suite (`just test`, gem rspec, rubocop, golangci-lint, `just build`) and record the siglip2 before/after (baseline: 181.9 ms p50 / 20.6 req/s). Verify: the after row shows the improvement in `bench_models.rb` output recorded in the change