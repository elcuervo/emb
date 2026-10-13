# Design

## Context

See `proposal.md` for motivation. This is a cross-cutting scheduling and release-validation change, so a design is required.

### Evidence ledger

Investigation date: 2026-10-13. These observations are historical evidence from the investigation, not results of the proposed implementation.

| Evidence | Observation | Meaning / limit |
|---|---|---|
| Release history | `v0.4.3.pre3` resolves to `a0ffc5b`; `v0.4.3.pre4` resolves to `0ab054e` | Confirm these against the actual deployed binary/image before calling pre3 the incident's last-known-good |
| Scoped source comparison | No differences from pre4 in `registry/{cgroup,cpu_budget,calibration,registry}.go`, `permit/budget.go`, `server/{inference_bench,script_smoke}_test.go` at investigation time | Local probes exercise the examined release logic, not necessarily its exact binary/toolchain |
| Two-model runtime probe | `GOMAXPROCS=4`, `test-two-models.yaml`, allowance 4, reserve 1, budget 3; minilm 1 worker x 1 thread; bge planned share 2; 160 minilm requests, zero errors, peak tokens 1 | Demonstrates stranded resident capacity with bge idle; does not measure production slowdown or prove the outage cause |
| Cold runtime probe | `GOMAXPROCS=2`, `config.yaml`; `EMB.READY` returned OK before lazy initialization; calibration logged 1.765 seconds; 80 requests, zero errors | Demonstrates first-request cost after registration readiness; not a benchmark baseline |
| Production entrypoint | `cmd/emb/main.go:49` constructs `NewWithConfig` | Governor enabled by default |
| Gate construction | `inference_bench_test.go:173` and `script_smoke_test.go:149` construct `registry.New()` | The nearest performance checks bypass the governor |
| Older incident context | Archived cross-model proposal records about 10 pairs/s/task; same-pre3 client dispatch change raised CPU from 2.5-3.1 to 3.8-4.3 CPUs; an Oct 12 request exemplar spent 1.13 of 1.39 seconds in embeddings | Workload motivation only; not pre4 outage evidence or a portable CPU target |
| Reference coverage | Active performance-insight tasks 8.2 unchecked; performance-validation docs acknowledge no end-to-end reference bundle | Do not reuse a checked planning task or same-binary noise run as proof of release safety |

Probe commands (all executed inside `nix develop`; the two servers were run sequentially and stopped afterward):

```sh
GOMAXPROCS=2 go run ./cmd/emb -config config.yaml -listen 127.0.0.1:16489
redis-cli -p 16489 EMB.READY
redis-benchmark -p 16489 -q -c 4 -n 80 EMB minilm "regression probe"
redis-cli -p 16489 EMB.INFO minilm
redis-cli -p 16489 EMB.STATS

GOMAXPROCS=4 go run ./cmd/emb -config test-two-models.yaml -listen 127.0.0.1:16489
redis-benchmark -p 16489 -q -c 8 -n 160 EMB minilm "regression probe"
redis-cli -p 16489 EMB.INFO minilm
redis-cli -p 16489 EMB.STATS
```

The development host was darwin/arm64 with 10 presented CPUs. GOMAXPROCS is not cgroup isolation or a limit on native ORT threads. The short redis-benchmark runs include initialization and are deliberately not used as throughput comparisons. No Linux quota reproduction, production logs, or exact release A/B was obtained.

### Mechanisms established by source

