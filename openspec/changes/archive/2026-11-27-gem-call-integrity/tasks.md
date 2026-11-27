# Tasks

## 1. Payload snapshot

- [x] 1.1 In `gems/emb/lib/emb/batch.rb`, copy the text in `build_batch_loader` so the item holds a private String; verify a new spec that mutates the caller's String after deferring still sends the original text (`spec/emb_batch_spec.rb`).
- [x] 1.2 In `gems/emb/lib/emb/multi.rb`, copy the text in `PairCollector#[]` and the bytes in `PairCollector#image` (or rely on 1.3's non-mutating coercion); verify a spec that mutates text and image Strings inside a `multi` block and asserts the sent argv (`spec/emb_multi_images_spec.rb`, `spec/emb_spec.rb`).
- [x] 1.3 In `gems/emb/lib/emb/proxy.rb`, make `coerce_image_bytes` return a new binary String (`.b`) for Strings and the read result for IOs, dropping the `dup if frozen?` branch; verify a spec that passes a UTF-8 String and asserts the caller's String keeps its encoding and contents, and that the wire arg is `BINARY`.

## 2. Deferral registry identity

- [x] 2.1 In `gems/emb/lib/emb/telemetry/deferred.rb`, key `record` / `dispatched` / `resolve` / `flush` by the item's stable identity instead of its content hash; verify the new mutation spec leaves `Thread.current[:emb_telemetry_deferrals]` empty after resolution.
- [x] 2.2 Add the drain assertion: a scope of resolved deferrals (mixed text and image) leaves the registry empty, and `flush` still emits spans for entries that never dispatched; verify with a spec in `spec/emb_telemetry_spec.rb` (or the existing telemetry spec file).

## 3. Serial failure isolation

- [x] 3.1 In `gems/emb/lib/emb/batch_dispatch.rb`, isolate a failed serial share to its own slice: resolve that slice's items with the batch error, prune only those items from the pending set, and continue with the remaining slices; keep today's clear-and-reraise for non-Redis errors. Verify `go`-equivalent Ruby specs: the existing `emb_batch_spec` fail-closed examples plus a new one where two clients share a scope and the first fails.
- [x] 3.2 Verify B's command is actually sent in that two-client spec (not merely that it stops raising), and that a local (non-Redis) error still clears the scope.
- [x] 3.3 Confirm `ShortReplyError` retry accounting is unchanged: `batch_error` still reports the same attempts for a protocol error and a connection error (existing `emb_batch_spec` examples pass unmodified).

## 4. Docs and release notes

- [x] 4.1 In `gems/emb/README.md`, state what a deferred or `multi` call captures at call time (texts, image bytes, format, projection), that identical deferred calls coalesce into one wire payload, and that the caller's Strings are not modified.
- [x] 4.2 Add a `### Fixed` entry to `gems/emb/CHANGELOG.md` covering the four fixes.

## 5. Integration verification

- [x] 5.1 Run the gem's server-free suites (`EMB_NO_SERVER=1 bundle exec rspec` on the batch, image, multi, telemetry, and contract spec files) and `bundle exec rubocop` on the touched files; verify green.
- [x] 5.2 Run the same suites with a server on `:16379` (`just all`) so the deferred and multi paths are exercised end to end; verify green.

## Workflow follow-up

- Archive the change after review; the gem README, CHANGELOG, and fix land together.
