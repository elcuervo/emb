## Context

`emb` separates RESP dispatch, model ownership, batching, native inference, and Ruby transport reasonably well. The gaps sit at ownership and concurrency boundaries. The implementation has native-backed sessions and tokenizers, lazy initialization, per-connection handler goroutines, and an asynchronous shutdown coordinator. Correct cleanup requires coordinating all of them.

The current redcon fork matters: `serve` closes accepted sockets when its accept loop returns. `Server.Shutdown` closes that listener before waiting for inference. Separately, `cmd/emb.run` waits only for `ListenAndServe`, so its deferred ONNX environment destruction can precede the signal goroutine's cleanup.

## Goals / Non-Goals

**Goals:** Make failure paths retryable, enforce configured request limits, preserve accepted replies during graceful shutdown, and validate runtime behavior before merge.

**Non-Goals:** New commands, new model families, distributed scheduling, a Ruby pool redesign, hard cancellation of native inference, hard padded-token budgets, broad performance refactoring, or dependency upgrades. The analysis records those follow-up opportunities separately.

## Decisions

### 1. One owner and a joined lifecycle for each inference pool

Use explicit accepting, closing, and closed states. Admission and transition to closing must be synchronized; a bare closed channel plus an unguarded send is insufficient. Introduce a stable internal closed error. Every accepted request receives one completion, and rejected requests do not enqueue.

Construct sessions before publishing a usable pool, or roll back every previously constructed worker on factory failure. Workers must have an exit path and a completion signal. The pool owns session closure; the registry retains tokenizer ownership. Join tokenizer producers and inference loops before releasing their sessions or the shared tokenizer. Repeated and concurrent closes share one completion and one stored error, including native release failures.

Drain accepted requests on ordinary close. If a caller abandons a request, result delivery must not block cleanup. Keep buffered result delivery. Server-level deadlines must not cause native resources to be destroyed underneath active work.

### 2. Admission and shutdown share one synchronization boundary

Centralize inference admission for EMB, EMB.MULTI, EMB.IMG, EMB.IMGMULTI, EMB.EVAL, and EMB.EVSHA. Under a short-held mutex, check draining and the configured cap, reserve a slot, and register active work. Release accounting exactly once on every handler exit. Shutdown changes the state under the same mutex before waiting, so a handler cannot register work after shutdown has observed zero active requests.

A CAS counter alone would fix cap overshoot but would leave shutdown registration as a separate problem. A shared gate covers both. Control commands remain outside inference capacity accounting. Audit model-initializing administrative work as well, notably script preload/load paths, so it cannot race registry destruction.

Keep `SetDraining` readiness semantics distinct from the final admission transition if current callers rely on that distinction.

### 3. Separate stopping accepts from closing clients, and join shutdown in main

Keep redcon's accept loop alive while accepted work drains. Reject new connections in its admission callback once shutdown begins, then close the transport after accepted work and replies complete. This preserves established sockets without changing the pinned dependency and is covered by a socket-level test.

The entrypoint owns the entire shutdown sequence and waits for its completion. Normal order: stop admission/acceptance, finish accepted work and replies, perform the bounded dirty snapshot, close connections, join pools, close model resources, destroy the ONNX environment, return.

When the deadline expires while native work is running, return a distinguishable timeout outcome to the entrypoint. The process terminates without calling native destruction on those resources; the OS reclaims them. Do not add a mutex that silently converts the 30-second bound into an unbounded wait. Prove the server boundary with a focused lifecycle test; keep process-level signal testing outside this focused pass.

### 4. Check batch limits before consuming another item

Guard the idle-drain loop with the current count and token threshold before dequeuing. The existing expression evaluates `tryAppendOne()` before checking `len(batch) < maxBatch`, admitting a second request when the first already filled a one-request batch.

Keep `max_batch` measured in requests. Keep the existing token threshold soft: an indivisible request may exceed it. This change does not reinterpret either limit as a hard bound on padded tensor memory. Test serial and asynchronous tokenization using gated fakes.

### 5. Publish completed downloads atomically

Create a unique temporary file in the destination directory. Copy the body, check copy and close errors, then rename to the final filename. Delete the temporary file on every failure. Use same-directory rename to avoid cross-filesystem publication. Never treat the presence of an in-progress temporary file as a cache hit.

Existing completed local files continue to work. Previously corrupted final files cannot be distinguished reliably without an integrity manifest; document removing and fetching affected artifacts again. Revision pinning, checksums, external ONNX data files, and download deadlines are separate follow-ups. Avoid presenting atomic publication as solving those concerns.

### 6. Keep validation focused

Run focused regression tests for pipeline, registry, server, and downloads. Add race coverage for simultaneous first model use and lifecycle tests where the existing local fixture is available.

Fix the observed RSS test assumption before making this race lane required. Process RSS is affected by reclamation of unrelated allocations. Test sampler contracts independently; use a subprocess, touched allocations, and liveness through the measurement for allocation experiments. Do not just loosen a byte threshold until the test happens to pass.

Do not add CI jobs, fixture provisioning, workflow helpers, or dependency changes in this reliability pass. The acceptance gates are the repository's existing test, vet, lint, build, and dead-code commands inside `nix develop`.

## Risks / Trade-offs

- **Transport drain can close replies too early** → Keep the accept loop alive and verify an established socket receives its response before transport closure.
- **Gate adds hot-path synchronization** → Hold it only for admission/accounting, never tokenization or inference; compare existing cache-hit and inference benchmarks.
- **Closing under load can strand work** → Test queue saturation, producer handoff, repeated close, and factory rollback with explicit gates, not sleeps.
- **Native inference cannot necessarily be interrupted** → Preserve safe ownership until completion and use process termination for the bounded exceptional path.
- **Fixture availability** → Report fixture-gated coverage honestly and keep deterministic fake-backed regression tests authoritative for this pass.
- **Old partial downloads remain indistinguishable** → Document recovery; integrity manifests are deferred.

## Migration Plan

1. Land deterministic resource-sampler and focused regression tests.
2. Land transactional pools, synchronized lazy publication, and lifecycle fixes.
3. Land admission/transport/entrypoint shutdown coordination.
4. Land batch-count and download-publication fixes.
5. Run existing test, vet, lint, build, and dead-code gates.

No stored-cache schema or protocol migration is required. Roll back individual behavior changes by revision if necessary; keep the regression tests and repository quality gates visible so a rollback does not silently restore an untracked defect.

## Open Questions

- What process exit code should distinguish forced timeout from graceful signal handling? Preserve the established zero exit for graceful completion and document the exceptional outcome.
