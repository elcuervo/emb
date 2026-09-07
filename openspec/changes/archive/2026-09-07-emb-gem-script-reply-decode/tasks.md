## 1. Decode layer (client core)

- [x] 1.1 `decode:` keyword plumbing: `Emb::Client#eval`/`#evalsha` and module-level `Emb.eval`/`Emb.evalsha` accept `decode: nil`; `parse_script_reply` forwards it; unknown modes (`decode: :bidirectional`) raise `ArgumentError` before any command is sent. Verify: rspec — unknown-mode raises with the mode in the message; existing no-decode specs stay green unchanged
- [x] 1.2 Top-level `decode: :f32`: a single-text String bulk is `unpack('e*')`'d into an Array<Float> (byte-identical to the embed path's decode); a numeric-array reply passes through as floats; multi-text replies decode each element to its own float array. Verify: rspec — a packed 768-float bulk equals manual `unpack('e*')`; legacy numeric-array reply unchanged; two texts → array of two float arrays
- [x] 1.3 Hash-field `decode: {field => :f32}`: applied after hash parsing (per element for multi-text); `{dim = 768, embedding = <packed bulk>}` yields a hash whose `embedding` is a float array and `dim` stays numeric; fields absent from a reply are left untouched. Verify: rspec — single-hash and multi-hash field decode cases
- [x] 1.4 Descriptive decode errors: `:f32` on a String whose length is not a multiple of 4 raises `ArgumentError` (not a bare `RangeError`); `:f32` on a hash/structured reply raises with the shape in the message; `{field => :f32}` on a non-Hash reply raises. Verify: rspec — the three error cases

## 2. Adoption and docs

- [x] 2.1 `gems/emb/bench/bench_models.rb`: the siglip2 scenario calls `evalsha(..., decode: :f32)` and drops its hand-rolled `.unpack('e*')`; the 768-dim check moves to the decoded array size. Verify: bench harness runs against a live `bench-3models.yaml` server (4×50) and the siglip2 row prints with dim 768
- [x] 2.2 Docs: `gems/emb/README.md` gains a "Script replies" section with the `decode:` recipes for the single-vector, multi-text, and hash-field scenarios; the main `README.md` "Writing your own script" Ruby snippet switches from manual `raw.unpack("e*")` to `decode: :f32`. Verify: snippets match the shipped API and render correctly

## 3. Validation

- [x] 3.1 Full suite: `cd gems/emb && bundle exec rake` (needs server on :16379), `bundle exec rubocop`, and `go test ./...` (server untouched — regression check only). Verify: rspec 0 failures, rubocop 0 offenses, go tests green