# Tasks — Trim Over-Engineering

## 1. Baseline and guardrails

- [x] 1.1 Confirm the tree is green before touching it: `nix develop --command bash -c 'just lint && just test'` exits 0, so any later failure is attributable to this change.
- [x] 1.2 Capture the current accepted CLI forms as a reference list (`-config`, `-model <name>` + `-model-*`, implicit `"model"`, `-ort-lib`, `-version`, lenient bad integers) to port against; store it in the new parser test rather than a scratch file.

## 2. Config: flag parser and shared validation

- [x] 2.1 Add a `lenientInt` `flag.Value` that ignores parse errors (matching today's `strconv.Atoi`-with-`_`), with a unit test asserting `abc → 0` and `-5 → -5`.
- [x] 2.2 Rewrite `ParseFlags` on `flag.FlagSet` using `flag.Func` for `-model` and every `-model-*` so section attachment stays command-line ordered; keep the `"__version__"` sentinel for `-version`. Verify with a new table-driven `ParseFlags` test covering the forms from 1.2 and asserting the resulting `FlagConfig`.
- [x] 2.3 Extract `(*Config).validate()` and call it from both `Load` and `ParseFlags`; keep each path's path-specific checks (script resolution; `hasConfig`/`hasModel`). Verify with `nix develop --command bash -c 'go test ./internal/config/ -count=1'` and by confirming the existing negative-test error strings still match.
- [x] 2.4 Delete the old hand-rolled switch loop. Verify with `nix develop --command bash -c 'go vet ./... && go test ./internal/config/ ./cmd/emb/ -count=1'` and `./bin/emb -version` printing the version.

## 3. Server: table-driven runtime-config setters

- [x] 3.1 Collapse `setConfigMaxTexts`/`MaxPairs`/`MaxImages`/`MaxImageBytes`/`MaxImagePixels` into one table-driven setter, keeping `max_command_bytes`'s `SetMaxBulkSize`/`SetMaxCommandSize` propagation as its own wrapper. Verify with `go test ./internal/server/ -run 'Config|CacheCommands' -count=1` and by confirming CONFIG GET/SET output for each key is unchanged.

## 4. Generic bounded per-model map

- [x] 4.1 Add `internal/bounded.Map[V]` (per-model, cap 1024, evict lexicographically smallest key, `Get`/`Put`/`Delete`/`Clear`/`Len`) with a unit test covering cap enforcement, deterministic eviction, and per-model clearing.
- [x] 4.2 Rewire `scriptCache` onto `bounded.Map[string]`, preserving key computation and `Load`'s `(sha, exists)` contract. Verify with `go test ./internal/server/ -run Script -count=1`.
- [x] 4.3 Rewire `script.Compiler`'s proto cache onto `bounded.Map[*lua.FunctionProto]`, keeping `Compiles` and `Flush`. Verify with `go test ./internal/script/ -run Compiler -count=1` and `go test ./internal/server/ -run 'Eval|Script' -count=1`.

## 5. Helper dedupe and deletions

- [x] 5.1 Replace `int64ArrayToLua` (`host.go`), `shapeTable` (`packed.go`), and `floatTable` (`math_ops.go`) with one generic `numberTable[T]`; verify with `go test ./internal/script/ -count=1`.
- [x] 5.2 Replace `server.parseInt` with `strconv.Atoi` plus the `n < 1 || n > 1<<30` guard; verify `EMB.EVAL` arg parsing still rejects non-integers and `numtexts < 1` via `go test ./internal/server/ -run Script -count=1`.
- [x] 5.3 Delete the `rankOf` and `PairCosine` alias wrappers and call `len` / `Cosine` directly; verify with `go build ./... && go test ./internal/script/ ./internal/embverify/ -count=1`.
- [x] 5.4 Delete the stray `// maxConcurrentReqs int` comment and move the orphaned HELLO doc block onto `handleHELLO`; verify with `golangci-lint fmt --diff ./...` reporting nothing (comment-only change).

## 6. Cross-package cleanups

- [x] 6.1 Hoist the duplicated `printf` helper from `cmd/emb-verify` and `cmd/emb-multi-verify` into `internal/embverify` as an exported `Printf` and call it from both. Verify with `nix develop --command bash -c 'CGO_ENABLED=0 go test ./internal/embverify/... ./cmd/emb-verify/ ./cmd/emb-multi-verify/'`.
- [x] 6.2 Merge `registry.CPUUserUsec`/`CPUSysUsec` into one `ProcessCPUTimes() (userUsec, sysUsec uint64)` opening the process handle once, and call it once from `infoSnapshot`. Verify with `go test ./internal/server/ -run 'Info|Stats' -count=1` and that the INFO `cpu` section values are unchanged.
- [x] 6.3 Replace `cmd/emb-top`'s `contains` with `slices.Contains`; verify with `CGO_ENABLED=0 go build ./cmd/emb-top/ && CGO_ENABLED=0 go test ./cmd/emb-top/ -count=1`.

## 7. Integration verification

- [x] 7.1 Run the full gate: `nix develop --command bash -c 'just lint && just test && just verify-harness && just deadcode'` exits 0.
- [x] 7.2 Build and smoke the server per AGENTS.md: `just build`, then `./bin/emb -config test-two-models.yaml -cache auto -listen :16379` in `nix develop` and `redis-cli -p 16379 EMB minilm "hello world"` returns a vector; `INFO` and `CONFIG GET max_texts` still answer.
- [x] 7.3 Confirm the diff is confined to the intended files and is net-negative. Measured: tracked net −184 lines; new production `internal/bounded/map.go` (93) + `internal/embverify/print.go` (13); new tests 259 lines. The flag port is roughly size-neutral (it buys correctness/`--flag` support, not lines); the deletions come from the config/server/script dedupe. `git status --short` shows only `internal/config`, `internal/server`, `internal/script`, `internal/registry`, `internal/embverify`, `internal/bounded` (new), `cmd/emb-verify`, `cmd/emb-multi-verify`, `cmd/emb-top`.
