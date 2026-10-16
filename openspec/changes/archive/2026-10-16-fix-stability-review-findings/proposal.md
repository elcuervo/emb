# Proposal

## Why

A pre-release stability review of 0.4.3 — six independent review passes over the
`v0.4.2..HEAD` delta plus a central build/vet/lint/deadcode/coverage sweep —
found defects that the release's own gates cannot see, and spec text that
contradicts the behavior the release ships. The three findings that matter most
are all *invisible in green CI*: the admission wait that is attributed to a
shared session slot makes `run_us` non-monotonic and feeds the new autotuner a
negative per-run time; a named run that fails while building its inputs leaks
the ORT values it already created; and the mutation sweep that is supposed to
prove the smoke gate can fail no longer applies to the tree it is run against.
Fixing the defects without closing the gate gaps would leave the next release
with the same blind spots, so each fix here arrives with the gate that catches
its regression.

## What Changes

- **Gate wait is attributed per run, not per session.** The wait a caller paid at
  the CPU admission gate is returned to that caller (the lease that carried it),
  never stashed on a session shared by several callers, and it is summed over
  every text of a request rather than only the last one. `run_us` and the
  per-run derivation the autotuner classifies on SHALL NOT decrease.
- **A failed named run releases what it built.** Every `RunNamed` failure path
  destroys the input values created before it — including a missing, duplicated
  or invalid input discovered partway through assembly — and a session never
  hands out an output tensor it has destroyed.
- **Batched image inference refuses a mismatched output batch** instead of
  slicing past it: an output whose batch dimension disagrees with the request's
  is a request error, never a panic.
- **`emb-top` rejects flag values it cannot render** (`-window` below 1) instead
  of panicking with the alternate screen still occupied.
- **The fleet aggregate carries the advice its headless sibling already
  carries**, so the TUI cannot name capacity where `-once` names a reservable
  fix; `-once` and `-frames` report the same cache attribution for one poll.
- **The smoke gate's machine-readable summary becomes attributable**, so a run in
  which the autotune arm did not execute cannot be reported green, and a check
  reads the arm's own summary rather than any summary in the log.
- **The planted-regression sweep is refreshed against the release tree and
  becomes a required check**; each mutant must name the assertion that caught it.
- **CI gains a race-instrumented pass** over the CGo-dependent suites, with the
  deadline-sensitive scripted fixtures taking the existing test-only
  `WithScriptDeadline` seam instead of running against a wall-clock bound that
  `-race` inflation cannot meet.
- **`model-lifecycle` states what the server actually does** on a cold lazy first
  request (refuse with `not ready`, load single-flight off the request path,
  report per-model status) rather than promising a blocking first request.
- **`EMB.INFO`'s element contract stops hard-coding 22/36 elements** and requires
  the declared count to equal the emitted count; **`MONITOR`'s field list gains
  `attributed`**, which the release already emits.
- **Prose that contradicts the shipped behavior is corrected** (`README.md`,
  `docs/configuration.md` on lazy loading, `docs/scripting.md` on
  `emb.API_VERSION`) with **no spec delta**: the existing `product-docs` and
  `scripting-reference` requirements already forbid the contradiction, so the fix
  is a documentation task, not a new contract.

No wire format changes. The one client-visible behavior change is
`model-lifecycle`'s refusal, which 0.4.3 already ships (the spec, not the code,
is the stale side); rejecting `emb-top -window 0`/negative changes an input that
previously panicked.

## Capabilities

### New Capabilities

None. Every defect lives in behavior a capability already specifies.

### Modified Capabilities

- `model-lifecycle`: "Lazy model loading" — a cold lazy first request is refused
  with a not-ready error and loaded off the request path, instead of blocking.
- `server-stats-observability`: "Per-model dispatch and run timings" — the gate
  wait is attributed to the run that paid it, including every text of a
  multi-text request, and the per-model counters are monotonic.
- `inference-capacity-autotune`: "Per-model traffic classification" — the
  classification reads a per-run measure that cannot go negative, and a
  non-positive run time is not evidence of latency.
- `script-resource-lifecycle`: "No leaks under sustained load or failure" — a
  failure while assembling a run's inputs releases the values already built;
  a new "A reused output tensor is never handed out after destruction"
  requirement covers the session's single-slot output buffer.
- `image-embeddings`: "Batched image inference" — the output's batch dimension
  must agree with the request's, and a disagreement is a request error.
- `emb-top-dashboard`: new "emb-top validates its own flag values" requirement.
- `emb-top-cluster-view`: "The fleet view recommends where capacity is needed" —
  the fleet recommendation carries each node's advice and the two headless
  surfaces agree on one poll's attribution.
- `performance-validation`: "Per-change smoke gate" — the summary is attributable
  per arm and a skipped assertion is never reported as run; "Gate self-verification"
  — the sweep applies to the release tree, is refreshed with the code, and is a
  required check.
- `coverage-gate`: "CI executes the CGo-dependent suites" — a race-instrumented
  pass runs alongside the plain one.
- `emb-info-array-count`: "EMB.INFO returns the correct number of array
  elements" — the contract is declared-equals-emitted with the key groups named,
  not two hard-coded totals.
- `emb-cmds`: "MONITOR exposes recent request events" — the event's field list
  gains `attributed` (11 fields under RESP2, 11 keys under RESP3).

## Impact

- **Code**: `internal/registry` (`cpu_budget.go`, `registry.go`, `sampler.go`),
  `internal/pipeline` (`pool.go`), `internal/permit` (`autotune.go`),
  `internal/onnx` (`named.go`, `runtime.go`), `internal/server` (`image.go`,
  `image.go`'s pooling entry, `server.go` MONITOR/INFO emission),
  `cmd/emb-top` (`fleet.go`, `latency.go`, `main.go`), `internal/embtop`
  (`advice.go`, `once.go`).
- **Tests**: `internal/server/emb_smoke_test.go` (attributable summary),
  `bench/smoke/mutants/*.patch`, `internal/server/laya_parity_test.go` (deadline
  seam), a new monotonicity/attribution test for the counters, a leak test for
  the failed-input path.
- **CI**: `.github/workflows/ci.yml` (`test-cgo` gains the race pass and the
  mutation sweep; the "green must mean it ran" check keys on the arm's summary).
- **Docs**: `README.md`, `docs/configuration.md`, `docs/scripting.md`,
  `docs/operations.md` (only where it contradicts the code), and the specs listed
  above.
- **Compatibility**: no RESP shape change; the `MONITOR` field is additive and
  already emitted; the model-loading refusal and the `emb-top` flag rejection are
  behavior changes for inputs the release already handles that way (refusal) or
  crashes on (negative window).
