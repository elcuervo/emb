## Why

The codebase has accumulated dead code (unused fields, methods, parameters), redundant patterns (triple logging, duplicated mask initialization), and C-style loops that obscure intent. Running `go vet ./...` and `golangci-lint` passes, but the code doesn't use Go 1.22+ idioms that improve clarity and safety. Cleaning this up reduces cognitive load and establishes patterns for future contributions.

## What Changes

- Remove dead struct fields (`idToToken`, `padID`, `maxLength` from tokenizer; `mu` from server; `MaxInput`/`preTokenizer`/`normalizer` from tokenizer config)
- Remove dead methods (`Registry.Get`, `Pool.NumWorkers`)
- Remove dead code branches (model_repo os.Stat in config validation, empty-string check in WordPiece)
- Convert C-style `for i := 0; i < N; i++` loops to `for i := range N` throughout
- Replace manual `max`/`min` with built-in `max()`/`min()`
- Use `clear()` for map cleanup in registry
- Extract shared helpers: mask initialization, float32 byte decoding
- Remove redundant preload logging (triple logging bug: shows dim=0 for auto-detected models)
- Handle unchecked `conn.Read` errors in emb-verify and emb-bench
- Fix go.mod: mark `golang.org/x/sys` as direct dependency

## Capabilities

### New Capabilities

- `code-cleanup`: Code quality improvements — dead code removal, Go 1.22+ idioms, redundant pattern extraction

### Modified Capabilities

None — all changes are internal implementation cleanup with no spec-level behavior changes.

## Impact

Files across `internal/config/`, `internal/pipeline/`, `internal/registry/`, `internal/server/`, `internal/tokenizer/`, `cmd/emb/`, `cmd/emb-bench/`, `cmd/emb-verify/`, and `go.mod`. No changes to the RESP protocol, embedding pipeline behavior, or public interfaces. All existing tests must continue to pass.
