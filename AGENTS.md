# AGENTS.md

Guidance for coding agents working in this repository.

## What this is

`emb` — a Redis-protocol (RESP2) text-embeddings server in Go (ONNX Runtime via CGo), with Ruby client gems (`gems/emb`, `gems/emb-server`). Models: `internal/{onnx,pipeline,registry,server,config,tokenizer,hfhub}`, entrypoint `cmd/emb`, binary `bin/emb`, version in `VERSION`. OpenSpec change proposals live in `openspec/changes/` (see `.pi/skills/` for the workflow).

## The single most important thing: run everything inside `nix develop`

The host shell has **no `go`** (`go: command not found`). The Nix dev shell provides the toolchain AND the CGo/runtime environment:

- Tools: `go`, `gopls`, `golangci-lint`, `just`, `python3`, `redis`, `ruby_3_4`, `bundler`, `act`, `xan`
- `flake.nix` `shellHook` exports `CGO_CFLAGS`/`CGO_LDFLAGS`, `C_INCLUDE_PATH`, `LIBRARY_PATH`, and — critically for running the server — `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` pointing at the nix **onnxruntime** lib.

**First entry may take a while** (builds/fetches onnxruntime + libtokenizers once).

```bash
nix develop            # interactive shell
nix develop --command bash -c 'go test ./internal/server/'   # one-shot
```

Running the server binary **outside** the dev shell fails with
`Error loading ONNX shared library .../bin/libonnxruntime.1.dylib` — always run it
inside `nix develop` (or pass `-ort-lib <nix-store-path>/lib`).

## Standard checks (`just`)

All targets assume you are inside `nix develop` (the justfile's `ort_lib` variable
greps `DYLD_LIBRARY_PATH` for the nix onnxruntime path; empty → runtime server
steps in `just all` may fail to find the ORT library).

| Command | What it runs |
|---|---|
| `just test` | `go test ./...` |
| `just lint` | `golangci-lint run ./...` **then** `go vet ./...` (this is the "go vet" check) |
| `just format` | `golangci-lint fmt ./...` (gofmt + goimports) |
| `just build` | CGo build with `-ldflags "-X main.version=$(cat VERSION)"` → `bin/emb` |
| `just all` | `just test` + `just build`, then starts a server on **:16379** (`test-two-models.yaml`) and runs the Ruby client suite (`cd gems/emb && bundle exec rake`) |
| `just validate-gems` | build + install + validate both gems locally |
| `just bench` / `baseline` | Go benchmarks / captured baseline |
| `just bench-cache-size size="auto"` | server with `-cache <size>` + `redis-benchmark` hit path |
| `just bench-ruby config="bench-cpu-partition.yaml"` | CPU-partitioned client harness (`app_cpus`/`bench_cpus` vars, taskset on Linux) |
| `just version` / `tag` / `release` | read/bump VERSION, tag, release artifacts |
| `just clean` | remove `bin/` |

Equivalent direct commands (inside the dev shell):

```bash
go build ./... && go vet ./...            # compile + vet
go test ./internal/server/ -count=1       # uncached server tests
golangci-lint run ./...
```

## Ruby client checks (`gems/emb`)

- Unit/integration specs: `cd gems/emb && bundle exec rake` — **requires a server running on port 16379** (`./bin/emb -config test-two-models.yaml -listen :16379`, inside `nix develop`).
- Lint: `cd gems/emb && bundle exec rubocop`.
- Bench harness: `bundle exec rake bench` (server must be running).

## Known pre-existing issue — do not chase it

`TestAsyncTokenizerOverlapsWork` (`internal/pipeline/batcher_budget_test.go`) is a
timing-sensitive test that **fails on this machine even on clean `main`**
(measured: wall ≈ 81ms vs "fully serialized ~105ms" expectation). It is unrelated
to cache/config/INFO work. `go test ./...` / `just all` will show this single
failure; verify your change with the targeted package (`go test ./internal/server/`)
and note the flake instead of "fixing" it. Its assertion is a relative-overlap
timing check that needs a faster/larger machine or a tolerance bump.

## Working with OpenSpec changes

- Active changes: `openspec list`; per-change status/artifacts: `openspec status --change <name> --json`; validate: `openspec validate <name>`.
- Skills in `.pi/skills/`: `openspec-propose` (create changes), `openspec-apply-change` (implement tasks, mark `- [ ]` → `- [x]` in `tasks.md`), `openspec-archive-change` (sync delta specs → `openspec/specs/`, move to `openspec/changes/archive/`), `openspec-explore`, `openspec-sync-specs`.
- Implemented-but-uncommitted work is the norm mid-change; commit as a bundle with the change's artifacts.

## Quick reference

```bash
# one-shot inside the dev shell
nix develop --command bash -c 'go vet ./... && go test ./internal/server/ -count=1'

# build + smoke server (inside dev shell)
just build
./bin/emb -config test-two-models.yaml -cache auto -listen :16379   # then redis-cli -p 16379 EMB minilm "hello world"
```