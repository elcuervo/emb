## Context

The repository has working Go code but no formatting standards, no task runner, no `.gitignore`, and models are downloaded via an external script. Benchmarking exists in tests but no baseline is tracked. The `config.yaml` only supports local paths for models — no auto-download. The existing `scripts/` directory with `run.sh` and `download-model.sh` will be replaced by `justfile` recipes.

## Goals / Non-Goals

**Goals:**
- `justfile` providing `format`, `lint`, `test`, `bench`, `baseline`, `dev`, `download-model`, `clean` targets
- `gofmt` + `goimports` + `golangci-lint` configured and passing
- `.gitignore` excluding `models/`, `/tmp/emb`, IDE files
- `config.yaml` model entry accepts optional `model_repo` field for HuggingFace auto-download
- Benchmark baseline captured as `benchmark-baseline.txt`
- All style fixes applied; any generics refactoring is performance-neutral or better
- Pre/post performance comparison documented in change artifacts

**Non-Goals:**
- Changing the RESP protocol or embedding output format
- Adding new model architectures or inference backends
- Replacing the worker pool architecture
- Adding CI/CD pipelines (just the local tooling)

## Decisions

### Task runner: `just` over Make
`just` is a modern `make` alternative with simpler syntax, automatic recipe listing, and no tab sensitivity issues. Available in nixpkgs. Added to `flake.nix` dev shell. The `scripts/` directory is removed — all recipes are inline in the `justfile`.

### Linter: `golangci-lint` with `staticcheck` preset
`golangci-lint` bundles `staticcheck`, `govet`, `gofmt`, and `goimports` in a single configurable run. Config at `.golangci.yml`. Conservative rules: enable `staticcheck`, `govet`, `gofmt`, `goimports` linters only — no opinionated style linters.

### Model auto-download: `model_repo` config field
```yaml
models:
  minilm:
    model_repo: sentence-transformers/all-MiniLM-L6-v2
    pooling: mean
    normalize: true
    max_length: 256
    dim: 384
```

If `model_repo` is set and the ONNX path doesn't exist, the server downloads and exports it on startup using optimum-cli (requires Python in the environment). If `model_repo` is absent, behavior is unchanged (local paths only).

### Benchmark baseline
Current `go test -bench=. ./...` output captured as `benchmark-baseline.txt`. Post-change benchmarks compared against it. Any regression >5% blocks the change.

### Response time benchmarks
The existing `go test -bench=.` benchmarks measure micro-benchmarks (pooling, normalization, mock RESP). Missing: end-to-end embedding response time against the running server with a real ONNX model.

A separate `bench-e2e` target in the justfile will:
1. Start the server with the minilm model
2. Send a series of `EMB` commands using redis-cli
3. Report p50/p95/p99 latency in milliseconds
4. Save results to `benchmark-responsetime.txt`

This is a black-box benchmark using redis-cli, not a Go test. It captures real-world performance including ONNX inference, CGo overhead, and RESP serialization.

### Generics approach
Generics used only where they eliminate code duplication without runtime cost:
- `PoolAndNormalize` already handles `float32` only (ORT requirement) — no generic benefit
- Tensor types are fixed `int64`/`float32` per ORT's API — no generic benefit
- The `Tokenizer` interface might benefit from a generic `Result[T]` type for error handling, but only if existing code becomes cleaner

Conservative: apply generics only where Go's compiler monomorphizes without overhead and the code is measurably cleaner.

## Risks / Trade-offs

- [Model download depends on Python + optimum] → Flake dev shell provides these. For non-nix setups, document the manual download script.
- [golangci-lint adds CI dependency] → Pinned version in flake.nix, config at `.golangci.yml` committed to repo.
- [Generics might reduce readability] → Used sparingly, only where the simplification is obvious. Peer review on generic changes.
- [Performance regression from style changes] → Every style change batch is followed by `go test -bench=.` comparison against the baseline.