1. `modelThreadShares` splits the process budget across configured names. `ensurePool` uses the share as calibration budget; candidates reject worker-count x width above it. Layouts remain fixed, so unused global tokens cannot enlarge a one-worker/one-thread embedding model. Equal-weight integer remainders depend on sorted names.
2. Fractional allowance is floored and the reserve deducted before creating one global FIFO gate. Low budgets or wide runs can serialize paired model requests even with correct detection.
3. Admission has neither cancellation nor timeout. The existing server request cap defaults to unlimited. A capacity loss can therefore accumulate accepted work; actual outage queue growth is still an inference.
4. The calibration predictor uses measured execution time, excluding session creation and warmup, while the outer deadline cannot interrupt native work. CPU time is checked for availability but throughput is scored by wall time; no CPU-efficiency win has been established.
5. Startup uses the hierarchical fractional detector; sampler and legacy worker sizing use a different integer/root-only detector. Mount locations are assumed rather than resolved. These are correctness gaps, not proven incident triggers.
6. Script `run_us` wraps the budgeted session, so global admission waiting is included in reported run time. Script admission charges `ceil(width/caller_cap)` per invocation; this is not proof that native CPU is bounded by reported tokens.
7. The pair benchmark merely drains RESP, can count application errors as completions, labels requested legs as ORT rows, includes driver CPU, does not enforce failure thresholds, and defaults to about twenty pairs per case.

### Existing-contract conflicts

The current CPU-budget and CPU-isolation specs require calibration inside a model's share. This change deliberately replaces that permanent ceiling with a process-wide, memory-bounded resident ceiling; spec deltas must change both requirements. Existing runtime no-reshape and explicit-override contracts remain intact. `EMB.READY` currently permits lazy paths; this design preserves that compatibility instead of silently preloading all configured models. Existing active performance-validation artifacts are not rewritten or marked complete by this change.

## Goals / Non-Goals

**Goals:** Make default capacity useful under skewed and paired load; retain hard bounds on admitted work and safe native-resource ownership; make regressions observable; produce a gate that actually executes the deployed policy and discriminates known-bad pre4.

**Non-Goals:** No predictive tuner, online ORT session reconstruction, OS-level CPU isolation claim, model/quantization changes, client retry redesign, Rails/OpenSearch changes, automatic deployment, or third benchmark framework. Do not fix unrelated parser/telemetry issues as part of this work. Do not claim universal optimality or an absolute cross-host performance target.

## Decisions

### D1. Establish deployment truth before release acceptance

Record the actual known-good and pre4 binary/image digests; Go/ORT versions; model/tokenizer/config digests; model paths, preload flags, batching verdicts, workers/threads; GOMAXPROCS; cgroup membership, mountinfo, quota/period and ancestor constraints; affinity; memory limit; startup CPU/calibration logs; request rate/deadlines; restart/OOM events. Redact credentials and customer text. The existing approximate 10 pairs/s/task is the starting research workload, not an invented production SLO.

Missing incident artifacts block incident attribution and the final production acceptance task, not deterministic repairs or public-model harness development. A parser/config failure, missing model, or startup crash in an old artifact does not count as reproducing its performance regression. If the expected pre4 rejection does not reproduce, retain the result and investigate the specific discrepancy rather than loosen thresholds or fabricate a baseline.

### D2. One resource view, separate quantities

Extend the existing registry resource resolver rather than add a second detector or dependency. On Linux resolve cgroup v1/v2 controller mounts and mount roots from mountinfo plus membership, walk only visible applicable ancestors, and take the tightest positive finite quota/period. Cover combined v1 mounts, hybrid hierarchies, remapped mount roots, namespaced roots, unlimited children with limited parents, malformed/unreadable files, and affinity tighter than quota. Missing data is reported with provenance, not interpreted as zero available CPU.

Keep raw fractional bandwidth, current affinity ceiling, scheduler parallelism and integer admission capacity distinct. Use a common resolution for startup and scripted CPU normalization. Preserve `max(1, floor(effective capacity))` for initial admission and existing reserve tiers; do not round up or replace quota with host count merely to improve a benchmark. Preserve explicit lower scheduler planning limits and explicit budget warnings. Integer helper callers delegate to the common resolver; remove duplicate parsing, not public semantics of `cpu_budget: off` scheduling.

Normalize sampler CPU by fractional effective bandwidth and actual monotonic elapsed sample time. A fully utilized 0.5-CPU quota must not look 50% idle because it was rounded to one. Known unmeasurable CPU data must not authorize speculative concurrency growth. Initial layout remains fixed; adapting to live quota changes by reshaping sessions is out of scope and requires restart/documentation.

