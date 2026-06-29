## 1. Dead Code Removal

- [x] 1.1 Remove `idToToken`, `padID`, `maxLength` fields and their assignments from `tokenizer/huggingface.go`
- [x] 1.2 Remove `MaxInput`, `preTokenizer`, `normalizer` types and fields from tokenizer config structs
- [x] 1.3 Remove `mu sync.Mutex` and `"sync"` import from `server/server.go`
- [x] 1.4 Remove `Registry.Get()` method from `registry/registry.go`
- [x] 1.5 Remove `Pool.NumWorkers()` method from `pipeline/pool.go`
- [x] 1.6 Simplify `Validate()` model_repo branch (dead os.Stat path) in `config/config.go`
- [x] 1.7 Remove dead WordPiece condition `string(fullWord[start:end]) == ""` in `tokenizer/huggingface.go`

## 2. Go 1.22+ Idioms

- [x] 2.1 Convert C-style loops to `range over int` in `pipeline/pooling.go` (5 loops)
- [x] 2.2 Convert C-style loops to `range over int` in `pipeline/batch.go` (1 loop)
- [x] 2.3 Convert C-style loops to `range over int` in `pipeline/pool.go` (1 loop)
- [x] 2.4 Convert C-style loops to `range over int` in `tokenizer/huggingface.go` (1 loop)
- [x] 2.5 Convert C-style loops to `range over int` in `server/server_test.go` (1 loop — Benchmarks)
- [x] 2.6 Replace manual `max` with built-in `max()` in `pipeline/batch.go`
- [x] 2.7 Use `clear()` for map cleanup in `registry/registry.go`

## 3. Helper Extraction

- [x] 3.1 Extract `makeMask(n int) []int64` helper for duplicate tokenizer mask initialization
- [x] 3.2 Extract `float32sFromBytes(data []byte) []float32` test helper (embedded in test helpers)

## 4. Bug Fixes

- [x] 4.1 Remove redundant preload logging from `cmd/emb/main.go` (dim=0 bug for auto-detected models)
- [x] 4.2 Fix unchecked `conn.Read` errors in `cmd/emb-bench/main.go` and `cmd/emb-verify/main.go`

## 5. Module Metadata

- [x] 5.1 Run `go mod tidy` to fix `golang.org/x/sys` direct dependency metadata

## 6. Verification

- [x] 6.1 Run `go build ./...` and `go vet ./...` — zero errors
- [x] 6.2 Run `just test` — all tests pass
- [x] 6.3 Run `just bench` — no regression vs baseline
- [x] 6.4 Run `just lint` — zero issues
