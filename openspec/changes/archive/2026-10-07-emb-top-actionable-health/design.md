# Design

## Context

See `proposal.md` — Why. The health verdict lives in `cmd/emb-top/health.go`:
`healthInput` gathers raw signals, `health()` turns them into chips and takes
the worst chip's level as the verdict. Cache hit rate is currently one of the
chips that can reach `healthCritical`. The cache already has its own gauge and
hit/miss/eviction readouts elsewhere on the dashboard, so the banner does not
need to carry the verdict for it.

## Goals / Non-Goals

**Goals:**
- The banner verdict reflects only signals an operator can act on.
- The cache hit rate stays visible as a banner chip.
- The intent is structural, so a non-actionable signal cannot silently drive
  the verdict again.

**Non-Goals:**
- Changing the cache gauge or per-model cache columns.
- Per-model health (never used cache).
- Adding or tuning thresholds for the driving signals.

## Decisions

### Signals carry a `driving` flag

Add a `driving bool` to `signal` and skip non-drivers when folding the worst
level. The cache chip is appended with `driving: false` and a neutral level
(no severity thresholds), so it always renders calmly regardless of value.

- *Alternative — neutralize the chip only*: drop the cache thresholds so the
  chip is always `healthy` and the existing worst-fold skips it. Smaller diff,
  but the reason cache cannot drive is implicit; a later edit could reintroduce
  thresholds and silently restore the bug. The flag states the invariant that
  caused the problem.
- *Alternative — relative cache baseline (EMA like p95)*: could flag a genuine
  cache regression. Rejected: hit rate is a workload property, absolute or
  relative, and the operator cannot act on it as node health.

### Fold the verdict, then append informational chips

`health()` computes the worst level over driving chips only, then appends the
informational chip afterward, preserving the current chip order
(connection, error, p95, cpu, cache) so the fixed-width layout is unchanged.

## Risks / Trade-offs

- **A real cache failure (e.g. cache silently disabled) no longer escalates the
  banner** → Mitigation: it is still visible in the cache chip and the cache
  gauge; this is a deliberate trade, since low hit rate is not node ill-health.
- **Removing the chip's severity could be read as "0% is fine"** → Mitigation:
  the gauge and hit/miss rates remain the place to judge cache effectiveness.
- **Spec/implementation drift on the stale "active requests" chip name** →
  corrected in the spec delta in this change; not added to the code.
