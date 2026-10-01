# Validation evidence

All commands run inside `nix develop` unless noted.

## Coverage (cross-package, `-coverpkg=./...`)

| metric | before | after |
|---|---|---|
| production total (no `cmd/*`, `bench/*`) | 85.5% (6062/7090) | **87.0% (6168/7090)** |
| whole repository | 78.1% (6401/8201) | **79.3% (6507/8201)** |
| `internal/registry` | 70.4% | **79.6%** |
| `internal/onnx` | 74.7% | **79.9%** |
| `internal/server` | 90.3% | **91.0%** |
| `website/repl` | 78.0% | **79.8%** |
| `internal/script` | 86.1% | **86.2%** |

`just coverage-gate` exits 0 against `coverage-floors.txt` (floors ratcheted to
the new baseline).

## Skipped tests

The registry harness had two ORT-init helpers: `initORT` (once, never torn
down) and per-test `InitEnvironment`/`DestroyEnvironment`. Once `initORT` ran,
the per-test helpers saw "already initialized" and skipped every model-backed
test. Fixed by routing `loadModelFixture`, `scriptFixture`, and `requireORT`
through `initORT`.

| run | skipped |
|---|---|
| `go test ./internal/... ./website/repl/` before | 26 |
| after | 12 |

The remaining 12 skips are the legitimately absent fixture/budget-gated tests
(vision export, GLiNER export, `EMB_BENCH_*` budgets). CI's `test-cgo` job
reports the count and names so a green run is not read as fully executed.

## System tests

- `just all` — **pass** (Go suite + build + server + Ruby client: 200 examples,
  0 failures).
- `just verify-emb-multi` — **pass** (5/5 byte-identical EMB.MULTI vs
  sequential EMB, two models).
- `just validate-gems` — **pass** with `GEM_HOME=/tmp/emb-gemhome` and
  `GEM_PATH` including a locally built `date` gem. The host gem home's
  `date-3.5.1` is ABI-incompatible with the Nix Ruby 3.4 and breaks `gem build`
  (pre-existing, unrelated to this change); both gems build and validate under
  the isolated env.
- `just verify-embeddings` — not run: requires `sentence-transformers`/`torch`
  (absent; the recipe builds a venv and downloads them). This change touches no
  embedding code; `verify-emb-multi` is the embedding end-to-end check that ran.

## Performance

- `just bench` — **pass**, full Go benchmark suite completes (post-change
  numbers captured; no production code changed, so the benchmark surface is
  unchanged by construction).
- Fargate-shaped harness diff and `redis-benchmark` — not runnable here: Docker
  is unavailable and `redis-benchmark` is not on the host outside the dev
  shell. Run on the reference Linux host against
  `bench/fargate/baseline.*.json`; the noise gate is ±5% req/s / ±10% p50.
- CI `test-cgo` suite wall-clock: 21s locally (budget 1200s).

## Static checks

- `just lint` — **pass** (`golangci-lint` 0 issues, `go vet` clean).
- `just deadcode` — **pass** (no unreachable production functions outside
  `deadcode-allow.txt`).
- `gofmt -l .` — **clean**. A pre-existing missing trailing newline in
  `internal/tokenizer/truncation.go` (from the `fix-tokenizer-truncation`
  commit, unrelated to this change) was fixed to make the repo-wide gofmt gate
  pass.

## CI

- New `test-cgo` job (`.github/workflows/ci.yml`): ONNX runtime + libtokenizers,
  cached minilm fixture, `go test -json ./internal/... ./website/repl/`, skip
  reporting, and a 1200s wall-clock budget. YAML validated.
- `release.yml` CGo step widened from `pipeline`+`registry` to
  `./internal/... ./website/repl/` with `LD_LIBRARY_PATH`.
