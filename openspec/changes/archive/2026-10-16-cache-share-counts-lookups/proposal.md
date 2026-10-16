# Proposal

## Why

`emb-top` renders the cache's share of the window beside its advice — `served
94% from cache` in the panel's second line, `cached_pct=` in the `-once` and
`-frames` outputs, and the same number inside the `grow the cache` sentence.
That share is computed as the cache's answered lookups over `total_requests`:

```go
cached   = delta(first.CacheHits, last.CacheHits)
requests = delta(first.TotalRequests, last.TotalRequests)
return float64(cached) / float64(requests) * 100
```

`total_requests` is `Σ per-model pool.Requests()` (`internal/server/info.go` →
`Worker.requests` in `internal/pipeline/pool.go`), which counts the requests that
**ran**: a cache-answered request never reaches a worker and is never counted
there. So the quotient is not a share of anything — it is the cache's answered
lookups per run, which is 100% at a 50% hit rate and climbs from there.

Measured on a live node (minilm, `cache: 256MB`):

| load | `cache_hits` | `cache_misses` | `total_requests` | rendered share | real hit rate |
|---|---|---|---|---|---|
| 300 identical texts | 299 | 1 | 1 | 29 900% | 99.7% |
| 200 distinct, then the same 200 | 200 | 200 | 200 | 100% | 50% |

The recorded plates are where it shows: the shipped take replays a window whose
header chip reads `cache 49%` (the server's own `cache_hit_rate`, a true share)
while the clause under it reads `served 97% from cache`, and the same take
reaches `served 178% from cache` as the scripted traffic's hit ratio climbs.
`openspec/specs/emb-top-dashboard` requires the readout to state a *share*, and a
share above 100% cannot be read as one.

## What Changes

- The window's cached share is counted over the cache's own counters — the
  answered over the answered plus the missed — so it is the window's own hit
  rate and is bounded by 100: `CachedShare` (the panel clause and `cached_pct=`),
  and `ModelCachedShare`/`cacheUndersized` for the per-model eviction share the
  `grow the cache` sentence prints, whose denominator was the model's runs with a
  comment claiming a hit is counted there (it is not).
- `ModelPoint` carries the model's own miss count, so the per-model shares are
  counted over lookups too.
- The spec states the unit: a share of the cache's lookups, never of
  `total_requests`, with a scenario pinning that a warm window's share stays
  within 0-100%.
- Every take is re-recorded (`just website-topviz`, `website-clusterviz`,
  `website-stressviz`), per the capture requirement: the clause is on every
  plate.

`cached_pct` keeps its token name and its place in the line; its value becomes
the share the dashboard states. Nothing else about the advice, the health
verdict or the gauges changes.

## Non-goals

- The cache-hit-ratio gauge, the hit/miss/eviction rates and the per-model cache
  counters: already shares of the lookups (`hits/(hits+misses)`) and unchanged.
- Redefining the second figure as "work the cache removed": that reading is what
  produced the unbounded number, and it is not what the readout is for.
- The CPU-per-run readout: `total_requests` is the correct denominator there and
  stays.
