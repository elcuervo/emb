# Proposal

## Why

The traffic-aware autotune change (`traffic-aware-autotune`) shipped per-model
traffic classification, a runtime concurrency controller, capacity profiles,
cgroup-aware sizing, and observability fields into the server, and documented
them in `docs/configuration.md` and `docs/operations.md`. The **site**
(`website/docs/index.html`) does not mention any of it: a reader who arrives
from the landing page finds no `capacity` profile table, no `autotune` switch,
no autotune observability fields, and no benchmark of the adaptive path. The
site is the surface the change was never carried to, so the hosted reference
undercuts the shipped capability.

The measurement work also surfaced facts the site cannot state today because
`BENCHMARK.md` does not carry them: the out-of-the-box scripted path runs
**2.0× the burst throughput and 2.0× better burst p50** of the pre-change
binary, at unchanged serial latency, and the controller's adaptation is
observable per second. Those numbers need a home in `BENCHMARK.md` before the
site may show them (the site's provenance rule), and once they do, they are
worth drawing rather than tabulating.

## What Changes

- **Record the adaptive-capacity measurements in `BENCHMARK.md`.** Add a
  section carrying the out-of-the-box comparison (pre-change binary vs HEAD,
  every knob unset), the per-second autotune shape trace, and the capacity
  profile tradeoff, each with its machine, corpus, and reproduction command.
- **Fold the capability into the docs surface**, without renumbering sections:
  - `04 · Configuration` — add `capacity` and `autotune` ledger rows and the
    creation-time profile choice.
  - `06 · Operations` — add the autotune observability fields
    (`script_traffic_class`, `script_inflight`,
    `script_concurrency_current`/`_target`, `script_autotune_active`), the
    CPU-saturation recommendation, and the `autotune: off` kill switch.
  - `07 · Benchmarks` — add an "Adaptive capacity" subsection carrying the
    measured figures and the reproduction commands.
- **Draw the capability as three on-brand SVG plates**, built from the poster's
  atoms (ruled graticule, mono caps labels, small filled circles, exactly one
  orange signal), each captioned with its real figures:
  1. the shape trace — class spans, in-flight area, and the concurrency
     allowance as the single orange step line against the cap;
  2. out-of-the-box — pre-change vs HEAD as a dumbbell per metric, with the two
     ×2.0 improvements and the serial-parity row shown honestly;
  3. the capacity profiles — serial p50 vs burst req/s, so the operator sees
     `auto` at the throughput corner and `latency` as a deliberate trade.
- **Fix the two precedence gaps the PR #48 review found, so the documented
  precedence is true.** An explicit `allow_spinning` SHALL win over either
  capacity profile (the profiles currently overwrite it), and
  `capacity: throughput` SHALL NOT force the controller on past
  `autotune: off` (it currently does). These are the server-side guardrails the
  docs would otherwise have to word around.
- **State the guardrails beside the figures**: replies byte-identical under the
  interleaved A/B harness, the Ruby client suite green against the same server,
  `EMB.READY` answering, and the controller refusing to grow under CPU
  saturation.
- **Do not over-claim.** The site says what the trace shows: the controller
  classifies traffic and moves the allowance within its cap, and the
  out-of-the-box 2.0× belongs to the adaptive-capacity line as a whole, not to
  the controller alone (measurement shows the controller is neutral against the
  static default it replaced).

Not **BREAKING**: the site and `BENCHMARK.md` change; the server, wire protocol,
configuration keys, and returned embeddings do not.

## Capabilities

### New Capabilities
<!-- None: this change alters what an existing surface must carry, not a new behavior. -->

### Modified Capabilities

- `product-docs`: the hosted reference must carry the autotune capability — the
  `capacity`/`autotune` configuration keys, the autotune observability fields,
  an adaptive-capacity benchmark subsection, and the measured figures with
  their `BENCHMARK.md` provenance — inside the existing section order rather
  than a new section.
- `script-inference-performance`: an explicit `allow_spinning` and an explicit
  `autotune: off` must win over a `capacity` profile, so the configuration
  precedence the docs state is the behavior the server has.

## Impact

- **Surfaces:** `website/docs/index.html` (sections 04, 06, 07 and their
  in-page anchors); the docs surface brief under `website/.impeccable/surfaces/`
  gains an amendment recording the plates and the fold-in decision.
- **Reference:** `BENCHMARK.md` gains the adaptive-capacity measurements and
  their reproduction commands; it remains the source of truth the site cites.
- **Code:** `internal/registry/registry.go` profile selection keeps an explicit
  `allow_spinning` and an explicit `autotune: off`; covered by registry tests.
  This is the only server behavior the change touches.
- **Assets:** three inline SVG plates in the docs surface, composed only from
  existing `styles.css` tokens; no new font, colour, or dependency.
- **Unchanged:** the landing page, `demos/`, `gem/`, the sandbox, the wire
  protocol, the returned embeddings, and every configuration key's meaning
  except the two corrected precedence cases.
