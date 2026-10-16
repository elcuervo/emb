# Design

## Context

See `proposal.md` for motivation. The constraints that shape the approach:

- Every finding lives on a path the release's own gates exercise but cannot see.
  The smoke gate asserts attribution by *total* (`admission-wait-reported`,
  `queue-bounded`), never by monotonicity, and CI reads any
  `emb-smoke: assertions=` line rather than the autotune arm's line. No workflow
  runs `-race`, so the shared-session path is only ever exercised without
  instrumentation. The mutation sweep that proves the gate can fail is not
  wired into CI at all, and one of its patches no longer applies to `HEAD`.
- The admission wait is currently carried on the *session*
  (`budgetedNamedSession.pending`, `budgetedSession.pending`), while a session is
  deliberately leased to `script_callers_per_session` callers (default 4) and
  `permit.Pool` hands the same item to several of them. One slot, many payers.
- The embedding pool takes the wait **once**, after `process()` has already run
  one inference per text (`internal/pipeline/pool.go`), so a multi-text request
  attributes only its last text. The batcher and the pool clamp a negative run
  time to zero; `ScriptResources.RunNamed` does not.
- ONNX value ownership in `onnx.RunNamed` is a `defer` registered *after* the
  input loop, so an early return inside the loop abandons the values already
  created. The session's reused output tensor is destroyed before the
  replacement is created, with the field left pointing at the destroyed value
  when creation fails.
- `EMB.INFO`'s element count grew to 90/104 elements while
  `openspec/specs/emb-info-array-count` and `docs/scripting.md` still assert
  22/36 and a "1.x" script API version; `MONITOR` gained an eleventh field
  (`attributed`) that `openspec/specs/emb-cmds` does not list.
- `openspec/specs/model-lifecycle` still promises a first request that blocks
  until the model is loaded, while 0.4.3 refuses it (measured refusal window
  5.0–5.6 s for `all-MiniLM-L6-v2` on this host) and loads off the request path.
  `docs/operations.md` describes the shipped behavior; `README.md` and
  `docs/configuration.md` do not.

## Goals / Non-Goals

**Goals:**

- The gate wait travels with the run that paid it, so per-model counters are
  monotonic and the autotuner's input cannot invert.
- Every failure path of a named run releases what it built, and no session hands
  out a tensor it destroyed.
- The release cannot ship a green gate whose evidence did not execute: the
  summaries are attributable, the mutation sweep applies to the tree under test
  and runs on the pull request, and a race-instrumented pass exists.
- The specs and the docs say what the shipped code does.

**Non-Goals:**

- Measuring the leaked value's exact byte cost. The defect is a fixed-size leak
  per failed run; the fix removes it without needing the number, and a 20 000-run
  RSS sample could not separate it from allocator churn (~0.13 kB/call on both
  the leaking and the non-leaking path).
- Changing `EMB.MULTI`'s null-on-not-ready element, or making the Ruby client
  retry a not-ready refusal. The first is already specified as null-on-failure;
  the second is a client-side policy decision, not a server contract.
- The pre-existing `registry.go` divide-by-zero on a 0-byte model, the dropped
  `os.Remove` error in `hfhub`, and `fleet.go`'s constant `(resolve failed)`
  string. Noted, unfixed here.
- Adding a second guard to the autotuner (for example ignoring windows whose
  counters barely moved). The spec now forbids the minimum that made it
  necessary; anything more is speculative.

## Decisions

**1. Return the wait from the lease, not from the session.**
The wrappers (`budgetedNamedSession`, `budgetedSession`) stop storing the wait on
the session and hand it back to the caller on the path the result already
travels, so the caller that paid records it. *Alternatives:* clamping the
subtraction to zero (rejected: it hides a mis-attributed wait instead of
attributing it, and leaves the autotuner reading a number no run produced);
`context.WithValue` (rejected: allocation on the request path, and the scripted
host API carries no context). The `run < 0` clamp stays as a backstop at the two
call sites that sum it, matching `pipeline/pool.go` and `pipeline/batcher.go`.

**2. Sum per run, not per request.**
`Worker.process` accumulates the wait inside its per-text loop, so a multi-text
request attributes every claim it paid. The counter update stays at the call site
that knows the shape rather than inside the session.

**3. Monotonicity becomes a tested invariant.** A test drives concurrent callers
onto one shared session with gated fakes (the `batcher_budget_test.go` pattern —
channels, no `time.Sleep`) and asserts `run_us`/`admission_wait_us` never
decrease and that each caller's wait lands in its own total.