Alternative rejected: trusting GOMAXPROCS alone. It is Go parallelism, not a native-thread or fractional-bandwidth guarantee. Background: https://go.dev/blog/container-aware-gomaxprocs.

### D3. Borrowable capacity without online reconstruction

Keep one process admission controller. Separate model fair-share weights from the resident candidate ceiling: the ceiling is the process budget intersected with explicit dimensions and safe memory headroom, not the number of configured idle peers. For batching, consider one session with widths up to this ceiling; for unbatched models consider worker/width pairs within it. Continue the existing bounded candidate set and determinism-first ordering. A fully measured narrow winner remains valid; do not promise that every model benefits from every CPU.

Use model weights for fair scheduling under contention rather than fixed resource reservations. Maintain FIFO within each model and work-conserving selection across model queues, using weighted round-robin admission turns among runnable models; charge width against the process budget. Prevent indefinitely bypassing a wide/oversized head by granting it the next drained-budget turn once it has been bypassed for a full scheduling round. Idle queues retain no tokens or accumulated credits. Include model identity in the internal acquisition API; migrate embedding, script, image and calibration wrappers together.

Before expanding resident sessions, calculate incremental memory against the effective process/container limit, current RSS, remaining configured model reservations and cache allowance. Reuse the current model-size estimate but include external ONNX data and distinguish estimates from measured resident use. Serialize calibration/session expansion decisions so simultaneous lazy loads cannot each claim the same headroom. If a larger candidate cannot fit, retain a valid smaller layout and publish the memory-limited reason. Do not independently allocate half of total RAM to every model. Explicit settings remain authoritative but insufficient memory must fail clearly before an unsafe allocation when detectable.

Shared-session script charging must not divide width by a configured caller cap and assume that proves a CPU bound. Account for a live session's shared worker pool once while active and for additional concurrent caller execution explicitly; integrate the lease lifecycle with the existing permit pool. Tests must cover one caller, full caller cap, multiple sessions and mixed paths. Report unmodeled inter-op/spinning effects; admission is not an OS CPU cap.

Alternative rejected: only set each model's calibration budget to the full process budget. It removes one ceiling but can multiply memory and permits head-of-line starvation unless admission and memory accounting are validated together. Online resizing is unnecessary for this correction.

### D4. Calibration and serving readiness

Measure a candidate's total cost from before session creation until its temporary sessions are closed; use that total for deadline prediction. Separate measured inference timing for scoring from total calibration duration. Reject invalid shapes/nonfinite outputs; preserve output parity, batch-determinism degradation, explicit dimensions and the existing p95 guard. Temporary sessions remain alive until native work returns. Document that the deadline is soft around a noninterruptible native call.

Store immutable outcome metadata: selected workers/threads, candidate ceiling, source, reason and total duration. `calibrate: false` must select a documented process-capable memory-safe layout, not reproduce the old permanent share restriction; it is distinct from `cpu_budget: off`.

Reuse existing preload flags. Explicitly preloaded embedding/script/image resources must finish their initialization before readiness. Production gate configuration enables preload for required serving paths and exercises one representative request per path before traffic routing. Registration-only lazy readiness remains compatible, but stats expose which paths are uninitialized and docs state that `EMB.READY` is not a latency warranty for lazy paths. Do not invent an all-model preload default or execute arbitrary scripts during startup solely to satisfy a health check.

### D5. Bounded queue waits, safe native execution

Add boot-time `cpu_admission_timeout` using the existing duration parser: omitted/zero means unlimited for compatibility; negative/invalid values fail validation. A finite value bounds each wait at the global CPU gate, not the whole command and not an already-running ORT call. The error is `ERR busy: CPU admission timeout`; preserve existing command-specific per-item error/null behavior for multi-item operations rather than change reply shape.

Change admission to return `(release, error)` and accept cancellation plus model/width. Cancellation removes its waiter atomically and wakes eligible successors; grant/cancel races have exactly one owner and no token leak. Connect lifecycle cancellation at available request boundaries and registry shutdown. Do not claim immediate peer-disconnect cancellation on transports that do not expose it; the configured finite admission timeout is the backstop. Existing accepted work/native ownership and shutdown deadlines remain authoritative.

