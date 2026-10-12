# Proposal

## Why

The site's performance claims no longer match what the server does by default, and
they say nothing about the two things it now decides for itself: the process CPU
budget and the startup calibration.

`config.yaml` carries no tuning keys, so it runs under `cpu_budget: auto` +
`calibrate: true` + default batching — one batcher session at a calibrated width,
verified in this session's startup log (`workers=1 intra_op_threads=8`). But
`BENCHMARK.md` still documents the same file as "Ten ONNX Runtime workers, all CPU
cores", and the landing hero cites `2010.05 req/s` from that section with a
`reproduce` link. The site's own rules are therefore broken right now: capability
claims must trace to README/BENCHMARK, and the site must not present a measurement
it did not take.

The same session's measurements show why the numbers themselves cannot simply be
pasted in. On this host, same binary and same config, back to back:

| run | default (`auto`) | `allow_spinning: true` | `cpu_budget: off` |
|---|---|---|---|
| c=16, 2000 requests | 1463 → 2092 | 2242 → 2247 | 2215 |
| c=16 again | 1426 → 1996 | 2101 → 2174 | 2088 |

A 30–45% spread between repetitions of one configuration, while p50 moved only
7.1–7.8 ms. Publishing any single number from this machine would publish noise.
Two results did hold up under interleaving and are worth the site's attention:
the admission gate costs nothing measurable (2085 vs 2088 at c=16; 2083 vs 2083
twice), and forcing `allow_spinning: false` — which the budget policy does when
the key is unset — measured ~9% slower at the documented c=16 point (median 2055
vs 2238; p50 7.6 ms vs 7.1 ms) while being slightly faster at c=64.

The showcase itself is cheap, because the plumbing already exists: `emb-top` now
renders the plan, and the sandbox's live view is a relay of one `emb-top -frames`
producer per sandbox, so the plan reaches the live console with no frontend work
— only a hand-deployed sandbox and a re-recorded plate.

## What changes

- **Landing**: the hero measurement and one capability card (or section) present
  the process CPU budget and the startup calibration — the detected allowance, the
  admitted budget, the selected layout, and the fallback when nothing validates —
  each traceable to a recorded run or a named, reproducible recipe, with the
  measured spread stated rather than a single pristine number.
- **Documentation surface**: a process-budget section (allowance, reserve, budget,
  weight, `calibrate`, precedence, selected layout, fallback reasons), and the
  benchmarks section corrected so its prose and its `reproduce` command match the
  configuration it names. The existing adaptive-capacity attribution stays scoped
  to the script capacity controller.
- **`emb-top` dashboard**: the process CPU plan becomes a specified readout — the
  node dashboard's plan line, the cluster's summed line, the per-node sections in
  `-once`/`-frames`, and a blank line (never `off`) for a server that reports no
  plan.
- **The `emb-top` plate and documented capture are re-recorded** so they show the
  plan, using the existing single-command take.
- **Benchmark methodology**: the reproduce recipe becomes safe to run on a shared
  machine (explicit port and a preflight so another server cannot be measured by
  accident), and published results state the policy, the effective layout, and the
  run-to-run spread.
- **Live sandbox**: redeployed so the plan appears in the live view. No frontend
  requirement changes; the view is already specified as a relay of the dashboard's
  frames.

Out of scope, and deliberately so:

- **Changing the default spinning policy.** The ~9% at c=16 is real but
  workload-shaped (the reverse held at c=64), and it is a product-behaviour
  decision, not a website one. It belongs in its own change, against
  `inference-cpu-budget`.
- Joint multi-model calibration (still future work in the archived change), and
  ARM64/Fargate re-measurement (optional deployment evidence).
- Any new claim about the two-model improvements measured earlier in this session
  (`evalbench` fixed-concurrency, unique texts): that harness is not the published
  one, so those numbers stay out of the site until the recipe is documented.

## Capabilities

### New Capabilities

None. The site is an existing capability set, and the dashboard and benchmark
requirements this change needs already have owners.

### Modified Capabilities

- `product-site`: the landing presents the process CPU budget and the startup
  calibration, with every figure traceable to a recorded run or a named recipe.
- `product-docs`: the docs surface carries the process budget and the selected
  layout, and its benchmarks section stops describing a layout the documented
  configuration no longer produces.
- `emb-top-dashboard`: the process CPU plan is a specified readout of the node
  dashboard, the cluster aggregate, and the headless modes.
- `emb-top-capture`: the take stays in step with the dashboard's readouts, so the
  plate and the documented capture are re-recorded when a readout is added.
- `redis-benchmark-metrics`: the reproduction recipe is safe on a shared machine,
  and published results name the policy, the layout, and the measured spread.

## Impact

- **Site source**: `website/index.html` (hero + capability block),
  `website/docs/index.html` (budget section + benchmarks section), the shared
  `website/assets/js/topviz.js` plate, `website/assets/css/styles.css` for any new
  atom, and `website/README.md` for the recording and check notes.
- **Site tooling**: `website/tools/topviz/` re-records the plate and capture under
  the existing single-command take; `website/tools/stamp-*` and
  `published-tree.py` govern the hashes and the published tree.
- **Benchmarks**: `BENCHMARK.md` prose and tables, and the `just bench-redis-*`
  recipes for the port and preflight.
- **Sandbox**: `website/repl/` redeployed by hand (`just sandbox-deploy`) — CI
  never deploys it, so the live view must be verified after the deploy.
- **Already implemented, specs catching up**: `cmd/emb-top` and
  `internal/embtop` render the plan today (with tests); this change gives that
  behaviour a requirement rather than adding code.
- **Constraints**: every rendered number must come from a recorded run
  (`website/tools/topviz/runs/`) or a documented recipe; existing site checks
  (`just website-ink`, `published-tree.py`, `website-presets-check`,
  `website-topviz`) must keep passing; no new colour, font or device family.
