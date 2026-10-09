# Tasks

## 1. Library: isolate per-share failures

- [x] 1.1 In `gems/emb/lib/emb/batch.rb`, replace `fail_batch!` with a non-raising
      `batch_error(error, slice:, budget:)` that returns the `Emb::ServerError`
      with the original redis error as `cause` (raise-inside-rescue helper), and
      update the private method list. Verify the serial fail-closed examples
      still pass:
      `nix develop <repo> -c bash -c 'cd gems/emb && BUNDLE_PATH=$PWD/vendor/bundle bundle exec rspec spec/emb_batch_spec.rb -e "fail-closed batches with retries"'`.
- [x] 1.2 In `gems/emb/lib/emb/batch_dispatch.rb`, add the `FailedShare`
      `BasicObject` poison, rewrite `resolve_outcomes` to resolve `:ok` shares,
      poison each failed redis share's items via `loader.call(item, FailedShare.new(batch_error(...)))`,
      defer non-redis errors to a single clear-and-reraise after the loop, and
      remove `collect_outcomes`. Verify with a focused run of the two `:batch`
      examples updated in section 2 (they must go from red to green).
- [x] 1.3 In `gems/emb/lib/emb/batch_dispatch.rb`, make `dispatch_serial` do
      `clear_batch_pending!; raise batch_error(...)` so the `:multi` and
      single-share `:batch` path is unchanged. Verify the serial and single-share
      examples in `spec/emb_batch_spec.rb` (`describe 'parallel batch execution'`
      single-chunk example, `describe 'fail-closed batches with retries'`) pass.

## 2. Specs: pin the new behaviour

- [x] 2.1 Update `spec/emb_batch_spec.rb` "fails the unknown-model share alone
      under batch mode": assert `minilm.first == 1.0`, `nope.first` raises
      `Emb::ServerError` (cause `RedisClient::ReadTimeoutError`, twice, no
      re-send), and exactly 2 commands were sent. Verify:
      `nix develop <repo> -c bash -c 'cd gems/emb && BUNDLE_PATH=$PWD/vendor/bundle bundle exec rspec spec/emb_batch_spec.rb -e "fails the unknown-model share alone"'`.
- [x] 2.2 Replace the "fails closed on a terminal share failure" example with a
      "isolates a terminal share failure" example (`OneFailClient`,
      `batch_size = 1`): healthy `b == [2.0, 2.0]`, `a.first` raises
      `Emb::ServerError` with cause `RedisClient::ReadTimeoutError` twice,
      `client.commands.size == 2`, and the pending set is empty. Verify the
      example passes in isolation.
- [x] 2.3 Confirm the serial fail-closed specs (`describe 'fail-closed batches
      with retries'`) are unchanged and green, then run the whole file:
      `nix develop <repo> -c bash -c 'cd gems/emb && BUNDLE_PATH=$PWD/vendor/bundle bundle exec rspec spec/emb_batch_spec.rb'`.

## 3. Documentation

- [x] 3.1 Add a `### Breaking` entry to `gems/emb/CHANGELOG.md` (unreleased):
      under `:batch`, a failed share's items now raise on every use instead of
      resolving to `[]`, and the force no longer raises for a sibling's failure.
      Verify the entry is under the unreleased section and names both changes.
- [x] 3.2 Update the "Fail-closed batches" section of `gems/emb/README.md` to
      state the parallel-path carve-out (sibling failures do not fail the force;
      failed items raise `Emb::ServerError` on use, never `[]`). Verify the
      documented example still matches the library behaviour.

## 4. Integration verification

- [x] 4.1 With an emb server on `127.0.0.1:16379`
      (`./bin/emb -config test-two-models.yaml -listen 127.0.0.1:16379`), confirm
      the siglip2 + hyperclusters matrix on `:batch`: none -> both ok;
      hyperclusters down -> siglip2 ok and hyperclusters raises
      `Emb::ServerError`; siglip2 down -> siglip2 raises and hyperclusters ok.
      Verify no `EMB.MULTI` is sent and no command is re-sent on reuse.
- [x] 4.2 Run the whole gem suite and compare failures with `origin/main`:
      `nix develop <repo> -c bash -c 'cd gems/emb && BUNDLE_PATH=$PWD/vendor/bundle bundle exec rspec'`.
      Verify the only failures are the same model-dependent examples that fail
      on `origin/main`, and `spec/emb_batch_spec.rb` is fully green.
- [x] 4.3 Run RuboCop on the touched files and confirm no new offenses versus
      `origin/main`: `nix develop <repo> -c bash -c 'cd gems/emb && BUNDLE_PATH=$PWD/vendor/bundle bundle exec rubocop lib/emb/batch.rb lib/emb/batch_dispatch.rb spec/emb_batch_spec.rb'`.
- [x] 4.4 Validate the change:
      `openspec validate isolate-batch-share-failures --strict`.
      Verify it passes with no warnings.

## Workflow follow-up

- Do not push or release unless asked.
- Archive the change after the acceptance checks above pass and review is satisfied.