The release workload config must set finite `max_concurrent_requests`, connection limits and admission timeout below the application's request deadline. Reuse the existing request cap rather than add another queue-length option. Control commands remain outside inference admission. This does not introduce a full pipeline request-timeout API: local-pool waits are bounded in count by the request cap, and client/harness deadlines measure whole-command behavior.

### D6. Diagnose policy, not just symptoms

Add process `cpu_admission_wait_us`, `cpu_admission_waiters`, and `cpu_admission_timeouts`; per-path admission wait counters; calibration source/reason/duration/ceiling; and initialized state for embedding/script/image paths. Preserve existing CPU allowance/source/budget/active/peak fields and expose the limiting resource separately from model weight/share. Counters use atomics or the existing gate lock and do not scan queues/models on the inference hot path.

Separate local dispatch wait, global admission wait and native Run duration at their actual boundaries. Existing run counters must not continue to include global waiting after the change. Update INFO/EMB.STATS, relevant telemetry mappings, parsers and documented timing semantics; verify exact RESP element counts and unknown-field compatibility. Stats reads never load a lazy model or perform cgroup filesystem I/O.

### D7. Repair and reuse the workload driver

Move reusable open-loop scheduling/RESP measurement out of the in-process test-only wrapper into the existing `bench/fargate` harness boundary. Keep any benchmark wrapper thin; use the same executable/config path as production for acceptance. The driver runs outside the measured server cgroup; if sharing a task/host, its CPU partition is recorded and excluded from the declared server allowance.

Use existing RESP decoding to distinguish valid float32 embeddings, null/truncated values, server errors, malformed replies and transport failures. Verify dimensions, finiteness and model-appropriate correctness against pinned references. Set connection/read deadlines and a bounded final drain. Every scheduled pair ends in exactly one of success, server/transport error, timeout or dropped-before-start; per-leg outcomes remain available without double-counting pairs. Measure latency from scheduled time as well as wire-start time, so delayed scheduling is visible. Measure throughput from wall elapsed time, not summed request latencies.

Prewarm the warm corpus explicitly; ensure cold inputs remain unique; cycle short/typical/long classes by request rather than leg index. Report actual model inference counters instead of requested legs labeled ORT rows. Capture server CPU/RSS through server/process counters, not the driver process. Missing prerequisites produce invalid evidence, never a passing skip. Retain the miniature scripted smoke and add nonzero-work/error assertions there without pretending it is a governor test.

### D8. Reproduction matrix and comparison contract

Use the same external driver for known-good, pre4 and candidate, interleaved for at least three rounds. Pin the exact production model pair when available; public minilm+bge and the committed scripted fixture are development/control coverage, not substitutes for incident validation.

Required phases: cold first use; explicitly preloaded startup; warmed one-hot load with idle peer; paired serial/parallel and EMB.MULTI; mixed short/typical/long and warm/cold cache; sustained rate; burst; recovery. Cover default batching and determinism-degraded single-row execution. CPU controls include 0.5, 1, 1.5, 2 and 4 CPU bandwidth plus the actual incident quota/period; affinity and lower scheduler controls are independent dimensions. Use selected combinations for routine CI rather than an unnecessary Cartesian product; the incident profile is mandatory.

Use an isolated linux/arm64 host with identical quotas per arm. Separate `reference` and `constrained-reproduction` evidence modes: reference qualification remains useful, but an intentional fractional limit is not grounds to skip the production test. Collect server-side observed limits per arm, not merely the orchestrator's fingerprint.

Default comparative guardrails are candidate useful throughput >=95% of known-good and pair p50/p95/p99 <=110% of known-good, with zero unexpected errors/timeouts/drops at the known-good sustainable load. Validate these thresholds first with good-versus-good interleaved runs; noise makes a result inconclusive, not a reason to widen thresholds automatically. Require at least 1,000 successful pairs for each gated steady-state tail cell and a duration of at least 120 seconds; cold and recovery phases have their own deadlines. Record CPU/completed pair and throttle deltas; do not invent a universal CPU-efficiency threshold. Production request/health/recovery deadlines are mandatory profile inputs captured from the deployment before the final acceptance run.

