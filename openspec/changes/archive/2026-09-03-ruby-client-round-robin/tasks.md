# Tasks: ruby-client-round-robin

## 1. Dispatcher implementation

- [x] 1.1 Create `gems/emb/lib/emb/round_robin_pool.rb` with `Emb::RoundRobinPool` (holds N `RedisClient`s, rotating next-connection selection, per-connection mutexes, `size`/`with` API) and verify a new unit spec passes: sequential `with` calls rotate through connection order and wrap, `pool: 1` always returns the same connection
- [x] 1.2 Wire `Emb::Client`: build the dispatcher instead of `connection_pool` (keeping the `pool:` option semantics, `attr_reader :pool`, and the `Emb.debug?` timing path in `send_command`) and verify `configuration_spec.rb` and the existing client specs still pass (`cd gems/emb && bundle exec rspec spec/emb/configuration_spec.rb spec/emb_spec.rb`)

## 2. Thread safety

- [x] 2.1 Add a concurrency spec using a fake `RedisClient` that records concurrent use per connection and asserts no connection is ever used by two threads at once, with up to `pool` commands completing in parallel — verify it passes under repeated runs (`bundle exec rspec spec/round_robin_pool_spec.rb` in a loop)
- [x] 2.2 Verify interleaved concurrent commands each receive their own reply (fake client echoes payload; assert no cross-talk)

## 3. Failure behavior compatibility

- [x] 3.1 Add spec that server error replies propagate as `RedisClient::CommandError` unchanged (fake client raising on `call`)
- [x] 3.2 Add spec that every pooled `RedisClient` is constructed with the forwarded redis options (`reconnect_attempts`, timeouts, driver, ssl, protocol) — extend `gem-redis-client-config` coverage if needed — and verify the full spec file passes

## 4. Batch path rotation

- [x] 4.1 Add a spec proving `Emb::Batch` chunks of `EMB.MULTI` (slices at `batch_size` via `send_command`) land on different rotated connections when a batch window spans more than one chunk, using a fake client — verify it passes

## 5. Integration, docs, and cleanup

- [x] 5.1 Run the full gem suite against a live server (`./bin/emb -config test-two-models.yaml -listen :16379` in `nix develop`, then `cd gems/emb && bundle exec rake`) and verify all specs green
- [x] 5.2 Update the README connections section: explain connection-level balancing (AWS Service Connect / NLB) pins each keep-alive connection to one instance, and recommend `pool:` ≥ expected emb instance count for full spread — verify rendered README reads correctly
- [x] 5.3 Grep `gems/emb` for remaining `connection_pool` usage; if none, remove it from `emb.gemspec` runtime dependencies and verify `bundle install` and the spec suite still pass
- [x] 5.4 Run rubocop/gofmt-style checks (`cd gems/emb && bundle exec rubocop`) and verify clean output for changed files; run `just validate-gems` to confirm the gem builds and installs
- [x] 5.5 Run `nix develop --command bash -c 'go test ./... && go vet ./...'` (server untouched, but confirm the workspace is green) and verify the OpenSpec change validates: `openspec validate ruby-client-round-robin`

## 6. Adversarial review follow-ups

- [x] 6.1 Fork safety: register pools in a WeakMap + `Process._fork` hook that closes inherited connections and rebuilds mutexes/index in the child (mirrors `connection_pool`'s `auto_reload_after_fork`); covered by a fork spec — verify `bundle exec rspec spec/emb/round_robin_pool_spec.rb` passes
- [x] 6.2 Same-thread reentrancy: nested `with` returns the already-held connection (no re-lock), preserving README-documented `client.pool.with { |c| c.pipelined … }` — reentrancy specs pass
- [x] 6.3 Document exhaustion divergence (no 5s checkout timeout; commands block until a connection frees) in README + design.md risk section
- [x] 6.4 Document that INFO/STATS/CONFIG are per-instance samples under rotation (README; cluster-wide aggregation is future work)
- [x] 6.5 Test gaps: add reconnect-recovery spec (transient `Errno::ECONNRESET` then success), bump parallelism assertion to pool size (>= 4), add `driver` to the options-forwarding assertions
- [x] 6.6 Artifact nits: tasks.md spec path corrected, design.md Decision 3/Risks wording aligned with actual behavior