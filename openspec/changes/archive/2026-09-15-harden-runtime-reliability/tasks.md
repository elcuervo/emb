## 1. Establish reliable baseline validation

- [x] 1.1 Run the preserved review probes and record their expected failures; promote each into the appropriate package as its fix lands.
- [x] 1.2 Replace the process-RSS monotonicity assertion with deterministic sampler contract tests and an isolated allocation measurement where needed; retain allocations through sampling and touch pages for RSS measurements.
- [x] 1.3 Keep CI workflows unchanged; verify the focused implementation passes the repository's existing vet and dead-code checks.

## 2. Repair pool and registry ownership

- [x] 2.1 Add transactional worker construction and rollback; prove later factory failure releases earlier sessions and goroutines.
- [x] 2.2 Synchronize lazy pool publication in GetOrInit and audit registry statistics/readers for the same pattern; run the concurrent cold-load probe under -race.
- [x] 2.3 Add pool/worker admission state and termination signals; prove accepted requests finish and submissions after close return a closed error.
- [x] 2.4 Join batcher producers and inference loops before session closure; cover queued work, tokenizer handoff, active inference, and buffered result delivery with gated fakes.
- [x] 2.5 Make repeated/concurrent close idempotent with stored errors; ensure registry tokenizer closure follows all users and verify repeated open/close cycles leave no workers behind.

## 3. Coordinate admission and process shutdown

- [x] 3.1 Centralize admission for all six inference commands; atomically reserve capacity and active-work registration under the drain gate, with once-only release on every exit.
- [x] 3.2 Add simultaneous-contender and shutdown/admission tests; audit resource-initializing administrative commands and preserve control responses under saturation.
- [x] 3.3 Preserve accepted sockets during drain using the existing transport and verify it with a focused socket-level test; do not update the dependency pin.
- [x] 3.4 Join the shutdown coordinator in cmd/emb and enforce normal cleanup ordering through registry closure and ONNX environment destruction; ensure early startup failures follow the same structured cleanup path.
- [x] 3.5 Return an explicit shutdown timeout outcome and avoid destroying live native resources; cover completed replies and timeout behavior with focused lifecycle tests.

## 4. Enforce batch request bounds

- [x] 4.1 Check count/token thresholds before idle dequeue; make the preserved max_batch=1 regression pass.
- [x] 4.2 Add gated boundary cases for count one and larger counts under serial and asynchronous tokenization; preserve multi-text request accounting and soft token-budget behavior.
- [x] 4.3 Run existing batch-budget and idle-flush tests and exercise the relevant pool and cache-hit benchmarks as focused regression smoke checks.

## 5. Make interrupted downloads retryable

- [x] 5.1 Use unique temporary sibling files, checked copy/close, atomic rename, and cleanup on every failure in hfhub.Download.
- [x] 5.2 Cover interrupted HTTP bodies, retries, close/rename errors, concurrent attempts, temporary-file cleanup, and existing completed-file reuse.
- [x] 5.3 Document recovery for partial files written by older versions and clearly distinguish atomic publication from integrity/revision verification.

## 6. Focused quality validation

- [x] 6.1 Run targeted race checks for pipeline, registry, and server lifecycle behavior using available local fixtures.
- [x] 6.2 Run `just test`, `just lint`, `just deadcode`, and `just build` inside `nix develop`.
- [x] 6.3 Validate the OpenSpec artifacts and review command/reply compatibility and shutdown behavior against the focused scenarios.
