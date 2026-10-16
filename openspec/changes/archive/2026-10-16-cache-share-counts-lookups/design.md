# Design

## Context

See `proposal.md` for motivation. What shapes the approach:

- The number has two candidate meanings and only one of them can be a
  percentage. `cached lookups / runs` is "how much work the cache removed" and
  is unbounded; `answered / (answered + missed)` is the window's own hit rate and
  is a share. The readout renders it as `served N% from cache` beside the
  banner's cache chip, so it has to be the share: a reader cannot tell an
  unbounded ratio from a share once it is printed with a percent sign. The
  "work removed" figure is not lost — it is `cache_hits` against `total_requests`,
  which both the panel and the headless line already carry as raw counters.
- The denominator was not a mistake in arithmetic but a mistake about a
  counter: `total_requests` is `Σ per-model pool.Requests()`, and a cache
  answered request never reaches a worker, so it is the runs. `advice.go`'s
  eviction rule carried the same belief in a comment ("a hit is a request too,
  so the requests served are the lookups") and divided by the model's runs, which
  is why the fix reaches `ModelCachedShare` and `cacheUndersized` too: the
  sentence those feed prints both percentages side by side.
- The cache's own counters are the only pair in the same unit. `EMB.STATS` and
  `EMB.INFO` both publish `cache_hits` and `cache_misses` per node and per model
  (`internal/server/info.go`), and `Point`/`ModelPoint` already carry both for
  the node — the model needed one field, not a new request.
- The window's share and the banner's ratio are then the same quantity at two
  scopes: the banner is the node's cumulative rate since start, the share is the
  window's. That is a deliberate overlap: a window share read from cumulative
  counters is the only version that survives a thin event ring, and the spec
  already prefers the counters over the ring's split for exactly that reason.
  When the window is too short for a delta, the ring's own command-level share
  stands in — a share of commands rather than of lookups, the same number while
  a command carries one text.

## Goals / Non-Goals

**Goals**

- The stated share is 0-100 by construction and equals the window's hit rate.
- One place decides what a lookup is (`CachedShare`, `ModelCachedShare`), so the
  clause, the headless token and the eviction rule cannot drift apart again.

**Non-Goals**

- Changing when the advice fires beyond correcting the eviction rule's unit
  (its threshold, 5%, is graded on the same lookups the docs name).
- Renaming `cached_pct`: the token keeps its name and its meaning, only its
  value stops exceeding 100.
- The hit-ratio gauge, the rates, or the CPU-per-run readout: all already
  counted over the right denominator.

## Decisions

- **Share of lookups, not share of traffic.** The only counters available in one
  unit are the cache's own, and "share of traffic" would need a command count the
  server does not publish (the event ring's is a sample). A share of lookups is
  what both the cache gauge and the per-model counters already report, so the
  panel speaks one unit.
- **Fix the unit where it is computed, not where it is printed.** The rendering,
  the token and the sentence stay as they are; a rewrite of the words would leave
  the next reader with the same arithmetic to rediscover.
- **Re-record the takes in this change.** The clause is on every plate and in
  `docs/assets/`, and the capture requirement forbids publishing a plate whose
  readout predates the change.

## Risks / Trade-offs

- The eviction rule's denominator grows by the model's misses, so `grow the
  cache` fires later on a warm node than it did — which is what its documented
  rule ("evicted ≥5% of its lookups") says it should do, but it is a behaviour
  change on a threshold that was tuned against the runs-based number.
- The window share and the banner's ratio now agree in kind; a node whose window
  is quiet while its lifetime ratio is not will show the two disagreeing in
  number, which is the point of the window figure.

## Migration Plan

Nothing to migrate: the token keeps its name and its shape, and a consumer
reading `cached_pct` sees the smaller, bounded value. Plates and captures are
re-recorded with the change.

## Open Questions

- Whether the panel should also state the "work removed" ratio (cached lookups
  per run) as its own figure, since the counters are already on the wire. Not
  decided here; it would be a new readout, not a correction.
