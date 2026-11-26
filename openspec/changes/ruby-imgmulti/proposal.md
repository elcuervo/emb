# Proposal

## Why

The server supports `EMB.IMGMULTI`, but the Ruby client has no supported cross-model image batch API. Ruby consumers otherwise must hand-build protocol commands and decode model-specific replies themselves, making ordering, partial failures, and BLOB/VALUES handling easy to get wrong.

## What Changes

- Add module-level and instance-level `image_multi(pairs, format: :binary)` client methods, where each pair contains a model name and raw image bytes.
- Decode results using existing image reply conventions: one keyed output map per pair, request order preserved, and `nil` for a failed pair; support `:values` as well as `:binary`.
- Document the byte-only contract and add focused protocol/reply-decoding coverage.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `emb-ruby-client` — add a supported Ruby API for the existing cross-model image command.

## Impact

- `gems/emb` public client API, image reply decoding, tests, and client documentation. No server protocol change and no remote URL fetching.
