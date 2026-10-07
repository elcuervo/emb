# Tasks

## 1. Separate driving signals from informational chips

- [x] 1.1 Add a `driving bool` field to `signal` in `cmd/emb-top/health.go` and make the worst-level fold skip chips with `driving == false`; verify `nix develop --command bash -c 'go build ./cmd/emb-top/'` succeeds.
- [x] 1.2 Render the cache hit-rate chip as informational: append it with a neutral level and `driving: false`, delete the now-unused `cacheDegradedPct`/`cacheCriticalPct` constants, and keep the chip order connection/error/p95/cpu/cache; verify `nix develop --command bash -c 'go vet ./cmd/emb-top/'` passes with no unused constants.

## 2. Tests

- [x] 2.1 Remove the "cache degrade" verdict case from `TestHealthVerdicts` and add cases asserting a low cache hit rate with otherwise-healthy signals stays `healthHealthy` and still emits the `cache` chip; verify `nix develop --command bash -c 'go test ./cmd/emb-top/ -run TestHealthVerdicts -count=1'` passes.
- [x] 2.2 Update `TestHealthChipsKeepFixedWidth` inputs to drop the removed severity thresholds' expectations, keeping the cache chip in the width check; verify `nix develop --command bash -c 'go test ./cmd/emb-top/ -count=1'` passes.

## 3. Verify the spec delta

- [x] 3.1 Run `openspec validate emb-top-actionable-health --strict` and confirm the health-banner requirement and the new "Low cache hit rate does not degrade health" scenario validate clean.

## Workflow follow-up

- Archive the change after the project's review requirements are satisfied.
