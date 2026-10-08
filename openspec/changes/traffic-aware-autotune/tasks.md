# Tasks

## 1. Container-aware sizing

- [ ] 1.1 Add effective CPU/memory resolution from cgroup v2 (`memory.max`, `cpu.max`) then v1 (`memory.limit_in_bytes`, `cpu.cfs_quota_us`/`cpu.cfs_period_us`), falling back to host values. Verify with unit tests covering each file layout and the fallback.
- [ ] 1.2 Use the effective limits in `autoTuneWorkers` and the derived thread count. Verify a test where a small cgroup memory/CPU limit yields fewer workers/threads than the host value.
- [ ] 1.3 Document container sizing in `docs/configuration.md`. Verify the documented behavior matches the test expectations.

## 2. Traffic classifier

- [ ] 2.1 Add a per-model in-flight gauge incremented at the inference boundary (`ScriptResources.RunNamed`, unbatched `pipeline.Pool.Embed`). Verify a test that cache-hit requests do not increment it.
- [ ] 2.2 Add the sampling classifier (window math, `idle|latency|throughput|saturated`) with an injectable clock and counter source. Verify deterministic unit tests for each class, including zero-run windows and the CPU gate.
- [ ] 2.3 Publish the class, in-flight, and sampler lifecycle on `ModelEntry` (start lazily with the resources, stop on close). Verify a test that the sampler stops and leaks no goroutine after `Close`.

## 3. Adaptive concurrency

- [ ] 3.1 Add a resizable permit pool (`Acquire`/`Release`/`SetCapacity`) bounded to `[1, cap]` that never revokes an in-flight permit. Verify unit tests for growth, shrink-below-in-flight, and the cap.
- [ ] 3.2 Switch `ScriptResources` and the unbatched `pipeline.Pool` to the permit pool. Verify the idle-dispatch, cap, and reply-parity tests still pass.
- [ ] 3.3 Add the AIMD controller (grow after 2 `throughput` windows at the current capacity with CPU headroom; shrink after 3 `latency`/`idle` windows; one change per window) plus the `saturated` recommendation log and batcher exclusion. Verify deterministic tests for grow, shrink, damping, saturation refusal, and batcher exclusion.

## 4. Capacity profiles and config

- [ ] 4.1 Add `capacity: auto|latency|throughput` and `autotune: off|callers|auto` config fields with defaults (`auto`/`auto`) and validation. Verify config tests for defaults, explicit values, and rejection of unknown values.
- [ ] 4.2 Apply profiles at session creation: `latency` pins spinning on and allowance 1 with the controller off, `throughput` pins spinning off and the cap with the controller on. Verify tests asserting the created session options and initial allowance.
- [ ] 4.3 Document the profiles, the switch, and the precedence rules in `docs/configuration.md`. Verify the doc's examples parse and match the config tests.

## 5. Observability

- [ ] 5.1 Add per-model `traffic_class`, `inflight`, `concurrency_current`, `concurrency_target`, and `autotune_active` to `EMB.INFO` and the per-model `EMB.STATS` strings; update the RESP array-count tests. Verify the array-count and field-presence tests pass.
- [ ] 5.2 Document the fields, how to read the class/allowance, the kill switch, and the recommendation log in `docs/operations.md`. Verify the documented fields match `EMB.INFO` output in a test.

## 6. Shape benchmark and validation

- [ ] 6.1 Add `cmd/evalbench -shape serial|burst|mixed` with fixed concurrency phases that sample `EMB.INFO` per phase. Verify a run against a live server prints the class/allowance transition.
- [ ] 6.2 Add a `just bench-script` shape run and record before/after in `BENCHMARK.md`. Verify the recorded command reproduces the table on the reference host.
- [ ] 6.3 Run `go test ./... -count=1`, `just lint`, `go test -race ./internal/registry ./internal/pipeline ./internal/server`, and `openspec validate traffic-aware-autotune --strict`; all must pass.

## Workflow follow-up

- Archive the change after review; confirm the main specs gain the `inference-capacity-autotune` capability and the three modified requirements.
