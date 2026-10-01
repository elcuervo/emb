# Testing and coverage

Everything runs inside `nix develop`, which provides Go, the ONNX runtime,
`libtokenizers`, and the CGo configuration the server tests need.

## What runs

```bash
just test            # go test ./... (all packages)
just verify-harness  # harness unit tests, no server/model/ONNX (CGO_ENABLED=0)
just lint            # golangci-lint + go vet
just deadcode        # fail on unreachable production functions
just cover           # cross-package statement coverage (report only)
just coverage-gate   # enforce coverage-floors.txt
```

`just all` runs the Go suite, builds the binary, starts a server, and runs the
Ruby client suite. The embedding verifiers (`just verify-embeddings`,
`just verify-emb-multi`) and `just validate-gems` are the end-to-end checks that
accept changes to the test suite.

Model- and fixture-dependent tests skip with a named message when their fixture
is absent (`models/minilm`, `models/sigl2-vision`, `models/gliner2`). Run
`just download-model` (and the other `download-*` targets) so they execute
rather than skip. CI downloads the minilm fixture for the CGo job; a skipped
test is reported, never silently treated as a pass.

## Coverage measurement

`just cover` measures **cross-package** coverage:

```bash
go test -coverpkg=./... -coverprofile=/tmp/emb-cover.out -covermode=atomic ./...
```

Cross-package instrumentation credits a package that is exercised only through
a sibling package's tests (for example, `internal/registry` is driven by
`internal/server` tests). The older package-local profile
(`go test ./...` without `-coverpkg`) reports materially lower numbers and is
not used for the gate.

The report prints every package plus two totals:

- **production** — every package except the `cmd/*` entrypoints and the
  `bench/*` harness, which are excluded by design (thin wiring and a
  benchmark driver, not product behavior).
- **whole repository** — everything instrumented, for reference.

## The gate

`coverage-floors.txt` records a floor per production package plus `TOTAL_PROD`.
`just coverage-gate` fails, naming the package, when coverage is below its
floor or when a production package has no recorded floor.

Floors are a ratchet, not a target: set each to the measured value rounded down
by one point so ordinary platform/toolchain jitter does not trip the gate.
Regenerate after a clean run on the reference toolchain:

```bash
just cover
```

then update each line from the printed percentages. Lowering a floor requires a
recorded justification in the change that does it.

## CI

The `test` job runs the CGo-free packages (config, hfhub, resp, emverify,
tokenizer, the verifier commands). A separate, path-filtered CGo job runs the
rest (`./internal/...` and `./website/repl/`) with the ONNX runtime and the
minilm fixture present, so the measured coverage is enforced rather than
local-only. The CGo job reports the skipped-test count so a green run is
distinguishable from one where the model-dependent tests did not run.
