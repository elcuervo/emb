## 1. Shared wire client

- [x] 1.1 Move `Reply` and the RESP2 wire client (`Dial`, `WriteArgv`, `Flush`, `ReadReply`, `Close`, timeouts, AUTH/TLS) out of `internal/embtop/resp.go` into a new `internal/resp` package with a package doc; verify `go build ./...` and `go test ./internal/resp/` pass.
- [x] 1.2 Make `internal/embtop` consume `internal/resp` (type alias / thin wrapper) without changing its exported API; verify `go test ./internal/embtop/ -count=1` passes unchanged.
- [x] 1.3 Add `internal/resp` unit tests for bulk, status, integer, array-with-nulls, error, and nil replies, plus an I/O timeout and a connection-refused case; verify the new tests fail if framing or the timeout is removed.

## 2. Embedding verification core (`internal/embverify`)

- [x] 2.1 Implement float32 little-endian decode plus pairwise cosine, ranking, and nDCG@k retention; verify unit tests cover identity, orthogonal, zero-vector, and known-ranking cases.
- [x] 2.2 Define the reference artifact struct (model, dim, sentences, embeddings, generator + dependency versions, checksum) with `Load`/`Validate`; verify tests accept a valid artifact and reject a checksum mismatch, a model mismatch, and a sentence-set mismatch with an error naming the field.
- [x] 2.3 Implement the embed helper over `internal/resp` (`Embed(model, text)` returning a float32 vector, surfacing server errors and nulls); verify tests against an in-process `net.Pipe` responder for bulk, array, error, and timeout replies.
- [x] 2.4 Verify `CGO_ENABLED=0 go test ./internal/resp/... ./internal/embverify/...` passes with no running server, no ONNX runtime, and no downloaded model.

## 3. Verifier commands

- [x] 3.1 Port `cmd/emb-verify` onto the shared client with `-addr`, `-model`, `-dim`, `-reference`, `-min-cosine`, `-timeout` flags and a per-sentence report; verify it builds and a unit test drives a fake server through pass and fail paths.
- [x] 3.2 Port `cmd/emb-verify-performance` onto the shared client with `-addr`, `-model-a`, `-model-b`, `-min-cosine` (0.99), `-min-ndcg` (0.95); verify the ranking math is unit-tested and the command builds.
- [x] 3.3 Port `cmd/emb-multi-verify` onto the shared client with configurable models/dims, and report the model and text of a differing element; verify a unit test exercises the array reply path.
- [x] 3.4 Verify the three commands report a non-zero exit with a named cause for: connection refused, unknown model, and unreadable reference.

## 4. Reference artifact

- [x] 4.1 Rewrite `cmd/emb-verify/generate-reference.py` to pin the model, record the generator and installed `sentence-transformers`/`torch` versions, and write the checksum over a canonical serialization; verify a freshly generated artifact passes `Validate`.
- [x] 4.2 Verify a tampered or foreign artifact is rejected by `-reference` before any comparison, and that `--refresh` regenerates it.

## 5. `just` recipes

- [x] 5.1 Fix `just verify-emb-multi`: reference the path `download-model` actually writes, add the `build` dependency, derive model/dim from the generated config, and poll readiness with a deadline; verify the recipe passes end to end with the downloaded models.
- [x] 5.2 Make `just verify-embeddings` check for the model and reference before starting, and fail with a clear message when either is absent; verify the failure path by running with the model directory moved aside.
- [x] 5.3 Add `just verify-harness` running the harness unit tests with `CGO_ENABLED=0`; verify it passes with no server running.

## 6. Harness hygiene gates

- [x] 6.1 Add `golang.org/x/tools` via a `tool` directive and a `just deadcode` target running the analysis without `-test`; verify it reports `onnx.NewRuntimeSession` as unreachable.
- [x] 6.2 Resolve `onnx.NewRuntimeSession`: either delete it, or keep it with a documented reason and an explicit exclusion in the dead-code target; verify `just deadcode` is clean afterwards.
- [x] 6.3 Add a `just cover` target that writes a coverage profile and prints per-package statement coverage; verify it reports `hfhub` and `onnx` coverage.
- [x] 6.4 Add the `CGO_ENABLED=0` `internal/resp` and `internal/embverify` tests to `.github/workflows/ci.yml`; verify the job runs without ONNX.
- [x] 6.5 Add at least one test for `internal/hfhub`'s download path using an `httptest` server; verify `go test ./internal/hfhub/` no longer reports 0.0% statement coverage.

## 7. Documentation and final validation

- [x] 7.1 Document the verification commands, the per-check tolerances, the reference artifact's provenance, and the `deadcode`/`cover` gates in `README.md` (and `AGENTS.md` for the dev-shell gates); verify every command as written runs in `nix develop`.
- [x] 7.2 Run `just test`, `just lint`, `just build`, and `just verify-harness` inside `nix develop` and confirm all pass.
- [x] 7.3 Run `just verify-emb-multi` against the downloaded two-model config and confirm it passes.
