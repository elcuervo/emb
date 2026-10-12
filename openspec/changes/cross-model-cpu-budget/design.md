## Design

### Goal and evidence

Autotune the inference process around completed, correct work per CPU second while protecting request tail latency and process memory. Keep these measures distinct:

- queue/admission wait before an ONNX run;
- time inside tokenization, batching, and `Session.Run`;
- completed rows/pairs, errors, and cache hits/misses;
- process CPU time, throttling/pressure where available, and RSS.

The supplied production observations report that parallel model calls raised CPU from 2.5–3.1 to 3.8–4.3 CPUs at about 10 pairs/s/task with server revision and model configuration held constant. Treat this as a throughput/load shape to reproduce, not as proof that concurrency should be reduced.

Datadog evidence from the linked 2026-10-12 one-hour production window: `/search/photos` handled 1,408 spans with p95 987ms and p99 1.84s. A representative sampled request completed in 1.387s; its `phrase:embeddings` span was 1.130s, with visible OpenSearch calls of 38ms and 42ms. The exact URL-selected `spanID` was not retrievable through the MCP span search, so the representative trace is not presented as that exact span. This is application-level wall time and cannot by itself distinguish CPU execution from queueing or downstream wait. It motivates per-ORT queue/run/CPU metrics and an embedding-heavy calibration scenario.

### Startup sequence

```text
resolve process limits
        ↓
plan budget and conservative memory ceiling
        ↓
load model metadata and probe batch determinism
        ↓
materialize bounded candidate sessions
        ↓
warm and measure representative input lengths
        ↓
select correct layout or safe fallback
        ↓
serve and adapt active concurrency within that layout
```

Calibration is local and bounded. It uses the loaded model/tokenizer, fixed text fixtures representing short, typical, and long requests, and the actual ONNX path. Warmup is excluded from measurements. Candidate widths and session counts are bounded by the process share and the existing model-size-based memory estimate. Each candidate receives repeated trials with the same requests. The tuner rejects correctness mismatches, unavailable CPU accounting, and candidates whose p95 exceeds twice the single-worker baseline; among the remaining candidates it selects the highest measured rows per wall-clock second and favors the lower-resource candidate on a tie. Models are calibrated independently; shared admission enforces the process budget under contention.

Calibration checks its deadline between candidates and waits for any in-flight CGo calls before closing temporary sessions. A slow individual model call can exceed the nominal deadline. Invalid fixtures, candidate failures, or unavailable CPU accounting fall back to one worker and width one (or explicit settings). Explicit dimensions remain fixed. `calibrate: false` uses conservative derived sizing while keeping admission enabled.

### CPU and memory planning

Each process resolves its own allowance; replicas never divide one another's budget. Preserve fractional cgroup quota for CPU accounting. The integer candidate ceiling is bounded by the most restrictive applicable quota, cpuset/affinity, and explicit lower scheduler limit. Keep Go-only workers scheduler-aware. A reserve applies once per process, not once per model. `cpu_budget: off` uses legacy sizing/spinning and bypasses the new governor.

The process plans one shared active token budget. Model `weight` affects minimum active share and tie-breaking, not a permanent reservation. A quiet model lends tokens immediately. Candidate resident sessions are bounded by the existing model-size estimate plus measured process memory headroom, cache budget, and an explicit safety margin. File size remains a planning estimate, never a claim of exact RSS. If a minimum layout cannot fit, startup keeps the affected path lazy or fails with the model and limiting resource identified; it does not silently exceed the ceiling.

### Runtime adaptation

One FIFO weighted admission controller covers embedding, script, image, and calibration runs. Acquire immediately before `Session.Run`; release on every return path. Local session-pool waiting and preprocessing do not own global tokens. An explicitly configured run wider than the budget is admitted alone and logged.

The controller periodically evaluates a rolling window per model:

- queued demand or admission wait with spare CPU and session capacity can grant one additional active slot;
- sustained CPU saturation or rising throttling can remove one active slot from the least efficient pressured allocation;
- insufficient samples, missing CPU signals, or contradictory signals freeze the last safe setting;
- changes require a dwell period; a trial is reverted if useful throughput falls or p95/p99/error rate crosses its guardrail.

It adjusts active allowances only. It does not rebuild sessions, alter explicit widths, or claim CPU isolation. Idle capacity is borrowable within the calibrated resident ceiling. The first implementation should keep the controller to a single bounded increase/decrease step per evaluation; no predictive model or external telemetry service is required.

### Determinism and compatibility

Probe determinism before calibration. A failed probe sets batching off, calibrates single-row work, and reports the actual resulting pool. Existing result parity is mandatory. Preserve explicit worker/thread/spinning values and current script profile precedence. Default shared embedding/image pools use spinning off; spinning outside an active `Run` is not controlled by admission and remains visible as an explicit risk.

For shared-session script calls, charge the session width once per actual serialized `Run`, not once per waiting caller. Inter-op threads remain explicitly reported as unmodeled unless the runtime can reliably account for them.

### Observability

Startup logs report detected quota/affinity/scheduler ceiling, operator overrides, budget/reserve, and each model's planned candidate and calibration outcome. `EMB.INFO` reports per-model/path planned and actual workers, width, spinning, active allowance, resident ceiling, calibration source/cost, queue wait, run time, and limiter. `EMB.STATS` reports fractional allowance, source, budget, active/peak tokens, CPU/throttle/PSI signals when available, and the reason growth is blocked. Metrics must not trigger lazy model load. Missing host signals are `unavailable`, not zero.

### Local validation

The default implementation gate uses Nix, the existing public local model fixtures, model-free fake-session tests, and a real two-model server smoke test. A reproducible local benchmark includes sequential and parallel model pairs at a fixed open-loop rate near 10 pairs/s, short/typical/long inputs, warm/cold cache, and single-model controls. It records per-leg and whole-pair latency, offered/completed/dropped work, CPU seconds per pair and per ORT row, errors, RSS, probe outcome, and selected layout. Hardware-specific ARM64/eight-replica comparisons are optional deployment evidence; they cannot block implementing or running the local change.

### Datadog reference

The user-provided resource flamegraph is the source for the linked-hour summary. MCP search could not look up a span by the URL's `trace__spanID`, so the detailed exemplar is separately linked by the trace ID returned from searching slow `/search/photos` spans:

- [Linked Rack resource flamegraph](<https://app.datadoghq.com/apm/resource/api-fargate/rack.request/72bcbabad600ef?dependencyMap.showNetworkMetrics=false&deployments=qson%3A(data%3A(hits%3A(selected%3Aversion_count%2CgroupBy%3A[version])%2Cerrors%3A(selected%3Aversion_count%2CgroupBy%3A[version])%2Clatency%3A(selected%3Ap90%2CgroupBy%3A[version])%2CtopN%3A!5)%2Cversion%3A!0)&env=*&fromUser=false&graphType=flamegraph&trace__spanID=3941709097520793195&start=1791817923674&end=1791821523674#errors>)
- [Representative slow `/search/photos` trace](https://app.datadoghq.com/apm/trace/1094615276211124003?spanID=1409578391632545373)

The representative trace returned HTTP 200 and contained no correlated logs. Datadog data informs workload design only; runtime calibration remains self-contained and stores no production request text.
