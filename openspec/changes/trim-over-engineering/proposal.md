# Trim Over-Engineering

## Why

A repo-wide over-engineering audit found roughly 270 lines of avoidable
complexity: a 204-line hand-rolled argument parser, validation logic duplicated
verbatim between two config entry points, six near-identical runtime-config
setters, two bounded caches with identical eviction logic, and several
reinventions of the standard library. None of it changes observable behavior;
all of it is maintenance drag and drift risk.

## What Changes

- Replace the hand-rolled 204-line flag loop in `config.ParseFlags` with the
  standard `flag` package (a `flag.FlagSet` plus one `flag.Var` for the
  repeated `-model-*` block). This fixes several latent CLI bugs: the
  `-model-dim`/`-model-pooling`/… flags were unreachable (the switch matched
  the unprefixed name under a `-model-` prefix guard) and `--model-repo`
  (double dash, required by the `emb-server-distribution` spec) was ignored.
  **BREAKING (CLI edge cases):** `--flag` is now accepted, unknown flags and
  missing flag values are now errors, and the previously-dead `-model-*` flags
  now take effect. Malformed numeric flag values stay lenient (→ 0) as before.
- Extract the validation logic duplicated between `config.Load` and
  `config.ParseFlags` (TLS pair, onnx-or-repo, persistence, max_texts/pairs,
  image limits) into one `(*Config).validate()`.
- Collapse the six near-identical `setConfigMax*` methods into one table of
  `{name, field, label}` setters, keeping only `max_command_bytes`'s
  reader-guard special case.
- Replace `scriptCache`'s and `script.Compiler`'s duplicated per-model
  bounded-map eviction with one generic bounded map.
- Replace `server.parseInt` with `strconv.Atoi` plus a range check.
- Remove the stray `// maxConcurrentReqs int` comment and move the orphaned
  HELLO doc block off `handleCLIENT`.
- Replace `cmd/emb-top`'s `contains` with `slices.Contains`.
- Deduplicate `int64ArrayToLua` / `shapeTable` / `floatTable` (one generic
  Lua number-array helper).
- Delete the `rankOf` and `PairCosine` alias wrappers; call `len` / `Cosine`.
- Hoist the duplicated `printf` helper from the two verify commands into
  `internal/embverify`.
- Sample process CPU times once per INFO render (was two `Times()` calls).

Explicitly out of scope: the `emb-top` TUI and its dependencies (intentionally
kept), and the `script.ReplyWriter` interface (kept).

## Capabilities

### New Capabilities

None. This is a pure refactor: no spec-level behavior changes.

### Modified Capabilities

None.

## Impact

- Code: `internal/config`, `internal/server`, `internal/script`,
  `internal/registry`, `internal/embverify`, `cmd/emb-verify`,
  `cmd/emb-multi-verify`, `cmd/emb-top`.
- No dependency changes.
- Behavior change is confined to CLI edge cases (see What Changes): the
  documented flag set, config keys, RESP protocol, and default behavior are
  otherwise unchanged. `openspec/specs/` is unchanged (`skip_specs: true`).
