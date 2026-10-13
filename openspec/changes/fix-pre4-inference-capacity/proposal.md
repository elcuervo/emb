# Proposal

## Why

The reported v0.4.3.pre4 performance regression made the service unavailable after a release that enabled process CPU admission, static per-model shares, and startup calibration by default. Investigation reproduced stranded inference capacity and first-request calibration delay, and found that the nearest performance checks bypass the production CPU policy; we need to repair those defects and require a deployment-shaped release comparison before claiming the outage cannot recur.

## What Changes

- Preserve the investigation evidence, release identities, runtime observations, and unproven hypotheses in the design; obtain the exact incident configuration and resource limits for final attribution rather than inventing them.
- Separate CPU bandwidth, scheduler parallelism, admission capacity, model fair share, and resident layout ceilings. Resolve applicable cgroup mounts/hierarchy and affinity once through shared resource-detection code, retaining fractional bandwidth for saturation accounting.
- Remove static model-share ceilings as the sole limit on automatic resident layouts. Let an active model use otherwise idle process capacity within a validated, memory-bounded layout; maintain fair cross-model admission and explicit operator settings.
- Measure full calibration cost, retain correctness and determinism checks, and protect explicitly preloaded serving paths from first-request initialization after readiness. Intentionally lazy paths remain supported and visibly uninitialized.
- Add bounded, cancellable CPU-admission waiting with an optional boot-time `cpu_admission_timeout` duration (zero preserves unlimited waiting), alongside the existing `max_concurrent_requests` cap. The production regression profile must configure finite limits. Never destroy native resources to implement a timeout.
- Expose global admission wait, queue depth/timeouts, calibration outcome, and actual path layouts without mixing admission time into native run time or initializing lazy resources.
- Repair the existing inference harness to drive external release artifacts through the production entrypoint, decode successful embeddings, account for all offered work, separate cold start from warm measurement, and compare last-known-good, pre4, and fixed builds under identical constraints.
- Make the minimal production-shaped regression a required release gate, including constrained hosts that are unsuitable for reference benchmarking. Retain the existing scripted smoke for its separate sampler coverage.

## Capabilities

### New Capabilities

None. Extend the existing CPU, lifecycle, observability, and benchmark contracts.

### Modified Capabilities

- `inference-cpu-budget`: unified limit resolution, borrow-capable memory-bounded layouts, full-cost calibration, and cancellable admission waiting.
- `inference-cpu-isolation`: derive automatic widths from process capacity and validated resident layouts, not a permanent model-share ceiling.
- `inference-performance`: protect one-hot and joint-model performance and make production-policy coverage mandatory.
- `inference-capacity-autotune`: normalize saturation against fractional effective CPU bandwidth and actual sample duration.
- `server-lifecycle`: distinguish configured preloaded readiness from lazy registration and preserve safe overload/shutdown behavior.
- `server-stats-observability`: separate admission, pool, and native timing; report selected layout and calibration provenance.
- `fargate-benchmark-harness`: fail-closed, deployment-shaped three-artifact regression evidence and enforced release gating.

## Impact

Primary code: `internal/registry/{cgroup,cpu_budget,calibration,registry,sampler,image}.go`, `internal/permit/`, inference timing in `internal/pipeline/` and `internal/onnx/`, `internal/config/`, `internal/server/`, and `cmd/emb/main.go`. Migrate affected internal callers and deterministic tests; no embedding format, model precision, cache-key, Ruby API, or protocol-version change is intended. Existing stats fields remain, with corrected timing semantics documented.

Harness: reuse `internal/server/inference_bench_test.go`, `bench/fargate/`, `bench/hostcheck/`, the existing RESP decoder and fixture/model caches; integrate through `justfile` and `.github/workflows/{ci,release}.yml`. Update configuration, operations, commands, performance-validation, testing, and benchmark documentation. No new runtime dependency or automatic production deployment is planned.

The active `performance-insight-and-smoke-gate` change remains independent: its task 8.2 is still incomplete and its scripted reference logs are not evidence that pre4's production governor is safe. This change extends the existing `fargate-benchmark-harness` capability without duplicating that change's unarchived `performance-validation` capability.

Changing default selected layouts is intentional behavior repair; explicit workers/threads/spinning and `cpu_budget: off` retain their contracts. Rollout requires a canary and verified memory headroom. Production root-cause attribution and known-bad rejection are required evidence tasks, not assumed accomplishments.
