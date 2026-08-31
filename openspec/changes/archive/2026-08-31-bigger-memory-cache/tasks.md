## 1. Auto-tune sizing

- [x] 1.1 Remove the `min(budget, 500MB)` cap in `autoTuneCache` and replace with a proportional ceiling of `mem/2` (keep floor 64MB, keep formula `0.20 × (mem − mem/10 − mem/4)`); verify `go test ./internal/server/` passes and a new `TestAutoTuneCache` asserts budget ≥ 64MB, ≤ `mem/2`, and on machines with ≥ 4GB RAM it exceeds 500MB (skip guard when memory is small)
- [x] 1.2 Drop the unused `defaultDim` parameter from `autoTuneCache` and update the `parseCacheConfig` call site (dead param from the old entries-based formula); verify `go vet ./...` and `go test ./...` are clean

## 2. Percent cache sizes

- [x] 2.1 Extend `parseCacheConfig` to accept `"N%"` (N = percent of `TotalSystemMemory()`, validated `0 < N ≤ 100`); sizes like `"10%"` parse to `int64(pct/100 × mem)`, and `"150%"`/`"0%"`/`"abc%"` fail startup with clear errors; verify `TestParseCacheConfig` gains the full percent matrix and existing empty/`512MB`/`invalid`/`auto` cases still pass
- [x] 2.2 Handle `TotalSystemMemory() == 0` for percent sizes with an explicit "cannot determine system memory" error (fallback platforms); verify a unit test exercises the error path

## 3. Docs and spec alignment

- [x] 3.1 Update `BENCHMARK.md` cache section: `-cache auto` described as ~13% of RAM (safety-adjusted, no 500MB cap) and the percent syntax documented (`-cache 25%`); verify rendered text no longer claims the old behavior
- [x] 3.2 Update the `cache:` comment in `config.yaml` to mention `"auto"`, human sizes, and `"N%"`; verify the comment matches `parseCacheConfig` accepted values

## 4. Validation

- [x] 4.1 Run the working-set experiment and record results in `BENCHMARK.md`: warm ~200k distinct texts, replay a mixed subset, and compare `EMB.INFO` `cache_hit_rate` and `cache_max_bytes` between `-cache 500MB` and `-cache auto` on a large-RAM machine; verify the write-up includes the hit-rate delta and memory used
- [x] 4.2 Full suite green: `just all` (Go tests + Ruby client integration on `test-two-models.yaml`) passes with the new cache sizing in place
- [x] 4.3 `EMB.INFO <model>` smoke check: with `-cache auto` on a ≥ 4GB box, `cache_max_bytes` reflects the new budget (> 500MB) and `cache_hit_rate` appears for a warm reuse; verify via `redis-cli` against a local server