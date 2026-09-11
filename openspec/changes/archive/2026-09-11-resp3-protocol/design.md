# Design — resp3-protocol

## Context

See `proposal.md` — Why. Today emb speaks RESP2 via `github.com/tidwall/redcon v1.6.2` and returns embeddings as raw float32 bulk strings; every reply is counted by `countingConn`, which hand-computes RESP2 wire sizes because redcon's `Write*` methods return nothing. The fork `github.com/elcuervo/redcon` (master `262156d`) keeps the module path `github.com/tidwall/redcon` and adds per-connection protocol state plus RESP3 encoders (`WriteDouble`, `WriteMap`, `WriteSet`, `WritePush`, `WriteNull`→`_`, `WriteHello`).

## Goals / Non-Goals

- **Goal**: swap in the RESP3-capable fork with zero import churn; `HELLO` negotiation; RESP3 idioms per connection (maps, `_` nulls, typed doubles); `BLOB|VALUES` grammar on `EMB`/`EMB.MULTI`; RedisAI envelope for VALUES; protocol-aware netOut.
- **Non-Goals**: emulating Redis server semantics beyond what emb has (no CLIENT TRACKING, no pub/sub beyond redcon's, no per-protocol script re-encoding, no RESP3 parsing in the Ruby gem).

## Decisions

### D1: Dependency swap via `replace`, not a module path change

The fork's `go.mod` declares `module github.com/tidwall/redcon` (go 1.20; no tags on master). So:

```
go.mod:
    require github.com/tidwall/redcon v1.6.2            (unchanged)
    replace github.com/tidwall/redcon => github.com/elcuervo/redcon v0.0.0-20260908152233-262156df7a31
```

`go mod tidy` resolves the pseudo-version. All existing imports compile unchanged; the new API surface appears through the same package name.

**Alternatives considered**: forking to a new module path (`github.com/elcuervo/redcon/v3`) → churn in every import and go.sum; vendoring → maintenance burden. Rejected.

### D2: Protocol state lives on the fork's Writer; emb only plumes it

The fork stores `ver` on its `Writer`, defaults to 2, and `WriteNull`/`WriteAny` already branch on it. emb's `HELLO` handler parses `HELLO [2|3]`, validates, and calls `conn.SetProtocolVersion(n)` before replying via `redcon.WriteHello`. No emb-side encoder logic duplicates the fork.

**Risks**: the fork's `WriteBulkFrom` writes through a second `bufio.Writer` and can interleave out of order with `Write*` on the same reply. emb never uses `WriteBulkFrom` — mitigate by not using it and noting it in the fork if touched later.

### D3: `countingConn` becomes protocol-aware

Today every `Write*` override adds a hand-computed RESP2 size. Post-swap:

- Add `WriteDouble`, `WriteMap`, `WriteSet`, `WritePush`, `WriteAttribute`, `WriteBigNumber`, `WriteVerbatim`, `WriteBlobError` overrides sized per encoding (maps/sets/pushes/attributes = `1 + digits + 3` header; doubles require formatting first — size via `strconv.AppendFloat` into a `sync.Pool`ed scratch buffer, mirroring the fork's own `appendDouble(f, 'g', -1, 64)`).
- `WriteNull` branches on `c.Conn.ProtocolVersion()`: 5 bytes (`$-1`) vs 3 (`_`).
- `WriteAny` sizes with `redcon.AppendAny3` when `ProtocolVersion()==3`.
- Reads of `netOut` remain atomic; the wrapper centralizes everything, so handlers need no per-call counting.

**Alternative**: switch counting to measure the fork's `Buffer()`/internal buffer at flush time — rejected: couples netOut to write buffering and changes semantics.

### D4: Grammar — G2 leading fixed-position keyword

`EMB <model> [BLOB|VALUES] <text...>` (keyword at args[2]) and `EMB.MULTI [BLOB|VALUES] <model> <text>...` (keyword at args[1]). Detection rule: case-insensitive match AND at least one payload arg after the keyword position; otherwise the position is treated as a text (EMB) or model (MULTI) and format defaults to BLOB. The tail is never scanned, so trailing text that reads `VALUES` embeds normally. `BLOB`/`VALUES` are reserved model names (config-load validation), removing ambiguity for EMB.MULTI position 1 entirely and limiting EMB's caveat to a first-text-named-keyword with ≥2 texts (documented in EMB.HELP).

**Alternatives considered**: count-prefix (`EMB m N t..`, EVAL-style) → collision-free but an arity regression for the common single-text case; trailing keyword → unsafe with free text; protocol-only (ZSCORE model) → doesn't let a RESP3 client force blobs or a RESP2 client force values, which the RedisAI precedent (per-query format) exists for.

### D5: VALUES envelope follows RedisAI's META+VALUES reply

`EMB ... VALUES` replies `[dtype, shape, values]` (flat pairs under RESP2, map under RESP3) where `dtype = "FLOAT"`, `shape = [m, dim]`, and `values` is a flat row-major array. `EMB.MULTI VALUES` replies one envelope per pair, each with a `model` key (dims are ragged across models, so each envelope is self-describing), nulls for failed pairs. Truncation tail shows as `shape[0] = m < requested` (values are dense; absent trailing texts are implied, not nulled).

**Alternative**: bare value arrays (array of doubles for 1 text, array of arrays for N) — rejected: not self-describing for generic RESP3 clients (n vs dim ambiguity; RedisAI's flat VALUES relies on META shape).

### D6: Value typing = Redis/RedisAI fidelity (f64), not shortest-f32

Each float32 dim is widened to float64 and serialized via the fork's `WriteDouble` (shortest-round-trip f64, same semantics as Redis's `d2string`/grisu). This matches what RedisAI emits (`RAI_TensorGetValueAsDouble` → `ReplyWithDouble`) and what generic Redis clients expect of a `,` double. Deliberately **not** the float-bloat fix (`%.9g`/f32-shortest): Redis itself doesn't do it, and interop fidelity is the requirement (proposal embed `embedding-reply-format`). Clients that want compact re-serialization apply their own quantization. Under RESP2 the same text goes out as decimal bulk strings.

### D7: RESP3 idioms — maps where semantic, INFO unchanged

`EMB.INFO`, `EMB.STATS`, `EMB.MODELS`, `CONFIG GET` reply as RESP3 maps (flat pairs / arrays under RESP2). `INFO` stays a bulk string in both protocols — that is exactly what Redis does (INFO is a text format); converting it would break redis-cli and dashboards for zero benefit. Errors stay simple errors in both protocols (Redis sends `-ERR` in RESP3 too). Script replies remain pre-encoded RESP2 bytes replayed via `WriteRaw` — RESP3 parsers accept the subset (bulk/array/int; RESP2-style `$-1` nulls are tolerated by the fork's reader and redis-py), so scripts are left alone this change.

### D8: HELLO is a new command handler, not mux-default

Add `mux.HandleFunc("hello", ...)`. Auth gate parity: bare `HELLO` / `HELLO 3` on an unauthenticated, password-protected server must return `NOAUTH` (Redis behavior), so HELLO is NOT added to the exempt list — except the `HELLO` reply itself uses `redcon.WriteHello(conn, ...keys)` for the correct map/array shape after setting the version. (Trivial `CLIENT SETINFO name value` → `OK` can ride along so redis-py handshakes don't error; only if it stays small.)

## Risks / Trade-offs

- [Fork is a personal fork at master, no tags, pinned pseudo-version] → pin the exact commit; upgrade consciously; keep tests green before/after the swap task.
- [Fork behavioral deltas vs tidwall v1.6.2 (rewritten `parseInt` via `unsafe`+`Atoi`, bufio in Writer)] → isolation task runs full `go test ./...` immediately after the swap, before any new feature work, to prove no regression.
- [G2 keyword shadowing for EMB first-text] → reserved model names remove it for MULTI; EMB caveat documented in HELP and spec scenarios; arity guard (`len≥4`) protects the single-text "VALUES" case.
- [Typed-double counting needs a format pass per dim] → only when `VALUES` requested; scratch-buffer reuse (sync.Pool) avoids per-dim allocs; BLOB path stays allocation-free.
- [RESP3 maps change reply shapes for introspection commands] → only when the client negotiated protocol 3; flat-pair compat preserved otherwise; emb gem keeps RESP2.

## Migration Plan

1. Land: fork swap → full test suite green (pure refactor, no behavior change).
2. Land: `HELLO` + plumbing + RESP3 idiom switching (maps, nulls) with new tests; RESP2 default means existing clients unaffected.
3. Land: `BLOB|VALUES` grammar + VALUES envelopes + counting; BLOB default keeps the existing wire.
4. Pre-release version bump + release notes: reserved model names, HELP text, format-aware arity errors are the only breakages and are pre-release-acceptable per proposal.
5. Rollback: revert the change bundle; the fork swap and grammar are compile-coupled, so roll back as one unit.

## Open Questions

None that change the specs or task breakdown. (Client-side `CLIENT SETINFO` handling is a small opportunistic nicety, tracked in tasks as optional.)