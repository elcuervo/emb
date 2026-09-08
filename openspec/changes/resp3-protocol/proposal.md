## Why

emb speaks plain RESP2 and returns embeddings as opaque float32 binary blobs — fast and compact for emb-to-emb traffic, but unusable for generic Redis clients, inspection tooling, and any consumer that cannot decode raw floats. Redis 6+ clients negotiate RESP3 to get semantic replies (typed doubles, maps, `_` nulls), and the Redis tensor precedent (RedisAI `AI.TENSORGET <key> [META] [BLOB|VALUES]`) shows how to offer both a binary fast path and a self-describing typed path. emb should speak RESP3 for all commands and let each embedding query pick its reply representation.

## What Changes

- **Swap the RESP library to the RESP3-capable fork** (github.com/elcuervo/redcon) via a `replace` directive. The fork keeps the `github.com/tidwall/redcon` module path, so no imports change; it adds per-connection protocol versioning and RESP3 encoders (doubles, maps, sets, pushes, `WriteNull` as `_`, `WriteHello`).
- **Add the `HELLO` command** (`HELLO [2|3]`, bare `HELLO` reports the current version) so clients negotiate RESP3 per connection. RESP2 stays the default, so existing clients keep working untouched.
- **Adopt RESP3 reply idioms for all commands** when the connection negotiated protocol 3: flat key/value pair arrays become maps (`EMB.INFO`, `EMB.STATS`, `CONFIG GET`, `EMB.MODELS`) and nulls encode as `_`. `INFO` stays a bulk string in both protocols, exactly as real Redis keeps it (text format parsed by redis-cli and dashboards). RESP2-shaped replies (flat pairs, `$-1`) are unchanged. Errors stay simple errors. Script replies remain pre-encoded RESP2 bytes (a legal subset for RESP3 parsers) — re-encoding scripts per protocol is explicitly out of scope.
- **Clean up the EMB / EMB.MULTI grammar with a leading format keyword**, emulating RedisAI's `BLOB|VALUES`:
  - `EMB <model> [BLOB|VALUES] <text> [<text>...]`
  - `EMB.MULTI [BLOB|VALUES] <model> <text> [<model> <text>...]`
  - `BLOB` (the default) is byte-identical to today's reply; `VALUES` returns per-element values.
  - The keyword is detected only at a fixed position (`args[2]` for `EMB`, `args[1]` for `EMB.MULTI`) and only when at least one payload arg follows; the free-text tail is never scanned, so trailing words like `VALUES` embed normally. The word-shadows-keyword caveat (a first text named `BLOB`/`VALUES` with two or more texts) is documented, and `BLOB`/`VALUES` become reserved configured model names.
- **`VALUES` replies use the RedisAI META+VALUES envelope**, self-describing for generic clients: `dtype: FLOAT`, `shape: [m, dim]`, `values: [flat m×dim]`. `EMB.MULTI` replies one envelope per pair (including `model`) with nulls for failed pairs, preserving MGET semantics. Truncated tails show via `shape` (`m` < requested) instead of null slots.
- **Value typing follows Redis/RedisAI exactly**: each float32 dimension is widened to float64 and serialized with the standard Redis double encoding — RESP3 typed `,` doubles, RESP2 decimal bulk strings. Deliberately **not** shortest-float32 text: the widened form is what Redis itself emits (`d2string`/`ReplyWithDouble`), and interop fidelity to the ecosystem wins over the float-bloat optimization. A float-bloat-averse client may re-quantize on its side.
- **Make `netOut` accounting protocol-aware**: the `countingConn` wrapper sizes replies per negotiated protocol (`_` vs `$-1`, double/map encodings) instead of the hard-coded RESP2 math.
- **BREAKING (pre-release, accepted)**: `BLOB`/`VALUES` are reserved model names; `EMB.HELP` text changes; arity errors become format-aware. Existing emb↔emb binary traffic (no keyword, RESP2) is unchanged.

## Capabilities

### New Capabilities
- `resp3-protocol`: HELLO negotiation, per-connection protocol state, and RESP3 reply idioms (maps, `_` nulls, typed values) across all existing commands with RESP2 as the default.
- `embedding-reply-format`: the `BLOB|VALUES` grammar on `EMB`/`EMB.MULTI`, the RedisAI envelope reply, per-protocol value typing, and truncation-as-shape semantics.

### Modified Capabilities
- `emb-cmds`: EMB.INFO / EMB.STATS / EMB.MODELS reply representation changes under RESP3 (flat pair arrays → maps); `INFO` itself is unchanged (stays a bulk string in both protocols).
- `emb-multi`: EMB.MULTI accepts a leading `[BLOB|VALUES]` keyword and gains the VALUES reply form.
- `emb-ruby-client`: the emb gem gains `format:` support (default `:binary` unchanged, optional `:values` with envelope decoding) and documents that RESP3 client-side parsing is out of scope.

## Impact

- **Dependencies**: `go.mod` gains `replace github.com/tidwall/redcon => github.com/elcuervo/redcon <pseudo-version>` (fork at master `262156d`, no tags; same module path, so only the replace directive changes — zero import churn).
- **Server** (`internal/server/`): new `HELLO` handler + per-conn protocol plumbed through `redcon.Conn` (`SetProtocolVersion`/`ProtocolVersion`); map-idiom replies in `handleConfig`, `EMB.INFO/STATS/MODELS`; grammar + VALUES path in `handleEMB`/`handleEMBMULTI`/`writeEmbReply`; protocol-aware sizes in `counting.go`; reserved-name validation in `config.go`; HELP text.
- **Config**: model names `BLOB`/`VALUES` rejected at load.
- **Ruby client** (`gems/emb`): `format: :binary|:values` on embed paths, envelope parsing, specs; server must run for the suite (port 16379).
- **Tests**: fork swap compile check; HELLO negotiation; per-protocol reply encodings (maps, `_`, `,`); grammar edge cases (keyword-as-text, `len<4` guard, arity); VALUES envelopes (EMB + MULTI incl. mixed-model dims and failures); truncation-tail shape semantics; protocol-aware netOut byte counts; emb gem rspec + rubocop; `just test`/`lint`/`all` in `nix develop`.
- **Out of scope**: RESP3 client-side parsing in the emb gem (gem stays RESP2); per-protocol script reply re-encoding; `CLIENT SETINFO` compatibility is a small nicety handled opportunistically if trivial.