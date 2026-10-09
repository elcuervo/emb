## 1. Batch mode shares

- [x] 1.1 Add failing spec: `:batch` mixed-model scope (`minilm` + `bge`, default `batch_size`) sends two plain `EMB` commands, no `EMB.MULTI`, overlapping (use `LatchTracker`)
- [x] 1.2 Add failing spec: `siglip2` + `hyperclusters` single-text scope forces both concurrently; second value resolves without another command
- [x] 1.3 In `BATCH_BLOCK`, for `parallel_batch?` clients build shares per model (`group_by` model, then `pack_slices`) instead of packing across models
- [x] 1.4 Dispatch via `dispatch_parallel` whenever shares > 1; keep single-share path on the forcing thread
- [x] 1.5 Scope existing mixed-model `EMB.MULTI` specs to `:multi`; add unknown-model-share fail-closed spec for `:batch`

## 2. Deferred scripts

- [x] 2.1 Add failing specs: deferred `eval`/`evalsha` send nothing at call time; under `:batch` an `EMB.EVSHA` share overlaps an `EMB` share; two `evalsha` calls stay independent; `:multi` sends serially; `decode: :f32` result equals eager
- [x] 2.2 Route `eval`/`evalsha` through a `build_script_loader` when the client lazy mode is deferred (validate `decode:` eagerly)
- [x] 2.3 Teach `dispatch_slice`/`resolve_slice`/`fail_batch!` the `:script` item shape (send original args, `parse_script_reply` on forcing thread)
- [x] 2.4 Ensure module-level `Emb.eval`/`Emb.evalsha` follow the default client's lazy mode

## 3. Docs and release

- [x] 3.1 Rewrite README "Lazy batching" section: `:multi` = coalesced serial (`EMB`/`EMB.MULTI`); `:batch` = concurrent plain `EMB` per model + scripts; note BREAKING for deferred `eval`
- [x] 3.2 CHANGELOG entry and gem version bump
- [x] 3.3 Run full gem suite (`bundle exec rspec` in gems/emb) and `openspec validate --strict`
