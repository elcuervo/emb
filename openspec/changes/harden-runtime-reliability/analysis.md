# Iterative codebase review

Reviewed 2026-09-14 at `c9ac2f8`. The working tree was clean at the start. This change contains analysis, proposal artifacts, and isolated evidence probes; production code is unchanged.

## Recommendation

Prioritize runtime reliability before adding more features. The package boundaries are useful and the test suite is substantial, but concurrency, teardown, and partial-failure behavior have gaps that ordinary successful-request tests miss. Implement [the proposal](proposal.md) in the stages in [tasks.md](tasks.md).

## Review method and scope

1. Mapped the entrypoint, configuration, registry, pipeline, native session ownership, RESP server, script embedding hosts, image inference, cache/persistence tests, Ruby routing/batching, workflows, and active OpenSpec changes.
2. Ran the full Go suite and lint/vet baseline, then traced resource ownership, concurrent first use, queue bounds, downloads, and shutdown across package boundaries.
3. Used temporary Go overlays to test specific hypotheses without changing production files or installing failing tests into the normal suite. Ran focused race checks and refined the proposal from their results.

This is a targeted engineering review, not an exhaustive audit of every line. Ruby routing and batching, website delivery, native wrappers, and snapshot paths received source/workflow inspection; the Ruby suite, browser checks, throughput benchmarks, deployment, and all optional model families were not separately validated in this review. No external product or library recommendations are needed for the findings.

## Architecture worth preserving

| Boundary | Existing strength | Improvement direction |
|---|---|---|
| RESP server → registry → pipeline → ONNX | Clear separation of dispatch, model ownership, scheduling, and inference | Make lifecycle and admission contracts explicit across those boundaries |
| Native and scripted embedding paths | Shared embedding/cache helpers and lazy script/image resource work | Extend ownership tests down into pools; avoid duplicating the completed parity changes |
| Batcher tests | Gated tokenizers/sessions and queue observation | Reuse them for boundary and teardown tests |
| Cache persistence | Dedicated coordinator, atomic snapshot handling, lifecycle tests, and a snapshot fuzz target | Add real transport/process shutdown coverage around the coordinator |
| Ruby transport | Explicit pre-send failover and bounded dispatch concurrency | Preserve retry semantics; measure connection checkout contention separately |
| Operator tooling | Shared RESP client and CGo-free verifier harness | Keep fast checks available alongside required native integration |

## Prioritized findings

Priorities indicate implementation order: P1 affects correctness, availability, or resource safety; P2 is validation or efficiency work. “Reproduced” means a local probe/check failed on this revision. “Inspected” means the control flow establishes the gap, but its production impact was not independently measured.

| Priority | Finding and evidence | Impact and proposed response |
|---|---|---|
| P1 | **Cold model initialization races — reproduced.** `internal/registry/registry.go:695` reads `entry.Pool` before entering `sync.Once`; `:377` writes it during initialization. A 32-caller cold-load probe produced a race report at these exact accesses. | Synchronize publication and audit other lazy readers. Once protects callers that enter it, not the preceding pointer read. |
| P1 | **Graceful shutdown closes clients before drain and is not joined by main — inspected.** `internal/server/server.go:379` closes the listener before waiting. The pinned redcon fork's `serve` defer closes accepted sockets on accept-loop exit. `cmd/emb/main.go:128` starts cleanup in a goroutine while `:139` waits only for serving; `:44` defers environment destruction. | Accepted requests can lose their replies and process cleanup can overtake drain/snapshot work. Separate stopping accepts from closing clients, then join shutdown in the entrypoint. |
| P1 | **Pool failure/close ownership is incomplete — reproduced and inspected.** `internal/pipeline/pool.go:123` returns on a later factory failure without rolling back earlier workers; the probe observed zero closes. `Worker.Close` only closes its session and never terminates `run`. `internal/pipeline/batcher.go:289` guards its done channel but closes the session on every call; the probe observed two closes. It does not join producers or inference. | Native/session leaks on failed construction, retained worker goroutines after close, and possible native use during destruction. Add transactional construction and joined, idempotent closure. Existing leak tests measure steady-state requests before deferred cleanup and do not prove repeated lifecycle safety. |
| P1 | **Concurrency admission is check-then-increment — inspected.** `internal/server/server.go:315` checks the atomic count and then increments separately. Handlers also check shutdown separately from `active.Add`. | Simultaneous handlers can both observe one free slot; shutdown can observe zero before a previously checked handler registers itself. Use a common admission/drain gate. Existing `TestMaxConcurrentRequests` deliberately starts the second request after the first has occupied its slot, so it misses the simultaneous-contender case. |
| P1 | **Interrupted downloads become successful cache hits — reproduced.** `internal/hfhub/hfhub.go:108` reuses any existing final path; `:129` writes directly to it. A truncated HTTP body leaves `partial`; retry returned those bytes successfully without another HTTP request. | One interrupted model fetch can persistently poison startup. Publish only after a completed copy and successful close, using a temporary sibling and rename. |
| P1 | **One-request batch bound is violated — reproduced.** `internal/pipeline/batcher.go:204` invokes `tryAppendOne()` before checking whether the existing batch is full. | With `max_batch: 1`, a queued follower joined the same inference run; the probe observed batch size two. Check the bound before dequeue. Keep existing soft token-budget semantics. |
| P2, deferred | **PR tests omit parts of the core runtime — inspected.** `.github/workflows/ci.yml:137` and `:142` enumerate config/hfhub/RESP/verifier/tokenizer tests. Server, pipeline, registry, scripting, ONNX, imageproc, and emb-top tests are absent. Release validation has another partial list. PR Ruby coverage is Rails boot only. | Keep workflow expansion outside this focused pass. Validate these fixes with the repository's existing full local test, vet, lint, dead-code, build, and targeted race commands. |
| P2 | **RSS test assumes monotonic growth — reproduced.** `internal/registry/resources_test.go:12` samples aggregate RSS, allocates, runs GC, and requires the second sample not to decrease. The race run observed `1211465728 → 1077542912` bytes and failed. `KeepAlive` also precedes the second sample. | This can destabilize a new CI race gate. Separate sampler correctness from process-memory experiments and hold touched allocations live through the final measurement. Do not infer a runtime memory defect from this assertion alone. |