The overload phase permits explicit busy responses, but not silent loss, unbounded memory, control-plane starvation or failure to recover. Compare recovery to the known-good envelope and the declared deployment deadline. No OOM/restart is acceptable.

### D9. Fail-closed evidence and CI

Extend the existing versioned Fargate bundle: exact artifact digests, model/tokenizer/config/corpus hashes, CPU model/runtime versions, server resource provenance, requested versus observed limits, phase schedule and counts, latency samples/quantiles, correctness, admission and inference timings, CPU/throttle/RSS, startup/calibration metadata, health and recovery, and named assertion outcomes.

Reject empty/duplicate/missing cells, mismatched inputs, invalid required assertions, zero successful controls, unapproved baseline provenance and incomplete runs before calculating ratios. Keep the baseline immutable and preserve failed bundles. A known-bad artifact must fail the intended service assertion, not a missing fixture or unrelated error. Old release metrics that do not exist are recorded unavailable; mandatory external correctness/latency/availability checks still run.

Add a bounded production-entrypoint smoke to existing relevant-code CI. Run the measured ARM regression job for scheduling/registry/pipeline/ONNX/server/harness changes and as an unconditional prerequisite of publishing release artifacts. Missing model access or unavailable runner must block that release gate, not bypass it. Full exploratory sweeps may remain manual; the exact incident profile and known-bad self-check must be enforced. Keep the active reference change's remaining work separate; reconcile shared files when applying rather than silently marking its tasks done.

## Risks / Trade-offs

- Larger resident layouts -> account for process-wide memory, serialize expansion, retain smaller validated candidates, and gate peak RSS with production artifacts.
- Wider runs can monopolize a small budget -> weighted fair admission with bounded bypass, mixed-path tests and paired tail-latency gates; no promise of OS isolation.
- Shared-session CPU cost is not exactly thread width -> account for active sessions and caller execution, retain measured CPU/throttle evidence and explicit spinning/inter-op warnings.
- Timer/cancellation races -> deterministic gated fakes for grant, timeout, cancellation, error, and shutdown; race checks; no sleep-based ordering assertions.
- Calibration remains soft-deadline -> full-cost prediction, explicit preload, visible fallback reason, and no unsafe native interruption.
- Optional timeout preserves an unlimited legacy default -> deployment profile must set finite timeout and existing request/connection caps; docs must not claim universal end-to-end deadlines.
- Production evidence may be unavailable -> block attribution/release acceptance, continue only independently verifiable implementation tasks, never mark the evidence tasks complete.
- Performance noise -> same-host interleaving, minimum sample count, good-versus-good qualification and an explicit inconclusive result.

## Migration Plan

1. Capture incident inputs and establish known-good/pre4 reproduction without changing production configuration.
2. Land fail-closed harness and deterministic behavior coverage; retain the failing pre4 artifact as a regression control.
3. Apply resource resolution, resident/admission, calibration and timing fixes with all affected callers migrated. Preserve the rollback switch and explicit settings.
4. Validate correctness, overload, native lifetime, memory and three-arm performance under the same profile; update operational/configuration and benchmark docs.
5. Canary the candidate with required paths preloaded and finite existing request/connection caps plus admission timeout. Observe useful completions, tails, queues, CPU and memory before expanding.
6. Roll back artifact/config on failure. `cpu_budget: off` is a diagnostic/containment arm, not the permanent fix; it restores legacy oversubscription risks. `calibrate: false` is not equivalent to disabling the governor.

## Open Questions

Only incident evidence values remain to be collected: exact deployed artifacts/configuration, quota/period and affinity, model hashes, request/health/recovery deadlines, and whether the recorded failure ended in OOM, restart or timeout-only unavailability. These are required acceptance inputs, not authority to change the selected design or silently narrow the matrix. If evidence contradicts the proposed mechanism, report it and revise this change with the user before claiming a fix.
