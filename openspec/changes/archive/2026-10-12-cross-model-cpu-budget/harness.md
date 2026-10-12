## Local autotune verification contract

This harness validates decisions made by the tuner and compares useful inference work on the local runtime. It does not depend on Datadog, a private application repository, cloud credentials, a dedicated ARM runner, or an eight-replica deployment.

### Inputs

- Reuse the repository's pinned public models, tokenizers, and correctness fixtures; verify their configured hashes before measuring.
- Use deterministic short, typical, and long input sets and seeded single-model, paired-model, serial/parallel, and skewed multi-model schedules.
- Offer requests on a fixed schedule independent of response completion. Record scheduled, started, completed, timed out, and dropped requests so slower candidates cannot appear faster by doing less work.
- Run warm-cache and cold-cache profiles separately. Record probe outcome and whether each inference was a cache hit or miss.

### Measurements

Persist enough JSON for a run to report:

- chosen and fallback workers, intra-op width, resident ceiling, active allowance, and calibration time;
- per-leg and whole-pair p50/p95/p99 latency;
- offered/completed/dropped rows and pairs, errors, and output correctness;
- process CPU seconds per completed pair and per ORT row, RSS, and cache hits/misses;
- effective process allowance, scheduler/cgroup source, model/tokenizer hashes, Go/runtime version, and machine/runtime metadata.

Queue/admission wait, local pool wait, preprocessing, and ONNX `Run` time are separate fields. Do not infer inference CPU cost from an outer HTTP span or request wall time. The supplied Datadog exemplar motivates embedding-heavy inputs but is not a performance baseline for this repository.

### Decision rules

- A candidate with incorrect output, memory-limit breach, initialization timeout, errors, or lost scheduled work is rejected.
- Prefer higher completed rows per CPU second when p95/p99 remain within the guardrail; otherwise retain the best validated fallback.
- A measured win must exceed run-to-run noise. When samples are insufficient or confidence intervals overlap, report `inconclusive` and keep the conservative layout.
- Tune each model and then confirm the joint multi-model plan. Single-model speedups cannot override a regression in the paired workload.
- The harness reports measurements and a deterministic local verdict. It does not claim universal optimality or change a baseline automatically.

### Test layers

1. Model-free unit tests use fake sessions, resource limits, CPU signals, and a fake clock to verify candidate filtering/scoring, fairness, saturation freeze, dwell, rollback, deadlines, and missing-signal behavior.
2. Local integration tests run the autotuner against pinned repository models, validate selected output, then start a two-model server with automatic settings and inspect runtime metrics.
3. `just bench-inference` (or an equivalent documented command) runs the seeded workload and writes a self-contained result directory. It may be run on any developer machine; its metadata makes cross-machine comparisons explicit.
4. Linux ARM64 and production-shaped routing runs are optional release evidence. Their absence does not block deterministic tests, local runtime validation, or completion of the implementation change.
