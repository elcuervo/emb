# Design

## Context

The measurement harnesses exist but were built for a quieter assumption than
they now face. `just bench-redis-single`/`-multi` hardcode port 6379, start the
server, run one `redis-benchmark` invocation, and print the result; BENCHMARK.md
takes that one number per cell. On a machine where another `emb` already owns
6379, the recipe silently measures that process instead (observed: `rps=0.0`,
`ERR model 'minilm' not found`). And on this host the same binary and config
produced 1463 and 2092 req/s in back-to-back 2000-request runs, so a single
sample is not a publishable figure.

The site's surfaces are already constrained in ways this change must respect.
`product-site` requires every claim to trace to README/BENCHMARK and every
capability figure to be real output rather than a drawing; the `emb-top` plate
is a replay of a recorded run produced by `just website-topviz`, and
`published-tree.py`, `stamp-presets.py` and the `website-ink` width probe gate
what may be published. The documentation surface has a fixed section order and
must not gain a new top-level section.

What already exists in code: `emb-top` renders the plan (node line, cluster sum,
headless fields) with tests, and the sandbox's live view relays one
`emb-top -frames` producer per sandbox, so the plan reaches the live console
without frontend work.

## Goals / Non-Goals

**Goals**

- Make the reproduction recipe trustworthy on a shared machine: an explicit,
  verified port, and a figure that is the median of repeated interleaved runs
  with its spread, rather than one sample.
- Present the process CPU budget and the startup calibration on both site
  surfaces using the site's existing atoms, inside the existing section order.
- Keep every rendered figure traceable to a recorded run or a named recipe, and
  keep the `emb-top` plate in step with the dashboard's readouts.

**Non-Goals**

- Changing the default spinning policy, the calibration algorithm, or any
  server behaviour. See proposal.md — Why for the evidence and the deferral.
- Publishing the fixed-concurrency `evalbench` two-model numbers: that harness is
  not the published driver, and documenting it is separate work.
- Re-measuring on ARM64/Fargate (optional deployment evidence, per the archived
  `cross-model-cpu-budget` change).
- A new demo plate. The showcase fits the landing's capability block and the
  documentation's existing sections; a gallery plate is a larger piece of design
  work with its own staging mechanism and can follow if the card is not enough.

## Decisions

### Measure with an explicit port and a preflight

The recipe takes the port as a parameter and refuses to start when the port is
already owned (`lsof`/dial check), naming the process it found. Rationale: the
failure mode is silent and produces plausible-looking numbers (a foreign server
answering `PONG`). Alternative considered: keep 6379 and document "stop your
other server first" — rejected because it depends on the reader noticing, which
is exactly what failed here.

### Publish p50 with an rps median and spread

p50 moved 7.1–7.8 ms across repetitions while rps moved 30–45%, so the latency
figure is the publishable one and the throughput figure must carry its spread.
Alternative considered: publish rps only, best-of-N — rejected as presenting the
most optimistic sample as the typical one, which the site's own
"must not present a measurement it did not take" rule already discourages.

### Compare policies in one session, interleaved

An A/B of `cpu_budget: auto` against `cpu_budget: off` runs the arms alternately
in a single session, because host state drifts over minutes. This is how the
spinning observation was made trustworthy enough to quote (median 2055 vs 2238
at c=16) and how the gate was cleared as free (2085 vs 2088).

### State the spinning behaviour, do not change it

The automatic policy disables ORT spinning when the key is unset, which measured
~9% slower at the documented c=16 point and slightly faster at c=64. The site
states this as a documented consequence under the automatic policy; changing the
default is a product decision and belongs to its own change against
`inference-cpu-budget`. Alternative considered: flip the default in this change —
rejected as scope creep into server behaviour and as unverified at c=64.

### Put the showcase in the existing atoms and the existing order

The landing uses the capability vocabulary it already has (a block alongside the
operations cards), and the documentation extends the configuration, operations
and benchmarks sections it already owns. Every number in a figure is captioned
with the value it draws and traces to BENCHMARK.md, following the
adaptive-capacity precedent.

### Re-record the take rather than draw a new plate

`just website-topviz` already produces the plate and the documented capture from
one real run; the take re-runs against a node under the default policy so the
plan appears in the replayed frames. Alternative considered: a hand-built SVG of
the plan line — rejected: `product-site` requires the `emb-top` presentation to
be the dashboard's real output.

## Risks / Trade-offs

- **Noisy host yields a bad baseline** → the recipe's repeated, interleaved runs
  and its stated spread make the noise visible; a headline figure is only taken
  on a quiet host, and the spread is quoted when it is not.
- **Stale numbers return as the default policy evolves** → BENCHMARK.md sections
  must state the policy and layout they were measured under, so a future default
  change visibly invalidates a section instead of silently dating it.
- **Live sandbox shows a small budget** → the sandbox runs on a small VM, so the
  plan line will read its real (small) allowance. That is honest and is why the
  view is a relay of real frames; the surrounding copy must not promise a large
  budget on the live panel.
- **Re-recording churns hashed assets** → the plate and capture are content-named
  already (`A published artifact's name changes when its bytes change`), so the
  churn is expected; `published-tree.py` and the stamp checks must be re-run in
  the same change.
- **Documentation surface grows past its order** → the process-budget material
  goes into the existing configuration, operations and benchmarks sections; the
  modified requirement keeps "no new top-level section".

## Open Questions

- Which landing block carries the budget claim: a fifth capability card, or an
  addition to the existing "what it does to the machine" card? Both fit the
  vocabulary; the choice is a design judgement for the implementation pass and
  does not change the specs or the tasks.
- Whether the documentation's budget figure is a plate like the adaptive-capacity
  one or a table of the measured plan/layout pairs. Both satisfy the figure
  requirement (drawn from BENCHMARK.md, captioned with its values).
