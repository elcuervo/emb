# Design — Trim Over-Engineering

## Context

See `proposal.md` — Why. The audit findings are independent, behavior-preserving
cleanups across `internal/config`, `internal/server`, `internal/script`,
`internal/registry`, `internal/embverify`, and two `cmd/` tools. The only shared
constraint is that observable behavior must not change: the CLI surface, config
keys, RESP replies, cache identity, and INFO/STATS output all stay byte-identical.

## Goals / Non-Goals

**Goals:**

- Delete ~270 lines of duplication and standard-library reinvention.
- Keep one implementation of each repeated concern so copies cannot drift.
- Preserve every externally observable behavior; existing tests plus a small
  number of new regression tests are the gate.

**Non-Goals:**

- No behavior, protocol, config-key, CLI-flag, or dependency changes.
- No change to `emb-top`'s look, charting, or dependency set (intentionally kept).
- No removal of `script.ReplyWriter` (kept).
- No new OpenSpec spec deltas (`skip_specs: true`).

## Decisions

### 1. Replace the hand-rolled flag loop with `flag.FlagSet` + `flag.Func`

The current `ParseFlags` is a 204-line loop over `os.Args` with a giant `switch`.
An audit of its branches found three latent bugs that decide this port:

- The model-scoped value cases are written unprefixed (`case "-dim":`, `case
  "-pooling":`, …) inside a `strings.HasPrefix(arg, "-model-")` branch, so
  every one of them is **unreachable** — `-model-dim 128` is silently ignored.
- Only single-dash flags are recognized, so `emb --model-repo …` (the form the
  `emb-server-distribution` spec requires) silently starts with no models.
- Unknown flags, stray positional args, and a trailing `-flag` with no value
  are silently ignored.

Use a `flag.FlagSet` (ContinueOnError, output discarded). Register `-model` and
`-model-onnx`/`-model-repo`/`-model-tokenizer` plus the intended
`-model-pooling`, `-model-normalize`, `-model-output-tensor`,
`-model-pad-output`, `-model-dim`, `-model-max-length`, `-model-quantize`,
`-model-workers`, `-model-tokenize-workers`, `-model-intra-op-threads`,
`-model-inter-op-threads` with `flag.Func`/`flag.BoolFunc`, whose closures run
in command-line order and attach to the current section (implicit `"model"`
when none was opened). Route the numeric flags through one `lenientInt`
`flag.Value` that ignores parse errors, preserving `-max-connections abc → 0`.
Keep the `"__version__"` sentinel so `cmd/emb/main.go` is untouched.

This intentionally accepts `flag`'s strictness for unknown flags and missing
values, and makes the previously-dead `-model-*` flags work. Those three deltas
are the only behavior changes in the whole change; they are surfaced in
`proposal.md` and covered by the new parser tests.

*Alternatives considered:* `flag.Visit` (rejected — visits in lexical, not
command-line, order, which breaks section attachment); one custom `flag.Value`
per flag (rejected — more boilerplate than `flag.Func`); keeping the hand-rolled
loop (rejected — the `--model-repo` spec violation and the dead model flags are
already there); preserving the silent-ignore behavior (rejected — it would
require a hand-rolled pre-filter, undoing the point of using `flag`).

### 2. One `(*Config).validate()`

`Load` and `ParseFlags` currently repeat the same six checks verbatim
(TLS pair, onnx-or-repo, `validatePersistence`, max_texts, max_pairs,
`validateImageLimits`) with slightly different error wording. Extract one
`(*Config).validate() error` that both call. `Load` keeps its extra
script-path resolution and model-name validation; `ParseFlags` keeps its
`hasConfig`/`hasModel` requirement. Error strings that differ by only a
leading `-` on flag names stay as-is where tests assert them.

*Alternative considered:* leaving the duplication (rejected — the two paths
already drifted once; one place is the point of the cleanup).

### 3. Table-driven runtime-config setters