**4. Destroy-by-default in `RunNamed`.** The entry-destroying `defer` is
registered before the first value is created, so every early return unwinds it —
including the input-assembly failures the spec now names. The session's reused
output tensor is cleared before its replacement is created, so a failed
allocation leaves the session re-creating rather than handing out freed memory.

**5. Refuse a mismatched image batch explicitly.** The pooling entry points
compare the output's batch dimension with the request's and return an error
naming both. *Alternative:* a `recover()` in the RESP handler (rejected: it
converts a configuration error into a silent failure and would hide the next
one too).

**6. Validate flags in one place, before the screen is taken.** `emb-top`'s
`parseFlags` owns the check (`-window >= 1`, alongside the existing `-interval`
and `-samples` checks), so the TUI, `-once` and `-frames` inherit it and no mode
can panic with the alternate screen active.

**7. One builder for the fleet and the headless views.** The aggregate's record
input is built by the same helper `once.go` uses (including the node's advice)
instead of a second, narrower builder, and `-once`'s cache share comes from the
same window function the panel renders. *Alternative:* pass the advice at the one
call site (rejected: the two builders are the reason they diverged).

**8. The smoke gate emits one summary per arm, naming the arm.** CI's "a green
run must mean it ran" check keys on the autotune arm's own summary and fails when
that arm produced none; a host-dependent skip stays a reported skip (the spec
blesses it) and is listed, not turned into a pass. *Alternative:* fail on any
skip (rejected: on a host that cannot reach the saturation threshold the gate
would never be green, which would delete the assertion instead of reporting it).

**9. The mutant sweep is refreshed and required.** Each patch is regenerated
against `HEAD`, the sweep fails when a patch no longer applies, and a job runs it
on changes to the gate, the admission path, the autotuner or the scripted
runtime. Cost measured on this host: ~11 min for the nine mutants plus the clean
run, so it is path-filtered like the sandbox job rather than added to every PR.

**10. `-race` joins the CGo job, with the deadline seam.** The instrumented pass
covers `server`, `registry`, `pipeline`, `onnx`, `script` and `permit`; the two
laya episode tests pass `WithScriptDeadline` (the existing test-only seam, already
on the deadcode allow-list) rather than raising `DefaultDeadline`, which is
production behavior. Measured here: ~229 s for the race subset, so the job's
wall-clock budget is raised with it.

**11. No spec delta for the docs prose.** `product-docs` already requires that the
docs surface not contradict `README.md` or the shipped code, and
`scripting-reference` already requires the version to be derived rather than
typed. The stale sentences are task-level corrections, so the change carries them
as tasks instead of inventing requirements to restate an existing obligation.

## Risks / Trade-offs

- [Widening the admission-aware session interface touches every session call site
  — pool, batcher, script, image, calibration] → the change is confined to
  `internal/registry` and `internal/pipeline`; the smoke gate's attribution
  assertions, the new monotonicity test, and `just test` cover the call sites.
- [`-race` and the mutant sweep lengthen CI] → the race pass is scoped to the
  packages that own the defect, and the sweep is path-filtered rather than run on
  every pull request. Both budgets are measured, not guessed.
- [Blessing the lazy-load refusal in `model-lifecycle` makes a cold client's first
  requests fail by contract] → `docs/operations.md` already states the retry
  contract and `preload: true` for a model on a serving path; this change makes
  the other documents agree instead of promising a blocking load.
- [The accounting fix could shift `admission_wait_us` totals that existing
  assertions bound] → the smoke gate asserts a p99 bound and a
  reported-vs-unreported distinction, not a total; if a bound tightens, that is
  the gate working as intended.
- [A refreshed mutant may stop detecting its regression] → the sweep's redness
  criterion requires the named assertion to fail, so a mutant that no longer
  catches anything fails the sweep rather than passing quietly.

## Migration Plan

No data or wire-format migration. The behavior changes are the ones 0.4.3 already
ships (the not-ready refusal) or rejects an input that previously panicked
(`emb-top -window < 1`). Rollback is a revert of the change: the spec deltas
re-add the previous requirement text, and no released artifact depends on the
new counters' values.

## Open Questions

- Whether the race pass belongs inside `test-cgo` or as a sibling job that reuses
  the fixture cache. Either satisfies `coverage-gate`; the choice is a CI-cost
  question that does not change the specs or the tasks.
