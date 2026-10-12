# Tasks

## 1. Make the benchmark recipe trustworthy

- [x] 1.1 Give `just bench-redis-single`/`bench-redis-multi` (and the cache/`bench-all` recipes that compose them) a `port` parameter defaulting to 6379, and add a preflight that fails with the owning process named when the port is already held (dial or `lsof`), verified by holding the port with another server and confirming the recipe refuses instead of printing a result.
- [x] 1.2 Add a repeatable comparison helper that starts one arm, runs a cell, stops it, starts the other arm and repeats, so `cpu_budget: auto` and `cpu_budget: off` are measured alternately in one session; verify it prints both arms' samples for a cell and that a re-run of one arm alone reproduces a sample inside the previously observed spread.
- [x] 1.3 Record, per cell, the policy in effect, the layout the server logged (sessions × width) and the run-to-run spread, and verify the recorded `EMB.INFO`/startup-log values match what the same config prints on a fresh start.

## 2. Re-establish the published baselines

- [x] 2.1 Re-measure the single-threaded and multi-threaded sections of `BENCHMARK.md` with the recipe from group 1, and verify the published figure is the median of repeated interleaved runs with its spread stated.
- [x] 2.2 Correct the stale prose in those sections: replace the "Ten ONNX Runtime workers, all CPU cores" description with the layout the named configuration now selects, and verify the section's words, its table and its `reproduce` command all agree with a fresh run of that exact command.
- [x] 2.3 Add an out-of-the-box vs explicitly-tuned comparison for `minilm` (default policy against `cpu_budget: off` and against an explicit width), each arm labelled with its policy and layout, and verify every number in the new table is reproducible by the command printed beside it.
- [x] 2.4 Keep the cache sections and the scripted-inference sections consistent with the re-measured set: verify any figure whose layout the budget changed is either re-measured or marked with the policy it was taken under.

## 3. Documentation surface

- [x] 3.1 Document the process-budget keys in the configuration section — `cpu_budget`, `cpu_reserve`, `calibrate`, model `weight`, their defaults, their precedence against explicit `workers`/`intra_op_threads`, and `cpu_budget: off` restoring the legacy width rule — verified by running each documented value against a throwaway config and checking the startup log matches the documented outcome.
- [x] 3.2 Document the plan fields and the fallback in the operations section — `cpu_allowance`, `cpu_allowance_source`, `cpu_budget`, `cpu_active_tokens`, `cpu_peak_tokens`, each model's selected width, the trial startup cost and the fallback reasons — verified by forcing a fallback (an explicit over-budget width) and checking the reason the docs name is the reason logged.
- [x] 3.3 Add the benchmark subsection for the budget inside the existing benchmarks section (no new top-level section), with the figure or table drawn from the group-2 numbers, captioned with the values it draws, and a sentence stating that the budget bounds admitted intra-op work rather than physical CPU, verified by comparing every captioned value against `BENCHMARK.md`.
- [x] 3.4 State the policy behind each figure and keep the attributions separate — the process budget's own baseline, the capacity controller's out-of-the-box gains, and the neutral/negative effects (including that shared embedding and image sessions do not spin under the automatic policy) — verified by reading the section for any figure that lacks its policy label.

## 4. Landing page

- [x] 4.1 Present the plan on the landing (a capability block alongside the operations cards, or an addition to the existing machine card — either satisfies the spec): detected allowance and source, admitted budget, selected sessions × width, and the fallback when nothing validates, verified by starting the documented server and checking every stated value against its log line.
- [x] 4.2 Update the hero measurement: name the policy, the machine and the corpus, and state the spread from group 2 rather than a single figure; verify the `reproduce` link lands on the section that carries the same policy label.
- [x] 4.3 Verify the landing still satisfies the composition rules — the new material uses the poster's existing atoms (no new hue, gradient, radius or shadow), text ink stays inside the sheet at every supported width, and the page renders with JavaScript disabled — using `just website-ink` and a script-disabled load.

## 5. Re-record the emb-top plate and capture

- [ ] 5.1 Re-run the take with `just website-topviz` against a node running the default CPU policy so the replayed frames show the plan line, and verify the committed plate and the documentation capture both show it and their names changed with their bytes.
- [ ] 5.2 Verify the plate's figures still come from that run's own sample log, and that the recorded run's date, commit and node configuration are recorded with it.

## 6. Live sandbox

- [ ] 6.1 Redeploy the sandbox by hand (`just sandbox-deploy`) and verify the live view relays frames containing the plan line, and that its standalone terminal still links to it.
- [ ] 6.2 Verify the sandbox's own reported plan is what the panel shows (a small VM allowance is expected) and that no surrounding copy promises a larger budget than the live panel reports.

## 7. Integration checks

- [x] 7.1 Run the site's own gates together — `just published-tree`, `just website-presets-check`, `just website-version-check`, `just website-ink` on both surfaces — and verify each passes with the re-recorded assets. (`website-published`, `website-presets-check`, `website-version-check`, `website-examples-check` pass; `website-ink` PASS at 24 widths on the landing and docs. The plate itself was NOT re-recorded — see 5.1.)
- [x] 7.2 Re-read the two surfaces for stale claims: verify no page describes a layout, a budget or a policy that the current default no longer produces, and that every number still traces to a recorded run or a named recipe.

## Workflow follow-up

- Archive the change once the site is live and the checks pass.
- If the default spinning policy is revisited, that is a separate change against `inference-cpu-budget`, and the claims added here carry the policy label it would need to update.
