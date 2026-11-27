# Proposal

## Why

The server supports `EMB.IMGMULTI`, but the Ruby client has no way to include image calls in its existing multi-model batch API. Ruby consumers otherwise must hand-build protocol commands and decode model-specific replies themselves, making ordering, partial failures, and BLOB/VALUES handling easy to get wrong.

## What Changes

- Let image calls join the existing eager `Emb.multi` / `Client#multi` block, exactly like text: `m[:model].image(bytes, *more, format:, outputs:)` collects image pairs and is sent as `EMB.IMGMULTI` when the block runs.
- Decode results using existing image reply conventions: one keyed output map per image, submission order preserved (interleaved with text results), and `nil` for a failed or truncated pair; support `:values` as well as `:binary`.
- Document the byte-only contract and add focused protocol/reply-decoding coverage. No separate pairs-list API.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `emb-ruby-client` — support image calls inside the existing multi-model batch block.

## Impact

- `gems/emb` public client API, image reply decoding, tests, and client documentation. No server protocol change and no remote URL fetching.
