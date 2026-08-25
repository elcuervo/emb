## Why

Every ONNX run allocates fresh input tensors (input_ids, attention_mask) and copies the output tensor into a new Go slice (`runtime.go:142-144`). A 32×512×384 batch is ~25MB copied per run through CGo on every request — on ARM64/Graviton, with finite memory and GC-sensitive tails (Fargate tasks are memory-capped), that copy + allocation churn directly taxes both throughput and p99 latency. TEI stays fully native and writes the embedding straight into the response.

## What Changes

- Introduce pooled, per-model buffer reuse for input tensors (backing `[]int64` for input_ids / attention_mask via `sync.Pool`, pre-sized to batch×seqLen).
- Eliminate the redundant output copy: obtain the ORT output backing slice once per run and return it with ownership, so the pooling step and RESP reply read the same bytes.
- Buffer lifetime is pinned until the RESP bulk reply is written, then returned to the pool (release-charged pattern used by redcon's conn API).
- Add `go test -bench` micro-benchmarks (`Inference`-themed) to quantify allocs/op and B/op before/after.
- No protocol change; outputs are byte-identical.

## Capabilities

### New Capabilities

- `zero-copy-inference`: pooled tensor buffers and removal of the post-run output copy to cut allocation and memcpy pressure.

### Modified Capabilities

(none — internal performance, no behavior/spec change)

## Impact

Files: `internal/onnx/runtime.go` (pooled buffers, borrowed-output semantics), `internal/pipeline/pipeline.go` (ownership handoff), `internal/server/server.go` (write reply from borrowed buffer, then release), tests/benchmarks. CGo boundary unchanged; `onnx.Session` interface unchanged.

## Validation

All inside `nix develop`:

```
$ nix develop
$ go test -bench=BenchmarkInference -benchmem -run=^$ ./internal/...
$ just bench-fargate-diff <baseline> <after>
```

- **Alloc gate:** allocs/op of the inference path ≥ **−40%**; B/op ≥ **−30%** vs current microbench baseline.
- **Throughput gate:** req/s at 16 clients / 8 vCPU ≥ **+10%**.
- **Tail gate:** p99 at 16 clients / 2 vCPU ≤ baseline p99 (GC-pressure relief).
- **Correctness gate:** `just verify-embeddings` passes unchanged (identical bytes).