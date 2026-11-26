# Tasks

## 1. Bound deferred image inputs

- [ ] 1.1 Add global and per-client `max_image_batch_bytes` configuration with a 32 MiB default and validate its value; verify defaults, overrides, and invalid limits with configuration specs.
- [ ] 1.2 Capture deferred image bytes safely at call time, enforce `batch_size` and `max_image_batch_bytes` before queue insertion, and account/release retained bytes with the existing scope lifecycle; verify IO, mutable-string, boundary, and exception-cleanup cases.

## 2. Coalesce and resolve deferred image calls

- [ ] 2.1 Integrate image loaders with existing `:multi` and `:batch` dispatch, grouping only compatible calls and preserving result order and null slots; verify serial chunks, concurrent independent groups, and incompatible projections with batch specs.
- [ ] 2.2 Document eager versus deferred image behavior, byte capture timing, configuration limits, and examples; verify documented names and defaults against the public Ruby API.
- [ ] 2.3 Run the focused Ruby batch-loading and image client specs to verify eager image calls and existing text/script batching remain unchanged.
