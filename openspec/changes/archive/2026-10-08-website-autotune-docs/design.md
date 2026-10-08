# Design

## Context

See `proposal.md — Why`. Three constraints shape this work:

- **No build step.** `product-docs` requires the docs surface to render over
  `file://` with no bundler, no generator, and no third-party request. Any
  figure must ship as committed markup.
- **One source of truth for numbers.** `product-docs` requires a documented
  number to appear in `BENCHMARK.md` with its machine and method, and forbids
  the surface contradicting the reference. So the measurements are recorded in
  `BENCHMARK.md` first and the site cites them.
- **A settled section order.** `product-docs` fixes the section order
  (Configuration → Lua API → Operations → Benchmarks). The autotune material
  already has natural homes inside it, so no renumbering is warranted.

The autotune capability is already documented in the repository (`docs/configuration.md`,
`docs/operations.md`); this change carries it to the hosted surface and adds the
measured proof, which neither document carries.

## Goals / Non-Goals

**Goals:**

- Carry the `capacity`/`autotune` keys, the observable autotune fields, and the
  adaptive-capacity benchmark into the docs surface without touching the order.
- Record the adaptive-capacity measurements in `BENCHMARK.md` with exact
  reproduction commands, so every figure on the site has provenance.
- Draw the capability and the improvement as SVG plates built from the poster's
  atoms and captioned with their real figures.
- State the guardrails beside the figures.

**Non-Goals:**

- A new top-level docs section. The fold-in is the decision.
- A chart library, canvas, or any runtime drawing.
- Rewriting `docs/configuration.md` or `docs/operations.md`; they already carry
  the capability and are the site's reference.
- Changing anything else in the server beyond the two precedence lines: the wire
  protocol, the returned embeddings, and every other configuration meaning are
  untouched.

## Decisions

**1. Fold into sections 04, 06, 07 rather than add a section.**
The `product-docs` section order is a requirement, and the capability already
splits cleanly: keys belong to Configuration, observable state to Operations,
numbers to Benchmarks. A new numbered section would renumber six sections, need
a surface-brief amendment, and put a key reference away from the other keys.
*Alternative:* a dedicated `05 · Adaptive capacity` section — rejected; it
duplicates the config and ops rows and breaks the anchor order the landing
deep-links to.

**2. Hand-author the plates as inline SVG carrying the figures.**
`product-docs` forbids a build step, and `BENCHMARK.md` is the provenance. A
generator plus a `--check` gate would be more machinery than three static
figures merit while the numbers are stable.
*Alternative:* a `website/tools/` stamp like `stamp-presets.py` — deferred; add
it if `BENCHMARK.md`'s adaptive-capacity table starts churning.

**3. Record the measurements before drawing them.**
`BENCHMARK.md` gains a section carrying three tables, all reproduced on the
reference host (Apple M4, 10 CPU, 24 GB, macOS 26.6.2) with GLiNER2
`model_int8.onnx`, `bench/script/gliner-corpus.txt`, reply cache off:

*Out of the box* (`script_workers` and every other knob unset), two independent
cycles, `python3 /tmp/trace.py --workers 0 --phases "1:15,8:15,1:15,8:15,1:15"`:

| metric | pre-change (`c779fe5`) | candidate | |
|---|---|---|---|
| burst req/s (c=8) | 69.0 / 67.8 | 137.4 / 135.4 | **2.0×** |
| burst p50 | 115.8 / 117.3 ms | 58.2 / 59.2 ms | **2.0×** |
| burst p99 | 121.8 / 137.7 ms | 78.8 / 82.8 ms | ~1.6× |
| serial p50 | 15.8 / 15.6 ms | 16.1 / 15.7 ms | parity |

*Shape trace* (`script_workers: 4`, `SHAPE=mixed`, per-second `EMB.INFO`):
allowance 4 → 2 after ~10 sustained `latency` windows; the first burst window
classifies `throughput` and expands to the cap in one tick; `saturated` then
holds at the cap (`inflight` 8, CPU ≥ 0.90) and logs one provisioning
recommendation; the cycle repeats.

*Controller and profiles* (`script_workers: 4`):

| arm | serial p50 | burst req/s | burst p99 |
|---|---|---|---|
| `autotune: off` (static cap) | 21.25 ms | 150.5 | 96.2 ms |
| `capacity: auto` | 21.29 ms | 149.7 | 102.6 ms |
| `capacity: throughput` | 21.3 ms | 151.3 | 94.5 ms |
| `capacity: latency` | 20.2 ms | 119.1 | 79.9 ms |

Also recorded: the interleaved A/B harness's `OK: replies identical` line, and
the Ruby suite result (203 examples, 0 failures) against the candidate server.

**4. Attribution is part of the design, not a footnote.**
The out-of-the-box ×2 belongs to the adaptive-capacity line as a whole; the
static-shared default it shipped with was already the measured best, so the
controller's own effect is parity. The site says this plainly, and the figure
shows the parity row rather than hiding it.

**5. Fix the precedence before wording it.**
The PR #48 review found the profiles override configured values:

```go
// internal/registry/registry.go — profile selection
case "latency":
    spinning = true          // overwrites explicit allow_spinning: false
case "throughput":
    spinning = false         // overwrites explicit allow_spinning: true
    autotune = true          // overwrites explicit autotune: off
```

The fix keeps the profile as a default and lets the operator win: apply the
profile's spinning only when `cfg.AllowSpinning == nil`, and drop the
`autotune = true` assignment so `cfg.AutotuneEnabled()` flows through
(`latency` still pins the controller off, which is its defined layout). Docs
that state precedence are then true rather than aspirational.
*Alternative:* word the docs around the bug — rejected; it documents a bug as a
feature and leaves the kill switch broken.

**6. Plate composition (one accent each).**
Each plate is a ruled graticule, mono caps labels placed on their own spans, and
small filled marks. Exactly one orange signal per plate:
1. **Shape** — `concurrency_current` as the orange step line, `inflight` as a
   faint ink area, the cap as a dashed hairline, class spans named in mono caps.
2. **Out of the box** — dumbbell per metric; the improvement runs in ink, the
   parity row unaccented.
3. **Profiles** — scatter with the `latency`, `auto`, `throughput` points named;
   the `auto` point carries the accent.

## Risks / Trade-offs

- **Figures drift from `BENCHMARK.md`** → the plate's numbers are transcribed
  from the recorded table in the same commit; any future `BENCHMARK.md` edit to
  that table must re-open the figures.
- **Over-claiming the controller** → the `product-docs` scoping requirement makes
  the attribution a contract, and the parity row is drawn.
- **A plate breaks the mobile type floor or contrast** → plates use `viewBox`
  scaling and carry no text below the committed floor; the detector runs once
  over the changed files and is compared against `HEAD`, and every plate colour
  is checked against its own ground.
- **The CodeRabbit precedence gap makes the docs wrong** → resolved by Decision 5:
  the fix lands in task group 2 before the configuration section is written, and
  the registry tests pin it.
- **A new visual language sneaks into the plates** → the `product-docs` delta
  requires the poster's existing atoms and a single accent; a plate that adds a
  second hue or a gradient is rejected at review.

## Open Questions

None. The precedence fix is folded into this change (Decision 5), so the docs
state the intended precedence directly.
