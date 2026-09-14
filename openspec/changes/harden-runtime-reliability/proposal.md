## Why

The review at `c9ac2f8` found reproducible session cleanup, batching-limit, and interrupted-download defects despite a passing Go suite. Pull-request CI omits the main runtime test packages, while request admission and shutdown contain synchronization gaps that existing sequential tests do not exercise; see [analysis.md](analysis.md) for evidence and priorities.

## What Changes

- Make pool construction transactional and pool/batcher closure idempotent; stop admission, settle queued work, join workers and tokenizer producers, then release native resources.
- Synchronize lazy model publication; a race-enabled concurrent cold-load probe reproduced conflicting pool reads and writes.
- Make request admission atomic with both the concurrency limit and shutdown transition. Preserve control-command availability during saturation.
- Enforce `max_batch` before taking another request, including `max_batch: 1`, while preserving idle flushing and the existing soft token-budget semantics.
- Download into temporary sibling files and publish only completed, successfully closed downloads. Failed transfers must remain retryable.
- Add focused regression and race coverage for the repaired runtime boundaries, and require the repository's vet and dead-code checks to pass.

No command grammar or successful embedding reply format changes. Shutdown timeout handling changes internally to avoid destroying resources still used by native inference.

## Capabilities

### New Capabilities

- `pipeline-resource-lifecycle`: transactional construction, idempotent closure, request completion, and safe native resource release.
- `runtime-quality-validation`: focused regression tests plus passing vet and dead-code checks.

### Modified Capabilities

- `server-lifecycle`: synchronized inference admission and drain; safe process termination when native inference outlives the shutdown deadline.
- `smart-batching`: strict request-count bounds on every dispatch, including idle draining.
- `huggingface-model-download`: atomic publication and reliable retry after interrupted transfers.

## Impact

- Runtime: `internal/pipeline`, `internal/server`, `internal/registry`, `cmd/emb`; review native session ownership in `internal/onnx`.
- Model acquisition: `internal/hfhub` and its failure-path tests.
- Validation: existing tests, `go vet ./...`, and `just deadcode`; no workflow or dependency changes.
- Transport: keep the existing redcon accept loop alive during drain, reject new connections in the admission callback, then close transport after accepted work completes.
- Extend the completed `script-runtime-parity` ownership work to the underlying embedding pipeline without duplicating its cache/session-parity changes.
- Deliver in small stages: lifecycle/admission, batching correction, download correction, then focused regression, race, vet, and dead-code validation.
