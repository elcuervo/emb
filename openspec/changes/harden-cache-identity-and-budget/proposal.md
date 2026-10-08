## Why

The shared cache can exceed its configured byte budget, and scripted reply keys can alias a long input with a short literal input, returning the wrong result. Independent correctness, performance, and stability reviews rank these localized, deterministic defects above scheduling changes whose benefit still needs workload measurements.

## What Changes

- Enforce the existing positive cache budget on both insert and replacement; skip oversized values before copying or evicting useful entries.
- Preserve an existing value, recency, counters, and mutation generation when its oversized replacement is rejected.
- Give literal and hashed script inputs distinct representations and version the script reply-key identity so previously ambiguous persisted keys cannot be reused.
- Add deterministic regression tests for key separation, replacement eviction, oversized writes, and snapshot compatibility.
- Preserve the existing per-text script cache contract, public commands, snapshot file format, and dependencies.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `lru-cache`: Specify budget enforcement for oversized inserts and growing replacements, including rejected-write semantics.
- `script-eval`: Require unambiguous input representation and separation from legacy reply-key identities.
- `cache-snapshots`: Require old script reply identities to remain unservable after the key change while compatible embedding entries remain reusable.

## Impact

Implementation centers on `internal/server/cache.go`, `internal/script/cache.go`, and their tests, with server-level snapshot regression coverage. Script reply caches become cold after upgrading; text/image embedding keys remain compatible. No new configuration, dependency, wire protocol, or model inference changes are required.

## Non-goals

Sibling-dependent script caching, worker scheduling, batching timeout semantics, cache-miss coalescing, download lifecycle changes, cache sharding, and a new cache implementation. The design records their relative ROI and why they are deferred.
