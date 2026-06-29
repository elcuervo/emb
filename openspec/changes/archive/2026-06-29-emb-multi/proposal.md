## Why

A single connection pipelines `EMB siglip2 "text"` `EMB e5 "query: test"` sequentially — each blocks until the previous finishes, even though the models are independent. The user needs to send mixed-model workloads (e.g., different models for different text types) without serializing inference.

## What Changes

- Add `EMB.MULTI` command: takes `model text` pairs, fires them concurrently across model pools, returns an array of embeddings in order
- Unknown models or inference failures return nil per-pair (MGET semantics), not a command-level error
- Internal fan-out via goroutines: each pair calls its model's `Pool.Embed()` concurrently, results collected by index
- Server counts each pair as one request in `EMB.STATS` (N pairs = N requests)
- No changes to the existing `EMB` command, config, or protocol

## Capabilities

### New Capabilities

- `emb-multi`: Concurrent cross-model embedding via `EMB.MULTI` with MGET-style partial failure semantics

## Impact

Files: `internal/server/server.go` (new handler, one mux registration), `cmd/emb-multi-verify/main.go` (new e2e verification tool), `justfile` (new `verify-emb-multi` recipe). No changes to pipeline, ONNX runtime, tokenizer, config, or registry. Backward compatible — no wire format change, no config change, no breaking changes to existing commands.

## Benchmarks

No baseline change expected for single-model workloads. Multi-model fan-out benchmark to be measured: N concurrent model calls vs N sequential `EMB` calls.

## E2E Verification

Two real ONNX models are downloaded and registered. `EMB.MULTI` is exercised across models and results are compared byte-for-byte against sequential `EMB` calls. See `design.md` for the full approach.
