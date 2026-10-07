# Tasks

## 1. Work-conserving pool

- [x] 1.1 Replace the fixed-index mutex selection in `gems/emb/lib/emb/round_robin_pool.rb` with a free-connection FIFO queue (`@free`), popping in `with` and pushing in `ensure`, and rebuild `@free` in `reload_after_fork!`; verify `bundle exec rspec spec/emb/round_robin_pool_spec.rb` passes, including the existing rotation, reentrancy, concurrency, and fork scenarios
- [x] 1.2 Add a deterministic gated-fake spec asserting a waiting command runs on a freed connection while another is still held; verify it fails against the pre-fix selection and passes after 1.1 (no wall-clock assertions)
- [x] 1.3 Confirm idle rotation and multi-instance ordering are unchanged; verify the "rotates through connections in order and wraps around" and "rotates across instances first, then across connections" specs pass
- [x] 1.4 Run the Ruby gem suite from `nix develop` (`cd gems/emb && bundle exec rake`) against a server on 127.0.0.1:16379 and confirm no regression

## 2. Benchmark gate

- [x] 2.1 Add a model-free concurrent pool gate under `bench/repro/pool-hol/` (mock server with a slow-reply fraction, gate driver over the real client) and a `just bench-pool-gate` target; verify `just bench-pool-gate` prints PASS
- [x] 2.2 Document the p90 threshold and the work-conserving baseline in `BENCHMARK.md`; verify `just bench-pool-gate` exits 0 (fixed-index discrimination is covered by the deterministic spec)
- [x] 2.3 Record the post-fix reference run (p50/p90/p99, mock server) in `BENCHMARK.md` and confirm p90 parity with the 0.3.0 work-conserving baseline

## 3. Docs and integration validation

- [x] 3.1 Update the `gems/emb/README.md` connection-pool section: selection is work-conserving, the unbounded wait remains, and `pool` is the concurrency budget; verify the documented behavior matches the specs
- [x] 3.2 Run `just test`, `just lint`, `just deadcode`, and `just verify-harness` and confirm all pass
- [x] 3.3 Run `just bench-script` (with a downloaded model) and confirm no regression against the recorded baseline

## Workflow follow-up

- Archive the change after review; sync the delta specs into `openspec/specs/`.
- Cut the patch release via the project release flow.
