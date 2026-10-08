## 1. Cache admission and replacement

- [x] 1.1 Add deterministic regression cases in the existing server cache tests for oversized inserts/replacements, growing/shrinking replacements, and exact-fit boundaries; assert preserved or updated values, LRU order, per-model accounting, counters, and generation as specified.
- [x] 1.2 Update shared `Cache.Set` to check the current budget before copying, reject oversized writes without mutation, and enforce LRU eviction for admissible replacements; retain existing non-positive budget and owned-value semantics.
- [x] 1.3 Verify concurrent writes/resizes and existing snapshot immutability coverage under the race detector, extending a deterministic check only where coverage is missing.

## 2. Script reply identity

- [x] 2.1 Add key-level tests for the long-payload/literal-digest collision, binary inputs, inline threshold, stable repeat keys, and separation from explicit legacy fixtures.
- [x] 2.2 Add the reply-key derivation-version domain and representation discriminator to the existing metadata hash; preserve the outer key shape and update `TestReplyCacheKeyUnchanged` intentionally without changing `APIVersion`.
- [x] 2.3 Add a scripted endpoint regression proving both collision priming orders return the same replies as uncached evaluations, with repeat calls hitting the corrected keys.

## 3. Persistence compatibility

- [x] 3.1 Extend snapshot tests with legacy script entries and compatible text/image entries; prove legacy script misses and embedding hits after restore, with sufficient capacity and matching fingerprints.
- [x] 3.2 Verify current script entries survive save/restore and hit normally, including lazy/script-only admission, without changing snapshot encoding or adding legacy-key fallback.
- [x] 3.3 Update existing cache/persistence documentation to state that this upgrade makes script replies cold while retaining compatible embedding entries; explain oversized cache writes are skipped without failing inference.

## 4. Validation

- [x] 4.1 Inside `nix develop`, run `go test ./internal/server ./internal/script -count=1` and focused race checks for cache admission, snapshot immutability/restore, and script identity; address failures without broadening scope.
- [x] 4.2 Compare existing `BenchmarkCacheGet`, `BenchmarkCacheSet`, `BenchmarkServerEMBCached`, and `BenchmarkServerEMBUncached` against an isolated unchanged baseline with repeated samples and allocation reporting; satisfy the existing disabled-persistence gate (no additional allocation/op, at most 2 percent median throughput regression), investigate noise rather than assuming a speedup.
  - Results (Apple M1 Pro, detached `HEAD` worktree baseline, interleaved runs, benchstat): allocs/op and B/op unchanged on all four. `CacheSet` and `ServerEMBCached` not significant (n=20 / n=10). `ServerEMBUncached` +11.5% at n=10 (±16%) was noise: not significant at n=20 with reversed order (p=0.40). `CacheGet` +1.6–2.7%, though `Get` is byte-identical: the change binary places `(*Cache).Get` at a 16-byte-aligned address (`…cf0`) while the baseline and a copy-before-lock variant place it 64-byte aligned, and that variant shows no `CacheGet` delta. This is code-placement drift, not a logic regression; reject-before-copy is kept as specified.
- [x] 4.3 Run `go test ./... -count=1`, `just lint`, and `openspec validate harden-cache-identity-and-budget --strict`; record results and any environment limitations.
  - Results: `go test ./... -count=1` passes in every package; `just lint` reports 0 golangci-lint issues and a clean `go vet`; `openspec validate --strict` passes. Limitation: no `benchstat` in the dev shell, so it ran via `go run golang.org/x/perf/cmd/benchstat@latest`.
