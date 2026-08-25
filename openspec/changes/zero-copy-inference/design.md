## Context

`RuntimeSession.Run` (runtime.go) allocates three fresh tensors per run and then does `result := make([]float32, flatSize); copy(result, data)` on the output. For a 32×512×384 batch that's ~25MB of copying per run discarded immediately after. Combined with the input allocations, the hot loop is GC-churn-heavy — a real p99 problem on memory-capped Fargate tasks. `onnxruntime_go`'s `GetData()` returns a Go slice over the tensor's backing memory; a spare copy exists only because the current code allocates the output tensor inside the run and copies data out before destroying it.

## Goals / Non-Goals

**Goals:**
- Pool input-tensor backing arrays per model (sized by batch×seqLen)
- Return the ORT output backing slice directly with ownership semantics (no extra copy)
- Keep the buffer alive through the RESP write, then release to the pool
- Prove allocs/op and B/op reductions with benchmem; show p99 relief under load

**Non-Goals:**
- Changing the `onnx.Session` interface or the CGo library
- Async writes (buffer must outlive the synchronous reply write)
- Cross-model buffer sharing

## Decisions

### `session.Run` returns a `BorrowedOut` owning a pooled buffer
The session hands back `([]float32, release func())` where `release` returns the backing buffer to the pool. The pipeline runs pooling/normalization in place into a pooled output slot, and the server writes the RESP bulk reply from that slot before calling `release`. Ownership flow is explicit at the call site (no hidden global state).

### Pool sizing per model
`sync.Pool` seeded with buffers sized to `max_batch × max_len × dim` (the worst-case single run for the token-budget config). Requests smaller than that reuse the big buffer; oversize single-command runs allocate a one-off that is discarded (not pooled) to avoid pinning huge buffers.

### Input tensors share one pool entry per request
input_ids and attention_mask get contiguous slabs (2×seqLen int64 each) from one pooled slice, halving pool churn and improving locality versus separate allocs.

### Release must be fast-path-safe
`release` is a no-op `sync.Pool.Put` guarded by the server after `WriteBulk` returns. Because redcon's `WriteBulk` copies into its own write buffer synchronously, releasing immediately after is safe.

## Risks / Trade-offs

- [Use-after-release if a client holds a pooled buffer] → buffers are internal-only (the RESP write copies bytes); documented invariant + bench/test that asserts release-before-return.
- [Pool never shrinks under idle] → bounded by worst-case batch size per model; acceptable for a per-model server, satisfies Fargate memory caps (size = one batch).
- [onnxruntime_go output tensor memory layout uncertainty] → design assumption is `GetData()` aliases ORT-owned memory valid until `Destroy`; if that proves otherwise, fall back to a pre-sized Go buffer passed into the session (same pooling benefit, one explicit copy retained). Recorded in Open Questions.
- [Pooled big buffers inflate RSS baseline] → one max-batch buffer per model is small vs model weights (MiniLM ~90MB fp32); explicit in design, measured by the harness memory cell.

## Migration Plan

1. Add `BorrowedOut` (data + release) to the session run path; keep the existing copy path behind a flag briefly for A/B in tests.
2. Pool input slabs; wire release through `pipeline.processBatch` → server reply site (both batcher and worker-pool paths, and the cache-miss store path).
3. Land benchmem microbenchmarks and the P0 harness gates; then remove the copy path.

## Open Questions

- Does `GetData()` keep ORT output memory stable until `Destroy()` in the current `onnxruntime_go` version? (Determines whether we can fully avoid the output copy.)