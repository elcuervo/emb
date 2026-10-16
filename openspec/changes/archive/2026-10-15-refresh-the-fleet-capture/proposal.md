# Proposal

## Why

`emb-top-course-of-action` changed what the dashboard renders — the fleet view's
sizing line now leads with the named node's action and the phase behind its tail,
the latency band carries the runs' p95 beside the service percentile, the CPU
readout is a share of the node's capacity, and each model row shows its own cache
counters — and re-recorded only the single-node take. The fleet take the
documentation surface plays and the fleet capture in `docs/assets/` are still the
2026-10-14 recording of `v0.4.3.pre5` at 38556b1, so the surface shows
pre-change output beside prose that already documents the new line.

Nothing in the tree fails on it. `published-tree.py` checks the shape of the
served set and `stamp-version.py --check` checks the stamped strings, but a
capture's version is deliberately excluded from the stamped set, and nothing
compares the takes to each other — so a change can re-record one take, leave its
sibling behind, and still pass every gate in CI.

The same change updated `docs/commands.md` and `docs/operations.md` for the new
`MONITOR` attribution fields and the per-model scope of `EMB.INFO`'s cache
counters, while the hosted docs surface still describes the six-field event and
the process-wide cache — from a page that stamps itself as documenting
`0.4.3.pre6`.

## What Changes

- Re-record the fleet take from the current build (`just website-clusterviz`), so
  the fleet plate on the documentation surface and `docs/assets/emb-top-cluster-*.gif`
  show what the fleet view renders now: new artifact names, the previous pair
  deleted, the fleet block of `docs/operations.md` and the `fleetviz` region and
  served-set entry written by the rig itself.
- **New check**: the published takes must agree on the dashboard they recorded.
  Every plate's recorded-run label — the single-node plate on both pages and the
  fleet plate — must name the same `emb-top` version, so a take left behind by a
  change that re-recorded its sibling fails instead of shipping.
- The hosted docs surface names the shipped wire fields: its `EMB.INFO` and
  `MONITOR` rows carry the embedding path's timings and runs, the per-model cache
  scope, and the event's `cached_texts`/`attributed`/`dispatch_us`/`admit_us`/`run_us`
  fields, matching `docs/commands.md`.
- The checked-but-untrue verification note in `emb-top-course-of-action`'s task
  7.2 is corrected to record that only the single-node take was re-recorded, and
  this change carries the fleet one.

## Capabilities

### New Capabilities

None. The check is behavior of the capture surface the repository already has,
not a new capability.

### Modified Capabilities

- `emb-top-capture`: "The take stays in step with the dashboard's readouts"
  widens from *the take* to every published take of the dashboard — the fleet view
  is the same binary's other readout — and a new requirement makes the agreement
  checkable: the recorded-run labels the pages carry must name one `emb-top`
  version.
- `product-site`: "The documentation surface carries the fleet run the way it
  carries the single-node run" gains that the fleet plate must be a take of the
  dashboard the surface's other plate was recorded from, not merely of *a* run;
  its disagreement scenario names the recorded version.
- `product-docs`: "The docs surface is the hosted form of the reference" gains
  that the surface's command and reply reference must name the same fields the
  shipped `docs/commands.md` and `docs/operations.md` name, so a surface row
  cannot describe an older wire shape than the version the surface stamps.

## Impact

- Artifacts replaced by the rig: `website/assets/cast/emb-top-cluster-<sha8>.cast`,
  `docs/assets/emb-top-cluster-<sha8>.gif`, the `clusterviz` block of
  `docs/operations.md`, the `fleetviz` region of `website/docs/index.html`.
- Edited by hand: `website/docs/index.html`'s `EMB.INFO` and `MONITOR` rows,
  `website/tools/published-tree.py` (the agreement check),
  `openspec/changes/emb-top-course-of-action/tasks.md` (the corrected note).
- No server, protocol, dashboard or client behavior changes; no API, config key or
  dependency is added.
- Cost: the fleet rig needs `nix develop`, the four topviz models, and about a
  minute of real load; it cannot run in CI.
- Sequencing: this change's deltas stack on `emb-top-course-of-action`'s, which are
  not yet synced into `openspec/specs/`. Archive that change before this one.
