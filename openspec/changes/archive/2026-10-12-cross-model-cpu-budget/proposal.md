## Why

`emb` currently derives every model's pool independently from machine CPU count. On a constrained multi-model process this can create far more ONNX Runtime threads and sessions than the container can use. Probe failure can also leave a model with the worker count intended for batching. Operators then have to discover safe thread and worker settings by hand.

The goal is an out-of-the-box autotuner that chooses ONNX Runtime thread width and session count from actual process limits and measurements of each loaded embedding model. It should improve useful throughput across short, typical, and long requests while preserving correctness and respecting a process-wide inference budget. Unsupported or noisy trials must fall back safely. The process budget is admission control, not an OS CPU limit.

The supplied Datadog view is useful workload evidence. During 2026-10-12 15:12–16:12 UTC, production `/search/photos` had 1,408 requests, p95 near 0.99s, and p99 near 1.84s. A representative 1.39s request trace from the same Rack resource spent 1.13s in `phrase:embeddings`; two visible OpenSearch calls took about 38ms and 42ms. Datadog could not resolve the URL's exact selected span ID, so the trace is an exemplar, not the exact selected request. This supports measuring embedding queue time, inference time, useful completions, and CPU cost separately; it does not establish that every request is CPU-bound.

The change also addresses the reported same-pre3 comparison: parallel client calls raised CPU from 2.5–3.1 to 3.8–4.3 CPUs at roughly 10 pairs/s per task. Client dispatch, cache state, and server version remain separate benchmark dimensions so higher CPU is not mistaken for a server regression or a throughput improvement.

## What changes

- Automatically resolve each process's usable CPU allowance from cgroup quota/hierarchy, cpuset/affinity, and Go scheduler limits; retain fractional allowance for accounting.
- Establish a conservative process-wide inference budget and share it across embedding, script, and image ONNX runs. Idle models lend unused active capacity.
- After determinism probing, run bounded local calibration for unset embedding thread/session settings using representative short, typical, and long inputs. Preserve explicit settings.
- Reuse the selected model layout for its embedding, image, and scripted paths, with the existing process-wide admission gate protecting all model runs.
- Default shared embedding/image pools to spinning off, preserve explicit settings and script-profile precedence, and keep `cpu_budget: off` as a policy rollback.
- Expose the selected width and live worker/session counts through existing `EMB.INFO` and the process budget through `EMB.STATS`.
- Add deterministic calibration/budget tests and validate with the repository's local ONNX fixtures. Cloud and ARM hardware comparisons are optional deployment evidence.

No RESP command shape, embedding result, cache key, or client API changes. Existing explicit `workers`, `intra_op_threads`, and spinning settings remain authoritative. A failed probe continues to serve single-row inference.

## Out of scope

- Tuning the Rails/Unsplash application, OpenSearch, or its network dependencies.
- Reading Datadog credentials or depending on Datadog at runtime. The trace is used only to inform workload dimensions and measurement boundaries.
- Automatically changing model weights, quantization, request semantics, or cache policy.
- Claiming universal optimality or changing a running layout. The tuner selects a measured safe startup configuration for this process.
- Runtime predictive control, online resizing, and external telemetry integrations.

## Capabilities

### New capabilities

- `inference-cpu-budget`: process-local limits, bounded calibration, shared admission, and rollback.

### Modified capabilities

- `inference-performance`: measured defaults, correctness, CPU efficiency, and tail-latency protection.
- `inference-cpu-isolation`: container-aware allowance and a process reserve.
- `batch-determinism`: probe-before-calibration and safe single-row fallback.
- `server-stats-observability`: selected layout and process budget reporting.

## Configuration

Defaults require no model-specific tuning. `cpu_budget` remains `auto|off|<positive integer>`; optional `cpu_reserve` and positive finite model `weight` remain supported. Unset embedding widths/session counts are calibrated by default. Explicit settings bypass selection for that dimension. `calibrate: false` skips startup trials and uses conservative sizing while retaining process admission.

## Acceptance

The default configuration must start and serve all configured models without tuning keys, respect the detected process budget for admitted work, and select a correct measured layout or a visible safe fallback. Tests cover fractional/container limits, calibration candidate bounds, probe failure, admission fairness, errors, and explicit overrides. Local validation runs the full Go suite, lint/vet, and a two-model runtime smoke test.

Calibration must preserve output correctness and stay within the local latency guardrail. Hardware-specific performance claims remain separate from local acceptance.
