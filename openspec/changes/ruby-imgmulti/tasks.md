# Tasks

## 1. Implement the client API

- [ ] 1.1 Add `image_multi(pairs, format:)` to module and instance clients; validate pair shape and format before sending, and verify exact `EMB.IMGMULTI` arguments with client specs.
- [ ] 1.2 Decode ordered BLOB/VALUES image maps and per-pair nulls using the existing image decoding path; verify binary bytes, empty input, partial failures, and malformed reply handling.

## 2. Document and verify

- [ ] 2.1 Document module-level and instance-level `image_multi` usage, result shapes, byte-only input, and server image limits; verify examples match the API contract.
- [ ] 2.2 Run focused Ruby client image and multi-command specs to verify existing `proxy.image` and text `multi` behavior remains unchanged.
