# Tasks: Ruby Client Batch Loading

## 1. Dependency

- [x] 1.1 Add `batch-loader` to `gems/emb/emb.gemspec` dependencies (no version pin beyond latest stable, e.g. `'~> 2.0'`)
- [x] 1.2 Run `bundle install` in `gems/emb/` and confirm the lockfile resolves `batch-loader`

## 2. Batch block core

- [x] 2.1 Create `gems/emb/lib/emb/batch.rb` with a single constant batch block (`key: :emb`) that expands items into `EMB.MULTI` args, unpacks non-nil responses with `'e*'`, and calls `loader.call(item, value)` per item — mapping server nulls to `nil`
- [x] 2.2 Handle multi-text items: expand `[:model, ["a","b"]]` into multiple MULTI pairs and regroup so one vector is returned for a single text, an array of vectors for many (matching the eager `Proxy#[]` shape)
- [x] 2.3 Preserve per-pair ordering between `EMB.MULTI` args and results when regrouping

## 3. Public API

- [x] 3.1 Add `Emb::BatchProxy` (module-level `Emb.batch` + instance `client.batch`) returning memoized per-model lazy proxies with `["text", ...]` chains
- [x] 3.2 Add `batch:` option to `Emb::Client#initialize` (default `false`) and expose it to `Proxy#[]`
- [x] 3.3 Modify `Proxy#[]` to return a lazy batched embedding when `batch: true`, keeping the current eager path byte-for-byte when `batch: false`
- [x] 3.4 Require `emb/batch` from `lib/emb.rb` and wire module-level `Emb.batch` to the default client

## 4. Tests

- [x] 4.1 Unit-test the batch block against a stubbed command sink: single text, multi-text shape, mixed-model coalescing, per-pair ordering, null → `nil`
- [x] 4.2 Test the create-then-consume contract: loaders created from different call sites coalesce into one `EMB.MULTI` (batch-key regression guard)
- [x] 4.3 Test caching: repeated use and identical pairs send exactly one command; failed pairs materialize as `nil` without re-sending
- [x] 4.4 Test `batch: true` client routes `client[:model]["text"]` lazily; default client stays eager (spec scenarios for `emb-ruby-client` delta)
- [x] 4.5 Test `Emb.multi` is untouched and still works in both configurations

## 5. Docs & Validation

- [x] 5.1 Update `gems/emb/README.md` with `Emb.batch` usage, the `batch` config option, and the create-then-consume + per-thread-scope contract (including the silent-drop caveat)
- [x] 5.2 Run `just format`, `just lint`, and `just test` (or the gem's rubocop/rspec equivalents) and fix any issues
- [x] 5.3 Confirm `openspec validate ruby-client-batch-loader --type change` passes and update the `emb-ruby-client` main spec via archive/sync when the change is complete