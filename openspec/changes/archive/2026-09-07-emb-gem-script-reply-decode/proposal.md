## Why

Scripted replies now support a scenario the gem can't serve: `emb.math.float32_bytes(vec)` returns a single opaque bulk string, and `eval`/`evalsha` hand it back raw — the embed path (`Emb::Proxy#[]`) auto-decodes fp32 bulks to float arrays, but scripted calls force clients to hand-roll `unpack('e*')` (the bench harness does exactly that today). Script replies are schema-less by design (any RESP value), so the client cannot guess shape — decoding must be explicit per call.

## What Changes

- `Emb.eval` / `Emb.evalsha` (client + module level) gain a `decode:` keyword:
  - `decode: nil` (default) → today's parsing, unchanged (backward compatible).
  - `decode: :f32` → the top-level reply (single text) or each element (multi text) is decoded as a little-endian float32 vector: a packed bulk is `unpack('e*')`'d into an `Array<Float>`; a numeric Lua array passes through as floats (legacy `{...}` style); anything else raises `ArgumentError`.
  - `decode: {field: :f32}` → after hash parsing, decodes the named field the same way (single hash or arrays of hashes per text). Absent fields are left alone; present-but-wrong-type raises.
- Unknown `decode:` values raise `ArgumentError` at call time; non-`4*n`-byte bulks raise a clear error instead of Ruby's cryptic `RangeError`.
- `gems/emb/bench/bench_models.rb` adopts `decode: :f32` and drops its manual `.unpack('e*')` — the fix demonstrated on the change's own scenario.
- Docs: gem README + main README's script section show the decode recipes for the siglip2 / sst2 scenarios.

## Capabilities

### New Capabilities
(none — the decode surface lives in the existing client capability)

### Modified Capabilities
- `emb-ruby-client`: adds script-reply decoding requirements to the command-wrappers surface (`eval`/`evalsha` `decode:`), covering vector bulks, multi-text arrays, and hash fields — the gem-side counterpart of the server's `emb.math.float32_bytes`.

## Impact

- **`gems/emb/lib/emb/commands.rb`** — `eval`/`evalsha` signatures + `parse_script_reply` decode layer (pure client-side; no server or reply-shape changes).
- **`gems/emb/lib/emb.rb`** — module-level `Emb.eval` / `Emb.evalsha` keyword passthrough.
- **`gems/emb/bench/bench_models.rb`** — siglip2 scenario uses `decode: :f32`, dim check stays.
- **`gems/emb/spec/emb_script_spec.rb`** — new decode cases (single/multi/hash/legacy/errors/backward-compat).
- **Docs**: `gems/emb/README.md`, `README.md` script section.
- **Validation**: rspec + rubocop green; no gem version bump decision here (see `gems-release-lifecycle`).