## Validation results

All commands ran inside `nix develop`.

| Check | Result |
|---|---|
| `go test ./... -count=1` | Passed; `cmd/emb` has no tests. Fixture-gated tests can still skip, so this does not establish all model-family coverage. |
| `just lint` | Passed: golangci-lint reported zero issues and `go vet ./...` passed. |
| `go test -race ./internal/pipeline ./internal/server ./internal/registry -count=1` | Pipeline and server passed; registry failed its RSS growth assertion. No data-race warning in this baseline run. |
| Overlay probes for factory rollback, repeated close, batch size one, and interrupted-download retry | All four failed their intended correctness assertions, confirming the reported defects. |
| Separate race-enabled overlay probe with 32 simultaneous cold model lookups | Failed with a real data-race report between pool publication and the unsynchronized pointer read. MiniLM was available locally for this probe. |

The five probe cases are retained in [evidence](evidence/README.md). They are intentionally outside the normal test suite. They should become passing regression tests with the corresponding fixes. Native memory safety is not established by a clean Go race run: the detector does not prove CGo resource lifetimes safe.

## Follow-up opportunities after the reliability change

These are narrower proposals or measurement tasks, not additional requirements hidden in this change.

| Opportunity | Evidence and suggested next experiment |
|---|---|
| Context-aware queueing and explicit deadline scope | `Pool.Embed` and `Batcher.Embed` accept no context. Script evaluation installs a VM context, but synchronous Go embedding hosts cannot be interrupted at VM instruction boundaries while blocked. The current script spec explicitly says VM granularity, so this is a limitation to clarify, not a newly proven spec violation. Measure queue delay under saturation before proposing native cancellation. |
| Download reproducibility and bounded network waits | `hfhub.New` uses `http.DefaultClient`; requests have no caller context/deadline and URLs resolve mutable `main`. Propose immutable revisions, manifest/checksum validation, and bounded connection/header/body waits as a separate acquisition change. Consider external-data ONNX exports there. |
| Deduplicate cold text cache misses | `Server.embedTexts` appends each missing input index, including repeats, before any miss is populated. Start with per-command deduplication preserving result order; benchmark repeated versus unique inputs before adding cross-request singleflight and its cancellation complexity. |
| Strict configuration diagnostics | `config.Load` uses permissive `yaml.Unmarshal`; several CLI integer parses discard `Atoi` errors. Test misspelled settings and malformed values, then propose consistent validation with compatibility notes for previously ignored keys. |
| Scheduling under uneven load | Go workers and Ruby connections use round-robin selection. A caller can wait on a selected busy worker/connection while another becomes free. Benchmark skewed request durations and checkout wait; preserve Ruby's intentional distribution across connection-pinned upstreams. |
| Latency semantics | Batcher `totalLat` accumulates one inference-batch duration while `requests` counts individual requests; dividing them reports amortized service time, excluding queue/tokenization. Clarify metric names/docs and consider separate queue, tokenization, inference, and end-to-end histograms before claiming user latency improvements. |
| Reduce server maintenance burden | `server.go` is 1,483 lines and several handlers duplicate admission, model lookup, and result bookkeeping. Extract the shared admission boundary as part of the fix; defer a broad file split until behavior is protected. File size alone is not a defect. |
| Align planning state and delivery evidence | Four active changes are marked complete (`script-runtime-parity`, `script-api-quality-pass`, `image-embeddings`, `harden-embedding-validation`). Review/archive them through the normal spec workflow. `deploy-site-to-cloudflare` still has six unchecked delivery-verification tasks; keep them visible rather than treating deployment as fully validated. Its staged benchmark job is conditioned on `workflow_dispatch`, but CI's trigger list has no dispatch entry. |

## Delivery and acceptance

Use small reviewable stages, not a broad rewrite. Resource lifecycle and cold-load publication are the first production fixes. Transport/entrypoint shutdown needs focused socket-level review because the current dependency closes sockets on listener exit. Batch bounds and atomic downloads can land independently. Keep workflow expansion deferred; use deterministic local concurrency tests and the existing repository quality gates for this pass.

Success means the preserved probes pass as permanent tests, admitted replies survive graceful signals, shutdown never destroys live native resources, failed downloads retry cleanly, and a broken server test blocks a pull request. Benchmark only the paths changed by synchronization or batching; no speculative throughput target is attached to this reliability work.
