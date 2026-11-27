# Tasks

## 1. Add image calls to the multi block

- [x] 1.1 Add `image(bytes, *more, format:, outputs:)` to the `multi` block's pair collector, with the same arguments as `Proxy#image`; validate pair structure and format before any command is sent.
- [x] 1.2 Send image pairs as `EMB.IMGMULTI` grouped by format and output projection, chunked by `batch_size`; keep text pairs on `EMB.MULTI` and reassemble mixed results in submission order.

## 2. Decode, document, and verify

- [x] 2.1 Decode ordered BLOB/VALUES image results and per-pair nulls with the existing image decoding path; verify binary bytes, multi-image calls, empty blocks, partial failures, and the pair `model` tag under VALUES.
- [x] 2.2 Document block usage, result shapes, byte-only input, and server image limits; verify examples match the public Ruby API.
- [x] 2.3 Run focused Ruby client image and multi-command specs to verify existing `proxy.image` and text `multi` behavior remains unchanged.
