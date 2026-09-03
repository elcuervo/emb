## 1. Configuration defaults

- [x] 1.1 Set `read_timeout = 10.0`, `write_timeout = 10.0`, and `reconnect_attempts = 0` in `gems/emb/lib/emb/configuration.rb` (replacing `nil`/3)
- [x] 1.2 Update the README defaults table + rationale (10s timeouts, reconnect 0, why no auto-resend) and verify `spec/emb_spec.rb` forwarding tests still pass (`bundle exec rspec spec/emb_spec.rb`)
- [x] 1.3 Add/update gem specs asserting the new defaults: a default `Emb::Client` passes `read_timeout: 10`/`write_timeout: 10`/`reconnect_attempts: 0` to RedisClient, and overriding via `Emb.configure`/`Emb.new` still wins

## 2. Fail-closed batch handling

- [x] 2.1 In `gems/emb/lib/emb/batch.rb`, wrap each chunk's `send_command('EMB.MULTI', ...)` in a rescue that (a) removes the batch's pending items from `BatchLoader::Executor.current.items_by_block` via `[Emb::BATCH_BLOCK.source_location, Emb::BATCH_KEY]` and (b) re-raises the original error; add a small `clear_batch_pending!` helper guarding a nil executor
- [x] 2.2 Add gem specs (FakeEmbClient variant whose `send_command` raises) covering: error surfaces once; pending set empty after failure; re-resolution returns nil and sends no command; a later batch in the same scope sends only its own pairs; pending stays bounded across repeated failures
- [x] 2.3 Verify existing batch specs (chunking, MGET per-pair nulls, dedupe, caching) still pass — `bundle exec rspec spec/emb_batch_spec.rb`

## 3. Docs and reproduction evidence

- [x] 3.1 Commit `bench/repro/client-timeout/{mock_server.rb,repro.rb,README.md}` and update its README: after hardening, run with `MOCK_DELAY=15` to exercise the raw mechanism, or use as a guard that the loop stays dead (C/D phases are the fix's acceptance tests)
- [x] 3.2 Update `SPECS.md`/gem docs for the fail-closed semantics (failed batch → error once, re-resolution nil) and the new defaults

## 4. Validation

- [x] 4.1 `cd gems/emb && bundle exec rake` green (full gem suite) and `bundle exec rubocop` clean
- [x] 4.2 Re-run the repro against the hardened gem and confirm the A/B leak loop no longer reproduces with defaults in their original form (or documents the new expectations with `MOCK_DELAY=15`)
- [x] 4.3 `openspec validate secure-client-batch-defaults` and confirm all artifacts are done