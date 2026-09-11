# Tasks — resp3-protocol

All commands assume `nix develop`. Verify each task with the stated check before moving on.

## 1. Fork swap (no behavior change)

- [x] 1.1 Add the `replace` directive pinning `github.com/tidwall/redcon => github.com/elcuervo/redcon` at the resolved master pseudo-version, run `go mod tidy`, and confirm the module resolves without import changes. Verify: `grep replace go.mod` and `go build ./...` compiles unchanged.
- [x] 1.2 Run the full Go test suite to prove the fork swap introduced no behavior change. Verify: `nix develop --command bash -c 'go test ./... -count=1'` all green.
- [x] 1.3 Run lint/vet clean on the unmodified tree. Verify: `nix develop --command bash -c 'go vet ./... && golangci-lint run ./...'` with no findings.

## 2. HELLO and protocol plumbing

- [x] 2.1 Add `HELLO [2|3]` handler registered on the mux: validate the version (error on anything else, connection unchanged), call `conn.SetProtocolVersion(n)`, reply with server metadata (name, emb version, proto) via `redcon.WriteHello`. Bare `HELLO` reports the current version without switching. Verify: new `go test` cases in `internal/server/` cover HELLO 2/3/bare/invalid; `redis-cli -3 PING`-style round trip works.
- [x] 2.2 Auth parity: `HELLO` is not auth-exempt; unauthenticated `HELLO` on a password-protected server replies `NOAUTH`. Verify: test asserting `-NOAUTH` then success after `AUTH`.
- [x] 2.3 (Optional, only if small) Handle `CLIENT SETINFO <key> <value>` with `+OK` so redis-py-style handshakes don't error after HELLO 3. Verify: `redis-cli -3 CLIENT SETINFO lib-name x` returns OK; remove if it grows.

## 3. RESP3 idioms per connection

- [x] 3.1 Make `countConn` protocol-aware: branch `WriteNull` on `c.Conn.ProtocolVersion()` (5 vs 3 bytes), add sized overrides for `WriteDouble`/`WriteMap`/`WriteSet`/`WritePush`/`WriteAttribute`/`WriteBigNumber`/`WriteVerbatim`/`WriteBlobError` (double sizes via `strconv.AppendFloat` with a pooled scratch buffer), and size `WriteAny` with `AppendAny3` under protocol 3. Verify: unit tests asserting exact byte deltas per encoding and protocol; `total_net_output_bytes` matches wire captures.
- [x] 3.2 Convert `EMB.INFO`, `EMB.STATS`, `EMB.MODELS`, `CONFIG GET` reply writers to emit RESP3 maps when `conn.ProtocolVersion()==3` and keep the existing flat pair/array shape under RESP2, preserving field names and order. Verify: protocol-switched tests parse the map under `HELLO 3` and the identical-looking flat reply under RESP2 (existing tests stay green).
- [x] 3.3 Confirm `INFO` still replies as a bulk string under RESP3 (no change), and scripts still replay their RESP2-encoded replies identically. Verify: existing INFO and script tests pass after negotiation; a script eval on a HELLO-3 connection parses correctly through the fork's reader.

## 4. BLOB|VALUES grammar

- [x] 4.1 Parse `EMB <model> [BLOB|VALUES] <text...>`: keyword recognized only at args[2] (case-insensitive) when at least one text follows; otherwise args[2] is a text and format defaults to BLOB; format-aware arity errors. Existing BLOB path stays byte-identical. Verify: grammar tests (default, explicit BLOB, VALUES, keyword-as-tail-text, single-text keyword guard, arity errors) in `internal/server/server_test.go`.
- [x] 4.2 Parse `EMB.MULTI [BLOB|VALUES] <model> <text>...`: keyword only at args[1] (case-insensitive) when at least one pair follows; otherwise args[1] is a model. Verify: grammar tests for both parses incl. odd-pair and too-few errors.
- [x] 4.3 Reserve `BLOB`/`VALUES` as model names in config loading (case-insensitive startup failure with a descriptive error). Verify: config test with a model named `values` fails load.
- [x] 4.4 Update `EMB.HELP` to document the new syntax and the keyword-shadowing caveat. Verify: HELP reply includes both formats.

## 5. VALUES envelopes and typing

- [x] 5.1 Implement the VALUES reply writer for `EMB`: envelope keys `dtype` (`FLOAT`), `shape [m, dim]`, `values` flat row-major; flat alternating pairs under RESP2, map under RESP3; each value widened f32→f64 and written with `WriteDouble` (RESP3) or decimal bulk (RESP2). Verify: single- and multi-text envelope tests on both protocols, incl. row-major ordering.
- [x] 5.2 Implement VALUES for `EMB.MULTI`: one envelope per pair including `model` key; null for failed/truncated pairs; per-pair dims from each model. Verify: mixed-model and failing-pair tests (envelope vs null).
- [x] 5.3 VALUES truncation semantics: overflow tail reflected via `shape[0] = m` (processed count), dense `values` of `m×dim`. Verify: test over the configured text cap compares shape and value count.
- [x] 5.4 Fidelity: assert the widened decimal text round-trips to the original float32 on downcast (post-decode), and that BLOB/VALUES agree when compared as floats. Verify: test comparing VALUES floats to the binary-path unpack for the same texts.

## 6. Ruby client (gems/emb)

- [x] 6.1 Add `format: :binary|:values` to the per-instance embed path, proxy `[]`, and batch loaders: `:binary` unchanged (no keyword), `:values` sends the keyword and parses the envelope (flat pairs → Hash, decimal values → Float, row-major grouping). Verify: rspec adding format cases; existing specs unchanged and green.
- [x] 6.2 Document in the gem README: default binary wire, VALUES opt-in, envelope shape, and that RESP3 client-side parsing is out of scope. Verify: README section present.

## 7. Integration validation

- [x] 7.1 Full validation inside `nix develop`: `just test`, `just lint`, `just build`, then server on `:16379` with `test-two-models.yaml` and `cd gems/emb && bundle exec rake`. Verify: `just all` green.
- [x] 7.2 Manual RESP3 smoke: `redis-cli -3 HELLO 3`, `EMB minilm VALUES "hello"` returns a map with typed doubles; `EMB minilm "hello"` returns the binary blob; `INFO` remains text. Verify: observed replies match the spec scenarios.
- [x] 7.3 OpenSpec: `openspec validate resp3-protocol` passes and `openspec status` shows all artifacts done. Verify: valid + apply-ready.