`setConfigMaxTexts`, `setConfigMaxPairs`, `setConfigMaxImages`,
`setConfigMaxImageBytes`, `setConfigMaxImagePixels`, and
`setConfigMaxCommandBytes` are the same six-line "parse non-negative integer,
assign, return" body. Replace with a table entry `{name, ptr, useInt}` and one
generic setter. `max_command_bytes` additionally propagates to
`SetMaxBulkSize`/`SetMaxCommandSize`, so it keeps a small dedicated wrapper
referenced from the table. Error messages keep the existing per-key wording
(tests assert them).

*Alternative considered:* a `map[string]func` registry (rejected — same size as
the struct slice, less readable).

### 4. Generic bounded per-model map

`scriptCache` (source by model→sha) and `script.Compiler` (proto by
model→wrapped source) each implement the identical bounded-map policy: per-model
map, cap of 1024, evict the lexicographically smallest key, `Flush(model)` /
`Flush("")` to clear. Extract `internal/bounded.Map[V]` with
`Get`/`Put`/`Delete`/`Clear`/`Len`. Both call sites keep their own key
computation and value type; only the eviction/clear mechanics move.
`script.Compiler.Compiles` stays on the compiler.

*Alternatives considered:* leaving the two copies (rejected — the eviction rule
is subtle and already mirrored in comments as "mirroring X"); hosting the
generic in `internal/script` (rejected — a cache utility is not a Lua concern
and `internal/server` would import it oddly). A three-file package is the
smallest shared home.

### 5. One generic Lua number-array helper

`int64ArrayToLua` (`internal/script/host.go`), `shapeTable`
(`internal/script/packed.go`), and `floatTable` (`internal/script/math_ops.go`)
are the same loop over a numeric slice. Replace with
`numberTable[T int64 | int | float64](ls, vals)` in one file. Callers keep
their names as thin call sites or are updated directly.

*Alternative considered:* leaving three (rejected — pure copy-paste).

### 6. Stdlib replacements and alias deletions

- `server.parseInt` → `strconv.Atoi` + `n < 1 || n > 1<<30` check.
- `cmd/emb-top` `contains` → `slices.Contains`.
- `rankOf(shape)` → `len(shape)` at its one call site.
- `PairCosine` → call `Cosine` directly; delete the alias.
- Hoist the duplicated `printf` from both verify commands into
  `internal/embverify` as an exported `Printf`.
- `registry.CPUUserUsec`/`CPUSysUsec` → one `ProcessCPUTimes() (userUsec,
  sysUsec uint64)` that opens the process handle once; `infoSnapshot` calls it
  once.

### 7. Import-hygiene deletions

Delete the stray `// maxConcurrentReqs int` comment and move the orphaned
HELLO doc block from above `handleCLIENT` to above `handleHELLO`. Comment-only;
no code change.

## Risks / Trade-offs

- [Flag rewrite silently drops a flag or changes section attachment] → Port
  every branch; add a table-driven parser test covering the accepted forms
  (`-config`, `-model`+`-model-*`, implicit `"model"`, spaced values, `-version`,
  `-ort-lib`, `--flag`, `lenientInt`) and assert the resulting `FlagConfig`
  matches the intended output (including the now-working `-model-*` flags).
- [Lenient-int behavior accidentally made strict] → route all numeric flags
  through `lenientInt`; add a test that `-max-connections abc` still yields 0.
- [Generic bounded map changes eviction or clearing] → keep the exact
  smallest-key rule and caps; rely on existing `script_test.go` /
  `compiler_test.go` determinism assertions plus a new unit test on the generic.
- [Error strings asserted by tests change] → keep existing wording per key; run
  `go test ./...` and reconcile against test expectations, not the reverse.
- [Refactor hides a real behavior change] → land as one commit with no behavior
  intent; the diff should be provably mechanical (`-m` cleanups, renames,
  extracted helpers).

## Migration Plan

Single-PR refactor. No deploy or data migration. Rollback is `git revert`.
Validation: `just lint`, `just test`, `just verify-harness`, and a smoke run of
`./bin/emb -config test-two-models.yaml` + `redis-cli EMB minilm "hello world"`.

## Open Questions

None. Every decision needed for the task breakdown is resolved above.
