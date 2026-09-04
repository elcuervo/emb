# Tasks: Batch failure retries with a typed server error

## 1. Error type

- [x] 1.1 Add `gems/emb/lib/emb/errors.rb` defining `Emb::ServerError < StandardError` with an initializer accepting a message string and keyword `attempts:` (default 1) plus an `attr_reader :attempts`; require it from `gems/emb/lib/emb.rb` and verify `ruby -I lib -e "require 'emb'; raise Emb::ServerError.new('x', attempts: 3)"` prints the expected class and message.

## 2. Configuration

- [x] 2.1 Update the `reconnect_attempts` comment in `configuration.rb#initialize` to document the opt-in retry semantics (default `0` = fail closed after one attempt; `> 0` = transient batch failures re-send up to that many additional times, always terminating in `Emb::ServerError`); verify `Emb.configuration.to_h[:reconnect_attempts] == 0` still holds and `Emb.new(reconnect_attempts: 2)` overrides.
- [x] 2.2 Add coverage to `gems/emb/spec/emb/configuration_spec.rb` and `gems/emb/spec/emb_spec.rb` (default stays 0 in `to_h`, per-client override wins) and verify `bundle exec rspec spec/emb/configuration_spec.rb spec/emb_spec.rb` passes.

## 3. Typed raise in the batch path

- [x] 3.1 In `batch.rb`, replace the `raise e` in the chunk rescue with a private `fail_batch!(error, slice:, budget:)` (plus `clear_batch_pending!` — all three batch failure helpers are `private_class_method`) that raises `Emb::ServerError` with a message including the model(s), text count, attempt count, and the cause class/message (e.g. `EMB.MULTI failed after 3 attempt(s) (models: minilm, 512 text(s)) RedisClient::ReadTimeoutError: read timed out`); compute `attempts` as `budget + 1` when the error is transient (`RedisClient::ConnectionError` or `RedisClient::ProtocolError`), else `1`, with `budget` resolved like `batch_size` (per-client `reconnect_attempts` accessor added to `client.rb`, else `Emb.configuration`) and normalized to an Integer.
- [x] 3.2 Update `FailingEmbClient` in `emb_batch_spec.rb` to fail a configurable number of times then succeed (and keep an always-fail mode), and update the existing fail-closed specs: first force raises `Emb::ServerError` (not the raw redis error), exactly one command sent under the default config, pending set empty, later resolutions `[]` with no I/O, subsequent batches exclude failed items.
- [x] 3.3 Add retry scenarios to `emb_batch_spec.rb` using a per-client `reconnect_attempts: 2`: (a) two failures then success materializes real values with exactly 3 commands sent; (b) always-failing transient error raises `Emb::ServerError` with `attempts == 3` and `cause` being the redis error; (c) an operation error (`RedisClient::CommandError`-style) raises `Emb::ServerError` after exactly 1 command even with retries configured; (d) `Emb::ServerError` message includes model(s), text count, and attempt count; verify `bundle exec rspec spec/emb_batch_spec.rb` passes.

## 4. Docs

- [x] 4.1 Update `gems/emb/README.md`: the "Why the timeout and reconnect defaults matter" section and the "Fail-closed batches" paragraph to describe `Emb::ServerError` (rescued via `cause` for the original redis error), the opt-in retry semantics (`reconnect_attempts` default `0`; `> 0` re-sends transient failures up to that many times and every exhausted batch raises `Emb::ServerError`), the failure taxonomy (connection/timeout errors retry when configured; operation errors never), and a breaking-change callout (lazy batch forces raise `Emb::ServerError` instead of `RedisClient::*`; eager APIs unaffected).
- [x] 4.2 Verify the README config list still shows `reconnect_attempts: 0` as the default and no stale references to raising the raw redis error for batches remained.

## 5. Validation

- [x] 5.1 Run `cd gems/emb && bundle exec rubocop` and fix any lint issues from the new code.
- [x] 5.2 Run the full gem suite with a server on :16379 (`../bin/emb -config ../../test-two-models.yaml -listen :16379` in `nix develop`, then `cd gems/emb && bundle exec rake`) and verify all specs pass, including the integration spec touching the default client.