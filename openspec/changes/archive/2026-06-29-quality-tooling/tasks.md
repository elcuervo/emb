## 1. Benchmark Baseline

- [x] 1.1 Run `go test -bench=. ./...` and save output as `benchmark-baseline.txt`
- [x] 1.2 Run end-to-end timing: `EMB minilm test` via nc and save response time

## 2. Project Hygiene

- [x] 2.1 Create `.gitignore` excluding `models/`, `bin/`, `benchmark-baseline.txt`, IDE files
- [x] 2.2 Create `.golangci.yml` with `staticcheck`, `govet`, `gofmt`, `goimports` linters (v2 format)
- [x] 2.3 Add `just`, `golangci-lint` to `flake.nix` dev shell
- [x] 2.4 Create `justfile` with targets: `format`, `lint`, `test`, `bench`, `baseline`, `dev`, `download-model`, `clean`, `build`
- [x] 2.5 Remove `scripts/` directory (logic inlined into justfile recipes)

## 3. Model Auto-Download

- [x] 3.1 Add `ModelRepo` field to `config.ModelConfig` struct
- [x] 3.2 Implement download function: call `optimum-cli export onnx --model <repo> <path>` if ONNX file missing
- [x] 3.3 Wire download into `registry.LoadModel()`: check cache, download if needed

## 4. Style and Lint Fixes

- [x] 4.1 Run `golangci-lint fmt` and commit formatting changes
- [x] 4.2 Run `golangci-lint run --fix` and fix all reported issues (0 issues)
- [x] 4.3 Run `go vet ./...` and fix any warnings (0 warnings)

## 5. Generics Review

- [x] 5.1 Evaluate `pipeline/pooling.go` for generic potential: float32 only, ORT requirement — no benefit
- [x] 5.2 Evaluate `pipeline/batch.go` for generic potential: only int64 used — no benefit
- [x] 5.3 Applied generics: none found that reduce duplication without overhead
- [x] 5.4 Verify `go test ./...` still passes after generics changes

## 6. Performance Validation

- [x] 6.1 Re-run `go test -bench=. ./...` and compare against `benchmark-baseline.txt`
- [x] 6.2 Re-run end-to-end timing and confirm no regression vs pre-change baseline
- [x] 6.3 Document any performance differences: no regressions, all within <1% noise

## 7. Response Time Benchmarks

- [x] 7.1 Implement end-to-end response time benchmark (`cmd/emb-bench`) that connects to server, sends EMB commands, and measures latency
- [x] 7.2 Add `bench-e2e` target to justfile that starts server, runs benchmark, and stops server
- [x] 7.3 Run benchmark and save results to `benchmark-responsetime.txt`
- [x] 7.4 Add `bench-e2e` to the `just --list` documented targets

## 8. Final Verification

- [x] 8.1 Run `just lint` and confirm zero issues
- [x] 8.2 Run `just test` and confirm all tests pass
- [x] 8.3 Run `just bench` and confirm no regression vs baseline
- [x] 8.4 Start server with `just dev` and confirm EMB responds correctly
